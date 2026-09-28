# Architecture

TrainTracks is a real-time transit status system: it ingests GTFS-RT feeds from
agencies, tracks vehicles in near-real-time, estimates motion between updates,
and streams the result to the browser over Server-Sent Events.

```
┌─────────┐   ┌────────────┐   ┌───────────┐   ┌──────────────┐   ┌──────────┐
│ UTA     │   │ ingestor   │   │ redis     │   │ api          │   │ frontend │
│ GTFS-RT ├──▶│ adapter    │──▶│ live state│──▶│ REST + SSE   │──▶│ sveltekit│
│ feeds   │   │ → tracking │   │           │   │              │   │ + maplibre
└─────────┘   └─────┬──────┘   └─────┬─────┘   └──────┬───────┘   └──────────┘
                    │                │                 │
                    └──────┬─────────┘                 └── postgres (static data,
                           │                             map-match, predictions)
                    pub/sub `live:updates`
```

## Components

### Backend (`backend/`, Go)

- **Adapters** (`internal/adapters/uta`): fetch vehicle positions and trip
  updates from an agency's GTFS-RT endpoints, decode protobuf, and produce
  normalized `TrainObservation` / `StopTimePrediction` records.
- **Ingestor** (`internal/ingestor`): poll loops per adapter with jitter,
  retry with exponential backoff, a per-adapter circuit breaker, and health
  reporting written to Redis (`ingestor:status:{adapter}`).
- **Tracking** (`internal/tracking`): derives `TrainState` from a stream of
  observations: last position, heading, speed, active trip, route, next
  station, delay.
- **Estimation** (`internal/estimation`): the "make it move" engine (see
  `docs/methodology.md`). It sweeps fresh trains, decides whether to move them
  along their route, computes confidence, and writes ESTIMATED states.
- **Geo** (`internal/geo`): map-matching observations to PostGIS rail
  geometry and distance/heading helpers.
- **Storage**:
  - *Postgres* (`internal/storage/postgres`): static data (agencies, routes,
    stations, trips, trip stops), map matching, scheduled arrivals, track
    geometry, trip delay predictions.
  - *Redis* (`internal/storage/redis`): ephemeral live state with TTLs, the
    active-train set per agency, a geo index (`trains:geo`), rate limiting,
    and ingestion health.
- **API** (`internal/api`): chi router. REST endpoints plus an _SSE hub_
  (`internal/api/sse`) that broadcasts `train_update` / `train_removed` /
  `heartbeat` events and replays a snapshot on connect.
  OpenAPI served at `/api/v1/openapi.json` (source: `docs/api/openapi.yaml`).

### Frontend (`frontend/`, SvelteKit)

- Run-by-runes stores (`src/lib/state/*.svelte.ts`) holding live trains and
  filter state; the SSE client (`src/lib/api/live.ts`) fills them.
- MapLibre GL map (`src/lib/components/Map.svelte`) with layered track
  geometry, stations, and a train marker layer
  (`src/lib/components/TrainMarkerLayer.svelte`).
- Client-side interpolation between updates
  (`src/lib/animation/interpolate.ts`): ACTUAL/ESTIMATED trains glide,
  STALE/UNKNOWN hold still; ESTIMATED markers fade with confidence.
- Data flows through `/api/v1` (proxied to the API server in dev by vite).

### Deploy (`deploy/`)

- `docker-compose.dev.yml`: Postgres + Redis for local development.
- `docker-compose.yml`: the full stack (postgres, redis, migrate, ingestor,
  api, frontend). The API and frontend publish host ports (`API_PORT`,
  `FRONTEND_PORT`); TLS and reverse proxy are left to whatever you already
  run.

## Data flow

1. Ingestor polls an adapter, emitting `TrainObservation`s.
2. Tracking turns each observation into a `TrainState`, writes it to Redis
   (`train:{id}:state`, TTL 12m), adds the id to the agency active set, and
   publishes `live:updates`.
3. The estimation engine sweeps every ~5s: fresh trains get moved along their
   route (ESTIMATED) until new observations arrive or the window elapses.
4. The API hub subscribes to `live:updates`, filters by bbox/agency/route, and
   pushes to connected SSE clients. REST endpoints read the same stores.

## Conventions on provenance

Every served position carries its origin:
- `source`: `uta-gtfs-rt` (raw) or `estimation-engine` (interpolated).
- `type`: `ACTUAL` (recent raw), `ESTIMATED` (interpolated), `STALE`
  (gone quiet but still shown), `UNKNOWN` (position not trustworthy).

Clients must never present ESTIMATED/STALE positions as live telemetry; the
`confidence` field quantifies how trustworthy an ESTIMATED position is.