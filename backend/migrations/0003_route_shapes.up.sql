-- GTFS static route shapes (SPEC.md §8, route shapes). Loaded from GTFS
-- shapes.txt by cmd/gtfs-import; geometry covers both rail and bus corridors.
CREATE TABLE shapes (
    id            TEXT PRIMARY KEY,          -- '{agency_id}:{gtfs_shape_id}'
    agency_id     TEXT NOT NULL REFERENCES agencies(id),
    gtfs_shape_id TEXT NOT NULL,
    geom          GEOMETRY(LineString, 4326) NOT NULL
);
CREATE INDEX shapes_geom_idx ON shapes USING GIST (geom);
CREATE INDEX shapes_agency_idx ON shapes (agency_id);

-- Link between a route and the shapes its trips follow (from trips.txt). A
-- route usually has one shape per direction.
CREATE TABLE route_shapes (
    route_id TEXT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    shape_id TEXT NOT NULL REFERENCES shapes(id) ON DELETE CASCADE,
    PRIMARY KEY (route_id, shape_id)
);
CREATE INDEX route_shapes_shape_idx ON route_shapes (shape_id);