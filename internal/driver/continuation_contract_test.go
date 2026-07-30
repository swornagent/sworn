package driver

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type continuationContractState struct {
	mu       sync.Mutex
	body     []byte
	reported int64
	mode     ContinuationMode
	closes   int
	closeErr error
}

func (state *continuationContractState) continuationMode() ContinuationMode {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.mode
}

func (state *continuationContractState) continuationBytes() int64 {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.reported
}

func (state *continuationContractState) closeContinuation() error {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.closes++
	clearBytes(state.body)
	state.reported = 0
	state.mode = ""
	return state.closeErr
}

func (state *continuationContractState) snapshot() ([]byte, int64, int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append([]byte(nil), state.body...), state.reported, state.closes
}

func (state *continuationContractState) setReported(size int64) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.reported = size
}

type continuationContractAdapter struct {
	identity AdapterIdentity

	mu            sync.Mutex
	invocations   int
	turns         int
	resumes       int
	retain        bool
	stateMode     ContinuationMode
	stateBytes    int64
	stateCloseErr error
	resumeErr     error
	created       *continuationContractState
}

func newContinuationContractAdapter(key, configuration string) *continuationContractAdapter {
	return &continuationContractAdapter{
		identity: AdapterIdentity{
			Key:                 key,
			ID:                  "sworn." + key,
			Version:             "1.0.0",
			ConfigurationDigest: Digest([]byte(configuration)),
		},
		retain:     true,
		stateMode:  ContinuationModeTranscriptReplay,
		stateBytes: 64,
	}
}

func (adapter *continuationContractAdapter) Identity() AdapterIdentity {
	return adapter.identity
}

func (adapter *continuationContractAdapter) invoke(
	_ context.Context,
	invocation Invocation,
) (Observation, error) {
	adapter.mu.Lock()
	adapter.invocations++
	adapter.mu.Unlock()
	return continuationContractObservation(invocation)
}

func (adapter *continuationContractAdapter) invokeContinuation(
	_ context.Context,
	invocation Invocation,
) (Observation, continuationState, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.turns++
	observation, err := continuationContractObservation(invocation)
	if err != nil || !adapter.retain {
		return observation, nil, err
	}
	body := []byte("adapter-private-state")
	adapter.created = &continuationContractState{
		body:     body,
		reported: adapter.stateBytes,
		mode:     adapter.stateMode,
		closeErr: adapter.stateCloseErr,
	}
	return observation, adapter.created, nil
}

func (adapter *continuationContractAdapter) resumeContinuation(
	_ context.Context,
	invocation Invocation,
	prior continuationState,
) (Observation, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.turns++
	if prior != adapter.created {
		return Observation{}, fail("CONTINUATION_INVALID")
	}
	adapter.resumes++
	if adapter.resumeErr != nil {
		return Observation{}, adapter.resumeErr
	}
	return continuationContractObservation(invocation)
}

func (adapter *continuationContractAdapter) counts() (int, int, int) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.invocations, adapter.turns, adapter.resumes
}

type oneShotContractAdapter struct {
	identity AdapterIdentity
	mu       sync.Mutex
	calls    int
}

func (adapter *oneShotContractAdapter) Identity() AdapterIdentity {
	return adapter.identity
}

func (adapter *oneShotContractAdapter) invoke(
	_ context.Context,
	invocation Invocation,
) (Observation, error) {
	adapter.mu.Lock()
	adapter.calls++
	adapter.mu.Unlock()
	return continuationContractObservation(invocation)
}

func (adapter *oneShotContractAdapter) count() int {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.calls
}

func continuationContractObservation(
	invocation Invocation,
) (Observation, error) {
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return Observation{}, err
	}
	submission := Submission{
		SchemaVersion:  SubmissionSchemaVersion,
		InvocationID:   invocation.Request.InvocationID,
		Responsibility: descriptor.Responsibility,
		Summary:        "Continuation contract fixture.",
		Detail:         "Bounded fixture detail.\n",
	}
	switch descriptor.Responsibility {
	case PlannerProposal:
		submission.Plan, err = NewPlanBytes(validPlanBytes())
	case ImplementerImplementation, WorkVerification, AssemblyVerification:
		submission.Checks, err = NewCheckBytes([]byte("checks\n"))
	}
	if err != nil {
		return Observation{}, err
	}
	switch descriptor.Responsibility {
	case CaptainReview:
		submission.Decision, err = NewDecision(DecisionProceed)
	case WorkVerification, AssemblyVerification:
		submission.Decision, err = NewDecision(DecisionPass)
	}
	if err != nil {
		return Observation{}, err
	}
	body, err := EncodeSubmission(submission)
	if err != nil {
		return Observation{}, err
	}
	server, err := newSubmissionServer(invocation.Permission)
	if err != nil {
		return Observation{}, err
	}
	seal, sealBytes, err := server.Submit(body)
	if err != nil || !seal.Accepted {
		return Observation{}, err
	}
	return Observation{
		TransportStatus: Completed,
		Usage: UsageReceipt{
			TokenStatus: UsageUnavailable,
			CostStatus:  UsageUnavailable,
		},
		Diagnostic: Diagnostic{Code: "none"},
		Handoff: &SealedHandoff{
			SubmissionBytes:  body,
			SubmissionDigest: Digest(body),
			SealBytes:        sealBytes,
			SealDigest:       Digest(sealBytes),
		},
	}, nil
}

func continuationContractInvocation(
	t *testing.T,
	adapter Adapter,
	invocationID string,
	role Role,
	responsibility Responsibility,
	access WorkspaceAccess,
	fresh bool,
) Invocation {
	t.Helper()
	return continuationContractInvocationWithSelection(
		t,
		adapter,
		invocationID,
		role,
		responsibility,
		access,
		fresh,
		"continuation-profile",
		"continuation-model",
	)
}

func continuationContractInvocationWithSelection(
	t *testing.T,
	adapter Adapter,
	invocationID string,
	role Role,
	responsibility Responsibility,
	access WorkspaceAccess,
	fresh bool,
	profile string,
	model string,
) Invocation {
	t.Helper()
	selected := SelectedProfile{
		Profile: ProfileConfig{
			Key:     profile,
			Adapter: adapter.Identity().Key,
			Network: NetworkNone,
		},
		Adapter: adapter.Identity(),
		Model:   model,
		adapter: adapter,
	}
	request, err := NewRequest(
		invocationID,
		role,
		selected.Profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: access},
		[]Input{},
		fresh,
		Limits{TimeoutMillis: 60_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		containment,
		responsibility,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:       request,
		HostWorkspace: t.TempDir(),
		Selected:      selected,
		Permission:    permission,
		Inputs:        []InputContent{},
	}
}

func continuationContractBinding() ContinuationBinding {
	return ContinuationBinding{
		RunID:                 "run-continuation",
		Release:               "release-continuation",
		Slice:                 "W0-continuation",
		Attempt:               1,
		PlanAuthorityDigest:   Digest([]byte("plan-authority")),
		TargetAuthorityDigest: Digest([]byte("target-authority")),
		ToolContractDigest:    Digest([]byte("tool-transition")),
	}
}

func startContinuationFixture(
	t *testing.T,
	adapter *continuationContractAdapter,
) (*Continuation, *continuationContractState) {
	t.Helper()
	design := continuationContractInvocation(
		t,
		adapter,
		"continuation-design",
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
		true,
	)
	observation, handle, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		design,
		continuationContractBinding(),
		nil,
	)
	if err != nil || observation.Handoff == nil || handle == nil ||
		result.Mode != adapter.stateMode ||
		result.Status != ContinuationStatusSuspended ||
		adapter.created == nil {
		t.Fatalf(
			"start = observation %#v, handle %p, result %#v, error %v",
			observation,
			handle,
			result,
			err,
		)
	}
	return handle, adapter.created
}

func TestContinuationResumesOnlySameImplementerDesignToImplementation(
	t *testing.T,
) {
	t.Parallel()
	adapter := newContinuationContractAdapter(
		"continuation-adapter",
		"configuration-a",
	)
	handle, state := startContinuationFixture(t, adapter)
	alias := *handle
	implementation := continuationContractInvocation(
		t,
		adapter,
		"continuation-implementation",
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
		false,
	)
	observation, next, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		implementation,
		continuationContractBinding(),
		handle,
	)
	if err != nil || observation.Handoff == nil || next != nil ||
		result.Mode != ContinuationModeTranscriptReplay ||
		result.Status != ContinuationStatusResumed {
		t.Fatalf(
			"resume = observation %#v, next %p, result %#v, error %v",
			observation,
			next,
			result,
			err,
		)
	}
	body, reported, closes := state.snapshot()
	if !bytes.Equal(body, make([]byte, len(body))) ||
		reported != 0 || closes != 1 {
		t.Fatalf(
			"consumed state = body %q, bytes %d, closes %d",
			body,
			reported,
			closes,
		)
	}
	_, turns, resumes := adapter.counts()
	if turns != 2 || resumes != 1 {
		t.Fatalf("adapter turns = %d, resumes = %d", turns, resumes)
	}
	_, next, result, err = (Dispatcher{}).InvokeTurn(
		context.Background(),
		implementation,
		continuationContractBinding(),
		&alias,
	)
	if err != nil || next != nil ||
		result.Mode != ContinuationModeFreshRehydrate ||
		result.Status != ContinuationStatusClosed {
		t.Fatalf("second resume = next %p, result %#v, error %v", next, result, err)
	}
	_, turnsAfter, resumesAfter := adapter.counts()
	if turnsAfter != turns || resumesAfter != resumes {
		t.Fatal("single-use handle invoked the adapter twice")
	}
}

func TestContinuationMismatchAndForeignRolesReturnFreshWithoutSubstitution(
	t *testing.T,
) {
	t.Parallel()
	t.Run("authority binding", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"binding-adapter",
			"configuration-a",
		)
		handle, state := startContinuationFixture(t, adapter)
		implementation := continuationContractInvocation(
			t,
			adapter,
			"binding-implementation",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
		)
		changed := continuationContractBinding()
		changed.TargetAuthorityDigest = Digest([]byte("changed-target"))
		observation, next, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			changed,
			handle,
		)
		if err != nil || !reflect.DeepEqual(observation, Observation{}) || next != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusMismatch {
			t.Fatalf(
				"mismatch = observation %#v, next %p, result %#v, error %v",
				observation,
				next,
				result,
				err,
			)
		}
		_, _, closes := state.snapshot()
		_, turns, resumes := adapter.counts()
		if closes != 1 || turns != 1 || resumes != 0 {
			t.Fatalf(
				"mismatch state closes=%d turns=%d resumes=%d",
				closes,
				turns,
				resumes,
			)
		}
	})

	t.Run("profile model and adapter", func(t *testing.T) {
		source := newContinuationContractAdapter(
			"selection-source",
			"configuration-a",
		)
		target := newContinuationContractAdapter(
			"selection-target",
			"configuration-b",
		)
		handle, state := startContinuationFixture(t, source)
		implementation := continuationContractInvocationWithSelection(
			t,
			target,
			"selection-implementation",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
			"changed-profile",
			"changed-model",
		)
		_, _, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			continuationContractBinding(),
			handle,
		)
		if err != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusMismatch {
			t.Fatalf("selection mismatch = %#v, %v", result, err)
		}
		_, _, closes := state.snapshot()
		_, targetTurns, _ := target.counts()
		if closes != 1 || targetTurns != 0 {
			t.Fatalf("selection mismatch closes=%d target turns=%d", closes, targetTurns)
		}
	})

	t.Run("stale permission", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"permission-adapter",
			"configuration-a",
		)
		handle, state := startContinuationFixture(t, adapter)
		implementation := continuationContractInvocation(
			t,
			adapter,
			"permission-implementation",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
		)
		design := continuationContractInvocation(
			t,
			adapter,
			"permission-other-design",
			RoleImplementer,
			ImplementerDesign,
			ReadOnly,
			true,
		)
		implementation.Permission = design.Permission
		_, _, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			continuationContractBinding(),
			handle,
		)
		if err != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusMismatch {
			t.Fatalf("stale permission = %#v, %v", result, err)
		}
		_, _, closes := state.snapshot()
		if closes != 1 {
			t.Fatalf("stale permission state closes = %d", closes)
		}
	})

	t.Run("reused invocation identity", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"identity-adapter",
			"configuration-a",
		)
		handle, state := startContinuationFixture(t, adapter)
		implementation := continuationContractInvocation(
			t,
			adapter,
			"continuation-design",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
		)
		_, _, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			continuationContractBinding(),
			handle,
		)
		_, _, closes := state.snapshot()
		if err != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusMismatch ||
			closes != 1 {
			t.Fatalf(
				"reused identity = result %#v, closes %d, error %v",
				result,
				closes,
				err,
			)
		}
	})

	t.Run("adapter rejects replay", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"invalid-replay-adapter",
			"configuration-a",
		)
		adapter.resumeErr = fail("CONTINUATION_INVALID")
		handle, state := startContinuationFixture(t, adapter)
		implementation := continuationContractInvocation(
			t,
			adapter,
			"invalid-replay-implementation",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
		)
		observation, next, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			continuationContractBinding(),
			handle,
		)
		if err != nil || !reflect.DeepEqual(observation, Observation{}) ||
			next != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusMismatch {
			t.Fatalf(
				"invalid replay = observation %#v, next %p, result %#v, error %v",
				observation,
				next,
				result,
				err,
			)
		}
		body, reported, closes := state.snapshot()
		if !bytes.Equal(body, make([]byte, len(body))) ||
			reported != 0 || closes != 1 {
			t.Fatalf(
				"invalid replay state = body %q, bytes %d, closes %d",
				body,
				reported,
				closes,
			)
		}
		_, turns, resumes := adapter.counts()
		if turns != 2 || resumes != 1 {
			t.Fatalf(
				"invalid replay calls = turns %d, resumes %d",
				turns,
				resumes,
			)
		}
	})

	for _, test := range []struct {
		name           string
		role           Role
		responsibility Responsibility
	}{
		{"captain", RoleCaptain, CaptainReview},
		{"verifier", RoleVerifier, WorkVerification},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			adapter := newContinuationContractAdapter(
				test.name+"-adapter",
				"configuration-a",
			)
			handle, state := startContinuationFixture(t, adapter)
			fresh := continuationContractInvocation(
				t,
				adapter,
				test.name+"-invocation",
				test.role,
				test.responsibility,
				ReadOnly,
				true,
			)
			observation, _, result, err := (Dispatcher{}).InvokeTurn(
				context.Background(),
				fresh,
				continuationContractBinding(),
				handle,
			)
			if err != nil || !reflect.DeepEqual(observation, Observation{}) ||
				result.Mode != ContinuationModeFreshRehydrate ||
				result.Status != ContinuationStatusMismatch {
				t.Fatalf(
					"foreign role = observation %#v, result %#v, error %v",
					observation,
					result,
					err,
				)
			}
			_, _, closes := state.snapshot()
			if closes != 1 {
				t.Fatalf("foreign role state closes = %d", closes)
			}
			observation, err = (Dispatcher{}).Invoke(
				context.Background(),
				fresh,
			)
			invocations, turns, resumes := adapter.counts()
			if err != nil || observation.Handoff == nil ||
				invocations != 1 || turns != 1 || resumes != 0 {
				t.Fatalf(
					"fresh fallback = observation %#v, calls %d/%d/%d, error %v",
					observation,
					invocations,
					turns,
					resumes,
					err,
				)
			}
		})
	}
}

func TestContinuationLifecycleConsumesAndZerosEveryTerminalPath(t *testing.T) {
	t.Parallel()
	t.Run("close aliases", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"close-adapter",
			"configuration-a",
		)
		handle, state := startContinuationFixture(t, adapter)
		alias := *handle
		var group sync.WaitGroup
		errorsSeen := make(chan error, 32)
		for index := 0; index < 32; index++ {
			group.Add(1)
			go func(useAlias bool) {
				defer group.Done()
				if useAlias {
					errorsSeen <- alias.Close()
				} else {
					errorsSeen <- handle.Close()
				}
			}(index%2 == 0)
		}
		group.Wait()
		close(errorsSeen)
		for err := range errorsSeen {
			if err != nil {
				t.Fatal(err)
			}
		}
		body, reported, closes := state.snapshot()
		if !bytes.Equal(body, make([]byte, len(body))) ||
			reported != 0 || closes != 1 {
			t.Fatalf(
				"closed state = body %q, bytes %d, closes %d",
				body,
				reported,
				closes,
			)
		}
		cell := continuationCellFor(handle)
		cell.mu.Lock()
		defer cell.mu.Unlock()
		if cell.binding != ([32]byte{}) ||
			cell.sourceInvocation != ([32]byte{}) ||
			cell.expiresNano != 0 ||
			cell.mode != "" || cell.state != nil || !cell.closed {
			t.Fatalf("closed cell retained state: %#v", cell)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"cancel-adapter",
			"configuration-a",
		)
		handle, state := startContinuationFixture(t, adapter)
		implementation := continuationContractInvocation(
			t,
			adapter,
			"cancel-implementation",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
		)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, result, err := (Dispatcher{}).InvokeTurn(
			ctx,
			implementation,
			continuationContractBinding(),
			handle,
		)
		_, _, closes := state.snapshot()
		if !IsCode(err, "INVOCATION_CANCELLED") ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusCancelled ||
			closes != 1 {
			t.Fatalf(
				"cancel = result %#v, closes %d, error %v",
				result,
				closes,
				err,
			)
		}
	})

	t.Run("expiry", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"expiry-adapter",
			"configuration-a",
		)
		handle, state := startContinuationFixture(t, adapter)
		cell := continuationCellFor(handle)
		cell.mu.Lock()
		cell.expiresNano = time.Now().Add(-time.Second).UnixNano()
		cell.mu.Unlock()
		implementation := continuationContractInvocation(
			t,
			adapter,
			"expiry-implementation",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
		)
		_, _, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			continuationContractBinding(),
			handle,
		)
		_, _, closes := state.snapshot()
		if err != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusExpired ||
			closes != 1 {
			t.Fatalf(
				"expiry = result %#v, closes %d, error %v",
				result,
				closes,
				err,
			)
		}
	})

	t.Run("capture overflow", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"capture-overflow-adapter",
			"configuration-a",
		)
		adapter.stateBytes = maxContinuationStateBytes + 1
		design := continuationContractInvocation(
			t,
			adapter,
			"capture-overflow-design",
			RoleImplementer,
			ImplementerDesign,
			ReadOnly,
			true,
		)
		observation, handle, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			design,
			continuationContractBinding(),
			nil,
		)
		_, _, closes := adapter.created.snapshot()
		if err != nil || observation.Handoff == nil || handle != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusOverflow ||
			closes != 1 {
			t.Fatalf(
				"capture overflow = handle %p, result %#v, closes %d, error %v",
				handle,
				result,
				closes,
				err,
			)
		}
	})

	t.Run("suspended overflow", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"suspended-overflow-adapter",
			"configuration-a",
		)
		handle, state := startContinuationFixture(t, adapter)
		state.setReported(maxContinuationStateBytes + 1)
		implementation := continuationContractInvocation(
			t,
			adapter,
			"suspended-overflow-implementation",
			RoleImplementer,
			ImplementerImplementation,
			ReadWrite,
			false,
		)
		_, _, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			continuationContractBinding(),
			handle,
		)
		_, _, closes := state.snapshot()
		if err != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusOverflow ||
			closes != 1 {
			t.Fatalf(
				"suspended overflow = result %#v, closes %d, error %v",
				result,
				closes,
				err,
			)
		}
	})

	t.Run("completion without retained state", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"completed-adapter",
			"configuration-a",
		)
		adapter.retain = false
		design := continuationContractInvocation(
			t,
			adapter,
			"completed-design",
			RoleImplementer,
			ImplementerDesign,
			ReadOnly,
			true,
		)
		observation, handle, result, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			design,
			continuationContractBinding(),
			nil,
		)
		if err != nil || observation.Handoff == nil || handle != nil ||
			result.Mode != ContinuationModeFreshRehydrate ||
			result.Status != ContinuationStatusCompleted {
			t.Fatalf(
				"completion = handle %p, result %#v, error %v",
				handle,
				result,
				err,
			)
		}
	})

	t.Run("cleanup failure is closed and stable", func(t *testing.T) {
		adapter := newContinuationContractAdapter(
			"cleanup-adapter",
			"configuration-a",
		)
		const hostile = "private cleanup failure"
		adapter.stateCloseErr = errors.New(hostile)
		handle, state := startContinuationFixture(t, adapter)
		err := handle.Close()
		_, _, closes := state.snapshot()
		if !IsCode(err, "CONTINUATION_CLEANUP_FAILED") ||
			closes != 1 ||
			bytes.Contains([]byte(err.Error()), []byte(hostile)) {
			t.Fatalf("cleanup = closes %d, error %v", closes, err)
		}
		if second := handle.Close(); second != nil {
			t.Fatalf("second close = %v", second)
		}
	})
}

func TestUnsupportedContinuationKeepsOneShotInvocationExact(t *testing.T) {
	t.Parallel()
	adapter := &oneShotContractAdapter{identity: AdapterIdentity{
		Key:                 "one-shot-adapter",
		ID:                  "sworn.one-shot",
		Version:             "1.0.0",
		ConfigurationDigest: Digest([]byte("one-shot-configuration")),
	}}
	design := continuationContractInvocation(
		t,
		adapter,
		"one-shot-design",
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
		true,
	)
	observation, handle, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		design,
		continuationContractBinding(),
		nil,
	)
	if err != nil || observation.Handoff == nil || handle != nil ||
		result.Mode != ContinuationModeFreshRehydrate ||
		result.Status != ContinuationStatusUnsupported ||
		adapter.count() != 1 {
		t.Fatalf(
			"unsupported = observation %#v, handle %p, result %#v, calls %d, error %v",
			observation,
			handle,
			result,
			adapter.count(),
			err,
		)
	}
	second := continuationContractInvocation(
		t,
		adapter,
		"one-shot-second",
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
		true,
	)
	oneShot, err := (Dispatcher{}).Invoke(context.Background(), second)
	if err != nil || oneShot.Handoff == nil || adapter.count() != 2 {
		t.Fatalf(
			"one-shot Invoke = observation %#v, calls %d, error %v",
			oneShot,
			adapter.count(),
			err,
		)
	}
}

func TestContinuationHandleHasNoEncodingTextOrInspectionSurface(t *testing.T) {
	t.Parallel()
	handleType := reflect.TypeOf(Continuation{})
	if handleType.NumField() != 1 ||
		handleType.Field(0).Name != "cell" ||
		handleType.Field(0).IsExported() {
		t.Fatalf("handle fields = %#v", handleType)
	}
	pointerType := reflect.TypeOf((*Continuation)(nil))
	if pointerType.NumMethod() != 1 ||
		pointerType.Method(0).Name != "Close" {
		t.Fatalf("handle methods = %#v", pointerType)
	}
	handle := &Continuation{}
	if _, ok := any(handle).(json.Marshaler); ok {
		t.Fatal("handle implements json.Marshaler")
	}
	if _, ok := any(handle).(encoding.TextMarshaler); ok {
		t.Fatal("handle implements encoding.TextMarshaler")
	}
	if _, ok := any(handle).(fmt.Stringer); ok {
		t.Fatal("handle implements fmt.Stringer")
	}
	body, err := json.Marshal(handle)
	if err != nil || string(body) != "{}" {
		t.Fatalf("empty JSON projection = %q, %v", body, err)
	}
	adapter := newContinuationContractAdapter(
		"inspection-adapter",
		"configuration-a",
	)
	live, state := startContinuationFixture(t, adapter)
	defer live.Close()
	private, _, _ := state.snapshot()
	formatted := fmt.Sprintf("%#v", live)
	if bytes.Contains([]byte(formatted), private) ||
		bytes.Contains([]byte(formatted), []byte(continuationContractBinding().RunID)) {
		t.Fatalf("formatted handle exposed content: %s", formatted)
	}
	resultType := reflect.TypeOf(ContinuationResult{})
	if resultType.NumField() != 2 ||
		resultType.Field(0).Name != "Mode" ||
		resultType.Field(1).Name != "Status" {
		t.Fatalf("result fields = %#v", resultType)
	}
	for index := 0; index < resultType.NumField(); index++ {
		if resultType.Field(index).Tag != "" {
			t.Fatalf("result field has encoding tag: %#v", resultType.Field(index))
		}
	}
}
