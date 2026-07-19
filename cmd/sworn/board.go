package main

import (
	"context"
	"fmt"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/store"
)

func selectState(ctx context.Context, control *store.Store, runID string) (engine.State, error) {
	if runID != "" {
		return control.State(ctx, runID)
	}
	states, err := control.States(ctx)
	if err != nil {
		return engine.State{}, err
	}
	switch len(states) {
	case 0:
		return engine.State{}, fmt.Errorf("control store contains no runs")
	case 1:
		return states[0], nil
	default:
		return engine.State{}, fmt.Errorf("control store contains %d runs; select one by id", len(states))
	}
}
