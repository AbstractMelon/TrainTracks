#!/usr/bin/env bash
# Extracts Utah rail geometry for the TrainTracks track_segments table (SPEC §8).
#
# This is a one-off pipeline, NOT a running service. Re-run manually when OSM
# rail data changes.
#
# Two supported paths:
#   1. osmium extract from a Geofabrik regional .osm.pbf (fast, needs osmosis/osmium-tool),
#   2. an Overpass API query scoped to UTA's service area (no PBF download).
#
# Output: a GeoJSON fragment (Overpass `out geom json`) on stdout, or written
# to ./utah_rail.json when -o is given.

set -euo pipefail

OUT="${OUT:-utah_rail.json}"
METHOD="${METHOD:-overpass}"

bbox="(-112.6,40.3,-111.5,41.3)"  # UTA service area

if [ "$METHOD" = "overpass" ]; then
  query="\
  [out:json][timeout:120];\
  (\
    way[\"railway\"~\"^(rail|light_rail|tram|subway|narrow_gauge)\"]${bbox};\
  );\
  out geom;"
  if command -v curl >/dev/null 2>&1; then
    curl -s -G --data-urlencode "data=$query" https://overpass-api.de/api/interpreter > "$OUT"
  else
    echo "curl is required for the overpass method" >&2
    exit 1
  fi
elif [ "$METHOD" = "osmium" ]; then
  # Requires: osmium-tool + a Utah/regional PBF (https://download.geofabrik.de/)
  PBF="${PBF:?set PBF to a .osm.pbf for the region}"
  osmium tags-filter "$PBF" w/railway=rail w/railway=light_rail w/railway=tram w/railway=subway -o "$OUT.osm"
  osmium export "$OUT.osm" -o "$OUT"   # GeoJSON
else
  echo "unknown METHOD=$METHOD (supported: overpass|osmium)" >&2
  exit 1
fi

echo "wrote $OUT. Now run: go run ./import_to_postgis.go -input $OUT -url \$POSTGRES_URL"