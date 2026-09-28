// Command gtfs-import downloads an agency's GTFS static feed and loads it into
// Postgres: routes, stations (stops), trips, stop_times, and shapes geometry
// (bus + rail corridors). Idempotent: re-running refreshes from a fresh feed.
//
// Usage:
//
//	AGENCY_ID=uta GTFS_STATIC_URL=https://apps.rideuta.com/tms/gtfs/Static \
//		POSTGRES_URL=postgres://traintracks:traintracks@localhost:15432/traintracks?sslmode=disable \
//		go run ./cmd/gtfs-import
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"traintracks/internal/config"
	"traintracks/internal/models"
	"traintracks/internal/storage/postgres"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Default()

	agencyID := os.Getenv("AGENCY_ID")
	if agencyID == "" {
		agencyID = "uta"
	}
	staticURL := os.Getenv("GTFS_STATIC_URL")
	if staticURL == "" {
		staticURL = cfg.UTAGtfsStaticURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pg, err := postgres.New(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer pg.Close()

	logger.Info("downloading GTFS static", "url", staticURL)
	zr, err := fetchStatic(ctx, staticURL)
	if err != nil {
		logger.Error("download GTFS static", "err", err)
		os.Exit(1)
	}

	ds, err := parseDataset(zr, agencyID)
	if err != nil {
		logger.Error("parse GTFS static", "err", err)
		os.Exit(1)
	}

	logger.Info("loading into postgres",
		"agency", ds.Agency.ID,
		"routes", len(ds.Routes),
		"stations", len(ds.Stations),
		"trips", len(ds.Trips),
		"trip_stops", len(ds.TripStops),
		"shapes", len(ds.Shapes),
		"route_shapes", len(ds.RouteShapes))

	if err := pg.LoadStatic(ctx, ds); err != nil {
		logger.Error("load static", "err", err)
		os.Exit(1)
	}
	logger.Info("done", "agency", ds.Agency.ID)
}

// fetchStatic downloads the feed zip and returns a zip.Reader over it.
func fetchStatic(ctx context.Context, url string) (*zip.Reader, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TrainTracks.gtfs-import/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return zip.NewReader(bytes.NewReader(body), int64(len(body)))
}

// streamRows calls fn for every data row of a CSV file inside the zip,
// passing the trimmed record and a header index.
func streamRows(zr *zip.Reader, name string, fn func(rec []string, h map[string]int) error) error {
	f := findFile(zr, name)
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	r := csv.NewReader(rc)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return err
	}
	h := make(map[string]int, len(header))
	for i, col := range header {
		h[strings.TrimSpace(col)] = i
	}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(trimAll(rec), h); err != nil {
			return err
		}
	}
}

func findFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func trimAll(rec []string) []string {
	out := make([]string, len(rec))
	for i, s := range rec {
		out[i] = strings.TrimSpace(s)
	}
	return out
}

func get(rec []string, h map[string]int, col string) string {
	i, ok := h[col]
	if !ok || i >= len(rec) {
		return ""
	}
	return rec[i]
}

// parseDataset converts the feed zip into a postgres.StaticDataset.
func parseDataset(zr *zip.Reader, agencyID string) (postgres.StaticDataset, error) {
	var ds postgres.StaticDataset
	ds.Agency = models.Agency{ID: agencyID}

	if err := parseAgency(zr, &ds.Agency); err != nil {
		return ds, err
	}
	if err := parseShapes(zr, &ds, agencyID); err != nil {
		return ds, err
	}
	if err := parseRoutes(zr, &ds, agencyID); err != nil {
		return ds, err
	}
	if err := parseStations(zr, &ds, agencyID); err != nil {
		return ds, err
	}
	if err := parseTrips(zr, &ds, agencyID); err != nil {
		return ds, err
	}
	if err := parseStopTimes(zr, &ds, agencyID); err != nil {
		return ds, err
	}
	return ds, nil
}

func parseAgency(zr *zip.Reader, a *models.Agency) error {
	return streamRows(zr, "agency.txt", func(rec []string, h map[string]int) error {
		if a.Name == "" {
			a.Name = get(rec, h, "agency_name")
		}
		if a.Website == "" {
			a.Website = get(rec, h, "agency_url")
		}
		if a.Timezone == "" {
			a.Timezone = get(rec, h, "agency_timezone")
		}
		return nil
	})
}

func parseRoutes(zr *zip.Reader, ds *postgres.StaticDataset, agencyID string) error {
	return streamRows(zr, "routes.txt", func(rec []string, h map[string]int) error {
		gtfsRouteID := get(rec, h, "route_id")
		if gtfsRouteID == "" {
			return nil
		}
		ds.Routes = append(ds.Routes, models.Route{
			ID:          agencyID + ":" + gtfsRouteID,
			AgencyID:    agencyID,
			GTFSRouteID: gtfsRouteID,
			ShortName:   get(rec, h, "route_short_name"),
			LongName:    get(rec, h, "route_long_name"),
			Mode:        modeFromType(get(rec, h, "route_type")),
			Color:       normalizeColor(get(rec, h, "route_color")),
		})
		return nil
	})
}

func parseStations(zr *zip.Reader, ds *postgres.StaticDataset, agencyID string) error {
	return streamRows(zr, "stops.txt", func(rec []string, h map[string]int) error {
		stopID := get(rec, h, "stop_id")
		lat, errL := strconv.ParseFloat(get(rec, h, "stop_lat"), 64)
		lon, errLon := strconv.ParseFloat(get(rec, h, "stop_lon"), 64)
		if stopID == "" || errL != nil || errLon != nil {
			return nil
		}
		ds.Stations = append(ds.Stations, models.Station{
			ID:         agencyID + ":" + stopID,
			AgencyID:   agencyID,
			GTFSStopID: stopID,
			Name:       get(rec, h, "stop_name"),
			Lat:        lat,
			Lon:        lon,
		})
		return nil
	})
}

func parseTrips(zr *zip.Reader, ds *postgres.StaticDataset, agencyID string) error {
	seen := map[string]bool{}
	return streamRows(zr, "trips.txt", func(rec []string, h map[string]int) error {
		tripID := get(rec, h, "trip_id")
		gtfsRouteID := get(rec, h, "route_id")
		if tripID == "" || gtfsRouteID == "" {
			return nil
		}
		routeID := agencyID + ":" + gtfsRouteID
		serviceID := get(rec, h, "service_id")
		headsign := get(rec, h, "trip_headsign")
		var dir *int
		if s := get(rec, h, "direction_id"); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				dir = &n
			}
		}
		ds.Trips = append(ds.Trips, models.Trip{
			ID:          agencyID + ":" + tripID,
			RouteID:     routeID,
			AgencyID:    agencyID,
			ServiceID:   serviceID,
			Headsign:    headsign,
			DirectionID: dir,
		})
		if shapeID := get(rec, h, "shape_id"); shapeID != "" {
			key := routeID + "|" + shapeID
			if !seen[key] {
				seen[key] = true
				ds.RouteShapes = append(ds.RouteShapes, models.RouteShape{
					RouteID: routeID,
					ShapeID: agencyID + ":" + shapeID,
				})
			}
		}
		return nil
	})
}

func parseStopTimes(zr *zip.Reader, ds *postgres.StaticDataset, agencyID string) error {
	return streamRows(zr, "stop_times.txt", func(rec []string, h map[string]int) error {
		tripID := get(rec, h, "trip_id")
		stopID := get(rec, h, "stop_id")
		if tripID == "" || stopID == "" {
			return nil
		}
		seq, err := strconv.Atoi(get(rec, h, "stop_sequence"))
		if err != nil {
			return nil
		}
		ds.TripStops = append(ds.TripStops, models.TripStop{
			TripID:             agencyID + ":" + tripID,
			StationID:          agencyID + ":" + stopID,
			Sequence:           seq,
			ScheduledArrival:   normTimePtr(get(rec, h, "arrival_time")),
			ScheduledDeparture: normTimePtr(get(rec, h, "departure_time")),
		})
		return nil
	})
}

// parseShapes groups shapes.txt points and builds one LineString per shape,
// ordered by shape_pt_sequence with consecutive duplicate points removed.
func parseShapes(zr *zip.Reader, ds *postgres.StaticDataset, agencyID string) error {
	type point struct {
		seq int
		lat float64
		lon float64
	}
	byShape := map[string][]point{}

	err := streamRows(zr, "shapes.txt", func(rec []string, h map[string]int) error {
		shapeID := get(rec, h, "shape_id")
		lat, errL := strconv.ParseFloat(get(rec, h, "shape_pt_lat"), 64)
		lon, errLon := strconv.ParseFloat(get(rec, h, "shape_pt_lon"), 64)
		seq, errSeq := strconv.Atoi(get(rec, h, "shape_pt_sequence"))
		if shapeID == "" || errL != nil || errLon != nil || errSeq != nil {
			return nil
		}
		byShape[shapeID] = append(byShape[shapeID], point{seq: seq, lat: lat, lon: lon})
		return nil
	})
	if err != nil {
		return err
	}

	for shapeID, pts := range byShape {
		sort.Slice(pts, func(i, j int) bool { return pts[i].seq < pts[j].seq })
		line := [][]float64{}
		for _, p := range pts {
			pt := []float64{p.lon, p.lat}
			if n := len(line); n > 0 && line[n-1][0] == pt[0] && line[n-1][1] == pt[1] {
				continue
			}
			line = append(line, pt)
		}
		ds.Shapes = append(ds.Shapes, models.Shape{
			ID:          agencyID + ":" + shapeID,
			AgencyID:    agencyID,
			GTFSShapeID: shapeID,
			Line:        line,
		})
	}
	return nil
}

// normTimePtr normalizes a GTFS service time ("HH:MM:SS", possibly ">24:00" or
// breathy like " 4:15:00") into a TIME-acceptable "HH:MM:SS". Times ≥ 24h are
// rolled onto the current day so they fit the TIME column; late-night service
// shows as early-morning times (a documented simplification).
func normTimePtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return nil
	}
	hh, e1 := strconv.Atoi(parts[0])
	mm, e2 := strconv.Atoi(parts[1])
	ss, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || mm > 59 || ss > 59 {
		return nil
	}
	if hh >= 24 {
		hh -= 24
	}
	out := fmt.Sprintf("%02d:%02d:%02d", hh, mm, ss)
	return &out
}

func modeFromType(routeType string) string {
	switch routeType {
	case "0":
		return "light_rail" // GTFS: light rail / tram
	case "1":
		return "subway"
	case "2":
		return "commuter_rail" // GTFS: rail
	case "3":
		return "bus"
	case "4":
		return "ferry"
	case "5":
		return "cable_tram"
	case "6":
		return "aerial_lift"
	case "7":
		return "funicular"
	case "11":
		return "trolleybus"
	}
	n, err := strconv.Atoi(routeType)
	if err != nil {
		return "other"
	}
	switch {
	case n >= 700 && n < 800, n >= 200 && n < 300: // bus services
		return "bus"
	case n >= 100 && n < 200: // rail services
		return "commuter_rail"
	case n >= 400 && n < 500: // metro services
		return "subway"
	case n >= 900 && n < 1000: // tram services
		return "light_rail"
	}
	return "other"
}

func normalizeColor(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return ""
	}
	if !strings.HasPrefix(c, "#") {
		c = "#" + c
	}
	return c
}
