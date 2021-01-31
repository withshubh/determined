package db

import (
	"fmt"
	"strings"
	"time"

	_ "github.com/golang-migrate/migrate/source/file" // Load migrations from files.
	_ "github.com/jackc/pgx/v4/stdlib"                // Import Postgres driver.
	"github.com/pkg/errors"

	"github.com/determined-ai/determined/master/pkg/model"
)

// AddStep adds the step to the database.
func (db *PgDB) AddStep(step *model.Step) error {
	if !step.IsNew() {
		return errors.Errorf("unexpected state for new step: %v", step)
	}
	trial, err := db.TrialByID(step.TrialID)
	if err != nil {
		return errors.Wrapf(err, "error finding trial %v for new step", step.TrialID)
	}
	if trial.State != model.ActiveState {
		return errors.Errorf("can't add step to trial %v with state %v", trial.ID, trial.State)
	}
	err = db.namedExecOne(`
INSERT INTO steps
(trial_id, id, state, start_time, end_time, num_batches, prior_batches_processed)
VALUES (:trial_id, :id, :state, :start_time, :end_time, :num_batches, :prior_batches_processed)`,
		step)
	if err != nil {
		return errors.Wrapf(err, "error inserting step %v", *step)
	}
	return nil
}

// AddNoOpStep adds a no-op completed step to the database. This is used for trials with initial
// validations (used for testing models pre-fine-tuning).
func (db *PgDB) AddNoOpStep(step *model.Step) error {
	if step.State != model.CompletedState {
		return errors.Errorf("unexpected state for new step: %v", step)
	}
	trial, err := db.TrialByID(step.TrialID)
	if err != nil {
		return errors.Wrapf(err, "error finding trial %v for new step", step.TrialID)
	}
	if trial.State != model.ActiveState {
		return errors.Errorf("can't add step to trial %v with state %v", trial.ID, trial.State)
	}
	err = db.namedExecOne(`
INSERT INTO steps
(trial_id, id, state, start_time, end_time, num_batches, prior_batches_processed)
VALUES (:trial_id, :id, :state, :start_time, :end_time, :num_batches, :prior_batches_processed)`,
		step)
	if err != nil {
		return errors.Wrapf(err, "error inserting step %v", *step)
	}
	return nil
}

// StepByID looks up a step by (TrialID, StepID) pair, returning an error if none exists.
func (db *PgDB) StepByID(trialID, stepID int) (*model.Step, error) {
	var step model.Step
	if err := db.query(`
SELECT trial_id, id, state, start_time, end_time, metrics, num_batches, prior_batches_processed
FROM steps
WHERE trial_id = $1 AND id = $2`, &step, trialID, stepID); err != nil {
		return nil, errors.Wrapf(err, "error querying for step %v, %v", trialID, stepID)
	}
	return &step, nil
}

// UpdateStep updates an existing step. Fields that are nil or zero are not
// updated.  end_time is set if the step moves to a terminal state.
func (db *PgDB) UpdateStep(
	trialID, stepID int, newState model.State, metrics model.JSONObj) error {
	if len(newState) == 0 && len(metrics) == 0 {
		return nil
	}
	step, err := db.StepByID(trialID, stepID)
	if err != nil {
		return errors.Wrapf(err, "error finding step (%v, %v) to update", trialID, stepID)
	}
	toUpdate := []string{}
	if len(newState) != 0 {
		if !model.StepTransitions[step.State][newState] {
			return errors.Errorf("illegal transition %v -> %v for step (%v, %v)",
				step.State, newState, step.TrialID, step.ID)
		}
		step.State = newState
		toUpdate = append(toUpdate, "state")
		if model.TerminalStates[newState] {
			now := time.Now().UTC()
			step.EndTime = &now
			toUpdate = append(toUpdate, "end_time")
		}
	}
	if len(metrics) != 0 {
		if len(step.Metrics) != 0 {
			return errors.Errorf("step (%v, %v) already has metrics", trialID, stepID)
		}
		step.Metrics = metrics
		toUpdate = append(toUpdate, "metrics")
	}
	err = db.namedExecOne(fmt.Sprintf(`
UPDATE steps
%v
WHERE trial_id = :trial_id
AND id = :id`, setClause(toUpdate)), step)
	if err != nil {
		return errors.Wrapf(err, "error updating (%v) in step (%v, %v)",
			strings.Join(toUpdate, ", "), step.TrialID, step.ID)
	}
	return nil
}
