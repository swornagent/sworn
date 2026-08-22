//go:build linux

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

func consumingE2EManifest(
	t *testing.T,
	runID, repository, release string,
	fakeExecutable, fakeDigest string,
) ([]byte, []byte, baton.Plan) {
	t.Helper()
	manifestBody, _, original := e2eManifest(
		t,
		runID,
		repository,
		release,
		fakeExecutable,
		fakeDigest,
		"verifier-model",
	)
	metadata := original.Metadata()
	metadata.Tracks[0].DependsOn = []string{"T2"}
	metadata.Tracks[0].Slices[0].DependsOn = []string{"S2"}
	metadata.Tracks[0].Slices[0].Consumes = []string{"S2"}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nReal-binary consumed-base E2E plan.\n",
	)
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	var manifest swornruntime.Manifest
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Scripts {
		script := &manifest.Scripts[index]
		if script.Responsibility != driver.PlannerProposal {
			continue
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(script.Submission)
		if err != nil {
			t.Fatal(err)
		}
		submission, err := driver.DecodeSubmission(raw)
		if err != nil {
			t.Fatal(err)
		}
		submission.Plan, err = driver.NewPlanBytes(planBytes)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := driver.EncodeSubmission(submission)
		if err != nil {
			t.Fatal(err)
		}
		script.Submission = base64.StdEncoding.EncodeToString(encoded)
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := swornruntime.ParseManifest(body); err != nil {
		t.Fatal(err)
	}
	return body, planBytes, plan
}

func assertConsumedBaseRun(
	t *testing.T,
	repositoryPath, journalPath, runID, release string,
	wantEffects, wantNoops int,
) {
	t.Helper()
	state := readBatonState(t, repositoryPath, release)
	consumer, ok := state.Slice("S1")
	if !ok || consumer.Pass == nil || consumer.Outcome != "pass" {
		t.Fatalf("consumer did not PASS: %#v", consumer)
	}
	producer, ok := state.Slice("S2")
	if !ok || producer.Pass == nil ||
		producer.Pass.Receipt.ProductTree == nil {
		t.Fatalf("producer PASS is absent: %#v", producer)
	}
	var design, candidate *baton.ReceiptEntry
	for index := range consumer.History.Entries {
		entry := &consumer.History.Entries[index]
		switch {
		case entry.Receipt.Role == "implementer" &&
			entry.Receipt.Result == "designed":
			design = entry
		case entry.Receipt.Role == "implementer" &&
			entry.Receipt.Result == "candidate":
			candidate = entry
		}
	}
	pin := *producer.Pass.Receipt.ProductTree
	if design == nil || design.Receipt.Base == nil ||
		design.Receipt.Inputs == nil ||
		design.Receipt.Inputs["S2"] != pin ||
		design.Parent == *design.Receipt.Base {
		t.Fatalf("strict design marker = %#v", design)
	}
	if candidate == nil || candidate.Receipt.Base == nil ||
		candidate.Receipt.Candidate == nil ||
		candidate.Receipt.Inputs["S2"] != pin {
		t.Fatalf("strict candidate marker = %#v", candidate)
	}
	repository, err := gitx.Open(repositoryPath, e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	format := repository.ObjectFormat()
	candidateOID, err := gitx.ParseOID(
		format,
		*candidate.Receipt.Candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		*producer.Pass.Receipt.Candidate,
		producer.Pass.Receipt.Binds,
		producer.Pass.OID,
	} {
		ancestor, err := gitx.ParseOID(format, value)
		if err != nil {
			t.Fatal(err)
		}
		contained, err := repository.IsAncestor(ancestor, candidateOID)
		if err != nil || !contained {
			t.Fatalf("producer authority %s absent from candidate: %v", value, err)
		}
	}
	store, err := journal.OpenReadOnly(
		context.Background(),
		journalPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	effects := 0
	creates := 0
	noops := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind != "git.prepare_track_base" {
			continue
		}
		effects++
		if effect.State != journal.Succeeded {
			t.Fatalf("track-base effect is not succeeded: %#v", effect)
		}
		var result struct {
			Action  string `json:"action"`
			Changed bool   `json:"changed"`
		}
		if json.Unmarshal(effect.Result, &result) != nil {
			t.Fatalf("track-base result is invalid: %q", effect.Result)
		}
		switch {
		case result.Action == "create" && result.Changed:
			creates++
		case result.Action == "noop" && !result.Changed:
			noops++
		default:
			t.Fatalf("unexpected track-base result: %#v", result)
		}
	}
	if effects != wantEffects || creates != 1 || noops != wantNoops {
		t.Fatalf(
			"track-base effects=%d create=%d noop=%d",
			effects,
			creates,
			noops,
		)
	}
}

func prepareConsumedBaseRun(
	t *testing.T,
	binary, fakeBinary, fakeDigest string,
	runID, release string,
) (string, string, string) {
	t.Helper()
	repository := newProductRepository(t)
	runRoot := t.TempDir()
	journalPath := filepath.Join(runRoot, "run.sqlite")
	manifestBody, planBytes, plan := consumingE2EManifest(
		t,
		runID,
		repository,
		release,

		fakeBinary,
		fakeDigest)

	manifestPath := writeManifest(t, runRoot, manifestBody)
	stdout, _ := runBinary(
		t,
		binary,
		0,
		"run",
		"--manifest",
		manifestPath,
		"--journal",
		journalPath,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("planner output = %q", stdout)
	}
	authorizePlan(t, journalPath, runID, plan)
	installAndPassComponent(t, repository, release, planBytes)
	return repository, journalPath, manifestPath
}

func runRealBinaryConsumedBasePreparationAndRecovery(t *testing.T) {
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	normalBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, normalBinary, "./cmd/sworn", "")

	for _, crash := range []struct {
		name       string
		cut        string
		wantRefSet bool
	}{
		{
			name: "crash-before-ref-update",
			cut:  "testCrashBeforeEffect",
		},
	} {
		crash := crash
		t.Run(crash.name, func(t *testing.T) {
			runID := "e2e-consumed-" + crash.name
			release := runID + "-release"
			crashBinary := filepath.Join(buildRoot, "sworn-"+crash.name)
			buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
			crashEnvironment := map[string]string{
				crashHookEnvironmentName(t, crash.cut): "git.prepare_track_base",
				"SWORN_TEST_OWNER_LEASE_MILLIS":        testLeaseMillis,
			}
			repository, journalPath, manifestPath := prepareConsumedBaseRun(
				t,
				normalBinary,
				fakeBinary,
				fakeDigest,
				runID,
				release,
			)
			runBinaryWithEnvironment(
				t,
				crashBinary,
				86,
				crashEnvironment,
				"resume",
				"--run",
				runID,
				"--journal",
				journalPath,
				"--command",
				"resume-1",
				"--generation",
				"0",
			)
			ref := "refs/heads/track/" + release + "/T1"
			command := exec.Command(
				e2eGit,
				"-C",
				repository,
				"show-ref",
				"--verify",
				"--quiet",
				ref,
			)
			refSet := command.Run() == nil
			if refSet != crash.wantRefSet {
				t.Fatalf(
					"consumer ref set=%t, want %t at crash cut",
					refSet,
					crash.wantRefSet,
				)
			}
			leaseExpiryWait()
			runBinary(
				t,
				normalBinary,
				0,
				"takeover",
				"--run",
				runID,
				"--journal",
				journalPath,
				"--command",
				"takeover-1",
				"--generation",
				"1",
			)
			stdout, _ := runBinary(
				t,
				normalBinary,
				0,
				"run",
				"--manifest",
				manifestPath,
				"--journal",
				journalPath,
			)
			if !strings.Contains(stdout, "  state: complete") {
				t.Fatalf("takeover output = %q", stdout)
			}
			assertConsumedBaseRun(
				t,
				repository,
				journalPath,
				runID,
				release,
				3,
				2,
			)
		})
	}

	t.Run(
		"crash_after_preparation_forward_target_move_recovers_without_replan",
		func(t *testing.T) {
			const (
				runID   = "e2e-consumed-stale-prepared"
				release = runID + "-release"
			)
			crashBinary := filepath.Join(
				buildRoot,
				"sworn-crash-after-stale-prepared-base",
			)
			buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
			crashEnvironment := map[string]string{
				"SWORN_TEST_CRASH_AFTER_EFFECT": "git.prepare_track_base",
				"SWORN_TEST_OWNER_LEASE_MILLIS": testLeaseMillis,
			}
			repository := newProductRepository(t)
			runRoot := t.TempDir()
			journalPath := filepath.Join(runRoot, "run.sqlite")
			manifestBody, initialBytes, initialPlan := consumingE2EManifest(
				t,
				runID,
				repository,
				release,

				fakeBinary,
				fakeDigest)

			manifestPath := writeManifest(
				t,
				runRoot,
				manifestBody,
			)
			stdout, _ := runBinaryWithEnvironment(
				t,
				crashBinary,
				0,
				crashEnvironment,
				"run",
				"--manifest",
				manifestPath,
				"--journal",
				journalPath,
			)
			if !strings.Contains(stdout, "  state: awaiting_approval") {
				t.Fatalf("planner output = %q", stdout)
			}
			authorizePlan(t, journalPath, runID, initialPlan)
			installAndPassComponent(
				t,
				repository,
				release,
				initialBytes,
			)
			runBinaryWithEnvironment(
				t,
				crashBinary,
				86,
				crashEnvironment,
				"resume",
				"--run",
				runID,
				"--journal",
				journalPath,
				"--command",
				"resume-1",
				"--generation",
				"0",
			)
			consumerRef := "refs/heads/track/" + release + "/T1"
			stalePrepared := runGit(
				t,
				repository,
				"rev-parse",
				consumerRef,
			)
			beforeMove := readBatonState(t, repository, release)
			consumer, ok := beforeMove.Slice("S1")
			if !ok ||
				consumer.Stage != "design" ||
				consumer.Status != "ready" ||
				consumer.NextRole != "implementer" ||
				consumer.Candidate != nil ||
				consumer.PreparedBase != stalePrepared {
				t.Fatalf(
					"crashed prepared-base state = %#v",
					consumer,
				)
			}

			if err := os.WriteFile(
				filepath.Join(repository, "target-moved.txt"),
				[]byte("external target movement\n"),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			runGit(t, repository, "add", "--", "target-moved.txt")
			runGit(
				t,
				repository,
				"commit",
				"--quiet",
				"-m",
				"external target movement",
			)
			leaseExpiryWait()
			runBinary(
				t,
				normalBinary,
				0,
				"takeover",
				"--run",
				runID,
				"--journal",
				journalPath,
				"--command",
				"takeover-1",
				"--generation",
				"1",
			)
			stdout, _ = runBinary(
				t,
				normalBinary,
				0,
				"run",
				"--manifest",
				manifestPath,
				"--journal",
				journalPath,
			)
			if !strings.Contains(stdout, "  state: complete") {
				t.Fatalf("forward-target recovery = %q", stdout)
			}
			after := readBatonState(t, repository, release)
			consumer, ok = after.Slice("S1")
			if !ok ||
				after.Plan.Metadata.Revision != 1 ||
				len(after.Plan.History) != 1 ||
				after.Plan.TargetStale ||
				consumer.Pass == nil ||
				consumer.Outcome != "pass" ||
				runGit(
					t,
					repository,
					"rev-parse",
					consumerRef,
				) == stalePrepared {
				t.Fatalf(
					"forward-target prepared-base state: plan=%#v consumer=%#v",
					after.Plan,
					consumer,
				)
			}

			store, err := journal.OpenReadOnly(
				context.Background(),
				journalPath,
			)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := store.Snapshot(
				context.Background(),
				runID,
			)
			_ = store.Close()
			if err != nil {
				t.Fatal(err)
			}
			staleEffects, succeededEffects := 0, 0
			for _, effect := range snapshot.Effects {
				if effect.ErrorCode == "CHANGED_OWNER_HEAD" {
					t.Fatalf(
						"exact prepared base reached changed-owner failure: %#v",
						effect,
					)
				}
				if effect.Kind != "git.prepare_track_base" {
					continue
				}
				switch {
				case effect.State == journal.OperationalFailed &&
					effect.ErrorCode == "stale_authority":
					staleEffects++
				case effect.State == journal.Succeeded:
					succeededEffects++
				case effect.State == journal.Claimed ||
					effect.State == journal.Uncertain:
					t.Fatalf(
						"track-base effect remained nonterminal: %#v",
						effect,
					)
				}
			}
			if staleEffects != 1 || succeededEffects < 1 {
				t.Fatalf(
					"track-base recovery effects stale=%d succeeded=%d",
					staleEffects,
					succeededEffects,
				)
			}
		},
	)
}
