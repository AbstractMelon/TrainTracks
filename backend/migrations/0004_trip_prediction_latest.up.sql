-- Trip-update predictions are only ever read as the newest report per stop
-- (LatestTripPredictions' DISTINCT ON ... reported_at DESC, and every other
-- reader picks the latest row too), while the feed re-reports every stop on
-- every poll. The snapshot history was therefore pure bloat: millions of rows
-- each read had to scan and de-duplicate before returning a few thousand.
-- Keep one row per (trip, stop); the live feed repopulates the table within a
-- single poll, so nothing usable is lost.
TRUNCATE TABLE trip_delay_predictions;

ALTER TABLE trip_delay_predictions
    DROP CONSTRAINT trip_delay_predictions_pkey,
    ADD PRIMARY KEY (trip_id, stop_sequence);

-- Estimation sweeps filter on report age every few seconds.
CREATE INDEX trip_delay_predictions_reported_idx ON trip_delay_predictions (reported_at DESC);
