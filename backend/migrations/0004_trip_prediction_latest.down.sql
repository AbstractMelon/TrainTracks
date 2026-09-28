-- Restores the snapshot schema. Truncated prediction history is not restored;
-- the live feed repopulates the table on its next poll.
DROP INDEX IF EXISTS trip_delay_predictions_reported_idx;

ALTER TABLE trip_delay_predictions
    DROP CONSTRAINT trip_delay_predictions_pkey,
    ADD PRIMARY KEY (trip_id, stop_sequence, reported_at);
