// Command import_to_postgis loads OSM rail ways into track_segments and
// derives junctions at shared endpoints (SPEC.md §8). It is a one-off pipeline,
// not a running service.
//
// Input: the JSON emitted by `extract_utah_rail.sh` (Overpass `out geom json`
// format: an object with an `elements` array of ways carrying a `geometry` of
// {lat,lon} points and tags like `railway`). The osmium path produces GeoJSON,
// which is also supported.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

type overpassElement struct {
	Type     string            `json:"type"`
	ID       int64             `json:"id"`
	Tags     map[string]string `json:"tags"`
	Geometry []struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"geometry"`
	GeometryStr struct {
		Type        string      `json:"-"`
		Coordinates [][]float64 `json:"coordinates"`
	} `json:"-"`
}

type overpassDoc struct {
	Elements []overpassElement `json:"elements"`
}

func main() {
	input := flag.String("input", "utah_rail.json", "Overpass JSON or GeoJSON file")
	url := flag.String("url", os.Getenv("POSTGRES_URL"), "Postgres URL")
	dryRun := flag.Bool("dry-run", false, "parse and print counts without writing")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "set -url or POSTGRES_URL")
		os.Exit(1)
	}

	data, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(1)
	}

	elements, err := parseElements(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Printf("parsed %d rail ways\n", len(elements))
		return
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	inserted, junctions, err := importWays(ctx, pool, elements)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("inserted %d track_segments, %d junctions\n", inserted, junctions)
}

func parseElements(data []byte) ([]overpassElement, error) {
	// Overpass JSON: {"elements":[{"type":"way","id":..,"tags":{...},"geometry":[{lat,lon}...]}]}
	var doc overpassDoc
	if err := json.Unmarshal(data, &doc); err == nil && len(doc.Elements) > 0 {
		return doc.Elements, nil
	}
	// GeoJSON FeatureCollection fallback.
	var fc struct {
		Features []struct {
			Properties map[string]any  `json:"properties"`
			Geometry   json.RawMessage `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(data, &fc); err == nil {
		out := []overpassElement{}
		for i, f := range fc.Features {
			var geom struct {
				Type        string      `json:"type"`
				Coordinates [][]float64 `json:"coordinates"`
			}
			if err := json.Unmarshal(f.Geometry, &geom); err != nil || geom.Type != "LineString" {
				continue
			}
			e := overpassElement{
				Type: "way",
				Tags: map[string]string{},
				GeometryStr: struct {
					Type        string      `json:"-"`
					Coordinates [][]float64 `json:"coordinates"`
				}{Coordinates: geom.Coordinates},
			}
			for k, v := range f.Properties {
				e.Tags[k] = fmt.Sprintf("%v", v)
			}
			e.ID = int64(i)
			out = append(out, e)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported input format (want Overpass JSON or GeoJSON FeatureCollection)")
}

func importWays(ctx context.Context, pool *pgxpool.Pool, els []overpassElement) (int, int, error) {
	inserted := 0
	endpointCount := map[string]int{}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, e := range els {
		if e.Type != "way" {
			continue
		}
		railwayType := e.Tags["railway"]
		if railwayType == "" {
			continue
		}
		var coords [][]float64
		if len(e.Geometry) > 0 {
			for _, g := range e.Geometry {
				coords = append(coords, []float64{g.Lon, g.Lat})
			}
		} else {
			coords = e.GeometryStr.Coordinates
		}
		if len(coords) < 2 {
			continue
		}
		geojson, err := lineString(coords)
		if err != nil {
			return 0, 0, err
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO track_segments (osm_way_id, railway_type, name, geom)
			VALUES ($1, $2, $3, ST_SetSRID(ST_GeomFromGeoJSON($4), 4326))
			ON CONFLICT DO NOTHING RETURNING id`,
			e.ID, railwayType, e.Tags["name"], geojson).Scan(&id); err != nil {
			if err.Error() == "no rows in result set" {
				continue
			}
			return 0, 0, err
		}
		inserted++
		first := coords[0]
		last := coords[len(coords)-1]
		endpointCount[fmt.Sprintf("%.5f,%.5f", first[1], first[0])]++
		endpointCount[fmt.Sprintf("%.5f,%.5f", last[1], last[0])]++
	}

	// Junctions: endpoints shared by >= 2 distinct ways.
	junctionCount := 0
	for key, count := range endpointCount {
		if count < 2 {
			continue
		}
		var lat, lon float64
		if _, err := fmt.Sscanf(key, "%f,%f", &lat, &lon); err != nil {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO junctions (geom) VALUES (ST_SetSRID(ST_MakePoint($1, $2), 4326))`,
			lon, lat); err != nil {
			return 0, 0, err
		}
		junctionCount++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return inserted, junctionCount, nil
}

func lineString(coords [][]float64) (string, error) {
	raw := make([][]float64, 0, len(coords))
	for _, c := range coords {
		raw = append(raw, []float64{round6(c[0]), round6(c[1])})
	}
	b, err := json.Marshal(map[string]any{"type": "LineString", "coordinates": raw})
	if err != nil {
		return "", fmt.Errorf("marshal linestring: %w", err)
	}
	return string(b), nil
}

func round6(f float64) float64 {
	return math.Round(f*1e6) / 1e6
}
