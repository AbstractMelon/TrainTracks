CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE agencies (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    website         TEXT,
    gtfs_static_url TEXT,
    gtfs_rt_url     TEXT,
    timezone        TEXT NOT NULL DEFAULT 'America/Denver',
    license_note    TEXT
);

CREATE TABLE routes (
    id            TEXT PRIMARY KEY,
    agency_id     TEXT NOT NULL REFERENCES agencies(id),
    gtfs_route_id TEXT NOT NULL,
    short_name    TEXT,
    long_name     TEXT,
    mode          TEXT NOT NULL,
    color         TEXT
);
CREATE INDEX routes_agency_idx ON routes (agency_id);

CREATE TABLE stations (
    id           TEXT PRIMARY KEY,
    agency_id    TEXT NOT NULL REFERENCES agencies(id),
    gtfs_stop_id TEXT NOT NULL,
    name         TEXT NOT NULL,
    lat          DOUBLE PRECISION NOT NULL,
    lon          DOUBLE PRECISION NOT NULL,
    geom         GEOMETRY(Point, 4326) NOT NULL
);
CREATE INDEX stations_agency_idx ON stations (agency_id);
CREATE INDEX stations_geom_idx ON stations USING GIST (geom);

CREATE TABLE trips (
    id           TEXT PRIMARY KEY,
    route_id     TEXT NOT NULL REFERENCES routes(id),
    agency_id    TEXT NOT NULL REFERENCES agencies(id),
    service_id   TEXT,
    headsign     TEXT,
    direction_id INT
);
CREATE INDEX trips_route_idx ON trips (route_id);

CREATE TABLE trip_stops (
    trip_id            TEXT NOT NULL REFERENCES trips(id),
    station_id         TEXT NOT NULL REFERENCES stations(id),
    sequence           INT NOT NULL,
    scheduled_arrival  TIME,
    scheduled_departure TIME,
    PRIMARY KEY (trip_id, sequence)
);
CREATE INDEX trip_stops_station_idx ON trip_stops (station_id);

CREATE TABLE track_segments (
    id           BIGSERIAL PRIMARY KEY,
    osm_way_id   BIGINT,
    railway_type TEXT NOT NULL,
    name         TEXT,
    geom         GEOMETRY(LineString, 4326) NOT NULL
);
CREATE INDEX track_segments_geom_idx ON track_segments USING GIST (geom);

CREATE TABLE track_segment_routes (
    track_segment_id BIGINT NOT NULL REFERENCES track_segments(id),
    route_id         TEXT NOT NULL REFERENCES routes(id),
    PRIMARY KEY (track_segment_id, route_id)
);

CREATE TABLE junctions (
    id   BIGSERIAL PRIMARY KEY,
    geom GEOMETRY(Point, 4326) NOT NULL
);
CREATE INDEX junctions_geom_idx ON junctions USING GIST (geom);

CREATE TABLE train_observations (
    id          BIGSERIAL PRIMARY KEY,
    agency_id   TEXT NOT NULL REFERENCES agencies(id),
    train_id    TEXT NOT NULL,
    route_id    TEXT,
    trip_id     TEXT,
    lat         DOUBLE PRECISION NOT NULL,
    lon         DOUBLE PRECISION NOT NULL,
    speed       DOUBLE PRECISION,
    heading     DOUBLE PRECISION,
    observed_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    source      TEXT NOT NULL,
    raw         JSONB
);
CREATE INDEX train_observations_train_time_idx ON train_observations (train_id, observed_at);
CREATE INDEX train_observations_time_idx ON train_observations (observed_at);

-- GTFS-RT trip-update deltas, consumed by the estimation engine. Indexed by
-- trip within a snapshot window; the most recent snapshot wins per trip.
CREATE TABLE trip_delay_predictions (
    trip_id          TEXT NOT NULL,
    agency_id        TEXT NOT NULL REFERENCES agencies(id),
    stop_sequence    INT NOT NULL,
    station_id       TEXT,
    predicted_arrival TIMESTAMPTZ,
    predicted_departure TIMESTAMPTZ,
    delay_seconds    INT,
    reported_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (trip_id, stop_sequence, reported_at)
);
CREATE INDEX trip_delay_predictions_trip_time_idx ON trip_delay_predictions (trip_id, reported_at DESC);