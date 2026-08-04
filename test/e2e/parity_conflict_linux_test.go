//go:build linux

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

func conflictParityPlan(
	t *testing.T,
	repository string,
) ([]byte, baton.Plan) {
	t.Helper()
	slice := func(id, pathValue string) baton.Slice {
		return baton.Slice{
			ID: id, Outcome: "Deliver composition fixture " + id + ".",
			Scope: baton.Scope{
				Include: []string{pathValue},
				Exclude: []string{},
			},
			Acceptance: []baton.Criterion{{
				ID: "A-" + id, Text: id + " has its exact product.",
			}},
			Checks: []string{"check " + id},
			Constraints: []string{
				"deterministic conflict fixture",
			},
			DependsOn: []string{},
			Consumes:  []string{},
		}
	}
	producer := slice("S1", "shared.txt")
	priorConsumer := slice("S2", "shared.txt")
	consumer := slice("S3", "consumer.txt")
	consumer.Consumes = []string{"S1"}
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       "parity-conflict-release",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://parity-conflict-release/1",
		Tracks: []baton.Track{
			{
				ID: "T1", DependsOn: []string{},
				Slices: []baton.Slice{producer},
			},
			{
				ID: "T2", DependsOn: []string{"T1"},
				Slices: []baton.Slice{priorConsumer, consumer},
			},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nReal-binary derived-base conflict for " +
			repository + ".\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func conflictParityManifest(
	t *testing.T,
	repository string,
	fakeBinary string,
	fakeDigest string,
) ([]byte, baton.Plan) {
	t.Helper()
	const runID = "parity-conflict"
	planBytes, plan := conflictParityPlan(t, repository)
	var scripts []swornruntime.ScriptedAttempt
	for try := int64(1); try <= 3; try++ {
		scripts = append(scripts, swornruntime.ScriptedAttempt{
			Responsibility: driver.PlannerProposal,
			BatonAttempt:   1,
			Epoch:          1,
			Try:            try,
			Behavior:       "submit",
			Submission: exactPlannerSubmission(
				t,
				runID,
				1,
				try,
				planBytes,
			),
		})
		for _, sliceID := range []string{"S1", "S2", "S3"} {
			for _, responsibility := range []driver.Responsibility{
				driver.ImplementerDesign,
				driver.CaptainReview,
				driver.ImplementerImplementation,
				driver.WorkVerification,
			} {
				scripts = append(scripts, swornruntime.ScriptedAttempt{
					Slice:          sliceID,
					Responsibility: responsibility,
					BatonAttempt:   1,
					Epoch:          1,
					Try:            try,
					Behavior:       "submit",
					Submission: scriptedSubmission(
						t,
						runID,
						sliceID,
						responsibility,
						1,
						1,
						try,
					),
				})
			}
		}
		scripts = append(scripts, swornruntime.ScriptedAttempt{
			Responsibility: driver.AssemblyVerification,
			BatonAttempt:   1,
			Epoch:          1,
			Try:            try,
			Behavior:       "submit",
			Submission: scriptedSubmission(
				t,
				runID,
				"",
				driver.AssemblyVerification,
				1,
				1,
				try,
			),
		})
	}
	sort.Slice(scripts, func(left, right int) bool {
		leftKey := fmt.Sprintf(
			"%s/%s/%020d/%020d/%d",
			scripts[left].Responsibility,
			scripts[left].Slice,
			scripts[left].BatonAttempt,
			scripts[left].Epoch,
			scripts[left].Try,
		)
		rightKey := fmt.Sprintf(
			"%s/%s/%020d/%020d/%d",
			scripts[right].Responsibility,
			scripts[right].Slice,
			scripts[right].BatonAttempt,
			scripts[right].Epoch,
			scripts[right].Try,
		)
		return leftKey < rightKey
	})
	manifest := swornruntime.Manifest{
		SchemaVersion:     swornruntime.ManifestVersion,
		RunID:             runID,
		Repository:        repository,
		Release:           "parity-conflict-release",
		TargetRef:         "refs/heads/main",
		Intent:            "Prove a genuine derived-base composition conflict.",
		MaxParallelTracks: 2,
		Authority: swornruntime.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		Driver: &swornruntime.FakeDriverConfig{
			Executable: fakeBinary,
			Digest:     fakeDigest,
			AdapterKey: "parity-conflict-fake",
			Profile:    "parity-conflict-fake",
		},
		Roles: driver.RoleSelections{
			Planner: driver.RoleSelection{
				Profile: "parity-conflict-fake",
				Model:   "planner-model",
			},
			Implementer: driver.RoleSelection{
				Profile: "parity-conflict-fake",
				Model:   "composition-conflict",
			},
			Captain: driver.RoleSelection{
				Profile: "parity-conflict-fake",
				Model:   "captain-model",
			},
			Verifier: driver.RoleSelection{
				Profile: "parity-conflict-fake",
				Model:   "verifier-model",
			},
		},
		Automation: &swornruntime.AutomationSelections{
			Recovery: driver.RoleSelection{Profile: "parity-conflict-fake", Model: "recovery-model"},
		},
		Limits: driver.Limits{
			TimeoutMillis: 30_000,
			OutputBytes:   65_536,
		},
		Scripts: scripts,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := swornruntime.ParseManifest(body); err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func TestRealBinaryCompositionConflictParksWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fakeBinary := filepath.Join(root, "fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(root, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")
	repository := newProductRepository(t)
	manifestBody, plan := conflictParityManifest(
		t,
		repository,
		fakeBinary,
		fakeDigest,
	)
	manifestPath := writeManifest(t, root, manifestBody)
	journalPath := filepath.Join(root, "run.sqlite")
	targetBefore := runGit(t, repository, "rev-parse", "main")
	stdout, stderr := runBinary(
		t,
		swornBinary,
		0,
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("conflict start stdout=%q stderr=%q", stdout, stderr)
	}
	authorizePlan(t, journalPath, "parity-conflict", plan)
	stdout, stderr = runBinary(
		t,
		swornBinary,
		0,
		"resume",
		"--run", "parity-conflict",
		"--journal", journalPath,
		"--command", "resume-1",
		"--generation", "0",
	)
	for generation := int64(1); stderr == "" &&
		strings.Contains(stdout, "  state: running") &&
		generation <= 5; generation++ {
		stdout, stderr = runBinary(
			t,
			swornBinary,
			0,
			"resume",
			"--run", "parity-conflict",
			"--journal", journalPath,
			"--command", fmt.Sprintf("resume-%d", generation+1),
			"--generation", fmt.Sprintf("%d", generation),
		)
	}
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		state := readBatonState(
			t,
			repository,
			"parity-conflict-release",
		)
		var slices []string
		for _, slice := range state.Slices {
			slices = append(
				slices,
				fmt.Sprintf(
					"%s:a%d:%s:%s:%s",
					slice.Location.Slice.ID,
					slice.Attempt,
					slice.Stage,
					slice.Status,
					slice.NextRole,
				),
			)
		}
		store, _ := journal.OpenReadOnly(
			context.Background(),
			journalPath,
		)
		snapshot, _ := store.Snapshot(
			context.Background(),
			"parity-conflict",
		)
		_ = store.Close()
		var effects []string
		for _, effect := range snapshot.Effects {
			effects = append(
				effects,
				effect.Kind+":"+string(effect.State)+":"+
					effect.ErrorCode,
			)
		}
		t.Fatalf(
			"conflict resume stdout=%q stderr=%q slices=%v assembly=%#v effects=%v",
			stdout,
			stderr,
			slices,
			state.Assembly,
			effects,
		)
	}

	state := readBatonState(t, repository, "parity-conflict-release")
	s1, s1OK := state.Slice("S1")
	s2, s2OK := state.Slice("S2")
	s3, s3OK := state.Slice("S3")
	if !s1OK || !s2OK || !s3OK ||
		s1.Pass == nil || s2.Pass == nil || s3.Pass != nil ||
		s3.Stage != "design" || s3.Status != "ready" ||
		s3.NextRole != "implementer" || s3.PreparedBase != "" ||
		state.Assembly.Candidate != nil ||
		state.Assembly.Pass != nil ||
		state.Assembly.ResultCommit != "" ||
		runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf(
			"conflict state s1=%#v s2=%#v s3=%#v assembly=%#v",
			s1,
			s2,
			s3,
			state.Assembly,
		)
	}

	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), "parity-conflict")
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	var conflicts []journal.Effect
	var persisted struct {
		Request struct {
			ReleaseHead    string `json:"release_head"`
			TargetHead     string `json:"target_head"`
			ConsumerTrack  string `json:"consumer_track"`
			ConsumerSlice  string `json:"consumer_slice"`
			ConsumerBefore string `json:"consumer_before"`
		} `json:"request"`
	}
	for _, effect := range snapshot.Effects {
		switch effect.Kind {
		case "git.prepare_track_base":
			var command struct {
				Request struct {
					ConsumerSlice string `json:"consumer_slice"`
				} `json:"request"`
			}
			if err := json.Unmarshal(
				commands[effect.ReplayKey].Payload,
				&command,
			); err != nil {
				t.Fatal(err)
			}
			if command.Request.ConsumerSlice != "S3" {
				if effect.State != journal.Succeeded {
					t.Fatalf("prior track-base effect = %#v", effect)
				}
				continue
			}
			if effect.State != journal.OperationalFailed ||
				effect.ErrorCode != "MERGE_CONFLICT" {
				t.Fatalf("track-base conflict effect = %#v", effect)
			}
			conflicts = append(conflicts, effect)
			if err := json.Unmarshal(
				commands[effect.ReplayKey].Payload,
				&persisted,
			); err != nil {
				t.Fatal(err)
			}
		case "driver.dispatch":
			if !strings.HasSuffix(effect.ID, "/t1") {
				t.Fatalf("driver retried during conflict: %#v", effect)
			}
			if effect.State == journal.Succeeded {
				submission, err := driver.DecodeSubmission(effect.Result)
				if err != nil {
					t.Fatal(err)
				}
				if submission.InvocationID != "" &&
					(strings.Contains(submission.InvocationID, "/S3/") ||
						submission.Responsibility ==
							driver.AssemblyVerification) {
					t.Fatalf(
						"conflict dispatched forbidden model work: %#v",
						submission,
					)
				}
			}
		case "baton.prepare_assembly", "baton.assembly_verdict",
			"baton.merge":
			t.Fatalf("conflict reached %s: %#v", effect.Kind, effect)
		}
	}
	track, trackOK := state.Track("T2")
	if len(conflicts) != 3 || !trackOK ||
		persisted.Request.ConsumerTrack != "T2" ||
		persisted.Request.ConsumerSlice != "S3" ||
		state.Refs.Release.Head != persisted.Request.ReleaseHead ||
		state.Refs.Target.Head != persisted.Request.TargetHead ||
		track.Head != persisted.Request.ConsumerBefore {
		t.Fatalf(
			"conflicts=%d request=%#v track=%#v refs=%#v",
			len(conflicts),
			persisted.Request,
			track,
			state.Refs,
		)
	}
	t.Logf(
		"composition conflict evidence: path=shared.txt attempts=%d "+
			"release=%s target=%s consumer=%s",
		len(conflicts),
		state.Refs.Release.Head,
		state.Refs.Target.Head,
		track.Head,
	)
}
