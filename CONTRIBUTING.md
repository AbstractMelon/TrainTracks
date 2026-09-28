# Contributing

Thanks for helping with TrainTracks.

## Ground rules

- **Provenance is sacred.** Never present `ESTIMATED` / `STALE` / `UNKNOWN`
  positions as live telemetry. New endpoints must expose `type` and `source`.
- **Estimation stays honest.** No position may be ESTIMATED past
  `MAX_ESTIMATION_WINDOW`; never surface a synthetic position as `ACTUAL`.
- **Go: no unhandled errors.** Every error path is logged (log/slog) or
  returned.
- **JS/TS: Lucide icons only**, no emojis, no hand-rolled SVGs.

## Development flow

1. Branch from `main`.
2. For backend: `go build ./...`, `go vet ./...`, `go test ./internal/...`.
   Integration tests are opt-in via `TRAINTRACKS_INTEGRATION=1` (they need a
   Postgres + Redis, e.g. from `deploy/docker-compose.dev.yml`).
3. For frontend: `npm run check`, `npm run build`, `npm test`.
4. Keep docs in sync: API changes update `docs/api/openapi.yaml` *and*
   `backend/internal/api/openapi.json` (they must not diverge).

## Adding or changing an adapter

An adapter turns an agency's feed into `TrainObservation` /
`StopTimePrediction` records. See `docs/adapters/ADDING_AN_AGENCY.md` for the
walkthrough, including how static data, geometry, and the estimation per-agency
configuration expectations are wired up.

## Code of conduct

Be excellent to each other. Transit data is a public good; treat it as such.