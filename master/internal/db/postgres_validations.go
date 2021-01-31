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

// AddValidation adds the validation to the database and sets its ID.
func (db *PgDB) AddValidation(validation *model.Validation) error {
	if !validation.IsNew() {
		return errors.Errorf("unexpected state for new validation: %v", validation)
	}
	trial, err := db.TrialByID(validation.TrialID)
	if err != nil {
		return errors.Wrapf(err, "error finding trial %v for new validation", validation.TrialID)
	}
	if trial.State != model.ActiveState {
		return errors.Errorf("can't add validation to trial %v with state %v", trial.ID, trial.State)
	}
	step, err := db.StepByID(validation.TrialID, validation.StepID)
	if err != nil {
		return errors.Wrapf(err,
			"error finding step (%v, %v) to add validation", validation.TrialID, validation.StepID)
	}
	if step.State != model.CompletedState {
		return errors.Errorf("unexpected state %v for trial %v step %v",
			step.State, validation.TrialID, validation.StepID)
	}
	var count int
	err = db.namedGet(&count, `
SELECT COUNT(*)
FROM validations
WHERE trial_id = :trial_id
AND step_id = :step_id`, validation)
	if err != nil {
		return errors.Wrapf(err, "error checking at-most-one validation %v", *validation)
	}
	if count > 0 {
		return errors.Errorf("duplicate validation for trial %v step %v",
			validation.TrialID, validation.StepID)
	}
	err = db.namedGet(&validation.ID, `
INSERT INTO validations
(trial_id, step_id, state, start_time, end_time)
VALUES (:trial_id, :step_id, :state, :start_time, :end_time)
RETURNING id`, validation)
	if err != nil {
		return errors.Wrapf(err, "error inserting validation %v", *validation)
	}
	return nil
}

// ValidationByStep looks up a validation by trial and step ID, returning nil if none exists.
func (db *PgDB) ValidationByStep(trialID, stepID int) (*model.Validation, error) {
	var validation model.Validation
	if err := db.query(`
SELECT id, trial_id, step_id, state, start_time, end_time, metrics
FROM validations
WHERE trial_id = $1
AND step_id = $2`, &validation, trialID, stepID); errors.Cause(err) == ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "error querying for validation (%v, %v)",
			trialID, stepID)
	}
	return &validation, nil
}

// UpdateValidation updates an existing validation. Fields that are nil or zero
// are not updated. end_time is set if the validation moves to a terminal
// state.
func (db *PgDB) UpdateValidation(trialID, stepID int, newState model.State, metrics model.JSONObj,
) error {
	if len(newState) == 0 && len(metrics) == 0 {
		return nil
	}
	validation, err := db.ValidationByStep(trialID, stepID)
	if err != nil {
		return errors.Wrapf(err, "error querying for validation (%v, %v) to update",
			trialID, stepID)
	}
	if validation == nil {
		return errors.Wrapf(err, "can't update missing validation (%v, %v)",
			trialID, stepID)
	}
	toUpdate := []string{}
	if len(newState) != 0 {
		if !model.StepTransitions[validation.State][newState] {
			return errors.Errorf("illegal transition %v -> %v for validation %v",
				validation.State, newState, validation.ID)
		}
		validation.State = newState
		toUpdate = append(toUpdate, "state")
		if model.TerminalStates[newState] {
			now := time.Now().UTC()
			validation.EndTime = &now
			toUpdate = append(toUpdate, "end_time")
		}
	}
	if len(metrics) != 0 {
		if len(validation.Metrics) != 0 {
			return errors.Errorf("validation (%v, %v) already has metrics",
				trialID, stepID)
		}
		validation.Metrics = metrics
		toUpdate = append(toUpdate, "metrics")
	}
	err = db.namedExecOne(fmt.Sprintf(`
UPDATE validations
%v
WHERE id = :id`, setClause(toUpdate)), validation)
	if err != nil {
		return errors.Wrapf(err, "error updating (%v) in validation (%v, %v)",
			strings.Join(toUpdate, ", "), trialID, stepID)
	}
	return nil
}
