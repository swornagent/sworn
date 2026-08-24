package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type fixtureDriver func(
	context.Context,
	driver.Invocation,
) (driver.Observation, error)

func (invoke fixtureDriver) Invoke(
	ctx context.Context,
	invocation driver.Invocation,
) (driver.Observation, error) {
	return invoke(ctx, invocation)
}

// plannerSummaryAnswer and plannerSummaryQuestion are the one shared fixture
// pair for the production summary-before-plan boundary.
const (
	plannerSummaryQuestion = "Here is the result, scope, acceptance, " +
		"evidence, inputs, and limits I intend to plan. Confirm or correct."
	plannerSummaryAnswer = "Confirmed as summarised."
)

// plannerSummaryDispatcher is the single test fixture every production planner
// path now shares. It makes the production Planner behave the way the runtime
// requires: its first terminal is the human-only summary turn, and the plan
// bytes the wrapped terminal produces are emitted only by the responsibility
// resumed from that answered turn. Every other role is passed straight
// through, so a test that does not care about planning is unaffected.
type plannerSummaryDispatcher struct {
	terminal func(context.Context, driver.Invocation) (driver.Observation, error)
}

func (d *plannerSummaryDispatcher) Invoke(
	ctx context.Context,
	invocation driver.Invocation,
) (driver.Observation, error) {
	if invocation.Request.Role != driver.RolePlanner {
		return d.terminal(ctx, invocation)
	}
	observation := driver.Observation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
		Yield: &driver.Yield{
			SchemaVersion: driver.YieldSchemaVersion,
			InvocationID:  invocation.Request.InvocationID,
			Kind:          driver.YieldHumanConfirmation,
			Message:       plannerSummaryQuestion,
		},
	}
	return observation, nil
}

func (d *plannerSummaryDispatcher) InvokeRecoverableTurn(
	ctx context.Context,
	invocation driver.Invocation,
	_ driver.ContinuationBinding,
	_ *driver.Continuation,
	input *driver.RecoverableTurnInput,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	error,
) {
	result := driver.ContinuationResult{
		Mode:   driver.ContinuationModeFreshRehydrate,
		Status: driver.ContinuationStatusCompleted,
	}
	if input == nil {
		observation, err := d.Invoke(ctx, invocation)
		return observation, nil, result, err
	}
	if invocation.Request.Role != driver.RolePlanner ||
		input.SchemaVersion != driver.RecoverableTurnInputSchemaVersion ||
		input.Kind != driver.RecoverableInputAnswer ||
		input.Answer != plannerSummaryAnswer {
		return driver.Observation{}, nil, driver.ContinuationResult{},
			fmt.Errorf("unexpected planner resume input %#v", input)
	}
	observation, err := d.terminal(ctx, invocation)
	return observation, nil, result, err
}

// openPlannerSummaryTurn returns the one open human-only Planner turn, if the
// run is currently waiting on it.
func openPlannerSummaryTurn(
	ctx context.Context,
	service *Service,
	runID string,
) (journal.AttentionProjection, bool, error) {
	attentions, err := service.journal.Attentions(ctx, runID)
	if err != nil {
		return journal.AttentionProjection{}, false, err
	}
	for _, attention := range attentions {
		human := attention.Attention.HumanTurn
		if attention.State != journal.AttentionOpen || human == nil ||
			human.Responsibility != string(driver.PlannerProposal) {
			continue
		}
		return attention, true, nil
	}
	return journal.AttentionProjection{}, false, nil
}

// drivePlannerSummaryTurns answers every production Planner summary turn the
// run stops on, using the same operator command a person would. It returns as
// soon as the run stops for any other reason, so a caller that is proving a
// crash cut still observes that cut's exact error.
func drivePlannerSummaryTurns(
	t *testing.T,
	ctx context.Context,
	service *Service,
	runID string,
	status RunStatus,
	err error,
) (RunStatus, error) {
	t.Helper()
	for range 8 {
		if err != nil {
			return status, err
		}
		attention, found, readErr := openPlannerSummaryTurn(
			ctx, service, runID,
		)
		if readErr != nil {
			t.Fatalf("planner summary attentions = %v", readErr)
		}
		if !found {
			return status, nil
		}
		status, err = service.AnswerAttention(ctx, AnswerAttentionCommand{
			RunID:              runID,
			AttentionID:        attention.Attention.ID,
			ExpectedGeneration: 1,
			Answer:             plannerSummaryAnswer,
		})
		if err != nil {
			return status, err
		}
		status, err = service.Wait(ctx, runID)
	}
	t.Fatal("production planner summary turns did not settle")
	return RunStatus{}, nil
}

func runRuntimeGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
	body, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, body)
	}
	return strings.TrimSpace(string(body))
}

func productionRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repository, "init", "--quiet", "--initial-branch=main")
	if err := os.WriteFile(
		filepath.Join(repository, "README.md"),
		[]byte("production fixture\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repository, "add", "--", "README.md")
	runRuntimeGit(
		t,
		repository,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit", "--quiet", "-m", "production fixture",
	)
	return repository
}

func productionConfig(t *testing.T) driver.LoadedDriverConfig {
	t.Helper()
	credential := "token"
	config := driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key: "token", Kind: driver.CredentialEnvironment,
			Reference: "SWORN_TEST_TOKEN",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key: "openai", ID: "sworn.openai", Version: "1.0.0",
					Endpoint:         "https://example.invalid/v1/responses",
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{"token"},
					ResponseBytes:    driver.MaxProviderResponseBytes,
				},
				API:             driver.OpenAIResponsesAPI,
				ReasoningEffort: "medium",
			},
		}},
		Profiles: []driver.DriverProfile{
			{
				Key: "planner", Adapter: "openai",
				Network:             driver.NetworkRequired,
				CredentialSource:    &credential,
				CertificationModels: []string{"captain-model", "implementer-model", "planner-model", "verifier-model"},
			},
			{
				Key: "unused", Adapter: "openai",
				Network:             driver.NetworkRequired,
				CredentialSource:    &credential,
				CertificationModels: []string{"unused-model"},
			},
		},
	}
	body, err := driver.EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := driver.DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func productionManifest(
	t *testing.T,
	repository string,
	config driver.LoadedDriverConfig,
) admittedManifest {
	t.Helper()
	manifest, _, _ := fixtureManifest(t)
	manifest.RunID = "production-run-1"
	manifest.Repository = repository
	manifest.Driver = nil
	manifest.DriverConfigDigest = config.ConfigurationDigest()
	manifest.Scripts = nil
	manifest.MaxParallelTracks = 1
	manifest.Roles = driver.RoleSelections{
		Planner: driver.RoleSelection{
			Profile: "planner", Model: "planner-model",
		},
		Implementer: driver.RoleSelection{
			Profile: "planner", Model: "implementer-model",
		},
		Captain: driver.RoleSelection{
			Profile: "planner", Model: "captain-model",
		},
		Verifier: driver.RoleSelection{
			Profile: "planner", Model: "verifier-model",
		},
	}
	manifest.Automation = &AutomationSelections{
		Recovery: driver.ModelSelection{
			Profile: "planner",
			Model:   "planner-model",
		},
	}
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	return admitted
}

func plannerProductionAuthority(
	t *testing.T,
	engine *engine,
) (gitx.RefHead, string) {
	t.Helper()
	release, target, err := captureProposalRefs(
		engine.repository,
		engine.manifest,
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := planProposalAuthority{
		Release:    engine.manifest.value.Release,
		ReleaseRef: release.Ref,
		TargetRef:  target.Ref,
		TargetHead: target.Head.String(),
	}
	authority.Before = plannerAuthorityBefore(authority)
	return target, authority.Before
}

func TestProductionDriverConfigBindsDigestAndBuildsOnlySelectedProfiles(
	t *testing.T,
) {
	t.Parallel()

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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		production:    production,
		gitExecutable: gitExecutable,
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if got := engine.registry.Profiles(); len(got) != 1 || got[0] != "planner" {
		t.Fatalf("built profiles = %v", got)
	}

	drifted := manifest
	drifted.value.DriverConfigDigest =
		"sha256:" + strings.Repeat("f", 64)
	if _, err := production.registryFor(drifted); !IsCode(err, "DRIVER_CONFIG_DRIFT") {
		t.Fatalf("config drift = %v", err)
	}
	if _, err := (&Service{
		gitExecutable: gitExecutable,
	}).openEngine(manifest); !IsCode(err, "DRIVER_CONFIG_UNAVAILABLE") {
		t.Fatalf("missing production config = %v", err)
	}
	_, fakeBody, _ := fixtureManifest(t)
	scriptedManifest, err := admitManifest(fakeBody)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.openEngine(scriptedManifest); !IsCode(
		err,
		"DRIVER_CONFIG_UNEXPECTED",
	) {
		t.Fatalf("unexpected production config in fake mode = %v", err)
	}
	differentConfig, err := driver.DecodeDriverConfig(bytes.Replace(
		config.CanonicalJSON(),
		[]byte("example.invalid"),
		[]byte("different.invalid"),
		1,
	))
	if err != nil {
		t.Fatal(err)
	}
	differentRuntime, err := newProductionDriverRuntime(
		differentConfig,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := differentRuntime.registryFor(
		manifest,
	); !IsCode(err, "DRIVER_CONFIG_DRIFT") {
		t.Fatalf("different restart config = %v", err)
	}

	fakeCredential := "token"
	fakeConfigBody, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key: "token", Kind: driver.CredentialEnvironment,
			Reference: "SWORN_TEST_TOKEN",
		}},
		Adapters: []driver.DriverAdapterConfig{
			{
				Process: &driver.DriverProcessAdapterConfig{
					Key: "fake", ID: driver.FakeDriverID,
					Version: driver.FakeDriverVersion,
					Executable: driver.ExecutableIdentity{
						Path:   "/bin/true",
						Digest: "sha256:" + strings.Repeat("a", 64),
					},
				},
			},
			{
				OpenAI: &driver.OpenAIProfileConfig{
					HTTPProfileConfig: driver.HTTPProfileConfig{
						Key: "openai", ID: "sworn.openai", Version: "1.0.0",
						Endpoint:         "https://example.invalid/v1/responses",
						CredentialHeader: "Authorization",
						CredentialPrefix: "Bearer ",
						CredentialRefs:   []string{"token"},
						ResponseBytes:    driver.MaxProviderResponseBytes,
					},
					API:             driver.OpenAIResponsesAPI,
					ReasoningEffort: "medium",
				},
			},
		},
		Profiles: []driver.DriverProfile{
			{
				Key: "fake", Adapter: "fake", Network: driver.NetworkNone,
				CredentialSource:    nil,
				CertificationModels: []string{"fake-model"},
			},
			{
				Key: "planner", Adapter: "openai",
				Network:             driver.NetworkRequired,
				CredentialSource:    &fakeCredential,
				CertificationModels: []string{"planner-model"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fakeConfig, err := driver.DecodeDriverConfig(fakeConfigBody)
	if err != nil {
		t.Fatal(err)
	}
	fakeProduction, err := newProductionDriverRuntime(
		fakeConfig,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	fakeManifest := manifest
	fakeManifest.value.DriverConfigDigest =
		fakeConfig.ConfigurationDigest()
	fakeRole := driver.RoleSelection{
		Profile: "fake",
		Model:   "fake-model",
	}
	fakeManifest.value.Roles = driver.RoleSelections{
		Planner: fakeRole, Implementer: fakeRole,
		Captain: fakeRole, Verifier: fakeRole,
	}
	if _, err := fakeProduction.registryFor(fakeManifest); !IsCode(err, "DRIVER_UNAVAILABLE") {
		t.Fatalf("production fake profile = %v", err)
	}
}

func TestSelectedProfileMustBelongToConfiguredRegistry(
	t *testing.T,
) {
	t.Parallel()

	configured := &configuredRuntimeRegistry{
		families: map[string]driver.ProfileFamily{
			"native": driver.ProfileCodex,
			"http":   driver.ProfileOpenAIHTTP,
		},
	}
	selected := driver.SelectedProfile{
		Profile: driver.ProfileConfig{Key: "native"},
		Model:   "model-1",
	}
	if err := configured.validateSelected(selected); err != nil {
		t.Fatal(err)
	}
	selected.Profile.Key = "missing"
	if err := configured.validateSelected(selected); !IsCode(
		err,
		"DRIVER_SELECTION_FAILED",
	) {
		t.Fatalf("unknown profile = %v", err)
	}
}

func TestAssemblyEvidenceBindsTrackPinsToFinalPassedSlices(
	t *testing.T,
) {
	t.Parallel()

	passedSlice := func(trackID, sliceID, token string) *baton.SliceState {
		candidateOID := strings.Repeat(token, 40)
		passOID := strings.Repeat(string(token[0]+1), 40)
		candidate := strings.Repeat(string(token[0]+2), 40)
		productTree := "sha256:" + strings.Repeat(token, 64)
		return &baton.SliceState{
			Location: baton.SliceLocation{
				Track: baton.Track{ID: trackID},
				Slice: baton.Slice{ID: sliceID},
			},
			Candidate: &baton.ReceiptEntry{
				OID: candidateOID,
				Receipt: baton.Receipt{
					Slice: &sliceID, Role: "implementer",
					Result: "candidate", Candidate: &candidate,
					ProductTree: &productTree,
				},
			},
			Pass: &baton.ReceiptEntry{
				OID: passOID,
				Receipt: baton.Receipt{
					Slice: &sliceID, Role: "verifier", Result: "pass",
					Binds: candidateOID, Candidate: &candidate,
					ProductTree: &productTree,
				},
			},
		}
	}
	fixture := func() baton.State {
		a1 := passedSlice("T1", "A1", "1")
		a2 := passedSlice("T1", "A2", "4")
		b1 := passedSlice("T2", "B1", "7")
		treeOne := *a2.Candidate.Receipt.ProductTree
		treeTwo := *b1.Candidate.Receipt.ProductTree
		return baton.State{
			Release: "release",
			Tracks: []baton.TrackState{
				{
					ID: "T1", Ref: "refs/heads/track/release/T1",
					Head:          strings.Repeat("b", 40),
					AuthorityHead: strings.Repeat("b", 40),
					Slices:        []*baton.SliceState{a1, a2},
				},
				{
					ID: "T2", DependsOn: []string{"T1"},
					Ref:           "refs/heads/track/release/T2",
					Head:          strings.Repeat("c", 40),
					AuthorityHead: strings.Repeat("c", 40),
					Slices:        []*baton.SliceState{b1},
				},
			},
			Slices: []*baton.SliceState{a1, a2, b1},
			Assembly: baton.AssemblyState{
				InputPins: map[string]*string{
					"T1": &treeOne,
					"T2": &treeTwo,
				},
			},
		}
	}
	state := fixture()
	evidence, err := assemblyEvidence(state)
	if err != nil {
		t.Fatal(err)
	}
	treeOne := *state.Assembly.InputPins["T1"]
	treeTwo := *state.Assembly.InputPins["T2"]
	if len(evidence) != 2 ||
		evidence[0].Slice != "A2" ||
		evidence[0].ProductTree != treeOne ||
		evidence[1].Slice != "B1" ||
		evidence[1].ProductTree != treeTwo {
		t.Fatalf("assembly evidence = %#v", evidence)
	}

	cases := []struct {
		name   string
		mutate func(*baton.State)
	}{
		{
			name: "missing pin",
			mutate: func(value *baton.State) {
				delete(value.Assembly.InputPins, "T2")
			},
		},
		{
			name: "extra pin",
			mutate: func(value *baton.State) {
				extra := "sha256:" + strings.Repeat("e", 64)
				value.Assembly.InputPins["T3"] = &extra
			},
		},
		{
			name: "duplicate final slice",
			mutate: func(value *baton.State) {
				slice := value.Tracks[1].Slices[0]
				duplicate := "A2"
				slice.Location.Slice.ID = duplicate
				slice.Candidate.Receipt.Slice = &duplicate
				slice.Pass.Receipt.Slice = &duplicate
			},
		},
		{
			name: "slice track drift",
			mutate: func(value *baton.State) {
				value.Tracks[0].Slices[1].Location.Track.ID = "T2"
			},
		},
		{
			name: "pass shape and binding drift",
			mutate: func(value *baton.State) {
				pass := &value.Tracks[0].Slices[1].Pass.Receipt
				pass.Role = "captain"
				pass.Binds = strings.Repeat("f", 40)
			},
		},
		{
			name: "candidate identity drift",
			mutate: func(value *baton.State) {
				candidate := strings.Repeat("d", 40)
				value.Tracks[0].Slices[1].Pass.Receipt.Candidate =
					&candidate
			},
		},
		{
			name: "tree pin drift",
			mutate: func(value *baton.State) {
				tree := "sha256:" + strings.Repeat("f", 64)
				value.Assembly.InputPins["T1"] = &tree
			},
		},
		{
			name: "source drift",
			mutate: func(value *baton.State) {
				value.Tracks[1].AuthorityHead = strings.Repeat("d", 40)
			},
		},
		{
			name: "serial predecessor not passed",
			mutate: func(value *baton.State) {
				value.Tracks[0].Slices[0].Pass = nil
			},
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			value := fixture()
			testCase.mutate(&value)
			if _, err := assemblyEvidence(value); !IsCode(
				err,
				"INVALID_AUTHORITY_STATE",
			) {
				t.Fatalf("assembly drift = %v", err)
			}
		})
	}
}

func TestProductionWorkContextProjectsPlanReceiptCandidateAndEvidence(
	t *testing.T,
) {
	t.Parallel()

	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	coordinates := dispatchCoordinates{
		Slice:          "S1",
		Responsibility: driver.WorkVerification,
		BatonAttempt:   2,
		Epoch:          3,
		Try:            1,
	}
	planBody := []byte("bounded plan bytes\n")
	receiptBody := []byte("{\"bounded\":\"receipt\"}\n")
	receiptDetail := []byte("bounded receipt detail\n")
	contextValue := productionWorkContext{
		SchemaVersion:      productionWorkContextVersion,
		ManifestDigest:     manifest.digest,
		DriverConfigDigest: manifest.value.DriverConfigDigest,
		RunID:              manifest.value.RunID,
		Repository:         manifest.value.Authority.Project,
		Release:            manifest.value.Release,
		Intent:             manifest.value.Intent,
		InvocationID: dispatchInvocationID(
			manifest.value.RunID,
			coordinates,
		),
		Role:            driver.RoleVerifier,
		Track:           "T1",
		Slice:           coordinates.Slice,
		Responsibility:  coordinates.Responsibility,
		Attempt:         coordinates.BatonAttempt,
		Epoch:           coordinates.Epoch,
		Try:             coordinates.Try,
		Before:          "sha256:" + strings.Repeat("1", 64),
		WorkspaceAccess: driver.ReadOnly,
		Authority: productionAuthorityBinding{
			ReleaseRef: "refs/heads/release-wt/" +
				manifest.value.Release,
			ReleaseHead: strings.Repeat("2", 40),
			TargetRef:   manifest.value.TargetRef,
			TargetHead:  strings.Repeat("3", 40),
			TrackRef: "refs/heads/track/" +
				manifest.value.Release + "/T1",
			TrackHead: strings.Repeat("4", 40),
		},
		Plan: &productionPlanBinding{
			OID:      strings.Repeat("5", 40),
			Digest:   driver.Digest(planBody),
			Revision: 1,
			Input: driver.Input{
				Name: "plan", Path: productionPlanPath,
				Digest: driver.Digest(planBody),
			},
			body: planBody,
		},
		Receipt: &productionReceiptBinding{
			OID: strings.Repeat("6", 40),
			BodyInput: driver.Input{
				Name: "receipt", Path: productionReceiptPath,
				Digest: driver.Digest(receiptBody),
			},
			DetailInput: driver.Input{
				Name:   "receipt-detail",
				Path:   productionReceiptDetailPath,
				Digest: driver.Digest(receiptDetail),
			},
			body: receiptBody, detail: receiptDetail,
		},
		Candidate: &productionCandidateBinding{
			Receipt: strings.Repeat("6", 40),
			Commit:  strings.Repeat("7", 40),
			ProductTree: "sha256:" +
				strings.Repeat("8", 64),
		},
		Evidence: []productionEvidenceBinding{{
			Slice:            "S0",
			PassReceipt:      strings.Repeat("9", 40),
			CandidateReceipt: strings.Repeat("a", 40),
			Candidate:        strings.Repeat("b", 40),
			ProductTree: "sha256:" +
				strings.Repeat("c", 64),
			SourceRef: "refs/heads/track/" +
				manifest.value.Release + "/T0",
			SourceHead: strings.Repeat("d", 40),
		}},
	}
	if err := validateProductionWorkContext(
		manifest,
		contextValue,
	); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		responsibility driver.Responsibility
		access         driver.WorkspaceAccess
	}{
		{driver.ImplementerDesign, driver.ReadOnly},
		{driver.CaptainReview, driver.ReadOnly},
		{driver.ImplementerImplementation, driver.ReadWrite},
		{driver.WorkVerification, driver.ReadOnly},
	} {
		value := contextValue
		value.Responsibility = mode.responsibility
		value.Role, _ = roleForResponsibility(mode.responsibility)
		value.WorkspaceAccess = mode.access
		value.Candidate = nil
		value.DesignReceipt = nil
		if mode.responsibility ==
			driver.ImplementerImplementation {
			value.DesignReceipt = &productionReceiptBinding{
				OID: strings.Repeat("e", 40),
				BodyInput: driver.Input{
					Name:   "design-receipt",
					Path:   productionDesignReceiptPath,
					Digest: driver.Digest(receiptBody),
				},
				DetailInput: driver.Input{
					Name:   "design-receipt-detail",
					Path:   productionDesignDetailPath,
					Digest: driver.Digest(receiptDetail),
				},
				body: receiptBody, detail: receiptDetail,
			}
		}
		if mode.responsibility == driver.WorkVerification {
			value.Candidate = contextValue.Candidate
		}
		value.InvocationID = dispatchInvocationID(
			value.RunID,
			dispatchCoordinates{
				Slice:          value.Slice,
				Responsibility: value.Responsibility,
				BatonAttempt:   value.Attempt,
				Epoch:          value.Epoch,
				Try:            value.Try,
			},
		)
		if err := validateProductionWorkContext(
			manifest,
			value,
		); err != nil {
			t.Fatalf("%s exact workspace mode: %v", mode.responsibility, err)
		}
		if value.WorkspaceAccess == driver.ReadOnly {
			value.WorkspaceAccess = driver.ReadWrite
		} else {
			value.WorkspaceAccess = driver.ReadOnly
		}
		if err := validateProductionWorkContext(
			manifest,
			value,
		); !IsCode(err, "CORRUPT_JOURNAL") {
			t.Fatalf(
				"%s substituted workspace mode = %v",
				mode.responsibility,
				err,
			)
		}
	}
	contextBody := mustJSON(contextValue)
	contents, err := productionInputContents(
		contextValue,
		contextBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := productionRequestForContext(
		manifest,
		contextValue,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 4 ||
		len(request.Inputs) != 4 ||
		request.Inputs[0].Path != productionWorkContextPath ||
		request.Inputs[1].Path != productionPlanPath ||
		request.Inputs[2].Path != productionReceiptPath ||
		request.Inputs[3].Path != productionReceiptDetailPath {
		t.Fatalf("production inputs = %#v", request.Inputs)
	}
	requestBody, err := driver.EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	command := productionDispatchCommand{
		SchemaVersion: productionDispatchVersion,
		RequestDigest: driver.Digest(requestBody),
		Context:       contextValue,
	}
	resumeRequest, err := productionRequestForContextFreshness(
		manifest,
		contextValue,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	resumeBody, err := driver.EncodeRequest(resumeRequest)
	if err != nil {
		t.Fatal(err)
	}
	command.ResumeRequestDigest = driver.Digest(resumeBody)
	if _, err := parseProductionDispatchCommand(
		manifest,
		mustJSON(command),
	); err != nil {
		t.Fatal(err)
	}
	v1Context := contextValue
	v1Context.SchemaVersion = productionWorkContextVersionV1
	v1Context.Track = ""
	v1Context.PreparedBase = ""
	v1Context.DesignReceipt = nil
	v1Request, err := productionRequestForContext(
		manifest,
		v1Context,
	)
	if err != nil {
		t.Fatal(err)
	}
	v1RequestBody, err := driver.EncodeRequest(v1Request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseProductionDispatchCommand(
		manifest,
		mustJSON(productionDispatchCommand{
			SchemaVersion: productionDispatchVersionV1,
			RequestDigest: driver.Digest(v1RequestBody),
			Context:       v1Context,
		}),
	); err != nil {
		t.Fatalf("v1 recovery envelope = %v", err)
	}
	for name, hybrid := range map[string]productionDispatchCommand{
		"v1 command with v2 context": {
			SchemaVersion: productionDispatchVersionV1,
			RequestDigest: driver.Digest(requestBody),
			Context:       contextValue,
		},
		"v2 command with v1 context": {
			SchemaVersion: productionDispatchVersion,
			RequestDigest: driver.Digest(v1RequestBody),
			Context:       v1Context,
		},
	} {
		if _, err := parseProductionDispatchCommand(
			manifest,
			mustJSON(hybrid),
		); !IsCode(err, "CORRUPT_JOURNAL") {
			t.Fatalf("%s = %v", name, err)
		}
	}

	command.Context.Evidence[0].ProductTree = "not-a-digest"
	request, err = productionRequestForContext(
		manifest,
		command.Context,
	)
	if err != nil {
		t.Fatal(err)
	}
	requestBody, err = driver.EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	command.RequestDigest = driver.Digest(requestBody)
	if _, err := parseProductionDispatchCommand(
		manifest,
		mustJSON(command),
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("substituted evidence = %v", err)
	}
}

func TestProductionContextInputRehydrationRejectsMismatchedAuthority(
	t *testing.T,
) {
	t.Parallel()

	planBody := []byte("plan\n")
	input := driver.Input{
		Name: "plan", Path: productionPlanPath,
		Digest: driver.Digest(planBody),
	}
	persisted := productionWorkContext{
		Plan: &productionPlanBinding{Input: input},
	}
	current := productionWorkContext{
		Plan: &productionPlanBinding{
			Input: input,
			body:  planBody,
		},
	}
	rehydrated, err := rehydrateProductionContextInputs(
		persisted,
		current,
	)
	if err != nil ||
		!bytes.Equal(rehydrated.Plan.body, planBody) {
		t.Fatalf("exact rehydration = %#v, %v", rehydrated.Plan, err)
	}

	current.Plan.Input.Digest = driver.Digest([]byte("different\n"))
	if _, err := rehydrateProductionContextInputs(
		persisted,
		current,
	); !IsCode(err, "INVALID_AUTHORITY_STATE") {
		t.Fatalf("mismatched authority = %v", err)
	}
}

// TestProductionPlannerCannotEmitPlanBytesBeforeItsHumanTurn is the direct
// falsification of A2: a Planner that skips the summary boundary and hands
// back approval-ready plan bytes on its very first terminal is refused, and
// the run gains no proposal from it.
func TestProductionPlannerCannotEmitPlanBytesBeforeItsHumanTurn(
	t *testing.T,
) {
	t.Parallel()

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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 7, 3, 4, 5, 0, time.UTC)
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "journal.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release,
		TargetRef:  manifest.value.TargetRef, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: manifest.value.RunID, ReplayKey: "manifest",
		Kind: "start", Payload: manifest.raw, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(
		ctx, manifest.value.RunID, now, time.Minute, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	planBytes, _ := runtimePlan(
		t,
		manifest.value.Release,
		manifest.value.Authority.Project,
		manifest.value.TargetRef,
		"approval-release-1-v1",
	)
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		submission := driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocation.Request.InvocationID,
			Responsibility: driver.PlannerProposal,
			Summary:        "Plan without asking anybody.",
			Detail:         "Skips the human-only summary turn.",
		}
		submission.Plan, _ = driver.NewPlanBytes(planBytes)
		body, encodeErr := driver.EncodeSubmission(submission)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		seal, encodeErr := json.Marshal(driver.Seal{
			SchemaVersion:    driver.SealSchemaVersion,
			InvocationID:     submission.InvocationID,
			SubmissionDigest: driver.Digest(body),
			Accepted:         true,
			Code:             "accepted",
		})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		seal = append(seal, '\n')
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
				SealBytes:        seal,
				SealDigest:       driver.Digest(seal),
			},
		}, nil
	})
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, before := plannerProductionAuthority(t, engine)
	workspace, err := engine.workspaces.OpenSnapshot(target.Head)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	work := driverWorkIdentity(
		manifest.digest, "", driver.PlannerProposal, 1, before,
	)
	_, err = service.runDriverEffect(
		ctx,
		engine,
		workspace,
		driver.RolePlanner,
		dispatchCoordinates{
			Responsibility: driver.PlannerProposal,
			BatonAttempt:   1, Epoch: 1, Try: 1,
		},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1},
		before,
		owner,
		false,
	)
	if !IsCode(err, "INVALID_HUMAN_TURN") {
		t.Fatalf("unconfirmed plan bytes = %v", err)
	}
	effect, err := store.Effect(
		ctx, manifest.value.RunID, journal.AttemptEffectID(work, 1, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.OperationalFailed ||
		effect.ErrorCode != "invalid_human_turn" {
		t.Fatalf("refused planner effect = %#v", effect)
	}
	snapshot, err := store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range snapshot.Commands {
		if command.Kind == "planner_proposal" {
			t.Fatalf("unconfirmed proposal recorded = %#v", command)
		}
	}
}

func TestProductionDispatchPersistsRequestWithoutPreknownOutput(
	t *testing.T,
) {
	t.Parallel()

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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 2, 3, 4, 0, time.UTC)
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "journal.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: manifest.value.RunID, ReplayKey: "manifest",
		Kind: "start", Payload: manifest.raw, CreatedAt: now,
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

	planBytes, _ := runtimePlan(
		t,
		manifest.value.Release,
		manifest.value.Authority.Project,
		manifest.value.TargetRef,
		"approval-release-1-v1",
	)
	var observed driver.Invocation
	dispatcher := &plannerSummaryDispatcher{terminal: func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		observed = invocation
		submission := driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocation.Request.InvocationID,
			Responsibility: driver.PlannerProposal,
			Summary:        "Exact production plan.",
			Detail:         "Bound to the production request.",
		}
		submission.Plan, _ = driver.NewPlanBytes(planBytes)
		submissionBody, encodeErr := driver.EncodeSubmission(submission)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody, encodeErr := json.Marshal(driver.Seal{
			SchemaVersion:    driver.SealSchemaVersion,
			InvocationID:     submission.InvocationID,
			SubmissionDigest: driver.Digest(submissionBody),
			Accepted:         true,
			Code:             "accepted",
		})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody = append(sealBody, '\n')
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submissionBody,
				SubmissionDigest: driver.Digest(submissionBody),
				SealBytes:        sealBody,
				SealDigest:       driver.Digest(sealBody),
			},
		}, nil
	}}
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, before := plannerProductionAuthority(t, engine)
	workspace, err := engine.workspaces.OpenSnapshot(target.Head)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	coordinates := dispatchCoordinates{
		Responsibility: driver.PlannerProposal,
		BatonAttempt:   1,
		Epoch:          1,
		Try:            1,
	}
	work := driverWorkIdentity(
		manifest.digest,
		"",
		driver.PlannerProposal,
		1,
		before,
	)
	dispatch := func() (driver.Submission, error) {
		return service.runDriverEffect(
			ctx,
			engine,
			workspace,
			driver.RolePlanner,
			coordinates,
			journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1},
			before,
			owner,
			false,
		)
	}
	// A2: the production Planner's first terminal is the human-only summary
	// turn, so this dispatch parks instead of emitting plan bytes.
	if _, parkErr := dispatch(); !IsCode(parkErr, "EFFECT_PARKED") {
		t.Fatalf("planner summary park = %v", parkErr)
	}
	attention, found, readErr := openPlannerSummaryTurn(
		ctx, service, manifest.value.RunID,
	)
	if readErr != nil || !found ||
		attention.Question != plannerSummaryQuestion ||
		attention.Attention.HumanTurn.Kind !=
			string(driver.YieldHumanConfirmation) {
		t.Fatalf(
			"planner summary attention=%#v found=%t err=%v",
			attention, found, readErr,
		)
	}
	if _, err := store.AnswerAttention(ctx, journal.AnswerAttentionCommand{
		RunID:              manifest.value.RunID,
		Attention:          attention.Attention,
		ExpectedGeneration: attention.Generation,
		Answer:             plannerSummaryAnswer,
	}, now); err != nil {
		t.Fatal(err)
	}
	submission, err := dispatch()
	if err != nil {
		t.Fatal(err)
	}
	if submission.Responsibility != driver.PlannerProposal ||
		observed.Request.Profile != "planner" ||
		observed.Request.Model != "planner-model" ||
		len(observed.Inputs) != 1 ||
		observed.Inputs[0].Input.Path != productionWorkContextPath ||
		bytes.Contains(
			observed.Inputs[0].Bytes,
			[]byte(manifest.value.Repository),
		) {
		t.Fatalf("production invocation = %#v", observed)
	}

	effectID := journal.AttemptEffectID(work, 1, 1)
	effect, err := store.Effect(ctx, manifest.value.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.Succeeded ||
		effect.ExpectedDigest != productionOutputExpectation ||
		effect.ExpectedDigest == effect.ResultDigest ||
		effect.ResultDigest != sha256Digest(effect.Result) {
		t.Fatalf("production effect = %#v", effect)
	}
	snapshot, err := store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var command journal.Command
	for _, candidate := range snapshot.Commands {
		if candidate.ReplayKey == effectID {
			command = candidate
		}
	}
	persisted, err := parseProductionDispatchCommand(
		manifest,
		command.Payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	requestBody, err := driver.EncodeRequest(observed.Request)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RequestDigest != driver.Digest(requestBody) ||
		persisted.Context.DriverConfigDigest !=
			config.ConfigurationDigest() {
		t.Fatalf("persisted dispatch = %#v", persisted)
	}
	if _, _, err := validateSucceededDriverResult(
		manifest,
		command,
		effect,
	); err != nil {
		t.Fatalf("exact production result rejected: %v", err)
	}
	for name, mutate := range map[string]func(*journal.Command, *journal.Effect){
		"request digest": func(command *journal.Command, _ *journal.Effect) {
			var value productionDispatchCommand
			if json.Unmarshal(command.Payload, &value) != nil {
				t.Fatal("decode persisted command")
			}
			value.RequestDigest = "sha256:" + strings.Repeat("0", 64)
			command.Payload = mustJSON(value)
		},
		"dynamic sentinel": func(_ *journal.Command, effect *journal.Effect) {
			effect.ExpectedDigest = effect.ResultDigest
		},
		"result digest": func(_ *journal.Command, effect *journal.Effect) {
			effect.ResultDigest = "sha256:" + strings.Repeat("0", 64)
		},
		"submission invocation": func(_ *journal.Command, effect *journal.Effect) {
			value, decodeErr := driver.DecodeSubmission(effect.Result)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			value.InvocationID += "-substituted"
			effect.Result, decodeErr = driver.EncodeSubmission(value)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			effect.ResultDigest = sha256Digest(effect.Result)
		},
	} {
		t.Run(name, func(t *testing.T) {
			mutatedCommand := command
			mutatedCommand.Payload = append(
				[]byte(nil),
				command.Payload...,
			)
			mutatedEffect := effect
			mutatedEffect.Result = append([]byte(nil), effect.Result...)
			mutate(&mutatedCommand, &mutatedEffect)
			if _, _, validationErr := validateSucceededDriverResult(
				manifest,
				mutatedCommand,
				mutatedEffect,
			); !IsCode(validationErr, "CORRUPT_JOURNAL") {
				t.Fatalf("validation = %v", validationErr)
			}
		})
	}

	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := driver.DecodeDriverConfig(config.CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	restartedProduction, err := newProductionDriverRuntime(
		reloaded,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{
		journal: store, production: restartedProduction,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	restartedManifest, _, err := restarted.loadRun(
		ctx,
		manifest.value.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	restartedEngine, err := restarted.openEngine(restartedManifest)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()
}

type productionImplementationRecoveryFixture struct {
	ctx         context.Context
	repository  string
	config      driver.LoadedDriverConfig
	manifest    admittedManifest
	store       *journal.Store
	owner       journal.OwnerLease
	now         time.Time
	service     *Service
	engine      *engine
	state       baton.State
	slice       *baton.SliceState
	track       *baton.TrackState
	cycle       implementationCycle
	outer       journal.Effect
	workspace   *gitx.WorkspaceLease
	coordinates dispatchCoordinates
}

func newProductionImplementationRecoveryFixture(
	t *testing.T,
	dispatcher driver.Driver,
) *productionImplementationRecoveryFixture {
	t.Helper()
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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 5, 6, 7, 0, time.UTC)
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
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
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
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	planBytes, _ := runtimePlan(
		t,
		manifest.value.Release,
		manifest.value.Authority.Project,
		manifest.value.TargetRef,
		"approval-release-1-v1",
	)
	if _, err := engine.actions.RecordPlanRevision(
		baton.RecordPlanRevisionInput{
			PlanBytes: planBytes,
			Summary:   "Install the exact production recovery plan.",
			Detail:    []byte("Production recovery fixture."),
		},
	); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []baton.AppendReceiptInput{
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "implementer", Result: "designed",
			Summary: "Design the production recovery fixture.",
			Detail:  []byte("Exact design."),
		},
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "captain", Result: "proceed",
			Summary: "Proceed with the production recovery fixture.",
			Detail:  []byte("Exact review."),
		},
	} {
		if _, err := engine.actions.AppendReceipt(receipt); err != nil {
			t.Fatal(err)
		}
	}
	state, err := baton.ReadState(
		engine.git,
		manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, sliceOK := state.Slice("S1")
	track, trackOK := state.Track("T1")
	if !sliceOK || !trackOK || slice.CurrentReceipt == nil ||
		slice.Stage != "implement" ||
		slice.NextRole != "implementer" {
		t.Fatalf("implementation authority = %#v", state)
	}
	before := sliceFingerprint(state, "S1")
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle := implementationCycle{GitIdentity: runtimeTestGitIdentity,
		Release: state.Release, Slice: "S1",
		Binds: slice.CurrentReceipt.OID, Before: before,
		Plan: state.Plan.OID, ReleaseHead: state.Refs.Release.Head,
		TargetHead: state.Refs.Target.Head, Track: track.ID,
		TrackRef: track.Ref, TrackHead: track.Head,
		DispatchWork: workIdentity(
			outerWork,
			"driver.dispatch",
		),
		PreparedWork: workIdentity(
			outerWork,
			"git.seal.prepared",
		),
	}
	cycle.DispatchEffect = journal.AttemptEffectID(
		cycle.DispatchWork,
		1,
		1,
	)
	cycle.PreparedEffect = journal.AttemptEffectID(
		cycle.PreparedWork,
		1,
		1,
	)
	outerPayload := mustJSON(cycle)
	if err := store.EnsureAttempt(
		ctx,
		journal.Command{
			RunID: owner.RunID, ReplayKey: outerID,
			Kind: "git.seal", Payload: outerPayload, CreatedAt: now,
		},
		journal.Effect{
			RunID: owner.RunID, ID: outerID, ReplayKey: outerID,
			Kind: "git.seal", BeforeDigest: outerWork,
			ExpectedDigest: sha256Digest(outerPayload), UpdatedAt: now,
		},
		journal.EffectAttempt{WorkID: outerWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	outerClaim, err := store.ClaimOwned(
		ctx,
		owner,
		outerID,
		now,
		effectLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := engine.workspaces.OpenTrack(
		gitx.TrackKey{Release: state.Release, Track: track.ID},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &productionImplementationRecoveryFixture{
		ctx: ctx, repository: repository, config: config,
		manifest: manifest, store: store, owner: owner, now: now,
		service: service, engine: engine, state: state,
		slice: slice, track: track, cycle: cycle,
		outer: journal.Effect{
			RunID: owner.RunID, ID: outerID, Kind: "git.seal",
			State: journal.Claimed, CurrentClaim: outerClaim.Token,
		},
		workspace: workspace,
		coordinates: dispatchCoordinates{
			Slice: "S1", Responsibility: driver.ImplementerImplementation,
			BatonAttempt: slice.Attempt, Epoch: 1, Try: 1,
		},
	}
}

func productionImplementationSubmission(
	t *testing.T,
	invocationID string,
) (driver.Submission, []byte) {
	t.Helper()
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   invocationID,
		Responsibility: driver.ImplementerImplementation,
		Summary:        "Durable production candidate.",
		Detail:         "Bound to the exact production implementation request.",
	}
	submission.Checks, _ = driver.NewCheckBytes(
		[]byte("production implementation checks\n"),
	)
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	return submission, body
}

func productionImplementationObservation(
	t *testing.T,
	invocation driver.Invocation,
) driver.Observation {
	t.Helper()
	_, submissionBody := productionImplementationSubmission(
		t,
		invocation.Request.InvocationID,
	)
	sealBody, err := json.Marshal(driver.Seal{
		SchemaVersion:    driver.SealSchemaVersion,
		InvocationID:     invocation.Request.InvocationID,
		SubmissionDigest: driver.Digest(submissionBody),
		Accepted:         true,
		Code:             "accepted",
	})
	if err != nil {
		t.Fatal(err)
	}
	sealBody = append(sealBody, '\n')
	return driver.Observation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
		Handoff: &driver.SealedHandoff{
			SubmissionBytes:  submissionBody,
			SubmissionDigest: driver.Digest(submissionBody),
			SealBytes:        sealBody,
			SealDigest:       driver.Digest(sealBody),
		},
	}
}

func prepareClaimedProductionImplementation(
	t *testing.T,
	fixture *productionImplementationRecoveryFixture,
	contents string,
) (sealedRecord, journal.Claim, journal.Claim) {
	t.Helper()
	preparedDispatch, err := fixture.service.prepareDriverDispatch(
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
	if err := fixture.store.EnsureAttempt(
		fixture.ctx,
		journal.Command{
			RunID:     fixture.owner.RunID,
			ReplayKey: fixture.cycle.DispatchEffect,
			Kind:      "driver.dispatch", Payload: preparedDispatch.commandPayload,
			CreatedAt: fixture.now,
		},
		journal.Effect{
			RunID:          fixture.owner.RunID,
			ID:             fixture.cycle.DispatchEffect,
			ReplayKey:      fixture.cycle.DispatchEffect,
			Kind:           "driver.dispatch",
			BeforeDigest:   sha256Digest([]byte(fixture.cycle.Before)),
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
	dispatchClaim, err := fixture.store.ClaimOwned(
		fixture.ctx,
		fixture.owner,
		fixture.cycle.DispatchEffect,
		fixture.now,
		effectLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "one.txt"),
		[]byte(contents),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	submission, _ := productionImplementationSubmission(
		t,
		preparedDispatch.request.InvocationID,
	)
	record, preparedClaim, err :=
		fixture.service.prepareProductionImplementationCandidate(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			fixture.workspace,
			fixture.cycle,
			submission,
		)
	if err != nil {
		t.Fatal(err)
	}
	return record, preparedClaim, dispatchClaim
}

func TestCandidateHeadRefreshDispatchAndReceipt(t *testing.T) {
	for _, test := range []struct {
		name       string
		consumes   bool
		driverEdit bool
		outOfScope bool
	}{
		{name: "non-consuming clean adoption"},
		{name: "consuming clean adoption", consumes: true},
		{name: "refresh plus legitimate edit", driverEdit: true},
		{name: "out-of-scope clean adoption", outOfScope: true},
		{
			name:       "out-of-scope refresh plus legitimate edit",
			driverEdit: true, outOfScope: true,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runCandidateHeadRefreshDispatch(
				t,
				test.consumes,
				test.driverEdit,
				test.outOfScope,
			)
		})
	}
}

func runCandidateHeadRefreshDispatch(
	t *testing.T,
	consumes bool,
	driverEdit bool,
	outOfScope bool,
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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC)
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
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
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
	var invocations atomic.Int64
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		invocations.Add(1)
		if driverEdit {
			if err := os.WriteFile(
				filepath.Join(invocation.HostWorkspace, "two.txt"),
				[]byte("driver refresh\n"),
				0o600,
			); err != nil {
				return driver.Observation{}, err
			}
		}
		return productionImplementationObservation(t, invocation), nil
	})
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	_, parsed := runtimePlan(
		t,
		manifest.value.Release,
		manifest.value.Authority.Project,
		manifest.value.TargetRef,
		"approval-release-1-v1",
	)
	metadata := parsed.Metadata()
	if consumes {
		metadata.Tracks[1].Slices[0].Consumes = []string{"S1"}
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nConsuming refresh fixture.\n",
	)
	if _, err := engine.actions.RecordPlanRevision(
		baton.RecordPlanRevisionInput{
			PlanBytes: planBytes,
			Summary:   "Install the consuming refresh plan.",
			Detail:    []byte("Exact consuming refresh fixture."),
		},
	); err != nil {
		t.Fatal(err)
	}
	appendReceipt := func(input baton.AppendReceiptInput) baton.ActionResult {
		t.Helper()
		result, err := engine.actions.AppendReceipt(input)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	readState := func() baton.State {
		t.Helper()
		state, err := baton.ReadState(
			engine.git,
			manifest.value.Release,
			engine.inertness,
		)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	prepare := func(sliceID string) (baton.State, *baton.SliceState) {
		t.Helper()
		state := readState()
		slice, ok := state.Slice(sliceID)
		if !ok {
			t.Fatalf("slice %s is absent", sliceID)
		}
		state, slice, err = service.prepareTrackBaseForSlice(
			ctx,
			engine,
			owner,
			state,
			slice,
		)
		if err != nil {
			t.Fatal(err)
		}
		return state, slice
	}
	sealTrack := func(track, path, contents string) gitx.SealedCandidate {
		t.Helper()
		workspace, err := engine.workspaces.OpenTrack(
			gitx.TrackKey{
				Release: manifest.value.Release,
				Track:   track,
			},
			gitx.ImplementationView,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer workspace.Close()
		if err := os.WriteFile(
			filepath.Join(workspace.Path(), path),
			[]byte(contents),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		candidate, err := engine.workspaces.SealTrack(workspace)
		if err != nil {
			t.Fatal(err)
		}
		return candidate
	}

	_, _ = prepare("S1")
	appendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1",
		Role: "implementer", Result: "designed",
		Summary: "Design producer.", Detail: []byte("Exact producer design."),
	})
	appendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1",
		Role: "captain", Result: "proceed",
		Summary: "Proceed producer.", Detail: []byte("Exact producer review."),
	})
	producer := sealTrack("T1", "one.txt", "producer\n")
	appendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1",
		Role: "implementer", Result: "candidate",
		Summary: "Producer candidate.", Detail: []byte("Exact producer."),
		Candidate:    producer.Candidate.String(),
		CheckResults: []byte("producer checks\n"),
	})
	appendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1",
		Role: "verifier", Result: "pass",
		Summary: "Producer passes.", Detail: []byte("Exact producer verification."),
		Candidate:    producer.Candidate.String(),
		CheckResults: []byte("producer verification checks\n"),
	})

	_, _ = prepare("S2")
	appendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S2",
		Role: "implementer", Result: "designed",
		Summary: "Design consumer.", Detail: []byte("Exact consumer design."),
	})
	appendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S2",
		Role: "captain", Result: "proceed",
		Summary: "Proceed consumer.", Detail: []byte("Exact consumer review."),
	})
	consumerReady := readState()
	consumer, ok := consumerReady.Slice("S2")
	if !ok ||
		(consumes && consumer.PreparedBase == "") ||
		(!consumes && consumer.PreparedBase != "") {
		t.Fatalf("consumer base = %#v", consumer)
	}
	firstBase := consumer.PreparedBase
	firstCandidate := sealTrack("T2", "two.txt", "first consumer\n")
	firstReceipt := appendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S2",
		Role: "implementer", Result: "candidate",
		Summary: "First consumer candidate.", Detail: []byte("Exact first consumer."),
		Base: firstBase, Candidate: firstCandidate.Candidate.String(),
		CheckResults: []byte("first consumer checks\n"),
	})
	if firstReceipt.Receipt == nil {
		t.Fatal("first consumer receipt is absent")
	}
	refreshPath := "two.txt"
	refreshBody := "unreceipted refresh\n"
	if outOfScope {
		refreshPath = "outside.txt"
		refreshBody = "out-of-scope refresh\n"
	}
	unreceipted := sealTrack("T2", refreshPath, refreshBody)
	refreshed := readState()
	consumer, ok = refreshed.Slice("S2")
	track, trackOK := refreshed.Track("T2")
	stableBase := firstReceipt.ReceiptCommit
	baseStable := consumer.PreparedBase == ""
	if consumes {
		baseStable = consumer.PreparedBase == stableBase
	}
	if !ok || !trackOK ||
		!baseStable ||
		track.Head != unreceipted.Candidate.String() ||
		!candidateHeadRefresh(refreshed, consumer) {
		t.Fatalf("consuming refresh = track %#v slice %#v", track, consumer)
	}

	dispatchErr := service.implementSlice(
		ctx,
		engine,
		owner,
		refreshed,
		consumer,
	)
	if outOfScope {
		if !IsCode(dispatchErr, "EFFECT_PARKED") {
			t.Fatalf("out-of-scope refresh error = %v", dispatchErr)
		}
		rejected := readState()
		rejectedSlice, ok := rejected.Slice("S2")
		rejectedTrack, trackOK := rejected.Track("T2")
		if !ok || !trackOK ||
			rejectedTrack.Head != unreceipted.Candidate.String() ||
			rejectedSlice.CurrentReceipt == nil ||
			rejectedSlice.CurrentReceipt.OID != stableBase ||
			rejectedSlice.Stage != "implement" {
			t.Fatalf(
				"rejected refresh mutated authority = track %#v slice %#v",
				rejectedTrack,
				rejectedSlice,
			)
		}
		snapshot, err := store.Snapshot(ctx, owner.RunID)
		if err != nil {
			t.Fatal(err)
		}
		failures := 0
		for _, effect := range snapshot.Effects {
			if effect.Kind == "git.seal" &&
				effect.ErrorCode == "CANDIDATE_SCOPE_FAILED" {
				failures++
			}
			if effect.Kind == "git.seal.prepared" {
				t.Fatalf("out-of-scope refresh wrote prepared receipt: %#v", effect)
			}
		}
		// The S4 identical-failure guard parks after two consecutive
		// identical scope failures, before the third try burns.
		if failures != 2 {
			t.Fatalf("scope failures = %d, want 2", failures)
		}
		return
	}
	if dispatchErr != nil {
		t.Fatalf("dispatch candidate refresh: %v", dispatchErr)
	}
	completed := readState()
	consumer, ok = completed.Slice("S2")
	if !ok ||
		consumer.Stage != "verify" ||
		consumer.Attempt != 2 ||
		consumer.CurrentReceipt == nil ||
		consumer.CurrentReceipt.Receipt.Binds != stableBase ||
		consumer.CurrentReceipt.Receipt.Candidate == nil ||
		invocations.Load() != 1 {
		t.Fatalf("completed consuming refresh = %#v", consumer)
	}
	if consumes {
		if consumer.CurrentReceipt.Receipt.Base == nil ||
			*consumer.CurrentReceipt.Receipt.Base != stableBase {
			t.Fatalf("consuming receipt base = %#v", consumer.CurrentReceipt)
		}
	} else if consumer.CurrentReceipt.Receipt.Base != nil {
		t.Fatalf("non-consuming receipt base = %#v", consumer.CurrentReceipt)
	}
	snapshot, err := store.Snapshot(ctx, owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var sealed sealedRecord
	sealedCount := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind != "git.seal" || effect.State != journal.Succeeded ||
			len(effect.Result) == 0 {
			continue
		}
		if err := json.Unmarshal(effect.Result, &sealed); err != nil {
			t.Fatal(err)
		}
		sealedCount++
	}
	if sealedCount != 1 ||
		sealed.Before != unreceipted.Candidate.String() ||
		sealed.RefreshFrom != stableBase ||
		sealed.Candidate != *consumer.CurrentReceipt.Receipt.Candidate ||
		len(sealed.ChangedPaths) != 1 ||
		sealed.ChangedPaths[0] != "two.txt" {
		t.Fatalf("refresh seal evidence = %#v (count %d)", sealed, sealedCount)
	}
	wantCandidate := unreceipted.Candidate.String()
	if driverEdit {
		wantCandidate = *consumer.CurrentReceipt.Receipt.Candidate
		if wantCandidate == unreceipted.Candidate.String() {
			t.Fatal("driver edit was adopted without a new candidate")
		}
		candidate, parseErr := gitx.ParseOID(
			engine.repository.ObjectFormat(),
			wantCandidate,
		)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		parents, err := engine.repository.Parents(candidate)
		if err != nil || len(parents) != 1 ||
			parents[0] != unreceipted.Candidate {
			t.Fatalf("refreshed candidate parents = %#v, %v", parents, err)
		}
	} else if *consumer.CurrentReceipt.Receipt.Candidate != wantCandidate {
		t.Fatalf(
			"clean refresh candidate = %s, want adopted %s",
			*consumer.CurrentReceipt.Receipt.Candidate,
			wantCandidate,
		)
	}
	if body := runRuntimeGit(
		t,
		repository,
		"show",
		*consumer.CurrentReceipt.Receipt.Candidate+":two.txt",
	); body != map[bool]string{
		false: "unreceipted refresh",
		true:  "driver refresh",
	}[driverEdit] {
		t.Fatalf("refreshed consumer body = %q", body)
	}
}

type preparedHeadRefreshRecovery struct {
	fixture *productionImplementationRecoveryFixture
	cycle   implementationCycle
	head    gitx.OID
}

func prepareCleanHeadRefreshRecovery(
	t *testing.T,
) preparedHeadRefreshRecovery {
	t.Helper()
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.completeImplementationFailure(
		fixture.ctx,
		fixture.owner,
		fixture.outer.ID,
		fixture.outer.CurrentClaim,
		"superseded_test_cycle",
	); err != nil {
		t.Fatal(err)
	}
	seal := func(contents string) gitx.SealedCandidate {
		t.Helper()
		workspace, err := fixture.engine.workspaces.OpenTrack(
			gitx.TrackKey{
				Release: fixture.manifest.value.Release,
				Track:   "T1",
			},
			gitx.ImplementationView,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(workspace.Path(), "one.txt"),
			[]byte(contents),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		candidate, err := fixture.engine.workspaces.SealTrack(workspace)
		if closeErr := workspace.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		return candidate
	}
	first := seal("first candidate\n")
	firstReceipt, err := fixture.engine.actions.AppendReceipt(
		baton.AppendReceiptInput{
			Release:      fixture.manifest.value.Release,
			Slice:        "S1",
			Role:         "implementer",
			Result:       "candidate",
			Summary:      "First candidate.",
			Detail:       []byte("Exact first candidate."),
			Candidate:    first.Candidate.String(),
			CheckResults: []byte("first checks\n"),
		},
	)
	if err != nil || firstReceipt.Receipt == nil {
		t.Fatalf("first candidate receipt = %#v, %v", firstReceipt, err)
	}
	head := seal("unreceipted correction\n")
	state, err := baton.ReadState(
		fixture.engine.git,
		fixture.manifest.value.Release,
		fixture.engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice("S1")
	track, trackOK := state.Track("T1")
	if !ok || !trackOK ||
		!candidateHeadRefresh(state, slice) ||
		slice.CurrentReceipt == nil ||
		slice.CurrentReceipt.OID != firstReceipt.ReceiptCommit ||
		track.Head != head.Candidate.String() {
		t.Fatalf("prepared refresh authority = track %#v slice %#v", track, slice)
	}
	before := sliceFingerprint(state, "S1")
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle := implementationCycle{GitIdentity: runtimeTestGitIdentity,
		Release:     state.Release,
		Slice:       "S1",
		Binds:       slice.CurrentReceipt.OID,
		Before:      before,
		Plan:        state.Plan.OID,
		ReleaseHead: state.Refs.Release.Head,
		TargetHead:  state.Refs.Target.Head,
		Track:       track.ID,
		TrackRef:    track.Ref,
		TrackHead:   track.Head,
		RefreshFrom: slice.CurrentReceipt.OID,
		DispatchWork: workIdentity(
			outerWork,
			"driver.dispatch",
		),
		PreparedWork: workIdentity(
			outerWork,
			"git.seal.prepared",
		),
	}
	cycle.DispatchEffect = journal.AttemptEffectID(
		cycle.DispatchWork,
		1,
		1,
	)
	cycle.PreparedEffect = journal.AttemptEffectID(
		cycle.PreparedWork,
		1,
		1,
	)
	payload := mustJSON(cycle)
	if err := fixture.store.EnsureAttempt(
		fixture.ctx,
		journal.Command{
			RunID: fixture.owner.RunID, ReplayKey: outerID,
			Kind: "git.seal", Payload: payload, CreatedAt: fixture.now,
		},
		journal.Effect{
			RunID: fixture.owner.RunID, ID: outerID,
			ReplayKey: outerID, Kind: "git.seal",
			BeforeDigest:   outerWork,
			ExpectedDigest: sha256Digest(payload),
			UpdatedAt:      fixture.now,
		},
		journal.EffectAttempt{
			WorkID: outerWork,
			Epoch:  1,
			Try:    1,
		},
	); err != nil {
		t.Fatal(err)
	}
	outerClaim, err := fixture.store.ClaimOwned(
		fixture.ctx,
		fixture.owner,
		outerID,
		fixture.now,
		effectLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outerClaim.Token == "" {
		t.Fatal("refresh recovery outer claim is empty")
	}
	workspace, err := fixture.engine.workspaces.OpenTrack(
		gitx.TrackKey{
			Release: fixture.manifest.value.Release,
			Track:   "T1",
		},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	record, preparedClaim, err :=
		fixture.service.runProductionImplementationDispatch(
			fixture.ctx,
			fixture.engine,
			fixture.owner,
			workspace,
			cycle,
			dispatchCoordinates{
				Slice:          "S1",
				Responsibility: driver.ImplementerImplementation,
				BatonAttempt:   slice.Attempt,
				Epoch:          1,
				Try:            1,
			},
		)
	if closeErr := workspace.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if record.Before != head.Candidate.String() ||
		record.Candidate != head.Candidate.String() ||
		record.RefreshFrom != firstReceipt.ReceiptCommit ||
		preparedClaim.Token == "" {
		t.Fatalf("prepared clean adoption = %#v", record)
	}
	return preparedHeadRefreshRecovery{
		fixture: fixture,
		cycle:   cycle,
		head:    head.Candidate,
	}
}

func TestCleanHeadRefreshPreparedRecoveryNeverRewindsOrDuplicates(
	t *testing.T,
) {
	for _, foreign := range []bool{false, true} {
		name := "completes"
		if foreign {
			name = "foreign head fails closed"
		}
		t.Run(name, func(t *testing.T) {
			prepared := prepareCleanHeadRefreshRecovery(t)
			foreignHead := prepared.head
			if foreign {
				workspace, err := prepared.fixture.engine.workspaces.OpenTrack(
					gitx.TrackKey{
						Release: prepared.fixture.manifest.value.Release,
						Track:   "T1",
					},
					gitx.ImplementationView,
				)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(workspace.Path(), "one.txt"),
					[]byte("foreign correction\n"),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
				sealed, err := prepared.fixture.engine.workspaces.SealTrack(
					workspace,
				)
				if closeErr := workspace.Close(); err == nil {
					err = closeErr
				}
				if err != nil {
					t.Fatal(err)
				}
				foreignHead = sealed.Candidate
			}
			if err := prepared.fixture.engine.Close(); err != nil {
				t.Fatal(err)
			}
			production, err := newProductionDriverRuntime(
				prepared.fixture.config,
				driver.DriverFactoryOptions{},
			)
			if err != nil {
				t.Fatal(err)
			}
			restarted := &Service{
				journal:       prepared.fixture.store,
				production:    production,
				gitExecutable: prepared.fixture.service.gitExecutable,
				now: func() time.Time {
					return prepared.fixture.now
				},
			}
			restartedEngine, err := restarted.openEngine(
				prepared.fixture.manifest,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer restartedEngine.Close()
			recovered, err :=
				restarted.recoverImplementationClaims(
					prepared.fixture.ctx,
					restartedEngine,
					prepared.fixture.owner,
				)
			if foreign {
				if !recovered || !IsCode(err, "RECOVERY_UNCERTAIN") {
					t.Fatalf("foreign recovery = %t, %v", recovered, err)
				}
				if got := runRuntimeGit(
					t,
					prepared.fixture.repository,
					"rev-parse",
					prepared.cycle.TrackRef,
				); got != foreignHead.String() {
					t.Fatalf("foreign head was rewritten to %s", got)
				}
				state, stateErr := baton.ReadState(
					restartedEngine.git,
					prepared.fixture.manifest.value.Release,
					restartedEngine.inertness,
				)
				if stateErr != nil {
					t.Fatal(stateErr)
				}
				slice, _ := state.Slice("S1")
				for _, entry := range slice.History.Entries {
					if entry.Receipt.Role == "implementer" &&
						entry.Receipt.Result == "candidate" &&
						entry.Receipt.Attempt != nil &&
						*entry.Receipt.Attempt == 2 {
						t.Fatalf("foreign recovery wrote receipt %#v", entry)
					}
				}
				return
			}
			if err != nil || !recovered {
				t.Fatalf("clean recovery = %t, %v", recovered, err)
			}
			for attempts := 0; attempts < 3; attempts++ {
				recovered, err =
					restarted.recoverImplementationClaims(
						prepared.fixture.ctx,
						restartedEngine,
						prepared.fixture.owner,
					)
				if err != nil {
					t.Fatal(err)
				}
				if !recovered {
					break
				}
			}
			if recovered {
				t.Fatal("clean adoption recovery did not become quiescent")
			}
			state, err := baton.ReadState(
				restartedEngine.git,
				prepared.fixture.manifest.value.Release,
				restartedEngine.inertness,
			)
			if err != nil {
				t.Fatal(err)
			}
			slice, ok := state.Slice("S1")
			candidates := 0
			for _, entry := range slice.History.Entries {
				if entry.Receipt.Role != "implementer" ||
					entry.Receipt.Result != "candidate" ||
					entry.Receipt.Attempt == nil ||
					*entry.Receipt.Attempt != 2 {
					continue
				}
				candidates++
				if entry.Receipt.Candidate == nil ||
					*entry.Receipt.Candidate != prepared.head.String() {
					t.Fatalf("recovered candidate = %#v", entry)
				}
			}
			if !ok || slice.Stage != "verify" || candidates != 1 {
				t.Fatalf(
					"recovered slice = %#v, attempt-2 candidates=%d",
					slice,
					candidates,
				)
			}
		})
	}
}

func TestProductionImplementationRecoveryBeforeAnyChildRetriesKnownOldState(
	t *testing.T,
) {
	for _, initial := range []string{"pending", "claimed"} {
		t.Run(initial, func(t *testing.T) {
			fixture := newProductionImplementationRecoveryFixture(t, nil)
			if initial == "pending" {
				if err := fixture.store.ReconcileOwned(
					fixture.ctx,
					fixture.owner,
					journal.Completion{
						RunID: fixture.owner.RunID, EffectID: fixture.outer.ID,
						Token:     fixture.outer.CurrentClaim,
						EventKind: "test_outer_all_old",
						At:        fixture.now,
					},
					journal.RecoveryAllOld,
				); err != nil {
					t.Fatal(err)
				}
			}
			if err := fixture.workspace.Close(); err != nil {
				t.Fatal(err)
			}
			for _, childID := range []string{
				fixture.cycle.DispatchEffect,
				fixture.cycle.PreparedEffect,
			} {
				if _, err := fixture.store.Effect(
					fixture.ctx,
					fixture.owner.RunID,
					childID,
				); !journal.IsCode(err, "EFFECT_NOT_FOUND") {
					t.Fatalf("child %s before recovery = %v", childID, err)
				}
			}
			recovered, err := fixture.service.recoverImplementationClaims(
				fixture.ctx,
				fixture.engine,
				fixture.owner,
			)
			if err != nil || !recovered {
				t.Fatalf("no-child recovery = %t, %v", recovered, err)
			}
			outer, err := fixture.store.Effect(
				fixture.ctx,
				fixture.owner.RunID,
				fixture.outer.ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if outer.State != journal.OperationalFailed ||
				outer.ErrorCode != "implementation_interrupted" {
				t.Fatalf("recovered outer = %#v", outer)
			}
			for _, childID := range []string{
				fixture.cycle.DispatchEffect,
				fixture.cycle.PreparedEffect,
			} {
				if _, err := fixture.store.Effect(
					fixture.ctx,
					fixture.owner.RunID,
					childID,
				); !journal.IsCode(err, "EFFECT_NOT_FOUND") {
					t.Fatalf("child %s after recovery = %v", childID, err)
				}
			}
			if head := runRuntimeGit(
				t,
				fixture.repository,
				"rev-parse",
				fixture.track.Ref,
			); head != fixture.cycle.TrackHead {
				t.Fatalf("track head = %s, want %s", head, fixture.cycle.TrackHead)
			}
			recovered, err = fixture.service.recoverImplementationClaims(
				fixture.ctx,
				fixture.engine,
				fixture.owner,
			)
			if err != nil || recovered {
				t.Fatalf("terminal no-child recovery = %t, %v", recovered, err)
			}
		})
	}
}

func TestProductionPreparedCandidateWithClaimedDispatchBecomesCoupledUncertainty(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	record, preparedClaim, _ := prepareClaimedProductionImplementation(
		t,
		fixture,
		"claimed production candidate\n",
	)
	if preparedClaim.Token == "" ||
		runRuntimeGit(
			t,
			fixture.repository,
			"rev-parse",
			fixture.track.Ref,
		) != fixture.cycle.TrackHead ||
		runRuntimeGit(
			t,
			fixture.repository,
			"show",
			record.Candidate+":one.txt",
		) != "claimed production candidate" {
		t.Fatalf("prepared crash window = %#v", record)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Close(); err != nil {
		t.Fatal(err)
	}
	restartedProduction, err := newProductionDriverRuntime(
		fixture.config,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{
		journal: fixture.store, production: restartedProduction,
		gitExecutable: fixture.service.gitExecutable,
		now:           func() time.Time { return fixture.now },
	}
	restartedEngine, err := restarted.openEngine(fixture.manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()
	recovered, err := restarted.recoverImplementationClaims(
		fixture.ctx,
		restartedEngine,
		fixture.owner,
	)
	if !recovered || !IsCode(err, "RECOVERY_UNCERTAIN") {
		t.Fatalf("three-claim recovery = %t, %v", recovered, err)
	}
	for _, effectID := range []string{
		fixture.outer.ID,
		fixture.cycle.PreparedEffect,
		fixture.cycle.DispatchEffect,
	} {
		effect, effectErr := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			effectID,
		)
		if effectErr != nil {
			t.Fatal(effectErr)
		}
		if effect.State != journal.Uncertain {
			t.Fatalf("coupled effect %s = %#v", effectID, effect)
		}
	}
	recovered, err = restarted.recoverImplementationClaims(
		fixture.ctx,
		restartedEngine,
		fixture.owner,
	)
	if err != nil || recovered {
		t.Fatalf("current coupled uncertainty = %t, %v", recovered, err)
	}
	recovered, err = restarted.recoverStaleClaimedDispatches(
		fixture.ctx,
		restartedEngine,
		fixture.owner,
	)
	if err != nil || recovered {
		t.Fatalf("current dispatch uncertainty = %t, %v", recovered, err)
	}

	if err := os.WriteFile(
		filepath.Join(fixture.repository, "README.md"),
		[]byte("superseded production fixture\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, fixture.repository, "add", "--", "README.md")
	runRuntimeGit(
		t,
		fixture.repository,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit", "--quiet", "-m", "supersede production authority",
	)
	recovered, err = restarted.recoverStaleClaimedDispatches(
		fixture.ctx,
		restartedEngine,
		fixture.owner,
	)
	if err != nil || !recovered {
		t.Fatalf("stale coupled uncertainty = %t, %v", recovered, err)
	}
	for _, effectID := range []string{
		fixture.outer.ID,
		fixture.cycle.PreparedEffect,
		fixture.cycle.DispatchEffect,
	} {
		effect, effectErr := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			effectID,
		)
		if effectErr != nil {
			t.Fatal(effectErr)
		}
		if effect.State != journal.OperationalFailed ||
			effect.ErrorCode != "stale_authority" {
			t.Fatalf("resolved coupled effect %s = %#v", effectID, effect)
		}
	}
}

// C7: the dispatch-path claimed writer is reachable by seal-cycle children
// through runProductionImplementationDispatch, so its reconciliation gate
// must leave governed children on their coupled path: a claimed
// implementation dispatch still writes dispatch_uncertain and refuses with
// RECOVERY_UNCERTAIN instead of being independently cleared or preserved.
func TestGovernedSealCycleChildKeepsCoupledUncertaintyOnDispatchFallback(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	_, _, dispatchClaim := prepareClaimedProductionImplementation(
		t,
		fixture,
		"governed child contents\n",
	)
	if dispatchClaim.Token == "" {
		t.Fatal("production dispatch claim is empty")
	}
	_, err := fixture.service.runDriverEffectWithPreparation(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		journal.EffectAttempt{
			WorkID: fixture.cycle.DispatchWork,
			Epoch:  fixture.coordinates.Epoch,
			Try:    1,
		},
		fixture.cycle.Before,
		fixture.owner,
		nil,
		true,
	)
	if !IsCode(err, "RECOVERY_UNCERTAIN") {
		t.Fatalf("governed dispatch fallback = %v", err)
	}
	effect, effectErr := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if effectErr != nil {
		t.Fatal(effectErr)
	}
	if effect.State != journal.Uncertain ||
		effect.CurrentClaim != "" {
		t.Fatalf("governed coupled child = %#v", effect)
	}
	snapshot, snapshotErr := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	uncertainEvents := 0
	for _, event := range snapshot.Events {
		if event.Kind == "dispatch_uncertain" {
			uncertainEvents++
		}
	}
	if uncertainEvents != 1 {
		t.Fatalf("dispatch_uncertain events = %d, want 1", uncertainEvents)
	}
}

func TestProductionFailedDispatchTerminalizesItsPreparedCandidate(
	t *testing.T,
) {
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	record, preparedClaim, dispatchClaim :=
		prepareClaimedProductionImplementation(
			t,
			fixture,
			"orphaned production candidate\n",
		)
	if preparedClaim.Token == "" || dispatchClaim.Token == "" {
		t.Fatal("production child claims are empty")
	}
	if err := fixture.store.CompleteOwned(
		fixture.ctx,
		fixture.owner,
		journal.Completion{
			RunID: fixture.owner.RunID, EffectID: fixture.cycle.DispatchEffect,
			Token: dispatchClaim.Token, State: journal.OperationalFailed,
			ErrorCode: "stale_authority",
			EventKind: "test_dispatch_failed",
			At:        fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.completeImplementationFailure(
		fixture.ctx,
		fixture.owner,
		fixture.outer.ID,
		fixture.outer.CurrentClaim,
		"stale_dispatch",
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := fixture.service.recoverImplementationClaims(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
	)
	if err != nil || !recovered {
		t.Fatalf("failed dispatch recovery = %t, %v", recovered, err)
	}
	for effectID, expected := range map[string]struct {
		state journal.EffectState
		code  string
	}{
		fixture.outer.ID: {
			state: journal.OperationalFailed,
			code:  "stale_dispatch",
		},
		fixture.cycle.DispatchEffect: {
			state: journal.OperationalFailed,
			code:  "stale_authority",
		},
		fixture.cycle.PreparedEffect: {
			state: journal.OperationalFailed,
			code:  "orphaned_dispatch",
		},
	} {
		effect, readErr := fixture.store.Effect(
			fixture.ctx,
			fixture.owner.RunID,
			effectID,
		)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if effect.State != expected.state || effect.ErrorCode != expected.code {
			t.Fatalf("terminal effect %s = %#v", effectID, effect)
		}
	}
	if head := runRuntimeGit(
		t,
		fixture.repository,
		"rev-parse",
		fixture.track.Ref,
	); head != fixture.cycle.TrackHead {
		t.Fatalf("track head = %s, want %s", head, fixture.cycle.TrackHead)
	}
	if body := runRuntimeGit(
		t,
		fixture.repository,
		"show",
		record.Candidate+":one.txt",
	); body != "orphaned production candidate" {
		t.Fatalf("orphaned candidate body = %q", body)
	}
	recovered, err = fixture.service.recoverImplementationClaims(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
	)
	if err != nil || recovered {
		t.Fatalf("terminal failed dispatch recovery = %t, %v", recovered, err)
	}
}

func TestProductionEmptyImplementationCandidateIsKnownFailure(
	t *testing.T,
) {
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		_, submissionBody := productionImplementationSubmission(
			t,
			invocation.Request.InvocationID,
		)
		sealBody, err := json.Marshal(driver.Seal{
			SchemaVersion:    driver.SealSchemaVersion,
			InvocationID:     invocation.Request.InvocationID,
			SubmissionDigest: driver.Digest(submissionBody),
			Accepted:         true,
			Code:             "accepted",
		})
		if err != nil {
			t.Fatal(err)
		}
		sealBody = append(sealBody, '\n')
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submissionBody,
				SubmissionDigest: driver.Digest(submissionBody),
				SealBytes:        sealBody,
				SealDigest:       driver.Digest(sealBody),
			},
		}, nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	_, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	)
	if err == nil || IsCode(err, "RECOVERY_UNCERTAIN") ||
		stableErrorCode(err) != "EMPTY_CANDIDATE" {
		t.Fatalf("empty production candidate = %v", err)
	}
	dispatch, readErr := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if dispatch.State != journal.OperationalFailed ||
		dispatch.ErrorCode != "EMPTY_CANDIDATE" {
		t.Fatalf("empty candidate dispatch = %#v", dispatch)
	}
	if _, readErr := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.PreparedEffect,
	); !journal.IsCode(readErr, "EFFECT_NOT_FOUND") {
		t.Fatalf("empty candidate prepared effect = %v", readErr)
	}
}

func TestProductionImplementationHandoffRecoversItsDurablePreparedCandidate(
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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 4, 5, 6, 0, time.UTC)
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "journal.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
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
	var invocations atomic.Int64
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		invocations.Add(1)
		if err := os.WriteFile(
			filepath.Join(invocation.HostWorkspace, "one.txt"),
			[]byte("durable production implementation\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		submission := driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocation.Request.InvocationID,
			Responsibility: driver.ImplementerImplementation,
			Summary:        "Durable production candidate.",
			Detail:         "Prepared before the handoff becomes succeeded.",
		}
		submission.Checks, _ = driver.NewCheckBytes(
			[]byte("production implementation checks\n"),
		)
		submissionBody, encodeErr := driver.EncodeSubmission(submission)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody, encodeErr := json.Marshal(driver.Seal{
			SchemaVersion:    driver.SealSchemaVersion,
			InvocationID:     submission.InvocationID,
			SubmissionDigest: driver.Digest(submissionBody),
			Accepted:         true,
			Code:             "accepted",
		})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		sealBody = append(sealBody, '\n')
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submissionBody,
				SubmissionDigest: driver.Digest(submissionBody),
				SealBytes:        sealBody,
				SealDigest:       driver.Digest(sealBody),
			},
		}, nil
	})
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	planBytes, _ := runtimePlan(
		t,
		manifest.value.Release,
		manifest.value.Authority.Project,
		manifest.value.TargetRef,
		"approval-release-1-v1",
	)
	if _, err := engine.actions.RecordPlanRevision(
		baton.RecordPlanRevisionInput{
			PlanBytes: planBytes,
			Summary:   "Install the exact production test plan.",
			Detail:    []byte("Production recovery fixture."),
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release,
		Slice:   "S1",
		Role:    "implementer",
		Result:  "designed",
		Summary: "Design the production fixture.",
		Detail:  []byte("Exact design."),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release,
		Slice:   "S1",
		Role:    "captain",
		Result:  "proceed",
		Summary: "Proceed with the production fixture.",
		Detail:  []byte("Exact review."),
	}); err != nil {
		t.Fatal(err)
	}
	state, err := baton.ReadState(
		engine.git,
		manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice("S1")
	track, trackOK := state.Track("T1")
	if !ok || !trackOK || slice.CurrentReceipt == nil ||
		slice.Stage != "implement" ||
		slice.NextRole != "implementer" {
		t.Fatalf("implementation authority = %#v", state)
	}
	before := sliceFingerprint(state, "S1")
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle := implementationCycle{GitIdentity: runtimeTestGitIdentity,
		Release: state.Release, Slice: "S1",
		Binds: slice.CurrentReceipt.OID, Before: before,
		Plan: state.Plan.OID, ReleaseHead: state.Refs.Release.Head,
		TargetHead: state.Refs.Target.Head, Track: track.ID,
		TrackRef: track.Ref, TrackHead: track.Head,
		DispatchWork: workIdentity(
			outerID,
			"driver.dispatch",
		),
		PreparedWork: workIdentity(
			outerID,
			"git.seal.prepared",
		),
	}
	cycle.DispatchEffect = journal.AttemptEffectID(
		cycle.DispatchWork,
		1,
		1,
	)
	cycle.PreparedEffect = journal.AttemptEffectID(
		cycle.PreparedWork,
		1,
		1,
	)
	outerPayload := mustJSON(cycle)
	if err := store.EnsureAttempt(
		ctx,
		journal.Command{
			RunID: owner.RunID, ReplayKey: outerID,
			Kind: "git.seal", Payload: outerPayload, CreatedAt: now,
		},
		journal.Effect{
			RunID: owner.RunID, ID: outerID, ReplayKey: outerID,
			Kind: "git.seal", BeforeDigest: outerWork,
			ExpectedDigest: sha256Digest(outerPayload), UpdatedAt: now,
		},
		journal.EffectAttempt{WorkID: outerWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	outerClaim, err := store.ClaimOwned(
		ctx,
		owner,
		outerID,
		now,
		effectLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	outer := journal.Effect{
		RunID: owner.RunID, ID: outerID, Kind: "git.seal",
		State: journal.Claimed, CurrentClaim: outerClaim.Token,
	}
	workspace, err := engine.workspaces.OpenTrack(
		gitx.TrackKey{Release: state.Release, Track: track.ID},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	record, preparedClaim, err :=
		service.runProductionImplementationDispatch(
			ctx,
			engine,
			owner,
			workspace,
			cycle,
			dispatchCoordinates{
				Slice:          "S1",
				Responsibility: driver.ImplementerImplementation,
				BatonAttempt:   slice.Attempt,
				Epoch:          1,
				Try:            1,
			},
		)
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := store.Effect(
		ctx,
		owner.RunID,
		cycle.DispatchEffect,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := store.Effect(
		ctx,
		owner.RunID,
		cycle.PreparedEffect,
	)
	if err != nil {
		t.Fatal(err)
	}
	if dispatch.State != journal.Succeeded ||
		prepared.State != journal.Claimed ||
		prepared.CurrentClaim != preparedClaim.Token ||
		runRuntimeGit(t, repository, "rev-parse", track.Ref) != cycle.TrackHead ||
		runRuntimeGit(t, repository, "show", record.Candidate+":one.txt") !=
			"durable production implementation" {
		t.Fatalf(
			"crash window: dispatch=%#v prepared=%#v record=%#v",
			dispatch,
			prepared,
			record,
		)
	}
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := driver.DecodeDriverConfig(config.CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	restartedProduction, err := newProductionDriverRuntime(
		reloaded,
		driver.DriverFactoryOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Service{
		journal: store, dispatcher: fixtureDriver(func(
			context.Context,
			driver.Invocation,
		) (driver.Observation, error) {
			t.Fatal("recovery invoked the production driver again")
			return driver.Observation{}, nil
		}),
		production: restartedProduction, gitExecutable: gitExecutable,
		now: func() time.Time { return now },
	}
	restartedEngine, err := restarted.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()
	recovered, retry, err := restarted.recoverImplementationCycle(
		ctx,
		restartedEngine,
		owner,
		cycle,
		outer,
	)
	if err != nil {
		t.Fatal(err)
	}
	if retry || !sealedRecordMatchesCycle(recovered, cycle) ||
		recovered.Candidate != record.Candidate ||
		invocations.Load() != 1 ||
		runRuntimeGit(t, repository, "rev-parse", track.Ref) != record.Candidate {
		t.Fatalf(
			"recovery: retry=%t invocations=%d recovered=%#v",
			retry,
			invocations.Load(),
			recovered,
		)
	}
	for _, effectID := range []string{outerID, cycle.PreparedEffect} {
		effect, effectErr := store.Effect(ctx, owner.RunID, effectID)
		if effectErr != nil {
			t.Fatal(effectErr)
		}
		if effect.State != journal.Succeeded {
			t.Fatalf("recovered effect %s = %#v", effectID, effect)
		}
	}
}

func TestProductionClaimedDispatchRecoveryRetainsUncertaintyUntilAuthorityChanges(
	t *testing.T,
) {
	t.Parallel()

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
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 3, 4, 5, 0, time.UTC)
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "journal.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
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
	service := &Service{
		journal: store, dispatcher: driver.Dispatcher{},
		production: production, gitExecutable: gitExecutable,
		now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, before := plannerProductionAuthority(t, engine)
	workspace, err := engine.workspaces.OpenSnapshot(target.Head)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	coordinates := dispatchCoordinates{
		Responsibility: driver.PlannerProposal,
		BatonAttempt:   1,
		Epoch:          1,
		Try:            1,
	}
	prepared, err := service.prepareDriverDispatch(
		ctx,
		engine,
		workspace,
		driver.RolePlanner,
		coordinates,
		before,
	)
	if err != nil {
		t.Fatal(err)
	}
	work := driverWorkIdentity(
		manifest.digest,
		"",
		driver.PlannerProposal,
		1,
		before,
	)
	effectID := journal.AttemptEffectID(work, 1, 1)
	if err := store.EnsureAttempt(
		ctx,
		journal.Command{
			RunID: manifest.value.RunID, ReplayKey: effectID,
			Kind: "driver.dispatch", Payload: prepared.commandPayload,
			CreatedAt: now,
		},
		journal.Effect{
			RunID: manifest.value.RunID, ID: effectID,
			ReplayKey: effectID, Kind: "driver.dispatch",
			BeforeDigest:   sha256Digest([]byte(before)),
			ExpectedDigest: productionOutputExpectation,
			UpdatedAt:      now,
		},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimOwned(
		ctx,
		owner,
		effectID,
		now,
		effectLease,
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	state, stateErr := baton.ReadState(
		engine.git,
		manifest.value.Release,
		engine.inertness,
	)
	if baton.ErrorCode(stateErr) != "REF_NOT_FOUND" {
		t.Fatalf("initial Baton state = %#v, %v", state, stateErr)
	}
	recovered, err := service.recoverStaleClaimedDispatchesFromSnapshot(
		ctx,
		engine,
		owner,
		snapshot,
		state,
		stateErr,
	)
	if err != nil {
		t.Fatalf("claimed recovery = %t, %v", recovered, err)
	}
	if recovered {
		t.Fatal("unexpired current-authority claim was classified as recovered")
	}
	effect, err := store.Effect(ctx, manifest.value.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.Claimed ||
		effect.CurrentClaim == "" ||
		effect.CurrentClaimExpiresAt.IsZero() ||
		effect.ErrorCode != "" {
		t.Fatalf("current-authority recovery = %#v", effect)
	}
	// The same ownerless claimed shape on the direct dispatch path must
	// refuse without writing: the claim is unexpired, so the dispatcher may
	// still be mid-flight.
	_, err = service.runDriverEffectWithPreparation(
		ctx,
		engine,
		workspace,
		driver.RolePlanner,
		coordinates,
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1},
		before,
		owner,
		nil,
		false,
	)
	if !IsCode(err, "RECOVERY_UNCERTAIN") {
		t.Fatalf("unexpired dispatch fallback = %v", err)
	}
	effect, err = store.Effect(ctx, manifest.value.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.Claimed ||
		effect.CurrentClaim == "" {
		t.Fatalf("dispatch fallback mutated unexpired claim = %#v", effect)
	}

	if err := os.WriteFile(
		filepath.Join(repository, "authority-changed.txt"),
		[]byte("changed\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repository, "add", "--", "authority-changed.txt")
	runRuntimeGit(
		t,
		repository,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit", "--quiet", "-m", "change target authority",
	)
	snapshot, err = store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	state, stateErr = baton.ReadState(
		engine.git,
		manifest.value.Release,
		engine.inertness,
	)
	recovered, err = service.recoverStaleClaimedDispatchesFromSnapshot(
		ctx,
		engine,
		owner,
		snapshot,
		state,
		stateErr,
	)
	if err != nil || !recovered {
		t.Fatalf("stale recovery = %t, %v", recovered, err)
	}
	effect, err = store.Effect(ctx, manifest.value.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.OperationalFailed ||
		effect.ErrorCode != "stale_authority" {
		t.Fatalf("stale-authority recovery = %#v", effect)
	}
}
