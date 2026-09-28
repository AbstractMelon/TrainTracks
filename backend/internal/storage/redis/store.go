// Package redis implements live train state, pub/sub fan-out and rate limiting.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"traintracks/internal/models"
)

const (
	LiveChannel     = "live:updates"
	stateKeyPrefix  = "train:"
	activeKeyPrefix = "agency:"
	geoKey          = "trains:geo"
	statusKeyPrefix = "ingestor:status"

	// StateTTL mirrors SPEC.md §10: state survives MaxEstimationWindow + 2min.
	StateTTL = 12 * time.Minute
)

// Store is the Redis-backed live store.
type Store struct {
	rdb *redis.Client
}

func New(url string) (*Store, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &Store{rdb: redis.NewClient(opts)}, nil
}

func (s *Store) Close() error                   { return s.rdb.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.rdb.Ping(ctx).Err() }

func stateKey(id string) string       { return stateKeyPrefix + id + ":state" }
func activeKey(agency string) string  { return activeKeyPrefix + agency + ":active" }
func statusKey(adapter string) string { return statusKeyPrefix + adapter }

// ---------------------------------------------------------------------------
// Train state. See SPEC.md §6
// ---------------------------------------------------------------------------

func (s *Store) SetTrainState(ctx context.Context, st models.TrainState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, stateKey(st.TrainID), data, StateTTL)
	pipe.SAdd(ctx, activeKey(st.AgencyID), st.TrainID)
	pipe.GeoAdd(ctx, geoKey, &redis.GeoLocation{Longitude: st.Lon, Latitude: st.Lat, Name: st.TrainID})
	pipe.Publish(ctx, LiveChannel, st.TrainID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) GetTrainState(ctx context.Context, id string) (*models.TrainState, error) {
	data, err := s.rdb.Get(ctx, stateKey(id)).Bytes()
	switch {
	case err == redis.Nil:
		return nil, nil
	case err != nil:
		return nil, err
	}
	var st models.TrainState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) DeleteTrainState(ctx context.Context, id, agency string) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, stateKey(id))
	pipe.SRem(ctx, activeKey(agency), id)
	pipe.ZRem(ctx, geoKey, id)
	_, err := pipe.Exec(ctx)
	return err
}

// ListActiveTrains returns all active trains for an agency, pruning set entries
// whose state key has expired (SPEC.md review item: the SET never otherwise
// shrinks). Returns the pruned entries that are alive, plus any expired IDs that
// were removed so callers can emit train_removed.
func (s *Store) ListActiveTrains(ctx context.Context, agency string) ([]models.TrainState, []string, error) {
	ids, err := s.rdb.SMembers(ctx, activeKey(agency)).Result()
	if err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}
	// Pipeline existence checks so we only fetch live states.
	alive := []string{}
	var removed []string
	states := []models.TrainState{}
	for _, id := range ids {
		exists, err := s.rdb.Exists(ctx, stateKey(id)).Result()
		if err != nil {
			return nil, nil, err
		}
		if exists == 0 {
			removed = append(removed, id)
		} else {
			alive = append(alive, id)
		}
	}
	if len(removed) > 0 {
		if err := s.rdb.SRem(ctx, activeKey(agency), removed).Err(); err != nil {
			return nil, nil, err
		}
		if err := s.rdb.ZRem(ctx, geoKey, removed).Err(); err != nil {
			return nil, nil, err
		}
	}
	for _, id := range alive {
		st, err := s.GetTrainState(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if st != nil {
			states = append(states, *st)
		}
	}
	return states, removed, nil
}

// ListActiveByAgency returns all active trains for an agency. bbox filtering is
// applied in the API layer, precise and cheap at UTA scale.
func (s *Store) ListActiveByAgency(ctx context.Context, agency string) ([]models.TrainState, error) {
	ids, err := s.rdb.SMembers(ctx, activeKey(agency)).Result()
	if err != nil {
		return nil, err
	}
	var out []models.TrainState
	for _, id := range ids {
		st, err := s.GetTrainState(ctx, id)
		if err != nil {
			return nil, err
		}
		if st != nil {
			out = append(out, *st)
		}
	}
	return out, nil
}

// Publish notifies SSE subscribers that a train changed.
func (s *Store) PublishTrainUpdated(ctx context.Context, trainID string) error {
	return s.rdb.Publish(ctx, LiveChannel, trainID).Err()
}

// Subscribe returns a channel of train IDs that changed, and a cancel func.
func (s *Store) SubscribeLive(ctx context.Context) (<-chan string, func() error, error) {
	pubsub := s.rdb.Subscribe(ctx, LiveChannel)
	ch := make(chan string, 64)
	go func() {
		defer pubsub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-pubsub.Channel():
				if msg != nil {
					ch <- msg.Payload
				}
			}
		}
	}()
	return ch, pubsub.Close, nil
}

// ---------------------------------------------------------------------------
// Ingestion health, exposed via /api/v1/status (SPEC.md review item)
// ---------------------------------------------------------------------------

func (s *Store) SetIngestionHealth(ctx context.Context, h models.IngestionHealth) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, statusKey(h.Adapter), data, 25*time.Hour).Err()
}

func (s *Store) GetIngestionHealth(ctx context.Context, adapter string) (*models.IngestionHealth, error) {
	data, err := s.rdb.Get(ctx, statusKey(adapter)).Bytes()
	switch {
	case err == redis.Nil:
		return nil, nil
	case err != nil:
		return nil, err
	}
	var h models.IngestionHealth
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// ListIngestionHealth returns health for every adapter that has reported.
func (s *Store) ListIngestionHealth(ctx context.Context) ([]models.IngestionHealth, error) {
	var out []models.IngestionHealth
	iter := s.rdb.Scan(ctx, 0, statusKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		data, err := s.rdb.Get(ctx, key).Bytes()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		if err == redis.Nil {
			continue
		}
		var h models.IngestionHealth
		if err := json.Unmarshal(data, &h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, iter.Err()
}

// ---------------------------------------------------------------------------
// Rate limiting. See SPEC.md §11
// ---------------------------------------------------------------------------

// AllowFixedWindow is a fixed-window rate limit using a per-minute bucket key.
func (s *Store) AllowFixedWindow(ctx context.Context, key string, limit int) (remaining, resetSecs int, ok bool, err error) {
	now := time.Now()
	bucket := now.Format("20060102T1504")
	k := "ratelimit:" + key + ":" + bucket
	pipe := s.rdb.TxPipeline()
	pipe.Incr(ctx, k)
	pipe.Expire(ctx, k, time.Hour)
	res, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, false, err
	}
	n := res[0].(*redis.IntCmd).Val()
	reset := time.Now().Truncate(time.Minute).Add(time.Minute)
	remaining = limit - int(n)
	if remaining < 0 {
		remaining = 0
	}
	return remaining, int(reset.Sub(time.Now()).Seconds()), n <= int64(limit), nil
}
