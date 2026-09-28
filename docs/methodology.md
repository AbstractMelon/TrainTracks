# Position estimation methodology

This document describes how TrainTracks turns sparse real-time observations
into a believable, moving map. It defines the confidence model, the movement
engine, and the decision rules used for every estimated position.

## Sources and freshness

- An observation has a **source timestamp** (the time the agency says the
  position was valid) and an **ingestion timestamp** (when we received it).
- A train is **fresh** while `now - sourceTimestamp <= FRESH_WINDOW`
  (default **45s**).
- A train whose source timestamp is older than freshness but still inside the
  prune window is **stale** (default prune **12 min** via Redis TTL).
- `last_actual_at` is the timestamp of the most recent raw observation and is
  surfaced on every position so clients can judge recency.

## Position states

| State      | Meaning                                                        |
| ---------- | -------------------------------------------------------------- |
| `ACTUAL`   | We have a raw observation within the fresh window.             |
| `ESTIMATED`| No fresh observation; the engine moved the train meanwhile.    |
| `STALE`    | Last observation is beyond `FRESH_WINDOW`; train still shown.  |
| `UNKNOWN`  | Not enough data / position not trustworthy (shown dimmed).     |

## The movement engine (v1)

At each sweep the engine considers every train with a recent raw position:

1. **Map match** the last observation to the nearest rail segment within
   `MAP_MATCH_MAX_DIST_M` (default 75 m).
2. **Project** onto the segment and extend by `defaultSpeed * elapsed` meters
   along the segment geometry. If the train reaches the end, it continues at
   the next hop in its scheduled trip (first-order look-ahead).
3. **Emit** an ESTIMATED position with `source: estimation-engine`, capped by
   `MAX_ESTIMATION_WINDOW` (default 10 min): beyond that we stop pretending
   and let the state fall to STALE/UNKNOWN.
4. **Converge** when a fresh observation arrives: the estimate is replaced by
   the ACTUAL position.

### Trip timing v1 simplification

Linearly interpolate between the lat/lon of the current and next stations when
a trip is active. The full chainage-based interpolation along route geometry
(SPEC §7 "trip timing") is a later improvement; the pure decision logic is
isolated so it can be upgraded without touching callers.

## Confidence

Confidence is a number in **[0,1]** attached to every ESTIMATED position.

- Raw (ACTUAL) positions carry confidence `1.0` by convention (`nil` in the
  API is treated as unknown by clients).
- Interpolated (ESTIMATED) positions decay linearly from `1.0` toward
  `MIN_CONFIDENCE` (default 0.15) as the estimate ages from `FreshWindow`
  towards `MaxEstimationWindow`.
- Below `MIN_CONFIDENCE` the position is not emitted as ESTIMATED; it becomes
  UNKNOWN instead.

```
confidence(t) = 1 - (1 - MIN_CONFIDENCE) * clamp((t - freshWindow) / (maxWindow - freshWindow), 0, 1)
```

## Why not just connect the dots

The engine exists for UX and honesty, not to fabricate telemetry:

- The API never lets an estimate outlive `MAX_ESTIMATION_WINDOW`.
- `source`, `type`, and `confidence` are part of the API contract so clients
  can fade/annotate interpolated markers (the frontend dims ESTIMATED markers
  with confidence and holds STALE/UNKNOWN markers still).
- Estimations are only produced for trains that genuinely report; there is no
  synthetic "scheduled" producer in v1 (spec §9 `SCHEDULED` generator is
  out of scope for the first release).

## Tuning constants

| Constant                | Default | Config                       |
| ----------------------- | ------- | ---------------------------- |
| Fresh window            | 45s     | `FRESH_WINDOW`               |
| Max estimation window   | 10m     | `MAX_ESTIMATION_WINDOW`      |
| Min confidence          | 0.15    | `MIN_CONFIDENCE`             |
| Map-match max distance  | 75m     | `MAP_MATCH_MAX_DIST_M`       |
| Default speed estimate  | 15 m/s  | `DEFAULT_SPEED_ESTIMATE_MPS` |
| Prediction max age      | 30m     | (trip-update window)         |

## Unit tests

The pure decision functions (`TripUpdateConfidence`, `DeadReckonConfidence`,
`Evaluate`) are covered by `internal/estimation/estimate_test.go`; the geo
helpers by `internal/geo/geo_test.go`; the client-side interpolation by
`frontend/src/lib/animation/interpolate.test.ts`.