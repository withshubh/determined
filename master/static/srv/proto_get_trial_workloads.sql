WITH validations_vt AS (
  SELECT row_to_json(r1) AS validation,
    total_batch
  FROM (
      SELECT 'STATE_' || v.state as state,
        v.start_time,
        v.end_time,
        s.num_batches,
        s.prior_batches_processed,
        v.total_batch,
        v.metrics->'num_inputs' as num_inputs,
        v.metrics->'validation_metrics' as metrics
      FROM validations v
        INNER JOIN steps s ON v.trial_id = s.trial_id
        AND v.total_batch = s.total_batch
      WHERE v.trial_id = $1
    ) AS r1
),
trainings_vt AS (
  SELECT row_to_json(r1) AS training,
     total_batch
  FROM (
      SELECT s.start_time,
        s.end_time,
        'STATE_' || s.state as state,
        s.num_batches,
        s.prior_batches_processed,
        s.total_batch,
        s.metrics->'avg_metrics' as metrics,
        s.metrics->'num_inputs' as num_inputs
      FROM steps s
      WHERE s.trial_id = $1
    ) AS r1
),
checkpoints_vt AS (
  SELECT row_to_json(r1) AS checkpoint,
    total_batch
  FROM (
      SELECT 'STATE_' || c.state as state,
        c.start_time,
        c.end_time,
        c.uuid,
        c.total_batch,
        s.num_batches,
        s.prior_batches_processed,
        c.resources
      FROM checkpoints c
        INNER JOIN steps s ON c.trial_id = s.trial_id
        AND c.total_batch = s.total_batch
      WHERE c.trial_id = $1
    ) AS r1
)
SELECT v.validation::jsonb - 'total_batch' AS validation,
  t.training::jsonb - 'total_batch' AS training,
  c.checkpoint::jsonb - 'total_batch' AS checkpoint
FROM trainings_vt t
FULL JOIN checkpoints_vt c ON false
FULL JOIN validations_vt v ON false
ORDER BY coalesce(
    t.total_batch,
    v.total_batch,
    c.total_batch
) ASC
