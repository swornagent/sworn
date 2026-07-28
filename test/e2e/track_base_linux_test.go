//go:build linux

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

func consumingE2EManifest(
	t *testing.T,
	runID, repository, release string,
	issue int64,
	marker, fakeExecutable, fakeDigest string,
) ([]byte, []byte, baton.Plan) {
	t.Helper()
	manifestBody, _, original := e2eManifest(
		t,
		runID,
		repository,
		release,
		issue,
		marker,
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
	approvals *approvalServer,
	runID, release string,
	issue int64,
	marker string,
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
		issue,
		marker,
		fakeBinary,
		fakeDigest,
	)
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
	if !strings.Contains(stdout, "state awaiting_approval") {
		t.Fatalf("planner output = %q", stdout)
	}
	approvals.publish(issue, approvalFor(issue, marker, plan))
	installAndPassComponent(t, repository, release, planBytes)
	return repository, journalPath, manifestPath
}

func TestRealBinaryConsumedBasePreparationAndRecovery(t *testing.T) {
	approvals := &approvalServer{comments: make(map[int64][]approvalComment)}
	server := httptest.NewServer(http.HandlerFunc(approvals.serve))
	defer server.Close()
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	baseLDFlags := "-X=github.com/swornagent/sworn/internal/runtime.githubAPIBase=" +
		server.URL
	normalBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, normalBinary, "./cmd/sworn", baseLDFlags)

	t.Run("exact-design-captain-candidate-verifier-chain", func(t *testing.T) {
		const (
			runID   = "e2e-consumed-base"
			release = "e2e-consumed-base-release"
			issue   = int64(51)
			marker  = "approval-e2e-consumed-base-v1"
		)
		repository, journalPath, _ := prepareConsumedBaseRun(
			t,
			normalBinary,
			fakeBinary,
			fakeDigest,
			approvals,
			runID,
			release,
			issue,
			marker,
		)
		stdout, stderr := runBinary(
			t,
			normalBinary,
			0,
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
		if stderr != "" || !strings.Contains(stdout, "state complete") {
			t.Fatalf("resume stdout=%q stderr=%q", stdout, stderr)
		}
		assertConsumedBaseRun(
			t,
			repository,
			journalPath,
			runID,
			release,
			2,
			1,
		)
	})

	for _, crash := range []struct {
		name       string
		cut        string
		wantRefSet bool
		issue      int64
	}{
		{
			name:  "crash-before-ref-update",
			cut:   "testCrashBeforeEffect",
			issue: 52,
		},
		{
			name:       "crash-after-ref-update",
			cut:        "testCrashAfterEffect",
			wantRefSet: true,
			issue:      53,
		},
	} {
		crash := crash
		t.Run(crash.name, func(t *testing.T) {
			runID := "e2e-consumed-" + crash.name
			release := runID + "-release"
			marker := "approval-" + runID + "-v1"
			crashBinary := filepath.Join(buildRoot, "sworn-"+crash.name)
			buildBinary(
				t,
				crashBinary,
				"./cmd/sworn",
				fmt.Sprintf(
					"%s -X=github.com/swornagent/sworn/internal/runtime.%s=git.prepare_track_base -X=github.com/swornagent/sworn/internal/runtime.testOwnerLeaseMillis=1500",
					baseLDFlags,
					crash.cut,
				),
			)
			repository, journalPath, _ := prepareConsumedBaseRun(
				t,
				crashBinary,
				fakeBinary,
				fakeDigest,
				approvals,
				runID,
				release,
				crash.issue,
				marker,
			)
			runBinary(
				t,
				crashBinary,
				86,
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
			time.Sleep(1800 * time.Millisecond)
			stdout, _ := runBinary(
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
			if !strings.Contains(stdout, "state complete") {
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
}
