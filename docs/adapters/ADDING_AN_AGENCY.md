# Adding an agency

TrainTracks is designed so a new agency slots in with three pieces: **static
data**, a **realtime adapter**, and **entries in the database**. The UTA
adapter (`backend/internal/adapters/uta`) is the reference implementation.

## 1. Static data

Load the agency's GTFS static feed (GTFS.zip) into Postgres. A loader is not
shipped yet; UTA data was inserted by hand. The schema you must populate:

- `agencies`: one row per agency. `id` is the agency key used in train ids
  (`{agency}:{vehicle}`).
- `routes`, `stations` (stops), `trips`, `trip_stops`, `stop_times`: GTFS
  tables. `trip_stops.*_departure`/`*_arrival` are `HH:MM:SS` service times.
- Rail geometry comes from OpenStreetMap via `osm-import/`
  (`extract_utah_rail.sh` + `import_to_postgis.go`) into `track_segments`.

Set `agencies.gtfs_static_url` / `gtfs_rt_url` for future automated loading.

## 2. Realtime adapter

Create `backend/internal/adapters/{name}/adapter.go` implementing the adapter
interface (vehicle positions + trip updates → `TrainObservation` /
`StopTimePrediction`). Follow the UTA adapter closely:

- Parse GTFS-RT protobuf with
  `github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs`.
- Emit normalized observations: agency id, vehicle id, lat/lon, timestamp,
  optional trip/route refs, heading/speed when the feed provides them.
- Match stop ids to our `stations.gtfs_stop_id` (`Postgres.LookupStopTime`).
- Record the feed's *source timestamp*; freshness decisions depend on it.

Register the adapter in `cmd/ingestor/main.go`, and give it a distinct
`AdapterID` so health/status and rate-limit keys stay per-agency.

## 3. Database entries

- Insert the agency into `agencies` (timezone is used for HH:MM:SS service
  times).
- Insert routes/stations/trips/stop times.
- `INSERT ... ` utilities in `osm-import/` or ad-hoc SQL for geometry.

## 4. Config

Per-agency behavior is driven by config/env (see `backend/internal/config`):
the estimator's `FRESH_WINDOW`, `MAX_ESTIMATION_WINDOW`,
`MIN_CONFIDENCE`, and `DEFAULT_SPEED_ESTIMATE_MPS` are global in v1; give each
agency a row in a config table when they need different tuning.

## 5. Verification checklist

- [ ] `/api/v1/agencies` lists the new agency.
- [ ] A live train appears in `/api/v1/trains` with `source` and `type`.
- [ ] The ingestor writes health (`/api/v1/status`) for the adapter.
- [ ] Network geometry renders under `/api/v1/tracks?bbox=…`.
- [ ] Trip-update predictions land in `trip_delay_predictions` and influence
      estimated positions (`docs/methodology.md`).
- [ ] Unit tests: adapter decode fixtures committed under `testdata/`.