package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type continuationFixtureDriver struct {
	invoke func(context.Context, driver.Invocation) (driver.Observation, error)
	turn   func(
		context.Context,
		driver.Invocation,
		driver.ContinuationBinding,
		*driver.Continuation,
	) (
		driver.Observation,
		*driver.Continuation,
		driver.ContinuationResult,
		error,
	)
	recoverable func(
		context.Context,
		driver.Invocation,
		driver.ContinuationBinding,
		*driver.Continuation,
		*driver.RecoverableTurnInput,
	) (
		driver.Observation,
		*driver.Continuation,
		driver.ContinuationResult,
		error,
	)
	automation func(
		context.Context,
		driver.AutomationInvocation,
	) (driver.AutomationObservation, error)
}

func (fixture *continuationFixtureDriver) Invoke(
	ctx context.Context,
	invocation driver.Invocation,
) (driver.Observation, error) {
	return fixture.invoke(ctx, invocation)
}

func (fixture *continuationFixtureDriver) InvokeTurn(
	ctx context.Context,
	invocation driver.Invocation,
	binding driver.ContinuationBinding,
	handle *driver.Continuation,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	error,
) {
	return fixture.turn(ctx, invocation, binding, handle)
}

func (fixture *continuationFixtureDriver) InvokeRecoverableTurn(
	ctx context.Context,
	invocation driver.Invocation,
	binding driver.ContinuationBinding,
	handle *driver.Continuation,
	input *driver.RecoverableTurnInput,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	error,
) {
	if fixture.recoverable == nil {
		panic("unexpected recoverable turn")
	}
	return fixture.recoverable(
		ctx,
		invocation,
		binding,
		handle,
		input,
	)
}

func (fixture *continuationFixtureDriver) InvokeAutomation(
	ctx context.Context,
	invocation driver.AutomationInvocation,
) (driver.AutomationObservation, error) {
	if fixture.automation == nil {
		panic("unexpected automation turn")
	}
	return fixture.automation(ctx, invocation)
}

func continuationTestObservation(
	t *testing.T,
	invocation driver.Invocation,
	responsibility driver.Responsibility,
) driver.Observation {
	t.Helper()
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   invocation.Request.InvocationID,
		Responsibility: responsibility,
		Summary:        "Exact continuation fixture submission.",
		Detail:         "Bound to the current durable authority.",
	}
	if responsibility == driver.CaptainReview ||
		responsibility == driver.WorkVerification {
		var err error
		outcome := driver.DecisionProceed
		if responsibility == driver.WorkVerification {
			outcome = driver.DecisionPass
		}
		submission.Decision, err = driver.NewDecision(outcome)
		if err != nil {
			t.Fatal(err)
		}
	}
	if responsibility == driver.ImplementerImplementation ||
		responsibility == driver.WorkVerification {
		var err error
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("exact continuation fixture checks\n"),
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	return driver.Observation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
		Handoff: &driver.SealedHandoff{
			SubmissionBytes:  body,
			SubmissionDigest: driver.Digest(body),
		},
	}
}

func TestDesignContinuationPromotesAcrossFreshCaptainReview(
	t *testing.T,
) {
	ctx := context.Background()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	production, err := newProductionDriverRuntime(
		config,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "journal.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release,
		TargetRef:  manifest.value.TargetRef,
		CreatedAt:  now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(
		ctx,
		manifest.value.RunID,
		now,
		time.Minute,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	turnCalls := 0
	freshCalls := 0
	recoveryCalls := 0
	automationCalls := 0
	dispatcher := &continuationFixtureDriver{
		invoke: func(
			_ context.Context,
			invocation driver.Invocation,
		) (driver.Observation, error) {
			freshCalls++
			if !invocation.Request.FreshContext ||
				invocation.Request.Workspace.Access !=
					driver.ReadOnly {
				t.Fatalf(
					"fresh isolated invocation = %#v",
					invocation.Request,
				)
			}
			responsibility := driver.CaptainReview
			switch invocation.Request.Role {
			case driver.RoleCaptain:
				// Selected above.
			case driver.RoleVerifier:
				responsibility = driver.WorkVerification
			default:
				t.Fatalf(
					"unexpected fresh role = %s",
					invocation.Request.Role,
				)
			}
			return continuationTestObservation(
				t,
				invocation,
				responsibility,
			), nil
		},
		turn: func(
			_ context.Context,
			invocation driver.Invocation,
			_ driver.ContinuationBinding,
			handle *driver.Continuation,
		) (
			driver.Observation,
			*driver.Continuation,
			driver.ContinuationResult,
			error,
		) {
			turnCalls++
			if invocation.Request.Role != driver.RoleImplementer {
				t.Fatalf(
					"Implementer invocation = %#v, handle=%v",
					invocation.Request,
					handle,
				)
			}
			if handle != nil {
				if invocation.Request.FreshContext ||
					invocation.Request.Workspace.Access !=
						driver.ReadWrite ||
					len(invocation.Inputs) != 6 {
					t.Fatalf(
						"resumed implementation = %#v, inputs=%#v",
						invocation.Request,
						invocation.Inputs,
					)
				}
				if err := os.WriteFile(
					filepath.Join(
						invocation.HostWorkspace,
						"one.txt",
					),
					[]byte("resumed implementation\n"),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
				return continuationTestObservation(
						t,
						invocation,
						driver.ImplementerImplementation,
					),
					nil,
					driver.ContinuationResult{
						Mode: driver.
							ContinuationModeTranscriptReplay,
						Status: driver.
							ContinuationStatusResumed,
					},
					nil
			}
			if !invocation.Request.FreshContext ||
				invocation.Request.Workspace.Access !=
					driver.ReadOnly {
				t.Fatalf(
					"fresh design = %#v",
					invocation.Request,
				)
			}
			return driver.Observation{
					TransportStatus: driver.Completed,
					Usage: driver.UsageReceipt{
						TokenStatus: driver.UsageUnavailable,
						CostStatus:  driver.UsageUnavailable,
					},
					Diagnostic: driver.Diagnostic{Code: "none"},
					Yield: &driver.Yield{
						SchemaVersion: driver.YieldSchemaVersion,
						InvocationID: invocation.Request.
							InvocationID,
						Kind: driver.YieldQuestion,
						Message: "Which exact design constraint " +
							"controls?",
					},
				},
				&driver.Continuation{},
				driver.ContinuationResult{
					Mode: driver.
						ContinuationModeTranscriptReplay,
					Status: driver.
						ContinuationStatusSuspended,
				},
				nil
		},
		recoverable: func(
			_ context.Context,
			invocation driver.Invocation,
			binding driver.ContinuationBinding,
			handle *driver.Continuation,
			input *driver.RecoverableTurnInput,
		) (
			driver.Observation,
			*driver.Continuation,
			driver.ContinuationResult,
			error,
		) {
			if handle == nil && input == nil {
				freshCalls++
				if !invocation.Request.FreshContext ||
					invocation.Request.Workspace.Access !=
						driver.ReadOnly {
					t.Fatalf(
						"fresh isolated invocation = %#v",
						invocation.Request,
					)
				}
				responsibility := driver.CaptainReview
				switch invocation.Request.Role {
				case driver.RoleCaptain:
					// Selected above.
				case driver.RoleVerifier:
					responsibility = driver.WorkVerification
				default:
					t.Fatalf(
						"unexpected fresh role = %s",
						invocation.Request.Role,
					)
				}
				return continuationTestObservation(
						t,
						invocation,
						responsibility,
					),
					nil,
					driver.ContinuationResult{
						Mode: driver.
							ContinuationModeFreshRehydrate,
						Status: driver.
							ContinuationStatusCompleted,
					},
					nil
			}
			recoveryCalls++
			if handle == nil || input == nil ||
				input.Kind != driver.RecoverableInputAnswer ||
				input.Answer !=
					"Use the exact approved design constraint." ||
				input.TargetBinding == nil ||
				*input.TargetBinding != binding ||
				invocation.Request.Role !=
					driver.RoleImplementer ||
				!invocation.Request.FreshContext ||
				invocation.Request.Workspace.Access !=
					driver.ReadOnly {
				t.Fatalf(
					"recovered design invocation=%#v binding=%#v input=%#v handle=%p",
					invocation.Request,
					binding,
					input,
					handle,
				)
			}
			return continuationTestObservation(
					t,
					invocation,
					driver.ImplementerDesign,
				),
				&driver.Continuation{},
				driver.ContinuationResult{
					Mode: driver.
						ContinuationModeTranscriptReplay,
					Status: driver.
						ContinuationStatusSuspended,
				},
				nil
		},
		automation: func(
			_ context.Context,
			invocation driver.AutomationInvocation,
		) (driver.AutomationObservation, error) {
			automationCalls++
			if invocation.Recovery == nil ||
				invocation.Advisory != nil {
				t.Fatalf(
					"recovery automation = %#v",
					invocation,
				)
			}
			answer :=
				"Use the exact approved design constraint."
			return driver.AutomationObservation{
				TransportStatus: driver.Completed,
				Usage: driver.UsageReceipt{
					TokenStatus: driver.UsageUnavailable,
					CostStatus:  driver.UsageUnavailable,
				},
				Diagnostic: driver.Diagnostic{Code: "none"},
				Recovery: &driver.RecoveryDecision{
					SchemaVersion: driver.
						RecoveryDecisionSchemaVersion,
					InvocationID: invocation.Recovery.
						InvocationID,
					Action: driver.RecoveryResumeWorker,
					Answer: &answer,
				},
			}, nil
		},
	}
	service := &Service{
		journal: store, dispatcher: dispatcher,
		production: production, gitExecutable: gitExecutable,
		now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	planBytes, _ := runtimePlan(
		t,
		manifest.value.Release,
		manifest.value.Approval.Repository,
		manifest.value.TargetRef,
		"approval-release-1-v1",
	)
	if _, err := engine.actions.RecordPlanRevision(
		baton.RecordPlanRevisionInput{
			PlanBytes: planBytes,
			Summary:   "Install continuation scheduler fixture.",
			Detail:    []byte("Exact plan."),
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := service.advanceSlice(
		ctx,
		engine,
		owner,
		"S1",
	); err != nil {
		t.Fatal(err)
	}
	entry := service.takeContinuation(
		manifest.value.RunID,
		"S1",
	)
	if entry == nil || entry.handle == nil ||
		entry.designReceipt == "" || turnCalls != 1 ||
		freshCalls != 0 || recoveryCalls != 1 ||
		automationCalls != 1 {
		t.Fatalf(
			"promoted design continuation = %#v, turns=%d recovery=%d automation=%d fresh=%d",
			entry,
			turnCalls,
			recoveryCalls,
			automationCalls,
			freshCalls,
		)
	}
	if err := service.storeContinuation(
		manifest.value.RunID,
		"S1",
		entry,
	); err != nil {
		t.Fatal(err)
	}

	if err := service.advanceSlice(
		ctx,
		engine,
		owner,
		"S1",
	); err != nil {
		t.Fatal(err)
	}
	entry = service.takeContinuation(
		manifest.value.RunID,
		"S1",
	)
	state, err := baton.ReadState(
		engine.git,
		manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice("S1")
	if !ok || entry == nil || entry.handle == nil ||
		slice.CurrentReceipt == nil ||
		slice.CurrentReceipt.Receipt.Role != "captain" ||
		slice.CurrentReceipt.Receipt.Result != "proceed" ||
		slice.CurrentReceipt.Receipt.Binds != entry.designReceipt ||
		turnCalls != 1 || recoveryCalls != 1 ||
		automationCalls != 1 || freshCalls != 1 {
		t.Fatalf(
			"post-Captain state=%#v entry=%#v turns=%d recovery=%d automation=%d fresh=%d",
			slice,
			entry,
			turnCalls,
			recoveryCalls,
			automationCalls,
			freshCalls,
		)
	}
	if err := service.storeContinuation(
		manifest.value.RunID,
		"S1",
		entry,
	); err != nil {
		t.Fatal(err)
	}
	if err := service.advanceSlice(
		ctx,
		engine,
		owner,
		"S1",
	); err != nil {
		t.Fatal(err)
	}
	if entry := service.takeContinuation(
		manifest.value.RunID,
		"S1",
	); entry != nil {
		t.Fatal("implementation did not consume promoted continuation")
	}
	state, err = baton.ReadState(
		engine.git,
		manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok = state.Slice("S1")
	if !ok || slice.Candidate == nil ||
		slice.NextRole != "verifier" ||
		turnCalls != 2 || recoveryCalls != 1 ||
		automationCalls != 1 || freshCalls != 1 {
		t.Fatalf(
			"post-implementation state=%#v turns=%d recovery=%d automation=%d fresh=%d",
			slice,
			turnCalls,
			recoveryCalls,
			automationCalls,
			freshCalls,
		)
	}
	if err := service.advanceSlice(
		ctx,
		engine,
		owner,
		"S1",
	); err != nil {
		t.Fatal(err)
	}
	state, err = baton.ReadState(
		engine.git,
		manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok = state.Slice("S1")
	if !ok || slice.Pass == nil ||
		slice.Pass.Receipt.Result != "pass" ||
		turnCalls != 2 || freshCalls != 2 {
		t.Fatalf(
			"post-Verifier state=%#v turns=%d fresh=%d",
			slice,
			turnCalls,
			freshCalls,
		)
	}
	snapshot, err := store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	reuseObserved := false
	for _, event := range snapshot.Events {
		if event.Kind ==
			"dispatch_completed.continuation."+
				"transcript_replay.reuse" {
			reuseObserved = true
		}
	}
	if !reuseObserved {
		t.Fatalf("missing closed continuation event: %#v", snapshot.Events)
	}
}

func TestImplementationContinuationReuseAndFreshFallbackAreClosed(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	t.Cleanup(func() { _ = fixture.workspace.Close() })
	prepared, err := fixture.service.prepareDriverDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseProductionDispatchCommand(
		fixture.manifest,
		prepared.commandPayload,
	); err != nil {
		t.Fatalf("implementation recovery envelope = %v", err)
	}
	var command productionDispatchCommand
	if json.Unmarshal(prepared.commandPayload, &command) != nil {
		t.Fatal("implementation command did not decode")
	}
	validCommand := command
	command.ResumeRequestDigest =
		driver.Digest([]byte("substituted-resume-request"))
	if _, err := parseProductionDispatchCommand(
		fixture.manifest,
		mustJSON(command),
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("substituted resume request = %v", err)
	}
	freshOnly := validCommand
	freshOnly.Context.DesignReceipt = nil
	freshOnly.ResumeRequestDigest = ""
	freshRequest, err := productionRequestForContext(
		fixture.manifest,
		freshOnly.Context,
	)
	if err != nil {
		t.Fatal(err)
	}
	freshRequestBody, err := driver.EncodeRequest(freshRequest)
	if err != nil {
		t.Fatal(err)
	}
	freshOnly.RequestDigest = driver.Digest(freshRequestBody)
	if _, err := parseProductionDispatchCommand(
		fixture.manifest,
		mustJSON(freshOnly),
	); err != nil {
		t.Fatalf("fresh-only implementation command = %v", err)
	}
	freshOnly.ResumeRequestDigest = validCommand.ResumeRequestDigest
	if _, err := parseProductionDispatchCommand(
		fixture.manifest,
		mustJSON(freshOnly),
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("fresh-only command admitted resume request = %v", err)
	}
	binding, selectionDigest, err := continuationBindingForDispatch(
		prepared,
		fixture.coordinates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.productionContext.DesignReceipt == nil {
		t.Fatal("implementation context omitted exact design receipt")
	}
	designReceipt := prepared.productionContext.DesignReceipt.OID

	for _, test := range []struct {
		name            string
		entry           func() *retainedContinuation
		resumeResult    driver.ContinuationResult
		wantMode        driver.ContinuationMode
		wantOutcome     string
		wantFreshCalls  int
		wantResumeCalls int
	}{
		{
			name:           "restart without process state",
			wantMode:       driver.ContinuationModeFreshRehydrate,
			wantOutcome:    continuationOutcomeFallback,
			wantFreshCalls: 1,
		},
		{
			name: "authority mismatch",
			entry: func() *retainedContinuation {
				drifted := binding
				drifted.PlanAuthorityDigest =
					driver.Digest([]byte("other-plan"))
				return &retainedContinuation{
					handle:          &driver.Continuation{},
					binding:         drifted,
					selectionDigest: selectionDigest,
					designReceipt:   designReceipt,
				}
			},
			wantMode:       driver.ContinuationModeFreshRehydrate,
			wantOutcome:    continuationOutcomeFallback,
			wantFreshCalls: 1,
		},
		{
			name: "expired handle",
			entry: func() *retainedContinuation {
				return &retainedContinuation{
					handle:          &driver.Continuation{},
					binding:         binding,
					selectionDigest: selectionDigest,
					designReceipt:   designReceipt,
				}
			},
			resumeResult: driver.ContinuationResult{
				Mode:   driver.ContinuationModeFreshRehydrate,
				Status: driver.ContinuationStatusExpired,
			},
			wantMode:        driver.ContinuationModeFreshRehydrate,
			wantOutcome:     continuationOutcomeFallbackExpired,
			wantFreshCalls:  1,
			wantResumeCalls: 1,
		},
		{
			name: "exact retained transcript",
			entry: func() *retainedContinuation {
				return &retainedContinuation{
					handle:          &driver.Continuation{},
					binding:         binding,
					selectionDigest: selectionDigest,
					designReceipt:   designReceipt,
				}
			},
			resumeResult: driver.ContinuationResult{
				Mode:   driver.ContinuationModeTranscriptReplay,
				Status: driver.ContinuationStatusResumed,
			},
			wantMode:        driver.ContinuationModeTranscriptReplay,
			wantOutcome:     continuationOutcomeReuse,
			wantResumeCalls: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := fixture.service.discardContinuation(
				fixture.manifest.value.RunID,
				fixture.coordinates.Slice,
			); err != nil {
				t.Fatal(err)
			}
			if test.entry != nil {
				if err := fixture.service.storeContinuation(
					fixture.manifest.value.RunID,
					fixture.coordinates.Slice,
					test.entry(),
				); err != nil {
					t.Fatal(err)
				}
			}
			freshCalls := 0
			resumeCalls := 0
			fixture.service.dispatcher = &continuationFixtureDriver{
				invoke: func(
					_ context.Context,
					invocation driver.Invocation,
				) (driver.Observation, error) {
					freshCalls++
					if !invocation.Request.FreshContext ||
						invocation.Request.Workspace.Access !=
							driver.ReadWrite {
						t.Fatalf(
							"fresh fallback request = %#v",
							invocation.Request,
						)
					}
					return driver.Observation{
						TransportStatus: driver.Completed,
					}, nil
				},
				turn: func(
					_ context.Context,
					invocation driver.Invocation,
					gotBinding driver.ContinuationBinding,
					handle *driver.Continuation,
				) (
					driver.Observation,
					*driver.Continuation,
					driver.ContinuationResult,
					error,
				) {
					resumeCalls++
					if invocation.Request.FreshContext ||
						invocation.Request.Workspace.Access !=
							driver.ReadWrite ||
						handle == nil ||
						gotBinding != binding {
						t.Fatalf(
							"resume request = %#v, binding = %#v",
							invocation.Request,
							gotBinding,
						)
					}
					observation := driver.Observation{}
					if test.resumeResult.Mode !=
						driver.ContinuationModeFreshRehydrate {
						observation.TransportStatus =
							driver.Completed
					}
					return observation, nil, test.resumeResult, nil
				},
			}
			_, pending, fact, err := fixture.service.invokePreparedDriver(
				fixture.ctx,
				fixture.engine,
				fixture.workspace,
				fixture.coordinates,
				fixture.cycle.Before,
				prepared,
				fixture.owner,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if pending != nil || fact == nil ||
				fact.mode != test.wantMode ||
				fact.outcome != test.wantOutcome ||
				freshCalls != test.wantFreshCalls ||
				resumeCalls != test.wantResumeCalls {
				t.Fatalf(
					"pending=%v fact=%#v fresh=%d resume=%d",
					pending,
					fact,
					freshCalls,
					resumeCalls,
				)
			}
			if entry := fixture.service.takeContinuation(
				fixture.manifest.value.RunID,
				fixture.coordinates.Slice,
			); entry != nil {
				t.Fatal("implementation did not atomically consume handle")
			}
		})
	}
}

func TestFreshOnlyRepairBypassesContinuationAndFallbackMetrics(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "one.txt"),
		[]byte("repair candidate\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	candidate, err := fixture.engine.workspaces.SealTrack(
		fixture.workspace,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []baton.AppendReceiptInput{
		{
			Release:      fixture.state.Release,
			Slice:        fixture.slice.Location.Slice.ID,
			Role:         "implementer",
			Result:       "candidate",
			Summary:      "Candidate requiring fresh repair.",
			Detail:       []byte("Exact candidate."),
			Candidate:    candidate.Candidate.String(),
			CheckResults: []byte("candidate checks\n"),
		},
		{
			Release:      fixture.state.Release,
			Slice:        fixture.slice.Location.Slice.ID,
			Role:         "verifier",
			Result:       "fail",
			Summary:      "Require a fresh repair.",
			Detail:       []byte("Exact verifier failure."),
			Candidate:    candidate.Candidate.String(),
			CheckResults: []byte("fresh verifier checks\n"),
		},
	} {
		if _, err := fixture.engine.actions.AppendReceipt(
			receipt,
		); err != nil {
			t.Fatal(err)
		}
	}
	state, err := baton.ReadState(
		fixture.engine.git,
		fixture.state.Release,
		fixture.engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice(fixture.slice.Location.Slice.ID)
	if !ok || slice.Attempt != 2 ||
		slice.Stage != "implement" ||
		slice.NextRole != "implementer" {
		t.Fatalf("repair authority = %#v", slice)
	}
	workspace, err := fixture.engine.workspaces.OpenTrack(
		gitx.TrackKey{
			Release: state.Release,
			Track:   slice.Location.Track.ID,
		},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.Close() })
	coordinates := dispatchCoordinates{
		Slice:          slice.Location.Slice.ID,
		Responsibility: driver.ImplementerImplementation,
		BatonAttempt:   slice.Attempt,
		Epoch:          1,
		Try:            1,
	}
	before := sliceFingerprint(state, coordinates.Slice)
	prepared, err := fixture.service.prepareDriverDispatch(
		fixture.ctx,
		fixture.engine,
		workspace,
		driver.RoleImplementer,
		coordinates,
		before,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.productionContext.DesignReceipt != nil ||
		prepared.resumeRequest != nil ||
		prepared.resumePermission != nil {
		t.Fatalf("fresh repair dispatch = %#v", prepared)
	}
	if err := fixture.service.storeContinuation(
		fixture.manifest.value.RunID,
		coordinates.Slice,
		&retainedContinuation{handle: &driver.Continuation{}},
	); err != nil {
		t.Fatal(err)
	}
	freshCalls := 0
	turnCalls := 0
	fixture.service.dispatcher = &continuationFixtureDriver{
		invoke: func(
			_ context.Context,
			invocation driver.Invocation,
		) (driver.Observation, error) {
			freshCalls++
			if !invocation.Request.FreshContext ||
				invocation.Request.Workspace.Access !=
					driver.ReadWrite {
				t.Fatalf(
					"fresh repair invocation = %#v",
					invocation.Request,
				)
			}
			return driver.Observation{
				TransportStatus: driver.Completed,
			}, nil
		},
		turn: func(
			context.Context,
			driver.Invocation,
			driver.ContinuationBinding,
			*driver.Continuation,
		) (
			driver.Observation,
			*driver.Continuation,
			driver.ContinuationResult,
			error,
		) {
			turnCalls++
			return driver.Observation{}, nil,
				driver.ContinuationResult{}, nil
		},
	}
	_, pending, fact, err := fixture.service.invokePreparedDriver(
		fixture.ctx,
		fixture.engine,
		workspace,
		coordinates,
		before,
		prepared,
		fixture.owner,
		nil,
	)
	if err != nil || pending != nil || fact != nil ||
		freshCalls != 1 || turnCalls != 0 {
		t.Fatalf(
			"fresh repair pending=%p fact=%#v fresh=%d turns=%d error=%v",
			pending,
			fact,
			freshCalls,
			turnCalls,
			err,
		)
	}
	if retained := fixture.service.takeContinuation(
		fixture.manifest.value.RunID,
		coordinates.Slice,
	); retained != nil {
		t.Fatalf("fresh repair retained continuation = %#v", retained)
	}
}

func TestPendingV1ImplementationDispatchReplaysWithoutConflict(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	t.Cleanup(func() { _ = fixture.workspace.Close() })
	prepared, err := fixture.service.prepareDriverDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err = downgradePreparedProductionDispatchV1(
		fixture.manifest,
		prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.EnsureAttempt(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: fixture.cycle.DispatchEffect,
			Kind:      "driver.dispatch",
			Payload:   prepared.commandPayload,
			CreatedAt: fixture.now,
		},
		journal.Effect{
			RunID:     fixture.owner.RunID,
			ID:        fixture.cycle.DispatchEffect,
			ReplayKey: fixture.cycle.DispatchEffect,
			Kind:      "driver.dispatch",
			BeforeDigest: sha256Digest(
				[]byte(fixture.cycle.Before),
			),
			ExpectedDigest: productionOutputExpectation,
			UpdatedAt:      fixture.now,
		},
		journal.EffectAttempt{
			WorkID: fixture.cycle.DispatchWork,
			Epoch:  1,
			Try:    1,
		},
	); err != nil {
		t.Fatal(err)
	}
	invocations := 0
	fixture.service.dispatcher = fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		invocations++
		if !invocation.Request.FreshContext ||
			len(invocation.Inputs) != 4 {
			t.Fatalf(
				"v1 replay invocation = %#v, inputs=%#v",
				invocation.Request,
				invocation.Inputs,
			)
		}
		return continuationTestObservation(
			t,
			invocation,
			driver.ImplementerImplementation,
		), nil
	})
	submission, err := fixture.service.runDriverEffect(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		journal.EffectAttempt{
			WorkID: fixture.cycle.DispatchWork,
			Epoch:  1,
			Try:    1,
		},
		fixture.cycle.Before,
		fixture.owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Responsibility !=
		driver.ImplementerImplementation ||
		invocations != 1 {
		t.Fatalf(
			"v1 replay submission=%#v invocations=%d",
			submission,
			invocations,
		)
	}
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Events) == 0 ||
		snapshot.Events[len(snapshot.Events)-1].Kind !=
			"dispatch_completed.continuation."+
				"fresh_rehydrate.fallback" {
		t.Fatalf("v1 replay events = %#v", snapshot.Events)
	}
}

func TestContinuationBindingSurvivesOnlyExpectedCaptainTransition(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	t.Cleanup(func() { _ = fixture.workspace.Close() })
	implementation, err := fixture.service.prepareDriverDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		fixture.cycle.Before,
	)
	if err != nil {
		t.Fatal(err)
	}
	implementationBinding, implementationSelection, err :=
		continuationBindingForDispatch(
			implementation,
			fixture.coordinates,
		)
	if err != nil {
		t.Fatal(err)
	}

	design := implementation
	designContext := *implementation.productionContext
	designContext.Responsibility = driver.ImplementerDesign
	designContext.DesignReceipt = nil
	designContext.Authority.TrackHead =
		designContext.Receipt.OID
	design.productionContext = &designContext
	designCoordinates := fixture.coordinates
	designCoordinates.Responsibility = driver.ImplementerDesign
	designBinding, designSelection, err :=
		continuationBindingForDispatch(design, designCoordinates)
	if err != nil {
		t.Fatal(err)
	}
	if designBinding != implementationBinding ||
		designSelection != implementationSelection {
		t.Fatalf(
			"Captain-only transition changed binding: design=%#v implementation=%#v",
			designBinding,
			implementationBinding,
		)
	}

	drifted := design
	driftedContext := designContext
	driftedContext.Authority.TargetHead =
		"0000000000000000000000000000000000000000"
	drifted.productionContext = &driftedContext
	driftedBinding, _, err := continuationBindingForDispatch(
		drifted,
		designCoordinates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if driftedBinding == implementationBinding {
		t.Fatal("target drift retained the same continuation binding")
	}

	drifted = design
	drifted.selected.Model = "other-model"
	_, driftedSelection, err := continuationBindingForDispatch(
		drifted,
		designCoordinates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if driftedSelection == implementationSelection {
		t.Fatal("model drift retained the same selection binding")
	}
}

func TestImplementationContextRequiresExactCaptainAndDesignReceipts(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	t.Cleanup(func() { _ = fixture.workspace.Close() })
	design, err := currentImplementationDesignReceipt(
		fixture.state,
		fixture.slice,
		fixture.track,
	)
	if err != nil {
		t.Fatal(err)
	}
	if design.OID != fixture.slice.CurrentReceipt.Receipt.Binds {
		t.Fatalf(
			"design receipt = %s, Captain binds = %s",
			design.OID,
			fixture.slice.CurrentReceipt.Receipt.Binds,
		)
	}
	original := fixture.slice.CurrentReceipt.Receipt.Binds
	fixture.slice.CurrentReceipt.Receipt.Binds =
		"0000000000000000000000000000000000000000"
	if _, err := currentImplementationDesignReceipt(
		fixture.state,
		fixture.slice,
		fixture.track,
	); !IsCode(err, "INVALID_AUTHORITY_STATE") {
		t.Fatalf("substituted Captain binding = %v", err)
	}
	fixture.slice.CurrentReceipt.Receipt.Binds = original
	fixture.slice.CurrentReceipt.Receipt.Role = "verifier"
	fixture.slice.CurrentReceipt.Receipt.Result = "fail"
	design, err = currentImplementationDesignReceipt(
		fixture.state,
		fixture.slice,
		fixture.track,
	)
	if err != nil || design != nil {
		t.Fatalf(
			"fresh repair design receipt = %#v, error=%v",
			design,
			err,
		)
	}
}
