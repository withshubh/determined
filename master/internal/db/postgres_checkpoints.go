package db

import (
	"fmt"
	"strings"
	"time"

	_ "github.com/golang-migrate/migrate/source/file" // Load migrations from files.
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v4/stdlib" // Import Postgres driver.
	"github.com/pkg/errors"

	"github.com/determined-ai/determined/master/pkg/model"
)

// AddCheckpoint adds the checkpoint to the database and sets its ID.
func (db *PgDB) AddCheckpoint(checkpoint *model.Checkpoint) error {
	if !checkpoint.IsNew() {
		return errors.Errorf("unexpected state for new checkpoint: %v", checkpoint)
	}
	var count int
	err := db.namedGet(&count, `
SELECT COUNT(*)
FROM checkpoints
WHERE trial_id = :trial_id
AND total_batch = :total_batch`, checkpoint)
	if err != nil {
		return errors.Wrapf(err, "error checking at-most-one checkpoint %v", *checkpoint)
	}
	if count > 0 {
		return errors.Errorf("duplicate checkpoint for trial %v total batch %v",
			checkpoint.TrialID, checkpoint.TotalBatch)
	}
	err = db.namedGet(&checkpoint.ID, `
INSERT INTO checkpoints
(trial_id, total_batch, state, start_time, metadata, determined_version)
VALUES (:trial_id, :total_batch, :state, :start_time, :metadata, :determined_version)
RETURNING id`, checkpoint)
	if err != nil {
		return errors.Wrapf(err, "error inserting checkpoint %v", *checkpoint)
	}
	return nil
}

// CheckpointByTotalBatch looks up a checkpoint by trial and total batch,
// returning nil if none exists.
func (db *PgDB) CheckpointByTotalBatch(trialID, totalBatch int) (*model.Checkpoint, error) {
	var checkpoint model.Checkpoint
	if err := db.query(`
SELECT id, trial_id, total_batch, state, start_time, end_time, uuid, resources, metadata
FROM checkpoints
WHERE trial_id = $1
AND total_batch = $2`, &checkpoint, trialID, totalBatch); errors.Cause(err) == ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "error querying for checkpoint (%v, %v)",
			trialID, totalBatch)
	}
	return &checkpoint, nil
}

// CheckpointByUUID looks up a checkpoint by UUID, returning nil if none exists.
func (db *PgDB) CheckpointByUUID(id uuid.UUID) (*model.Checkpoint, error) {
	var checkpoint model.Checkpoint
	if err := db.query(`
SELECT id, trial_id, total_batch, state, start_time, end_time, uuid, resources, metadata
FROM checkpoints
WHERE uuid = $1`, &checkpoint, id.String()); errors.Cause(err) == ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "error querying for checkpoint (%v)", id.String())
	}
	return &checkpoint, nil
}

// LatestCheckpointForTrial finds the latest completed checkpoint for a trial, returning nil if
// none exists.
func (db *PgDB) LatestCheckpointForTrial(trialID int) (*model.Checkpoint, error) {
	var checkpoint model.Checkpoint
	if err := db.query(`
SELECT id, trial_id, total_batch, state, start_time, end_time, uuid, resources, metadata
FROM checkpoints
WHERE trial_id = $1 AND state = 'COMPLETED'
ORDER BY total_batch DESC
LIMIT 1`, &checkpoint, trialID); errors.Cause(err) == ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "error querying for latest trial checkpoint (%v)", trialID)
	}
	return &checkpoint, nil
}

// UpdateCheckpoint updates an existing checkpoint. Fields that are nil or zero
// are not updated. end_time is set if the checkpoint moves to a terminal
// state.
func (db *PgDB) UpdateCheckpoint(
	trialID, batchNum int,
	newCheckpoint model.Checkpoint,
) error {
	if len(newCheckpoint.State) == 0 && len(*newCheckpoint.UUID) == 0 &&
		len(newCheckpoint.Resources) == 0 && len(newCheckpoint.Metadata) == 0 {
		return nil
	}

	checkpoint, err := db.CheckpointByTotalBatch(trialID, batchNum)
	if err != nil {
		return errors.Wrapf(err, "error querying for checkpoint (%v, %v) to update",
			trialID, batchNum)
	}
	if checkpoint == nil {
		return errors.Wrapf(err, "can't update missing checkpoint (%v, %v)",
			trialID, batchNum)
	}

	toUpdate := []string{}
	if len(newCheckpoint.State) != 0 {
		if !model.CheckpointTransitions[checkpoint.State][newCheckpoint.State] {
			return errors.Errorf("illegal transition %v -> %v for checkpoint %v",
				checkpoint.State, newCheckpoint.State, checkpoint.ID)
		}
		checkpoint.State = newCheckpoint.State
		toUpdate = append(toUpdate, "state")
		if model.TerminalStates[newCheckpoint.State] {
			now := time.Now().UTC()
			checkpoint.EndTime = &now
			toUpdate = append(toUpdate, "end_time")
		}
	}
	if newCheckpoint.UUID != nil && len(*newCheckpoint.UUID) != 0 {
		if checkpoint.UUID != nil && len(*checkpoint.UUID) != 0 {
			return errors.Errorf("checkpoint (%v, %v) already has UUID",
				trialID, batchNum)
		}
		checkpoint.UUID = newCheckpoint.UUID
		toUpdate = append(toUpdate, "uuid")
	}
	if len(newCheckpoint.Resources) != 0 {
		if len(checkpoint.Resources) != 0 {
			return errors.Errorf("checkpoint (%v, %v) already has resources",
				trialID, batchNum)
		}
		checkpoint.Resources = newCheckpoint.Resources
		toUpdate = append(toUpdate, "resources")
	}
	if len(newCheckpoint.Metadata) != 0 {
		if len(checkpoint.Metadata) == 0 {
			checkpoint.Metadata = model.JSONObj{}
		}

		for k, v := range newCheckpoint.Metadata {
			checkpoint.Metadata[k] = v
		}

		toUpdate = append(toUpdate, "metadata")
	}

	if len(newCheckpoint.Framework) != 0 {
		if len(checkpoint.Framework) != 0 {
			return errors.Errorf("checkpoint (%v, %v) already has a framework", trialID, batchNum)
		}

		checkpoint.Framework = newCheckpoint.Framework
		toUpdate = append(toUpdate, "framework")
	}

	if len(newCheckpoint.Format) != 0 {
		if len(checkpoint.Format) != 0 {
			return errors.Errorf("checkpoint (%v, %v) already has a format", trialID, batchNum)
		}

		checkpoint.Format = newCheckpoint.Format
		toUpdate = append(toUpdate, "format")
	}

	err = db.namedExecOne(fmt.Sprintf(`
UPDATE checkpoints
%v
WHERE trial_id = :trial_id AND total_batch = :total_batch`, setClause(toUpdate)), checkpoint)
	if err != nil {
		return errors.Wrapf(err, "error updating (%v) in checkpoint (%v, %v)",
			strings.Join(toUpdate, ", "), trialID, batchNum)
	}
	return nil
}

// UpdateCheckpointMetadata updates an existing checkpoint with the metadata
// attached to the checkpoint passed into the method.
func (db *PgDB) UpdateCheckpointMetadata(checkpoint *model.Checkpoint) error {
	if checkpoint == nil {
		return errors.Errorf("checkpoint cannot be nil does not exist")
	}

	toUpdate := []string{"metadata"}

	err := db.namedExecOne(fmt.Sprintf(`
UPDATE checkpoints
%v
WHERE id = :id`, setClause(toUpdate)), checkpoint)
	if err != nil {
		return errors.Wrapf(err, "error updating (%v) in checkpoint (%v)",
			strings.Join(toUpdate, ", "), checkpoint.UUID)
	}
	return nil
}
