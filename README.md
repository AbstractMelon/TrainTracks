# TrainTracks

Real-time transit status maps. Ingest GTFS-RT feeds from transit agencies,
track vehicles near-real-time, estimate motion between sparse updates, and
stream the result to the browser over Server-Sent Events. No polling, no
refresh.

## Stack

- **Backend**: Go. Adapter → ingestor → tracking → estimation → REST + SSE.
- **Frontend**: SvelteKit + MapLibre GL + Lucide icons.
- **Data**: Postgres/PostGIS (static data + geometry), Redis (ephemeral live
  state), UTA GTFS-RT feeds.

## Quick start (development)

Backend targets live in `backend/Makefile`; run `make help` there to list them.

```bash
# 1. Bootstrap
cd backend
make setup

# 2. Seed static data (GTFS feed) + history; see docs/adapters/ADDING_AN_AGENCY.md
make seed

# 3. Ingest live feeds: one terminal for both, or one terminal each
make dev          # api + ingestor, both live-reloading
make api          # ...or just the API
make ingestor     # ...or just the ingestor

# 4. Frontend (from the repo root)
cd ../frontend && npm install && npm run dev   # http://localhost:5173
```

## Tests

```bash
cd backend && go test ./internal/...        # unit; integration gated by TRAINTRACKS_INTEGRATION=1
cd frontend && npm test                     # interpolation + geo
```

Full end-to-end integration tests need a live Postgres + Redis (the URLs above):

```bash
TRAINTRACKS_INTEGRATION=1 go test ./internal/integration/...
```

## Documentation

- [Architecture](docs/architecture.md)
- [Estimation methodology](docs/methodology.md)
- [API reference](docs/api/openapi.yaml) (also served at `/api/v1/openapi.json`)
- [Adding a new agency](docs/adapters/ADDING_AN_AGENCY.md)
- [Contributing](CONTRIBUTING.md)

## License

See `LICENSE` (pending). Attribution requirements for the UTA feed data apply;
