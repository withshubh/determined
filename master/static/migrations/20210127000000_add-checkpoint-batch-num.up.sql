-- Replace steps with total_batch in the checkpoints table.
ALTER TABLE checkpoints ADD COLUMN total_batch integer NULL;

UPDATE checkpoints AS c
SET total_batch = (
    SELECT
           s.prior_batches_processed + s.num_batches AS total_batch
    FROM steps s
    WHERE c.step_id = s.id AND c.trial_id = s.trial_id
)
WHERE total_batch IS NULL;

ALTER TABLE checkpoints
    ADD CONSTRAINT checkpoints_trial_total_batch_unique UNIQUE (trial_id, total_batch);

ALTER TABLE checkpoints DROP COLUMN step_id;

-- Replace steps with total_batch in the validations table.
ALTER TABLE validations ADD COLUMN total_batch integer NULL;

UPDATE validations AS v
SET total_batch = (
    SELECT
            s.prior_batches_processed + s.num_batches AS total_batch
    FROM steps s
    WHERE v.step_id = s.id AND v.trial_id = s.trial_id
)
WHERE total_batch IS NULL;

ALTER TABLE validations
    ADD CONSTRAINT validations_trial_total_batch_unique UNIQUE (trial_id, total_batch);

ALTER TABLE validations DROP COLUMN step_id;

-- Add total_batch in the steps table.
ALTER TABLE steps ADD COLUMN total_batch integer NULL;

UPDATE steps AS s
SET total_batch = s.prior_batches_processed + s.num_batches
WHERE total_batch IS NULL;

ALTER TABLE steps
    ADD CONSTRAINT steps_trial_total_batch_unique UNIQUE (trial_id, total_batch);
