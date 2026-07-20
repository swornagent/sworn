// Package board derives read-only Baton board projections from engine truth.
package board

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/swornagent/sworn/internal/engine"
)

const SchemaVersion = "delivery-board-v1"

type Projection struct {
	SchemaVersion  string `json:"schema_version"`
	DeliveryID     string `json:"delivery_id"`
	PlanDigest     string `json:"plan_digest"`
	SourceRevision int64  `json:"source_revision"`
	State          string `json:"state"`
	Work           []Work `json:"work"`
}

type Work struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Attempt int64  `json:"attempt"`
	engine.SubmissionBinding
	NextAction string `json:"next_action"`
}

func FromState(state engine.State) (Projection, error) {
	if err := state.Validate(); err != nil {
		return Projection{}, fmt.Errorf("project invalid engine state: %w", err)
	}
	projection := Projection{
		SchemaVersion:  SchemaVersion,
		DeliveryID:     state.DeliveryID,
		PlanDigest:     state.PlanDigest,
		SourceRevision: state.Revision,
		State:          string(state.Phase),
		Work:           make([]Work, len(state.Work)),
	}
	for index, work := range state.Work {
		projection.Work[index] = Work{
			ID: work.ID, State: string(work.State), Attempt: work.Attempt,
			SubmissionBinding: work.SubmissionBinding, NextAction: string(work.NextAction),
		}
		if work.State == engine.WorkChecking {
			projection.Work[index].State = string(engine.WorkActive)
		}
	}
	return projection, nil
}

func WriteJSON(output io.Writer, projection Projection) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(projection)
}

func WriteText(output io.Writer, projection Projection) error {
	if _, err := fmt.Fprintf(output, "%s  %s  revision %d\n", projection.DeliveryID, projection.State, projection.SourceRevision); err != nil {
		return err
	}
	for _, work := range projection.Work {
		if _, err := fmt.Fprintf(output, "  %s  %-8s  next=%s  attempt=%d\n", work.ID, work.State, work.NextAction, work.Attempt); err != nil {
			return err
		}
	}
	return nil
}
