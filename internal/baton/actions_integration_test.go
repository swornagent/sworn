package baton

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

const actionTestGit = "/usr/bin/git"

type actionGoldenFile struct {
	Flows []struct {
		ObjectFormat string `json:"object_format"`
		Results      []struct {
			Action        string `json:"action"`
			Changed       bool   `json:"changed"`
			Plan          string `json:"plan"`
			Head          string `json:"head"`
			Target        string `json:"target"`
			Candidate     string `json:"candidate"`
			ResultCommit  string `json:"result_commit"`
			ReceiptCommit string `json:"receipt_commit"`
		} `json:"results"`
	} `json:"flows"`
}

type stateGoldenFile struct {
	Flows []struct {
		ObjectFormat string `json:"object_format"`
		States       []struct {
			Label string          `json:"label"`
			State json.RawMessage `json:"state"`
		} `json:"states"`
	} `json:"flows"`
}

type gitGoldenFile struct {
	Flows []struct {
		ObjectFormat   string `json:"object_format"`
		Target         string `json:"target"`
		ReleaseHead    string `json:"release_head"`
		TargetTree     string `json:"target_tree"`
		ProductTree    string `json:"product_tree"`
		ProductEntries []struct {
			Path   string `json:"path"`
			Mode   string `json:"mode"`
			Type   string `json:"type"`
			Object string `json:"object"`
		} `json:"product_entries"`
		FirstParentHistory []struct {
			OID           string   `json:"oid"`
			Parents       []string `json:"parents"`
			Tree          string   `json:"tree"`
			MessageSHA256 string   `json:"message_sha256"`
		} `json:"first_parent_history"`
	} `json:"flows"`
}

func loadActionGolden(t *testing.T) actionGoldenFile {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value actionGoldenFile
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func loadStateGolden(t *testing.T) stateGoldenFile {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value stateGoldenFile
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func loadGitGolden(t *testing.T) gitGoldenFile {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "git.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value gitGoldenFile
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func nullableActionString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func projectActionState(state State) map[string]any {
	slices := make([]any, 0, len(state.Slices))
	for _, slice := range state.Slices {
		slices = append(slices, map[string]any{
			"id":              slice.Location.Slice.ID,
			"track":           slice.Location.Track.ID,
			"stage":           slice.Stage,
			"status":          slice.Status,
			"next_role":       slice.NextRole,
			"outcome":         slice.Outcome,
			"attempt":         slice.Attempt,
			"maximum_attempt": slice.History.MaximumAttempt,
			"input_pins":      slice.InputPins,
			"current_receipt": func() any {
				if slice.CurrentReceipt == nil {
					return nil
				}
				return slice.CurrentReceipt.OID
			}(),
			"candidate": func() any {
				if slice.Candidate == nil || slice.Candidate.Receipt.Candidate == nil {
					return nil
				}
				return *slice.Candidate.Receipt.Candidate
			}(),
			"pass": func() any {
				if slice.Pass == nil {
					return nil
				}
				return slice.Pass.OID
			}(),
			"retained":     slice.Retained,
			"stale_reason": nullableActionString(slice.StaleReason),
		})
	}
	trackRefs := make([]any, 0, len(state.Refs.Tracks))
	for _, track := range state.Refs.Tracks {
		trackRefs = append(trackRefs, map[string]any{
			"id": track.ID, "ref": track.Ref, "head": nullableActionString(track.Head),
		})
	}
	diagnostics := make([]any, 0, len(state.Diagnostics))
	for _, diagnostic := range state.Diagnostics {
		diagnostics = append(diagnostics, map[string]any{
			"code": diagnostic.Code, "release": nullableActionString(diagnostic.Release),
			"track":   nullableActionString(diagnostic.Track),
			"work":    nullableActionString(diagnostic.Work),
			"message": diagnostic.Message,
		})
	}
	return map[string]any{
		"release": state.Release,
		"plan": map[string]any{
			"oid": state.Plan.OID, "digest": state.Plan.Digest,
			"revision":     state.Plan.Metadata.Revision,
			"approval_oid": state.Plan.ApprovalOID,
			"target_stale": state.Plan.TargetStale,
			"contracts":    state.Plan.Metadata.Contracts,
		},
		"refs": map[string]any{
			"release": map[string]any{
				"ref": state.Refs.Release.Ref, "head": nullableActionString(state.Refs.Release.Head),
			},
			"target": map[string]any{
				"ref": state.Refs.Target.Ref, "head": nullableActionString(state.Refs.Target.Head),
			},
			"tracks": trackRefs,
		},
		"slices": slices,
		"assembly": map[string]any{
			"stage": state.Assembly.Stage, "status": state.Assembly.Status,
			"next_role": state.Assembly.NextRole, "outcome": state.Assembly.Outcome,
			"input_pins": state.Assembly.InputPins,
			"current_receipt": func() any {
				if state.Assembly.CurrentReceipt == nil {
					return nil
				}
				return state.Assembly.CurrentReceipt.OID
			}(),
			"candidate": func() any {
				if state.Assembly.Candidate == nil || state.Assembly.Candidate.Receipt.Candidate == nil {
					return nil
				}
				return *state.Assembly.Candidate.Receipt.Candidate
			}(),
			"pass": func() any {
				if state.Assembly.Pass == nil {
					return nil
				}
				return state.Assembly.Pass.OID
			}(),
			"stale_reason":  nullableActionString(state.Assembly.StaleReason),
			"result_commit": nullableActionString(state.Assembly.ResultCommit),
		},
		"diagnostics": diagnostics,
	}
}

func actionEnvironment(repo string, extra ...string) []string {
	home := filepath.Join(repo, ".golden-home")
	base := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"LANG=C", "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Golden Author",
		"GIT_AUTHOR_EMAIL=golden@baton.invalid",
		"GIT_AUTHOR_DATE=1000000000 +0000",
		"GIT_COMMITTER_NAME=Golden Author",
		"GIT_COMMITTER_EMAIL=golden@baton.invalid",
		"GIT_COMMITTER_DATE=1000000000 +0000",
	}
	return append(base, extra...)
}

func actionGit(t *testing.T, repo string, input []byte, extraEnv []string, args ...string) string {
	t.Helper()
	command := exec.Command(actionTestGit, append([]string{"-C", repo}, args...)...)
	command.Env = actionEnvironment(repo, extraEnv...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func createActionRepository(t *testing.T, format string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".golden-home"), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(actionTestGit, "init", "--quiet", "--object-format="+format, repo)
	command.Env = actionEnvironment(repo)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actionGit(t, repo, nil, nil, "add", "--", "base.txt")
	actionGit(t, repo, nil, nil, "commit", "--quiet", "-m", "base")
	actionGit(t, repo, nil, nil, "branch", "-M", "main")
	return repo
}

func actionPlanBytes(release string) []byte {
	return actionPlanRevisionBytes(release, 1, nil, []Track{
		{
			ID: "T1", DependsOn: []string{},
			Slices: []Slice{actionSlice("S1", "one.txt")},
		},
		{
			ID: "T2", DependsOn: []string{},
			Slices: []Slice{actionSlice("S2", "two.txt")},
		},
	})
}

func actionPlanRevisionBytes(release string, revision int64, previous *string, tracks []Track) []byte {
	metadata := Metadata{
		SchemaVersion: PlanVersion,
		Release:       release,
		Revision:      revision,
		PreviousPlan:  previous,
		Repository:    "golden/sworn",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   fmt.Sprintf("golden://approval/%s/%d", release, revision),
		Tracks:        tracks,
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		panic(err)
	}
	return []byte(planOpen + string(body) + planClose + "\nGolden plan.\n")
}

func appendActionReceipt(t *testing.T, actions *Actions, input AppendReceiptInput) ActionResult {
	t.Helper()
	result, err := actions.AppendReceipt(input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func appendRawActionReceipt(
	t *testing.T,
	actions *Actions,
	repoPath, ownerRef, ownerBefore, parent string,
	receipt Receipt,
	detail []byte,
) ReceiptEntry {
	t.Helper()
	subject := fmt.Sprintf(
		"baton(%s/%s): %s %s",
		receipt.Release,
		receipt.SliceID(),
		receipt.Role,
		receipt.Result,
	)
	message, err := RenderReceiptCommit(subject, detail, receipt)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := actions.repository.prepareMetadata(parent, message)
	if err != nil {
		t.Fatal(err)
	}
	if ownerBefore == "" {
		actionGit(
			t,
			repoPath,
			nil,
			nil,
			"update-ref",
			ownerRef,
			prepared.Commit,
		)
	} else {
		actionGit(
			t,
			repoPath,
			nil,
			nil,
			"update-ref",
			ownerRef,
			prepared.Commit,
			ownerBefore,
		)
	}
	parsed, err := ParseReceiptCommitMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	return ReceiptEntry{
		OID: prepared.Commit, Parent: parent, Tree: prepared.Tree,
		Subject: parsed.Subject, Detail: parsed.Detail, Receipt: parsed.Receipt,
	}
}

func prepareActionSliceBase(
	t *testing.T,
	actions *Actions,
	release string,
	sliceID string,
) string {
	t.Helper()
	state, err := actions.stateFor(release)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice(sliceID)
	if !ok {
		t.Fatalf("missing slice %s", sliceID)
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		t.Fatalf("missing track %s", slice.Location.Track.ID)
	}
	base := slice.PreparedBase
	if base == "" && len(slice.ConsumedInputs) > 0 {
		base, err = prepareConsumedTrackBase(
			actions.repository,
			track.Ref,
			slice.PreparationSeed,
			slice.ConsumedInputs,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if base == "" {
		return ""
	}
	switch {
	case track.Head == "":
		actionGit(
			t,
			actions.repository.root(),
			nil,
			nil,
			"update-ref",
			track.Ref,
			base,
		)
	case track.Head != base:
		actionGit(
			t,
			actions.repository.root(),
			nil,
			nil,
			"update-ref",
			track.Ref,
			base,
			track.Head,
		)
	}
	return base
}

func advanceActionSlice(
	t *testing.T,
	actions *Actions,
	repoPath, release, track, slice, file string,
	timestamp int64,
	verdict string,
) string {
	return advanceActionSliceProduct(
		t,
		actions,
		repoPath,
		release,
		track,
		slice,
		file,
		slice+"\n",
		timestamp,
		verdict,
	)
}

func advanceActionSliceProduct(
	t *testing.T,
	actions *Actions,
	repoPath, release, track, slice, file, body string,
	timestamp int64,
	verdict string,
) string {
	t.Helper()
	prepareActionSliceBase(t, actions, release, slice)
	designInput := AppendReceiptInput{
		Release: release, Slice: slice, Role: "implementer", Result: "designed",
		Summary: "Design " + slice + ".", Detail: []byte("design " + slice),
	}
	designed := appendActionReceipt(t, actions, designInput)
	state, err := actions.stateFor(release)
	if err != nil {
		t.Fatal(err)
	}
	current, ok := state.Slice(slice)
	if !ok {
		t.Fatalf("missing designed slice %s", slice)
	}
	if len(current.Location.Slice.Consumes) > 0 {
		if designed.Receipt == nil || designed.Receipt.Base == nil ||
			designed.Receipt.Inputs == nil ||
			!inputsEqual(designed.Receipt.Inputs, current.ReviewedPins) {
			t.Fatalf("consuming design lacks strict reviewed evidence: %#v", designed)
		}
		retry := appendActionReceipt(t, actions, designInput)
		if retry.Changed || retry.ReceiptCommit != designed.ReceiptCommit {
			t.Fatalf("strict design retry = %#v", retry)
		}
	}
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "captain", Result: "proceed",
		Summary: "Proceed " + slice + ".", Detail: []byte("review " + slice),
	})
	base := prepareActionSliceBase(t, actions, release, slice)
	ref := "refs/heads/track/" + release + "/" + track
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidate := commitActionProduct(t, repoPath, parent, ref, file, body, timestamp)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "implementer", Result: "candidate",
		Summary: "Candidate " + slice + ".", Detail: []byte("implementation " + slice),
		Base: base, Candidate: candidate, CheckResults: []byte("checks " + slice + "\n"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "verifier", Result: verdict,
		Summary: strings.ToUpper(verdict[:1]) + verdict[1:] + " " + slice + ".",
		Detail:  []byte("verification " + slice), Candidate: candidate,
		CheckResults: []byte("fresh checks " + slice + "\n"),
	})
	return candidate
}

func actionSlice(id, include string) Slice {
	return Slice{
		ID: id, Outcome: "Deliver " + id + ".",
		Scope:       Scope{Include: []string{include}, Exclude: []string{}},
		Acceptance:  []Criterion{{ID: "A-" + id, Text: id + " is exact."}},
		Checks:      []string{"check " + id},
		Constraints: []string{"deterministic"},
		DependsOn:   []string{},
		Consumes:    []string{},
	}
}

func commitActionProduct(
	t *testing.T,
	repo, parent, ref, path, contents string,
	timestamp int64,
) string {
	t.Helper()
	indexRoot := t.TempDir()
	index := filepath.Join(indexRoot, "index")
	indexEnv := []string{"GIT_INDEX_FILE=" + index}
	actionGit(t, repo, nil, indexEnv, "read-tree", parent+"^{tree}")
	blob := actionGit(t, repo, []byte(contents), nil, "hash-object", "-w", "--stdin")
	actionGit(t, repo, nil, indexEnv, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+path)
	tree := actionGit(t, repo, nil, indexEnv, "write-tree")
	date := fmt.Sprintf("%d +0000", timestamp)
	commit := actionGit(t, repo, []byte("product "+path+"\n"), []string{
		"GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_DATE=" + date,
	}, "commit-tree", tree, "-p", parent)
	actionGit(t, repo, nil, nil, "update-ref", ref, commit, parent)
	return commit
}

func inertActionResolver(request InertnessRequest) (InertnessDecision, error) {
	return InertnessDecision{
		Kind: request.Kind, Repository: request.Repository,
		RecordRoot: request.RecordRoot, Commit: request.Commit,
		Decision: "inert",
	}, nil
}

func createActionHarness(t *testing.T) (string, *gitx.Repository, *Actions) {
	t.Helper()
	repoPath := createActionRepository(t, "sha1")
	repository, err := gitx.Open(repoPath, actionTestGit)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := NewActions(UseGitRepository(repository), inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	return repoPath, repository, actions
}

func readActionState(t *testing.T, repository *gitx.Repository, release string) State {
	t.Helper()
	state, err := ReadState(UseGitRepository(repository), release, inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestUnverifiedCandidateHeadRefreshReturnsToImplementerWithoutVerdict(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "candidate-head-refresh"
	_, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, []Track{{
			ID:        "T1",
			DependsOn: []string{},
			Slices:    []Slice{actionSlice("S1", "one.txt")},
		}}),
		Summary: "Approve exact candidate refresh.",
		Detail:  []byte("approval"),
	})
	if err != nil {
		t.Fatal(err)
	}
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "Design S1.", Detail: []byte("design"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "captain", Result: "proceed",
		Summary: "Proceed S1.", Detail: []byte("review"),
	})
	track := trackRef(release, "T1")
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", track)
	firstCandidate := commitActionProduct(
		t, repoPath, parent, track, "one.txt", "candidate one\n", 1000001900,
	)
	firstReceipt := appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
		Summary: "Candidate S1.", Detail: []byte("implementation one"),
		Candidate: firstCandidate, CheckResults: []byte("checks one\n"),
	})
	if firstReceipt.Receipt == nil {
		t.Fatal("first candidate receipt is absent")
	}
	refreshedHead := commitActionProduct(
		t,
		repoPath,
		firstReceipt.ReceiptCommit,
		track,
		"one.txt",
		"candidate two\n",
		1000001901,
	)

	refreshed := readActionState(t, repository, release)
	slice, ok := refreshed.Slice("S1")
	trackState, trackOK := refreshed.Track("T1")
	if !ok || !trackOK ||
		trackState.Head != refreshedHead ||
		trackState.AuthorityHead != firstReceipt.ReceiptCommit ||
		slice.Stage != "implement" ||
		slice.Status != "ready" ||
		slice.NextRole != "implementer" ||
		slice.Outcome != "stale" ||
		slice.Attempt != 2 ||
		slice.Retained ||
		slice.CurrentReceipt == nil ||
		slice.CurrentReceipt.OID != firstReceipt.ReceiptCommit ||
		slice.Candidate == nil ||
		slice.Candidate.OID != firstReceipt.ReceiptCommit ||
		slice.StaleReason != "track head changed before verification was recorded" {
		t.Fatalf("refreshed state: track=%#v slice=%#v", trackState, slice)
	}

	secondReceipt := appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
		Summary: "Refresh exact S1 candidate.", Detail: []byte("implementation two"),
		Candidate: refreshedHead, CheckResults: []byte("checks two\n"),
	})
	if secondReceipt.Receipt == nil ||
		secondReceipt.Receipt.Binds != firstReceipt.ReceiptCommit ||
		secondReceipt.Receipt.Attempt == nil ||
		*secondReceipt.Receipt.Attempt != 2 {
		t.Fatalf("second candidate receipt = %#v", secondReceipt)
	}
	ready := readActionState(t, repository, release)
	slice, _ = ready.Slice("S1")
	if slice.Stage != "verify" ||
		slice.NextRole != "verifier" ||
		slice.Attempt != 2 ||
		slice.CurrentReceipt == nil ||
		slice.CurrentReceipt.OID != secondReceipt.ReceiptCommit {
		t.Fatalf("recorded refresh state = %#v", slice)
	}
}

func TestActionsMatchExactReferenceForSHA1AndSHA256(t *testing.T) {
	golden := loadActionGolden(t)
	stateGolden := loadStateGolden(t)
	gitGolden := loadGitGolden(t)
	if len(golden.Flows) != 2 || len(stateGolden.Flows) != 2 || len(gitGolden.Flows) != 2 {
		t.Fatalf("golden flows = %d", len(golden.Flows))
	}
	for flowIndex, expectedFlow := range golden.Flows {
		expectedFlow := expectedFlow
		t.Run(expectedFlow.ObjectFormat, func(t *testing.T) {
			expectedStates := stateGolden.Flows[flowIndex]
			expectedGit := gitGolden.Flows[flowIndex]
			if expectedStates.ObjectFormat != expectedFlow.ObjectFormat ||
				expectedGit.ObjectFormat != expectedFlow.ObjectFormat {
				t.Fatalf("golden formats = %s/%s, want %s",
					expectedStates.ObjectFormat, expectedGit.ObjectFormat, expectedFlow.ObjectFormat)
			}
			repoPath := createActionRepository(t, expectedFlow.ObjectFormat)
			gitRepository, err := gitx.Open(repoPath, actionTestGit)
			if err != nil {
				t.Fatal(err)
			}
			actions, err := NewActions(UseGitRepository(gitRepository), inertActionResolver)
			if err != nil {
				t.Fatal(err)
			}
			release := "golden-" + expectedFlow.ObjectFormat
			var results []ActionResult
			type observedState struct {
				label string
				state State
			}
			var states []observedState

			recordInput := RecordPlanRevisionInput{
				PlanBytes: actionPlanBytes(release),
				Summary:   "Approve the exact golden plan.",
				Detail:    []byte("protected approval"),
			}
			recorded, err := actions.RecordPlanRevision(recordInput)
			if err != nil {
				t.Fatal(err)
			}
			results = append(results, recorded)
			retry, err := actions.RecordPlanRevision(recordInput)
			if err != nil {
				t.Fatal(err)
			}
			if retry.Changed || retry.ReceiptCommit != recorded.ReceiptCommit {
				t.Fatalf("plan retry = %#v, want unchanged %s", retry, recorded.ReceiptCommit)
			}
			state, err := ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Slices) != 2 || state.Slices[0].Stage != "design" ||
				state.Slices[1].Stage != "design" || len(state.Diagnostics) != 2 {
				t.Fatalf("approved state = %#v", state)
			}
			states = append(states, observedState{label: "approved", state: state})

			for _, item := range []struct {
				track, slice, file string
				timestamp          int64
			}{
				{"T1", "S1", "one.txt", 1000000100},
				{"T2", "S2", "two.txt", 1000000200},
			} {
				designedInput := AppendReceiptInput{
					Release: release, Slice: item.slice,
					Role: "implementer", Result: "designed",
					Summary: "Design " + item.slice + ".",
					Detail:  []byte("design " + item.slice),
				}
				designed, err := actions.AppendReceipt(designedInput)
				if err != nil {
					t.Fatal(err)
				}
				results = append(results, designed)
				designedRetry, err := actions.AppendReceipt(designedInput)
				if err != nil {
					t.Fatal(err)
				}
				if designedRetry.Changed || designedRetry.ReceiptCommit != designed.ReceiptCommit {
					t.Fatalf("design retry = %#v", designedRetry)
				}

				proceeded, err := actions.AppendReceipt(AppendReceiptInput{
					Release: release, Slice: item.slice,
					Role: "captain", Result: "proceed",
					Summary: "Proceed " + item.slice + ".",
					Detail:  []byte("review " + item.slice),
				})
				if err != nil {
					t.Fatal(err)
				}
				results = append(results, proceeded)

				ref := "refs/heads/track/" + release + "/" + item.track
				parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
				candidate := commitActionProduct(
					t, repoPath, parent, ref, item.file, item.slice+"\n", item.timestamp,
				)
				candidateResult, err := actions.AppendReceipt(AppendReceiptInput{
					Release: release, Slice: item.slice,
					Role: "implementer", Result: "candidate",
					Summary:   "Candidate " + item.slice + ".",
					Detail:    []byte("implementation " + item.slice),
					Candidate: candidate, CheckResults: []byte("checks " + item.slice + "\n"),
				})
				if err != nil {
					t.Fatal(err)
				}
				results = append(results, candidateResult)

				pass, err := actions.AppendReceipt(AppendReceiptInput{
					Release: release, Slice: item.slice,
					Role: "verifier", Result: "pass",
					Summary:   "Pass " + item.slice + ".",
					Detail:    []byte("verification " + item.slice),
					Candidate: candidate, CheckResults: []byte("fresh checks " + item.slice + "\n"),
				})
				if err != nil {
					t.Fatal(err)
				}
				results = append(results, pass)
				state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
				if err != nil {
					t.Fatal(err)
				}
				slice, ok := state.Slice(item.slice)
				if !ok || slice.Pass == nil || slice.Outcome != "pass" {
					t.Fatalf("%s state = %#v", item.slice, slice)
				}
				states = append(states, observedState{label: "passed-" + item.slice, state: state})
			}

			assembly, err := actions.PrepareAssembly(PrepareAssemblyInput{
				Release: release, Summary: "Prepare exact assembly.",
				Detail: []byte("ordered composition"),
			})
			if err != nil {
				t.Fatal(err)
			}
			results = append(results, assembly)
			state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
			if err != nil {
				t.Fatal(err)
			}
			if state.Assembly.Candidate == nil || state.Assembly.NextRole != "verifier" {
				t.Fatalf("assembly state = %#v", state.Assembly)
			}
			states = append(states, observedState{label: "assembly-candidate", state: state})
			assemblyCandidate := *state.Assembly.Candidate.Receipt.Candidate
			assemblyPass, err := actions.AppendReceipt(AppendReceiptInput{
				Release: release, Role: "verifier", Result: "pass",
				Summary: "Pass exact assembly.", Detail: []byte("fresh assembly verification"),
				Candidate: assemblyCandidate, CheckResults: []byte("assembly checks\n"),
			})
			if err != nil {
				t.Fatal(err)
			}
			results = append(results, assemblyPass)
			state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
			if err != nil {
				t.Fatal(err)
			}
			states = append(states, observedState{label: "assembly-pass", state: state})
			merged, err := actions.MergePassedCandidate(MergePassedCandidateInput{
				Release: release, Summary: "Merge exact passed assembly.",
				Detail: []byte("deterministic merge"),
			})
			if err != nil {
				t.Fatal(err)
			}
			results = append(results, merged)
			mergeRetry, err := actions.MergePassedCandidate(MergePassedCandidateInput{
				Release: release, Summary: "Merge exact passed assembly.",
				Detail: []byte("deterministic merge"),
			})
			if err != nil {
				t.Fatal(err)
			}
			if mergeRetry.Changed || mergeRetry.ResultCommit != merged.ResultCommit {
				t.Fatalf("merge retry = %#v", mergeRetry)
			}
			state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
			if err != nil {
				t.Fatal(err)
			}
			if state.Assembly.Status != "complete" || state.Assembly.Outcome != "merged" ||
				state.Assembly.ResultCommit != merged.ResultCommit ||
				state.Refs.Target.Head != merged.ResultCommit {
				t.Fatalf("merged state = %#v", state.Assembly)
			}
			states = append(states, observedState{label: "merged", state: state})

			if len(results) != len(expectedFlow.Results) {
				t.Fatalf("results = %d, want %d", len(results), len(expectedFlow.Results))
			}
			for index, got := range results {
				want := expectedFlow.Results[index]
				if got.Action != want.Action || got.Changed != want.Changed ||
					got.Plan != want.Plan || got.Head != want.Head || got.Target != want.Target ||
					got.Candidate != want.Candidate || got.ResultCommit != want.ResultCommit ||
					got.ReceiptCommit != want.ReceiptCommit {
					t.Fatalf("result[%d] = %#v, golden = %#v", index, got, want)
				}
			}
			if len(states) != len(expectedStates.States) {
				t.Fatalf("states = %d, want %d", len(states), len(expectedStates.States))
			}
			for index, got := range states {
				want := expectedStates.States[index]
				if got.label != want.Label || !jsonValueEqual(projectActionState(got.state), want.State) {
					projected, _ := json.MarshalIndent(projectActionState(got.state), "", "  ")
					t.Fatalf("state[%d] %s differs from %s:\n%s\nwant:\n%s",
						index, got.label, want.Label, projected, want.State)
				}
			}

			recordAdmission, err := gitRepository.ResolveRecordPathAdmission()
			if err != nil {
				t.Fatal(err)
			}
			productAdmission, err := gitRepository.ResolveProductExclusion(recordAdmission, func(
				request gitx.RecordRootRequest,
			) (gitx.RecordRootDecision, error) {
				return gitx.RecordRootDecision{
					Kind: request.Kind, Repository: request.Repository,
					RecordRoot: request.RecordRoot, Commit: request.Commit,
					Decision: "inert",
				}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			targetOID, err := gitx.ParseOID(gitRepository.ObjectFormat(), state.Refs.Target.Head)
			if err != nil {
				t.Fatal(err)
			}
			product, err := gitRepository.ProductTreeIdentity(targetOID, productAdmission)
			if err != nil {
				t.Fatal(err)
			}
			releaseOID, err := gitx.ParseOID(gitRepository.ObjectFormat(), state.Refs.Release.Head)
			if err != nil {
				t.Fatal(err)
			}
			history, err := gitRepository.ReadFirstParentHistory(releaseOID, 8)
			if err != nil {
				t.Fatal(err)
			}
			entries := make([]any, 0, len(product.Entries))
			for _, entry := range product.Entries {
				entries = append(entries, map[string]any{
					"path": entry.Path, "mode": entry.Mode,
					"type": entry.Type, "object": entry.OID.String(),
				})
			}
			historyProjection := make([]any, 0, len(history))
			for _, entry := range history {
				parents := make([]string, len(entry.Parents))
				for index, parent := range entry.Parents {
					parents[index] = parent.String()
				}
				sum := sha256.Sum256(entry.Message)
				historyProjection = append(historyProjection, map[string]any{
					"oid": entry.OID.String(), "parents": parents,
					"tree": entry.Tree.String(), "message_sha256": hex.EncodeToString(sum[:]),
				})
			}
			gitProjection := map[string]any{
				"object_format": expectedFlow.ObjectFormat,
				"target":        state.Refs.Target.Head, "release_head": state.Refs.Release.Head,
				"target_tree":  product.CandidateTree.String(),
				"product_tree": product.ProductTree, "product_entries": entries,
				"first_parent_history": historyProjection,
			}
			if !jsonValueEqual(gitProjection, expectedGit) {
				projected, _ := json.MarshalIndent(gitProjection, "", "  ")
				wanted, _ := json.MarshalIndent(expectedGit, "", "  ")
				t.Fatalf("git projection differs:\n%s\nwant:\n%s", projected, wanted)
			}
		})
	}
}

func TestCandidateReceiptRejectsReservedRecordRootChangeFromPreparedBase(t *testing.T) {
	repoPath, _, actions := createActionHarness(t)
	release := "reserved-candidate"
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release),
		Summary:   "Approve reserved-root fixture.",
		Detail:    []byte("protected approval"),
	}); err != nil {
		t.Fatal(err)
	}
	prepareActionSliceBase(t, actions, release, "S1")
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "Design S1.", Detail: []byte("design"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "captain", Result: "proceed",
		Summary: "Proceed S1.", Detail: []byte("review"),
	})

	ref := "refs/heads/track/" + release + "/T1"
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidate := commitActionProduct(
		t,
		repoPath,
		parent,
		ref,
		".baton/releases/foreign/plan.md",
		"reserved\n",
		1000000500,
	)
	_, err := actions.AppendReceipt(AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
		Summary: "Candidate S1.", Detail: []byte("implementation"),
		Candidate: candidate, CheckResults: []byte("checks\n"),
	})
	if code := ErrorCode(err); code != "RESERVED_RECORD_ROOT_CHANGED" {
		t.Fatalf("code = %q (%v), want RESERVED_RECORD_ROOT_CHANGED", code, err)
	}
}

func TestReplayRejectsForgedCandidateRecordRootBeforeProductPolicy(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "reserved-replay"
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release),
		Summary:   "Approve replay fixture.",
		Detail:    []byte("protected approval"),
	}); err != nil {
		t.Fatal(err)
	}
	prepareActionSliceBase(t, actions, release, "S1")
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "Design S1.", Detail: []byte("design"),
	})
	proceed := appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "captain", Result: "proceed",
		Summary: "Proceed S1.", Detail: []byte("review"),
	})
	state := readActionState(t, repository, release)
	contract := state.Plan.Metadata.Contracts["S1"]
	sliceID, attempt := "S1", int64(1)
	ownerRef := trackRef(release, "T1")
	candidate := commitActionProduct(
		t,
		repoPath,
		proceed.ReceiptCommit,
		ownerRef,
		".baton/releases/foreign/plan.md",
		"reserved\n",
		1000000501,
	)
	productTree, err := actions.repository.productTree(candidate)
	if err != nil {
		t.Fatal(err)
	}
	checks := DigestBytes([]byte("forged checks\n"))
	appendRawActionReceipt(
		t,
		actions,
		repoPath,
		ownerRef,
		candidate,
		candidate,
		Receipt{
			Version: ReceiptVersion, Release: release,
			Slice: &sliceID, Role: "implementer",
			Result: "candidate", Attempt: &attempt,
			Plan: state.Plan.OID, Contract: &contract,
			Binds: proceed.ReceiptCommit, Detail: DigestBytes(nil),
			Summary:   "Forged candidate hides a reserved-root change.",
			Candidate: &candidate, ProductTree: &productTree,
			Inputs: map[string]string{}, Checks: &checks,
		},
		nil,
	)

	candidatePolicyCalls := 0
	_, err = ReadState(
		UseGitRepository(repository),
		release,
		func(request InertnessRequest) (InertnessDecision, error) {
			if request.Commit == candidate {
				candidatePolicyCalls++
				return InertnessDecision{}, fmt.Errorf(
					"unexpected candidate product-tree policy for %s",
					request.Commit,
				)
			}
			return InertnessDecision{
				Kind: request.Kind, Repository: request.Repository,
				RecordRoot: request.RecordRoot, Commit: request.Commit,
				Decision: "inert",
			}, nil
		},
	)
	if code := ErrorCode(err); code != "RESERVED_RECORD_ROOT_CHANGED" {
		t.Fatalf("code = %q (%v), want RESERVED_RECORD_ROOT_CHANGED", code, err)
	}
	if candidatePolicyCalls != 0 {
		t.Fatalf(
			"candidate product-tree policy ran %d times before replay rejection",
			candidatePolicyCalls,
		)
	}
}

func TestConsumingReplayRejectsForgedBaseBeforeCandidateProductPolicy(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "reserved-consuming-replay"
	s1 := actionSlice("S1", "one.txt")
	s2 := actionSlice("S2", "two.txt")
	s2.DependsOn = []string{"S1"}
	s2.Consumes = []string{"S1"}
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, []Track{
			{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
			{ID: "T2", DependsOn: []string{"T1"}, Slices: []Slice{s2}},
		}),
		Summary: "Approve consuming replay fixture.",
		Detail:  []byte("protected approval"),
	}); err != nil {
		t.Fatal(err)
	}
	advanceActionSlice(
		t,
		actions,
		repoPath,
		release,
		"T1",
		"S1",
		"one.txt",
		1000000510,
		"pass",
	)
	prepareActionSliceBase(t, actions, release, "S2")
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S2", Role: "implementer", Result: "designed",
		Summary: "Design S2.", Detail: []byte("design"),
	})
	proceed := appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S2", Role: "captain", Result: "proceed",
		Summary: "Proceed S2.", Detail: []byte("review"),
	})
	prepareActionSliceBase(t, actions, release, "S2")
	state := readActionState(t, repository, release)
	consumer, ok := state.Slice("S2")
	if !ok {
		t.Fatal("missing consuming slice S2")
	}
	contract := state.Plan.Metadata.Contracts["S2"]
	sliceID, attempt := "S2", int64(1)
	ownerRef := trackRef(release, "T2")
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ownerRef)
	forgedBase := commitActionProduct(
		t,
		repoPath,
		parent,
		ownerRef,
		".baton/releases/foreign/plan.md",
		"reserved\n",
		1000000520,
	)
	candidate := commitActionProduct(
		t,
		repoPath,
		forgedBase,
		ownerRef,
		"two.txt",
		"candidate\n",
		1000000521,
	)
	productTree, err := actions.repository.productTree(candidate)
	if err != nil {
		t.Fatal(err)
	}
	checks := DigestBytes([]byte("forged consuming checks\n"))
	appendRawActionReceipt(
		t,
		actions,
		repoPath,
		ownerRef,
		candidate,
		candidate,
		Receipt{
			Version: ReceiptVersion, Release: release,
			Slice: &sliceID, Role: "implementer",
			Result: "candidate", Attempt: &attempt,
			Plan: state.Plan.OID, Contract: &contract,
			Binds: proceed.ReceiptCommit, Detail: DigestBytes(nil),
			Summary:   "Forged consuming candidate self-declares a mutated base.",
			Base:      &forgedBase,
			Candidate: &candidate, ProductTree: &productTree,
			Inputs: cloneInputs(consumer.InputPins), Checks: &checks,
		},
		nil,
	)

	candidatePolicyCalls := 0
	_, err = ReadState(
		UseGitRepository(repository),
		release,
		func(request InertnessRequest) (InertnessDecision, error) {
			if request.Commit == candidate {
				candidatePolicyCalls++
				return InertnessDecision{}, fmt.Errorf(
					"unexpected candidate product-tree policy for %s",
					request.Commit,
				)
			}
			return InertnessDecision{
				Kind: request.Kind, Repository: request.Repository,
				RecordRoot: request.RecordRoot, Commit: request.Commit,
				Decision: "inert",
			}, nil
		},
	)
	if code := ErrorCode(err); code != "RESERVED_RECORD_ROOT_CHANGED" {
		t.Fatalf("code = %q (%v), want RESERVED_RECORD_ROOT_CHANGED", code, err)
	}
	if candidatePolicyCalls != 0 {
		t.Fatalf(
			"candidate product-tree policy ran %d times before replay rejection",
			candidatePolicyCalls,
		)
	}
}

func TestActionsFailClosedBeforeMutation(t *testing.T) {
	repoPath := createActionRepository(t, "sha1")
	gitRepository, err := gitx.Open(repoPath, actionTestGit)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := NewActions(UseGitRepository(gitRepository), inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	release := "fail-closed"
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release), Summary: "Approve.", Detail: nil,
	}); err != nil {
		t.Fatal(err)
	}
	headBefore := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/release-wt/"+release)
	cases := []AppendReceiptInput{
		{Release: release, Slice: "S1", Role: "verifier", Result: "pass", Summary: "Pass.", Candidate: strings.Repeat("a", 40), CheckResults: []byte("checks")},
		{Release: release, Slice: "S1", Role: "implementer", Result: "candidate", Summary: "Candidate.", Candidate: strings.Repeat("a", 40)},
		{Release: release, Slice: "missing", Role: "implementer", Result: "designed", Summary: "Design."},
	}
	for _, input := range cases {
		if _, err := actions.AppendReceipt(input); err == nil {
			t.Fatalf("invalid action was admitted: %#v", input)
		}
	}
	headAfter := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/release-wt/"+release)
	if headAfter != headBefore {
		t.Fatalf("release ref moved from %s to %s", headBefore, headAfter)
	}
}

func TestAttemptTransitionsAndDirectSingleTrackMerge(t *testing.T) {
	repoPath := createActionRepository(t, "sha1")
	gitRepository, err := gitx.Open(repoPath, actionTestGit)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := NewActions(UseGitRepository(gitRepository), inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	release := "attempt-matrix"
	plan := actionPlanRevisionBytes(release, 1, nil, []Track{{
		ID: "T1", DependsOn: []string{}, Slices: []Slice{actionSlice("S1", "one.txt")},
	}})
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: plan, Summary: "Approve attempt matrix.", Detail: []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "Design attempt one.", Detail: []byte("design one"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "captain", Result: "revise",
		Summary: "Revise attempt one.", Detail: []byte("review one"),
	})
	state, err := ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	slice, _ := state.Slice("S1")
	if slice.Attempt != 2 || slice.Stage != "design" || slice.NextRole != "implementer" ||
		slice.Outcome != "revise" || slice.History.MaximumAttempt != 1 {
		t.Fatalf("post-revise state = %#v", slice)
	}
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "Design attempt two.", Detail: []byte("design two"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "captain", Result: "proceed",
		Summary: "Proceed attempt two.", Detail: []byte("review two"),
	})
	ref := "refs/heads/track/" + release + "/T1"
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidateTwo := commitActionProduct(t, repoPath, parent, ref, "one.txt", "attempt two\n", 1000000300)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
		Summary: "Candidate attempt two.", Detail: []byte("candidate two"),
		Candidate: candidateTwo, CheckResults: []byte("checks two"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "verifier", Result: "fail",
		Summary: "Fail attempt two.", Detail: []byte("verify two"),
		Candidate: candidateTwo, CheckResults: []byte("fresh two"),
	})
	state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	slice, _ = state.Slice("S1")
	if slice.Attempt != 3 || slice.Stage != "implement" || slice.Outcome != "fail" ||
		slice.NextAttempts.Design != 3 || slice.NextAttempts.Candidate != 3 {
		t.Fatalf("post-fail state = %#v", slice)
	}
	parent = actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidateThree := commitActionProduct(t, repoPath, parent, ref, "one.txt", "attempt three\n", 1000000400)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
		Summary: "Candidate attempt three.", Detail: []byte("candidate three"),
		Candidate: candidateThree, CheckResults: []byte("checks three"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "verifier", Result: "pass",
		Summary: "Pass attempt three.", Detail: []byte("verify three"),
		Candidate: candidateThree, CheckResults: []byte("fresh three"),
	})
	state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	slice, _ = state.Slice("S1")
	if slice.Pass == nil || slice.Attempt != 3 || slice.History.MaximumAttempt != 3 {
		t.Fatalf("passed attempt state = %#v", slice)
	}
	assembly, err := actions.PrepareAssembly(PrepareAssemblyInput{
		Release: release, Summary: "Use direct candidate.", Detail: []byte("direct"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if assembly.Changed || !assembly.Direct || assembly.Candidate != candidateThree {
		t.Fatalf("direct assembly = %#v", assembly)
	}
	merged, err := actions.MergePassedCandidate(MergePassedCandidateInput{
		Release: release, Summary: "Merge direct candidate.", Detail: []byte("merge"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.ResultCommit != candidateThree {
		t.Fatalf("direct merge = %#v", merged)
	}
}

func TestPlanRevisionRetentionSelectiveInvalidationAndRetirement(t *testing.T) {
	repoPath := createActionRepository(t, "sha1")
	gitRepository, err := gitx.Open(repoPath, actionTestGit)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := NewActions(UseGitRepository(gitRepository), inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	release := "revision-matrix"
	s1 := actionSlice("S1", "one.txt")
	s2 := actionSlice("S2", "two.txt")
	s2.Consumes = []string{"S1"}
	tracks := []Track{
		{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
		{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
	}
	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve revision one.", Detail: []byte("approval one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000000500, "pass")
	advanceActionSlice(t, actions, repoPath, release, "T2", "S2", "two.txt", 1000000600, "pass")

	previous := recorded.Plan
	revisionTwo, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 2, &previous, tracks),
		Summary:   "Approve revision two.", Detail: []byte("approval two"),
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"S1", "S2"} {
		slice, _ := state.Slice(id)
		if slice.Pass == nil || !slice.Retained {
			t.Fatalf("%s was not retained under unchanged contract: %#v", id, slice)
		}
	}

	changedS1 := s1
	changedS1.Outcome = "Deliver revised S1."
	tracksThree := []Track{
		{ID: "T1", DependsOn: []string{}, Slices: []Slice{changedS1}},
		{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
	}
	previous = revisionTwo.Plan
	revisionThree, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 3, &previous, tracksThree),
		Summary:   "Approve revision three.", Detail: []byte("approval three"),
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	currentS1, _ := state.Slice("S1")
	currentS2, _ := state.Slice("S2")
	if currentS1.Pass != nil || currentS1.Attempt != 2 || currentS1.Stage != "design" {
		t.Fatalf("changed S1 contract was not reset: %#v", currentS1)
	}
	if currentS2.Pass != nil || currentS2.Retained || currentS2.Outcome != "stale" ||
		currentS2.Status != "waiting" || currentS2.NextRole != "none" ||
		currentS2.StaleReason == "" {
		t.Fatalf("S2 was not selectively invalidated: %#v", currentS2)
	}

	previous = revisionThree.Plan
	retired, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 4, &previous, []Track{tracksThree[0]}),
		Summary:   "Retire S2.", Detail: []byte("approval four"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(retired.Retirements) != 1 || retired.Retirements[0].Slice != "S2" ||
		retired.Retirements[0].Receipt.Role != "planner" ||
		retired.Retirements[0].Receipt.Result != "retired" {
		t.Fatalf("retirements = %#v", retired.Retirements)
	}
	state, err = ReadState(UseGitRepository(gitRepository), release, inertActionResolver)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Slice("S2"); ok || len(state.Slices) != 1 || len(state.Plan.History) != 4 {
		t.Fatalf("post-retirement state = %#v", state)
	}
	historicalInstall := state.Plan.History[3]
	if historicalInstall.InstallHead != retired.Head ||
		len(historicalInstall.Retirements) != 1 ||
		historicalInstall.Retirements[0].Slice != "S2" ||
		historicalInstall.Retirements[0].ReceiptCommit !=
			retired.Retirements[0].ReceiptCommit ||
		historicalInstall.Retirements[0].Receipt.Role != "planner" ||
		historicalInstall.Retirements[0].Receipt.Result != "retired" {
		t.Fatalf("historical install result = %#v", historicalInstall)
	}

	previous = retired.Plan
	later, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 5, &previous, []Track{tracksThree[0]}),
		Summary:   "Keep S2 retired.", Detail: []byte("approval five"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(later.Retirements) != 0 {
		t.Fatalf("later retirement was duplicated: %#v", later.Retirements)
	}
	before := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
	previous = later.Plan
	_, err = actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 6, &previous, tracksThree),
		Summary:   "Attempt to resurrect S2.", Detail: []byte("approval six"),
	})
	if ErrorCode(err) != "INVALID_RETIREMENT" {
		t.Fatalf("resurrection error = %v", err)
	}
	after := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
	if after != before {
		t.Fatalf("resurrection moved release ref from %s to %s", before, after)
	}
}

func TestUnavailableConsumedPassFailsClosed(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "unavailable-consumed-pass"
	s1 := actionSlice("S1", "one.txt")
	s2 := actionSlice("S2", "two.txt")
	s2.Consumes = []string{"S1"}
	tracks := []Track{
		{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
		{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
	}
	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve available inputs.", Detail: []byte("approval one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000000700, "pass")
	advanceActionSlice(t, actions, repoPath, release, "T2", "S2", "two.txt", 1000000800, "pass")
	previous := recorded.Plan
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 2, &previous, tracks),
		Summary:   "Retain exact contracts.", Detail: []byte("approval two"),
	}); err != nil {
		t.Fatal(err)
	}
	actionGit(t, repoPath, nil, nil, "update-ref", "-d", trackRef(release, "T1"))
	if _, err := ReadState(
		UseGitRepository(repository),
		release,
		inertActionResolver,
	); ErrorCode(err) != "STALE_BINDING" {
		t.Fatalf("unavailable consumed PASS error = %v", err)
	}
}

func TestConsumedCompositionConflictDoesNotInvalidateIndependentProjection(
	t *testing.T,
) {
	repoPath, repository, actions := createActionHarness(t)
	release := "consumed-conflict-isolation"
	producer := actionSlice("S1", "shared.txt")
	priorConsumerWork := actionSlice("S2", "shared.txt")
	consumer := actionSlice("S3", "consumer.txt")
	consumer.Consumes = []string{"S1"}
	independent := actionSlice("S4", "independent.txt")
	tracks := []Track{
		{ID: "T1", DependsOn: []string{}, Slices: []Slice{producer}},
		{
			ID: "T2", DependsOn: []string{"T1"},
			Slices: []Slice{priorConsumerWork, consumer},
		},
		{ID: "T3", DependsOn: []string{}, Slices: []Slice{independent}},
	}
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve conflict isolation.", Detail: []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	advanceActionSliceProduct(
		t,
		actions,
		repoPath,
		release,
		"T1",
		"S1",
		"shared.txt",
		"producer product\n",
		1000000850,
		"pass",
	)
	advanceActionSliceProduct(
		t,
		actions,
		repoPath,
		release,
		"T2",
		"S2",
		"shared.txt",
		"consumer-track product\n",
		1000000860,
		"pass",
	)

	state := readActionState(t, repository, release)
	conflicted, ok := state.Slice("S3")
	if !ok || conflicted.Stage != "design" ||
		conflicted.Status != "ready" ||
		conflicted.NextRole != "implementer" ||
		conflicted.PreparationSeed == "" ||
		conflicted.PreparedBase != "" ||
		len(conflicted.ConsumedInputs) != 1 {
		t.Fatalf("conflicted consumer projection = %#v", conflicted)
	}
	other, ok := state.Slice("S4")
	if !ok || other.Stage != "design" ||
		other.Status != "ready" ||
		other.NextRole != "implementer" {
		t.Fatalf("independent slice was not schedulable: %#v", other)
	}
	track, ok := state.Track("T2")
	if !ok {
		t.Fatal("missing consumer track")
	}
	before := actionGit(
		t,
		repoPath,
		nil,
		nil,
		"rev-parse",
		"--verify",
		track.Ref,
	)
	if _, err := prepareConsumedTrackBase(
		actions.repository,
		track.Ref,
		conflicted.PreparationSeed,
		conflicted.ConsumedInputs,
	); err == nil {
		t.Fatal("conflicting consumed products unexpectedly composed")
	}
	after := actionGit(
		t,
		repoPath,
		nil,
		nil,
		"rev-parse",
		"--verify",
		track.Ref,
	)
	if after != before {
		t.Fatalf("failed preparation moved consumer ref from %s to %s", before, after)
	}
	if _, err := ReadState(
		UseGitRepository(repository),
		release,
		inertActionResolver,
	); err != nil {
		t.Fatalf("local composition conflict invalidated projection: %v", err)
	}
}

func TestReviewedPinsRetainCrossPlanStagesWhenProductsAreUnchanged(t *testing.T) {
	for _, test := range []struct {
		name     string
		consumes bool
		decision string
	}{
		{name: "design_with_consumes", consumes: true},
		{name: "proceed_with_consumes", consumes: true, decision: "proceed"},
		{name: "revise_with_consumes", consumes: true, decision: "revise"},
		{name: "design_without_consumes"},
		{name: "proceed_without_consumes", decision: "proceed"},
		{name: "revise_without_consumes", decision: "revise"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			repoPath, repository, actions := createActionHarness(t)
			release := "pinless-" + strings.ReplaceAll(test.name, "_", "-")
			s1 := actionSlice("S1", "one.txt")
			s2 := actionSlice("S2", "two.txt")
			if test.consumes {
				s2.Consumes = []string{"S1"}
			}
			tracks := []Track{
				{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
				{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
			}
			recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
				Summary:   "Approve pinless stage.", Detail: []byte("approval one"),
			})
			if err != nil {
				t.Fatal(err)
			}
			advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000000900, "pass")
			prepareActionSliceBase(t, actions, release, "S2")
			appendActionReceipt(t, actions, AppendReceiptInput{
				Release: release, Slice: "S2", Role: "implementer", Result: "designed",
				Summary: "Design S2.", Detail: []byte("design S2"),
			})
			if test.decision != "" {
				appendActionReceipt(t, actions, AppendReceiptInput{
					Release: release, Slice: "S2", Role: "captain", Result: test.decision,
					Summary: "Review S2.", Detail: []byte("review S2"),
				})
			}
			previous := recorded.Plan
			_, err = actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(release, 2, &previous, tracks),
				Summary:   "Approve unchanged revision.", Detail: []byte("approval two"),
			})
			if err != nil {
				t.Fatal(err)
			}
			state := readActionState(t, repository, release)
			slice, _ := state.Slice("S2")
			wantRole, wantStage, wantOutcome, wantAttempt := "captain", "design", "none", int64(1)
			if test.decision == "proceed" {
				wantRole, wantStage, wantOutcome = "implementer", "implement", "proceed"
			}
			if test.decision == "revise" {
				wantRole, wantStage, wantOutcome, wantAttempt = "implementer", "design", "revise", 2
			}
			if !slice.Retained || slice.NextRole != wantRole ||
				slice.Stage != wantStage || slice.Outcome != wantOutcome ||
				slice.Attempt != wantAttempt {
				t.Fatalf("unchanged reviewed stage was not retained: %#v", slice)
			}
			if test.consumes &&
				(len(slice.ReviewedPins) != 1 ||
					!inputsEqual(slice.ReviewedPins, slice.InputPins)) {
				t.Fatalf("reviewed pins were not retained: %#v", slice)
			}
		})
	}
}

func TestChangedReviewedProductReroutesEveryLaterStageToFreshDesign(
	t *testing.T,
) {
	for _, stage := range []string{"designed", "candidate", "failed", "passed"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			repoPath, repository, actions := createActionHarness(t)
			release := "changed-reviewed-product-" + stage
			s1 := actionSlice("S1", "one.txt")
			s2 := actionSlice("S2", "two.txt")
			s2.Consumes = []string{"S1"}
			tracks := []Track{
				{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
				{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
			}
			recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
				Summary:   "Approve reviewed product.", Detail: []byte("approval one"),
			})
			if err != nil {
				t.Fatal(err)
			}
			advanceActionSliceProduct(
				t,
				actions,
				repoPath,
				release,
				"T1",
				"S1",
				"one.txt",
				"old product\n",
				1000001150,
				"pass",
			)

			prepareActionSliceBase(t, actions, release, "S2")
			appendActionReceipt(t, actions, AppendReceiptInput{
				Release: release, Slice: "S2", Role: "implementer",
				Result: "designed", Summary: "Design S2.",
				Detail: []byte("design S2"),
			})
			if stage != "designed" {
				appendActionReceipt(t, actions, AppendReceiptInput{
					Release: release, Slice: "S2", Role: "captain",
					Result: "proceed", Summary: "Proceed S2.",
					Detail: []byte("review S2"),
				})
				base := prepareActionSliceBase(t, actions, release, "S2")
				ref := trackRef(release, "T2")
				parent := actionGit(
					t, repoPath, nil, nil, "rev-parse", "--verify", ref,
				)
				candidate := commitActionProduct(
					t,
					repoPath,
					parent,
					ref,
					"two.txt",
					"consumer product\n",
					1000001160,
				)
				appendActionReceipt(t, actions, AppendReceiptInput{
					Release: release, Slice: "S2", Role: "implementer",
					Result: "candidate", Summary: "Candidate S2.",
					Detail: []byte("candidate S2"), Base: base,
					Candidate: candidate, CheckResults: []byte("checks S2\n"),
				})
				if stage == "failed" || stage == "passed" {
					result := "fail"
					if stage == "passed" {
						result = "pass"
					}
					appendActionReceipt(t, actions, AppendReceiptInput{
						Release: release, Slice: "S2", Role: "verifier",
						Result: result, Summary: "Verify S2.",
						Detail: []byte("verification S2"), Candidate: candidate,
						CheckResults: []byte("fresh checks S2\n"),
					})
				}
			}
			beforeRevision := readActionState(t, repository, release)
			beforeConsumer, _ := beforeRevision.Slice("S2")
			if len(beforeConsumer.ReviewedPins) != 1 {
				t.Fatalf("reviewed product was not established: %#v", beforeConsumer)
			}
			oldPin := beforeConsumer.ReviewedPins["S1"]

			changedS1 := s1
			changedS1.Outcome = "Deliver revised S1."
			revisedTracks := []Track{
				{ID: "T1", DependsOn: []string{}, Slices: []Slice{changedS1}},
				{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
			}
			previous := recorded.Plan
			if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(
					release,
					2,
					&previous,
					revisedTracks,
				),
				Summary: "Approve changed producer.",
				Detail:  []byte("approval two"),
			}); err != nil {
				t.Fatal(err)
			}
			advanceActionSliceProduct(
				t,
				actions,
				repoPath,
				release,
				"T1",
				"S1",
				"one.txt",
				"new product\n",
				1000001170,
				"pass",
			)

			state := readActionState(t, repository, release)
			consumer, _ := state.Slice("S2")
			if consumer.Stage != "design" ||
				consumer.Status != "ready" ||
				consumer.NextRole != "implementer" ||
				consumer.Outcome != "stale" ||
				consumer.Attempt != 2 ||
				consumer.Candidate != nil ||
				consumer.Pass != nil ||
				consumer.StaleReason !=
					"reviewed consumed input product changed or is absent" {
				t.Fatalf("changed review did not reroute %s: %#v", stage, consumer)
			}
			if consumer.ReviewedPins["S1"] != oldPin ||
				consumer.InputPins["S1"] == oldPin {
				t.Fatalf(
					"reviewed/current product evidence was not distinct: %#v",
					consumer,
				)
			}
			prepareActionSliceBase(t, actions, release, "S2")
			redesigned := appendActionReceipt(t, actions, AppendReceiptInput{
				Release: release, Slice: "S2", Role: "implementer",
				Result: "designed", Summary: "Design S2.",
				Detail: []byte("design S2"),
			})
			if !redesigned.Changed ||
				redesigned.Receipt == nil ||
				redesigned.Receipt.Attempt == nil ||
				*redesigned.Receipt.Attempt != 2 ||
				redesigned.Receipt.Base == nil ||
				redesigned.Receipt.Inputs["S1"] !=
					consumer.InputPins["S1"] {
				t.Fatalf(
					"fresh reviewed design was mistaken for a retry: %#v",
					redesigned,
				)
			}
		})
	}
}

func TestConsumedDriftCannotOverridePlannerBlocker(t *testing.T) {
	for _, blocker := range []string{"captain", "verifier"} {
		blocker := blocker
		t.Run(blocker, func(t *testing.T) {
			repoPath, repository, actions := createActionHarness(t)
			release := "consumed-blocker-" + blocker
			s1 := actionSlice("S1", "one.txt")
			s2 := actionSlice("S2", "two.txt")
			s2.DependsOn = []string{"S1"}
			s2.Consumes = []string{"S1"}
			tracks := []Track{
				{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
				{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
			}
			recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
				Summary:   "Approve blocker fixture.", Detail: []byte("approval one"),
			})
			if err != nil {
				t.Fatal(err)
			}
			advanceActionSlice(
				t,
				actions,
				repoPath,
				release,
				"T1",
				"S1",
				"one.txt",
				1000001180,
				"pass",
			)
			state := readActionState(t, repository, release)
			approval := state.Plan.Approval
			contract := state.Plan.Metadata.Contracts["S2"]
			sliceID, attempt := "S2", int64(1)
			ownerRef := trackRef(release, "T2")
			design := appendRawActionReceipt(
				t,
				actions,
				repoPath,
				ownerRef,
				"",
				approval.OID,
				Receipt{
					Version: ReceiptVersion, Release: release,
					Slice: &sliceID, Role: "implementer",
					Result: "designed", Attempt: &attempt,
					Plan: state.Plan.OID, Contract: &contract,
					Binds: approval.OID, Detail: DigestBytes(nil),
					Summary: "Legacy review has no consumed ancestry.",
				},
				nil,
			)

			var blocked ReceiptEntry
			if blocker == "captain" {
				blocked = appendRawActionReceipt(
					t,
					actions,
					repoPath,
					ownerRef,
					design.OID,
					design.OID,
					Receipt{
						Version: ReceiptVersion, Release: release,
						Slice: &sliceID, Role: "captain",
						Result: "escalate", Attempt: &attempt,
						Plan: state.Plan.OID, Contract: &contract,
						Binds: design.OID, Detail: DigestBytes(nil),
						Summary: "Planner intervention remains required.",
					},
					nil,
				)
			} else {
				proceed := appendRawActionReceipt(
					t,
					actions,
					repoPath,
					ownerRef,
					design.OID,
					design.OID,
					Receipt{
						Version: ReceiptVersion, Release: release,
						Slice: &sliceID, Role: "captain",
						Result: "proceed", Attempt: &attempt,
						Plan: state.Plan.OID, Contract: &contract,
						Binds: design.OID, Detail: DigestBytes(nil),
						Summary: "Proceed with the legacy review.",
					},
					nil,
				)
				candidate := commitActionProduct(
					t,
					repoPath,
					proceed.OID,
					ownerRef,
					"two.txt",
					"legacy candidate\n",
					1000001190,
				)
				productTree, err := actions.repository.productTree(candidate)
				if err != nil {
					t.Fatal(err)
				}
				wrongPin := "sha256:" + strings.Repeat("f", 64)
				candidateChecks := DigestBytes([]byte("candidate checks\n"))
				candidateReceipt := appendRawActionReceipt(
					t,
					actions,
					repoPath,
					ownerRef,
					candidate,
					candidate,
					Receipt{
						Version: ReceiptVersion, Release: release,
						Slice: &sliceID, Role: "implementer",
						Result: "candidate", Attempt: &attempt,
						Plan: state.Plan.OID, Contract: &contract,
						Binds: proceed.OID, Detail: DigestBytes(nil),
						Summary:   "Legacy candidate with stale pins.",
						Candidate: &candidate, ProductTree: &productTree,
						Inputs: map[string]string{"S1": wrongPin},
						Checks: &candidateChecks,
					},
					nil,
				)
				verifierChecks := DigestBytes([]byte("blocked checks\n"))
				blocked = appendRawActionReceipt(
					t,
					actions,
					repoPath,
					ownerRef,
					candidateReceipt.OID,
					candidateReceipt.OID,
					Receipt{
						Version: ReceiptVersion, Release: release,
						Slice: &sliceID, Role: "verifier",
						Result: "blocked", Attempt: &attempt,
						Plan: state.Plan.OID, Contract: &contract,
						Binds:     candidateReceipt.OID,
						Detail:    DigestBytes(nil),
						Summary:   "Planner intervention remains required.",
						Candidate: &candidate, ProductTree: &productTree,
						Inputs: map[string]string{"S1": wrongPin},
						Checks: &verifierChecks,
					},
					nil,
				)
			}

			state = readActionState(t, repository, release)
			consumer, _ := state.Slice("S2")
			if consumer.CurrentReceipt == nil ||
				consumer.CurrentReceipt.OID != blocked.OID ||
				consumer.Status != "blocked" ||
				consumer.NextRole != "planner" ||
				consumer.Outcome != map[string]string{
					"captain":  "escalate",
					"verifier": "blocked",
				}[blocker] {
				t.Fatalf("consumed drift overrode %s blocker: %#v", blocker, consumer)
			}

			previous := recorded.Plan
			revision, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(
					release,
					2,
					&previous,
					tracks,
				),
				Summary: "Approve planner recovery.",
				Detail:  []byte("approval two"),
			})
			if err != nil {
				t.Fatal(err)
			}
			state = readActionState(t, repository, release)
			consumer, _ = state.Slice("S2")
			if consumer.Stage != "design" ||
				consumer.Status != "ready" ||
				consumer.NextRole != "implementer" ||
				consumer.Outcome != "none" ||
				consumer.Attempt != 2 ||
				consumer.CurrentReceipt == nil ||
				consumer.CurrentReceipt.OID != revision.ReceiptCommit {
				t.Fatalf("approval did not reset %s blocker: %#v", blocker, consumer)
			}
		})
	}
}

func TestPlannerBlockedOutcomesResetAcrossRevision(t *testing.T) {
	for _, result := range []string{"escalate", "blocked"} {
		result := result
		t.Run(result, func(t *testing.T) {
			repoPath, repository, actions := createActionHarness(t)
			release := "blocked-reset-" + result
			s1 := actionSlice("S1", "one.txt")
			s2 := actionSlice("S2", "two.txt")
			tracks := []Track{
				{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
				{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
			}
			recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
				Summary:   "Approve blocked stage.", Detail: []byte("approval one"),
			})
			if err != nil {
				t.Fatal(err)
			}
			appendActionReceipt(t, actions, AppendReceiptInput{
				Release: release, Slice: "S1", Role: "implementer", Result: "designed",
				Summary: "Design S1.", Detail: []byte("design S1"),
			})
			appendActionReceipt(t, actions, AppendReceiptInput{
				Release: release, Slice: "S1", Role: "captain",
				Result: func() string {
					if result == "escalate" {
						return result
					}
					return "proceed"
				}(),
				Summary: "Review S1.", Detail: []byte("review S1"),
			})
			if result == "blocked" {
				ref := trackRef(release, "T1")
				parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
				candidate := commitActionProduct(t, repoPath, parent, ref, "one.txt", "blocked\n", 1000001000)
				appendActionReceipt(t, actions, AppendReceiptInput{
					Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
					Summary: "Candidate S1.", Detail: []byte("candidate S1"),
					Candidate: candidate, CheckResults: []byte("checks S1\n"),
				})
				appendActionReceipt(t, actions, AppendReceiptInput{
					Release: release, Slice: "S1", Role: "verifier", Result: "blocked",
					Summary: "Block S1.", Detail: []byte("blocked S1"),
					Candidate: candidate, CheckResults: []byte("fresh S1\n"),
				})
			}
			advanceActionSlice(t, actions, repoPath, release, "T2", "S2", "two.txt", 1000001100, "pass")
			previous := recorded.Plan
			revision, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(release, 2, &previous, tracks),
				Summary:   "Approve recovery revision.", Detail: []byte("approval two"),
			})
			if err != nil {
				t.Fatal(err)
			}
			state := readActionState(t, repository, release)
			blocked, _ := state.Slice("S1")
			unrelated, _ := state.Slice("S2")
			if blocked.Retained || blocked.Stage != "design" ||
				blocked.NextRole != "implementer" || blocked.Outcome != "none" ||
				blocked.Attempt != 2 || blocked.CurrentReceipt == nil ||
				blocked.CurrentReceipt.OID != revision.ReceiptCommit ||
				unrelated.Pass == nil || !unrelated.Retained {
				t.Fatalf("blocked reset=%#v unrelated=%#v", blocked, unrelated)
			}
		})
	}
}

func TestVerifierFailCarriesAcrossPlanAndAcceptsExactRepair(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "retained-fail-repair"
	s1 := actionSlice("S1", "one.txt")
	s2 := actionSlice("S2", "two.txt")
	s2.Consumes = []string{"S1"}
	tracks := []Track{
		{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
		{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
	}
	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve repair flow.", Detail: []byte("approval one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000001200, "pass")
	advanceActionSlice(t, actions, repoPath, release, "T2", "S2", "two.txt", 1000001300, "fail")
	previous := recorded.Plan
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 2, &previous, tracks),
		Summary:   "Retain exact failed candidate.", Detail: []byte("approval two"),
	}); err != nil {
		t.Fatal(err)
	}
	state := readActionState(t, repository, release)
	failed, _ := state.Slice("S2")
	if !failed.Retained || failed.Stage != "implement" ||
		failed.NextRole != "implementer" || failed.Outcome != "fail" ||
		failed.Attempt != 2 || len(failed.InputPins) != 1 {
		t.Fatalf("FAIL did not carry across the unchanged plan: %#v", failed)
	}
	ref := trackRef(release, "T2")
	base := prepareActionSliceBase(t, actions, release, "S2")
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidate := commitActionProduct(t, repoPath, parent, ref, "two.txt", "S2 repaired\n", 1000001400)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S2", Role: "implementer", Result: "candidate",
		Summary: "Repair S2.", Detail: []byte("repair S2"),
		Base: base, Candidate: candidate, CheckResults: []byte("repair checks\n"),
	})
	state = readActionState(t, repository, release)
	repaired, _ := state.Slice("S2")
	if repaired.Attempt != 2 || repaired.NextRole != "verifier" ||
		repaired.Candidate == nil || repaired.Candidate.Receipt.Plan != state.Plan.OID ||
		!inputsEqual(repaired.Candidate.Receipt.Inputs, repaired.InputPins) {
		t.Fatalf("cross-plan repair candidate = %#v", repaired)
	}
}

func TestSliceLineageRejectsPredecessorGapsReversionAndTrackMoves(t *testing.T) {
	t.Run("predecessor_insertion", func(t *testing.T) {
		repoPath, repository, actions := createActionHarness(t)
		release := "predecessor-insertion"
		s0 := actionSlice("S0", "zero.txt")
		s1 := actionSlice("S1", "one.txt")
		s2 := actionSlice("S2", "two.txt")
		tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1, s2}}}
		recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
			Summary:   "Approve serial work.", Detail: []byte("approval one"),
		})
		if err != nil {
			t.Fatal(err)
		}
		advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000001500, "pass")
		advanceActionSlice(t, actions, repoPath, release, "T1", "S2", "two.txt", 1000001600, "pass")
		revised := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1, s0, s2}}}
		previous := recorded.Plan
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 2, &previous, revised),
			Summary:   "Insert serial predecessor.", Detail: []byte("approval two"),
		}); err != nil {
			t.Fatal(err)
		}
		state := readActionState(t, repository, release)
		retained, _ := state.Slice("S1")
		reset, _ := state.Slice("S2")
		if retained.Pass == nil || !retained.Retained ||
			reset.Pass != nil || reset.Candidate != nil ||
			reset.Attempt != 2 || reset.Stage != "design" ||
			reset.Status != "waiting" {
			t.Fatalf("retained=%#v reset=%#v", retained, reset)
		}
	})

	t.Run("contract_a_b_a", func(t *testing.T) {
		repoPath, repository, actions := createActionHarness(t)
		release := "contract-a-b-a"
		original := actionSlice("S1", "one.txt")
		tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{original}}}
		recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
			Summary:   "Approve contract A.", Detail: []byte("approval one"),
		})
		if err != nil {
			t.Fatal(err)
		}
		advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000001700, "pass")
		changed := original
		changed.Outcome = "Deliver contract B."
		previous := recorded.Plan
		revisionTwo, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(
				release, 2, &previous,
				[]Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{changed}}},
			),
			Summary: "Approve contract B.", Detail: []byte("approval two"),
		})
		if err != nil {
			t.Fatal(err)
		}
		previous = revisionTwo.Plan
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 3, &previous, tracks),
			Summary:   "Return to contract A.", Detail: []byte("approval three"),
		}); err != nil {
			t.Fatal(err)
		}
		state := readActionState(t, repository, release)
		slice, _ := state.Slice("S1")
		if slice.Pass != nil || slice.Retained || slice.Attempt != 2 ||
			slice.Stage != "design" || slice.NextRole != "implementer" {
			t.Fatalf("contract A facts resurrected: %#v", slice)
		}
	})

	t.Run("track_move_is_rejected_before_publication", func(t *testing.T) {
		repoPath, repository, actions := createActionHarness(t)
		release := "track-move"
		s1 := actionSlice("S1", "one.txt")
		initial := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}
		recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, initial),
			Summary:   "Approve original owner.", Detail: []byte("approval one"),
		})
		if err != nil {
			t.Fatal(err)
		}
		advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000001800, "pass")
		moved := []Track{{ID: "T2", DependsOn: []string{}, Slices: []Slice{s1}}}
		previous := recorded.Plan
		releaseBefore := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
		_, err = actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 2, &previous, moved),
			Summary:   "Move exact slice owner.", Detail: []byte("approval two"),
		})
		if ErrorCode(err) != "REPLACED_SLICE_AUTHORITY" {
			t.Fatalf("track move error = %v", err)
		}
		if actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release)) != releaseBefore {
			t.Fatal("rejected track move changed the release ref")
		}
		state := readActionState(t, repository, release)
		slice, _ := state.Slice("S1")
		if slice.Pass == nil || slice.Location.Track.ID != "T1" ||
			state.Plan.Metadata.Revision != 1 {
			t.Fatalf("rejected track move changed authority: %#v", slice)
		}
	})
}

func TestPlanRevisionRejectsUnrecordedImplementationHead(t *testing.T) {
	repoPath, _, actions := createActionHarness(t)
	release := "plan-during-implementation"
	s1 := actionSlice("S1", "one.txt")
	tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}
	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve implementation.", Detail: []byte("approval one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "Design S1.", Detail: []byte("design"),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S1", Role: "captain", Result: "proceed",
		Summary: "Proceed S1.", Detail: []byte("review"),
	})
	track := trackRef(release, "T1")
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", track)
	commitActionProduct(
		t, repoPath, parent, track,
		"one.txt", "unrecorded implementation\n", 1000001850,
	)
	releaseBefore := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
	targetBefore := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	trackBefore := actionGit(t, repoPath, nil, nil, "rev-parse", track)
	previous := recorded.Plan
	_, err = actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 2, &previous, tracks),
		Summary:   "Do not hide implementation.", Detail: []byte("approval two"),
	})
	if ErrorCode(err) != "CHANGED_OWNER_HEAD" {
		t.Fatalf("in-flight plan revision error = %v", err)
	}
	if actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release)) != releaseBefore ||
		actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main") != targetBefore ||
		actionGit(t, repoPath, nil, nil, "rev-parse", track) != trackBefore {
		t.Fatal("rejected in-flight plan revision changed an authority ref")
	}
}

func TestPlanRevisionReconcilesOnlyExactPreparedBase(t *testing.T) {
	for _, test := range []struct {
		name       string
		moveTarget bool
		moveTrack  func(*testing.T, string, string, string, string) string
		wantCode   string
	}{
		{name: "exact_prepared_base_unchanged_target"},
		{name: "exact_prepared_base_moved_target", moveTarget: true},
		{
			name:       "prepared_base_with_unrecorded_descendant",
			moveTarget: true,
			moveTrack: func(
				t *testing.T,
				repoPath, track, prepared, _ string,
			) string {
				return commitActionProduct(
					t,
					repoPath,
					prepared,
					track,
					"unrecorded.txt",
					"unrecorded implementation\n",
					1000001861,
				)
			},
			wantCode: "CHANGED_OWNER_HEAD",
		},
		{
			name:       "unrelated_unrecorded_head",
			moveTarget: true,
			moveTrack: func(
				t *testing.T,
				repoPath, track, prepared, authority string,
			) string {
				tree := actionGit(
					t,
					repoPath,
					nil,
					nil,
					"rev-parse",
					prepared+"^{tree}",
				)
				unrelated := actionGit(
					t,
					repoPath,
					[]byte("unrelated implementation\n"),
					[]string{
						"GIT_AUTHOR_DATE=1000001862 +0000",
						"GIT_COMMITTER_DATE=1000001862 +0000",
					},
					"commit-tree",
					tree,
					"-p",
					authority,
				)
				actionGit(
					t,
					repoPath,
					nil,
					nil,
					"update-ref",
					track,
					unrelated,
					prepared,
				)
				return unrelated
			},
			wantCode: "CHANGED_OWNER_HEAD",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			repoPath, repository, actions := createActionHarness(t)
			release := "target-stale-prepared-" +
				strings.ReplaceAll(test.name, "_", "-")
			producer := actionSlice("S1", "one.txt")
			consumer := actionSlice("S2", "two.txt")
			consumer.Consumes = []string{"S1"}
			tracks := []Track{
				{
					ID: "T1", DependsOn: []string{},
					Slices: []Slice{producer},
				},
				{
					ID: "T2", DependsOn: []string{},
					Slices: []Slice{consumer},
				},
			}
			recorded, err := actions.RecordPlanRevision(
				RecordPlanRevisionInput{
					PlanBytes: actionPlanRevisionBytes(
						release,
						1,
						nil,
						tracks,
					),
					Summary: "Approve prepared-base recovery.",
					Detail:  []byte("approval one"),
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			advanceActionSlice(
				t,
				actions,
				repoPath,
				release,
				"T1",
				"S1",
				"one.txt",
				1000001860,
				"pass",
			)
			trackRef := trackRef(release, "T2")
			prepared := prepareActionSliceBase(
				t,
				actions,
				release,
				"S2",
			)
			beforeMove := readActionState(t, repository, release)
			beforeTrack, ok := beforeMove.Track("T2")
			if !ok {
				t.Fatal("consumer track is absent")
			}
			beforeSlice, ok := beforeMove.Slice("S2")
			if !ok ||
				beforeSlice.Stage != "design" ||
				beforeSlice.Status != "ready" ||
				beforeSlice.NextRole != "implementer" ||
				beforeSlice.PreparedBase != prepared ||
				beforeTrack.Head != prepared ||
				beforeTrack.AuthorityHead == prepared {
				t.Fatalf(
					"prepared consumer state: track=%#v slice=%#v",
					beforeTrack,
					beforeSlice,
				)
			}
			trackBefore := prepared
			if test.moveTrack != nil {
				trackBefore = test.moveTrack(
					t,
					repoPath,
					trackRef,
					prepared,
					beforeTrack.AuthorityHead,
				)
			}
			targetParent := actionGit(
				t,
				repoPath,
				nil,
				nil,
				"rev-parse",
				"refs/heads/main",
			)
			targetMoved := targetParent
			if test.moveTarget {
				targetMoved = commitActionProduct(
					t,
					repoPath,
					targetParent,
					"refs/heads/main",
					"target-moved.txt",
					"external target movement\n",
					1000001863,
				)
			}
			releaseBefore := actionGit(
				t,
				repoPath,
				nil,
				nil,
				"rev-parse",
				releaseRef(release),
			)
			previous := recorded.Plan
			revised, err := actions.RecordPlanRevision(
				RecordPlanRevisionInput{
					PlanBytes: actionPlanRevisionBytes(
						release,
						2,
						&previous,
						tracks,
					),
					Summary: "Approve the revised plan.",
					Detail:  []byte("approval two"),
				},
			)
			if test.wantCode != "" {
				if ErrorCode(err) != test.wantCode {
					t.Fatalf("plan revision error = %v", err)
				}
				if actionGit(
					t,
					repoPath,
					nil,
					nil,
					"rev-parse",
					releaseRef(release),
				) != releaseBefore ||
					actionGit(
						t,
						repoPath,
						nil,
						nil,
						"rev-parse",
						trackRef,
					) != trackBefore ||
					actionGit(
						t,
						repoPath,
						nil,
						nil,
						"rev-parse",
						"refs/heads/main",
					) != targetMoved {
					t.Fatal(
						"rejected plan revision changed an authority ref",
					)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after := readActionState(t, repository, release)
			afterTrack, ok := after.Track("T2")
			afterSlice, sliceOK := after.Slice("S2")
			if !ok || !sliceOK ||
				after.Plan.Metadata.Revision != 2 ||
				after.Plan.TargetStale ||
				after.Refs.Target.Head != targetMoved ||
				afterTrack.Head != afterTrack.AuthorityHead ||
				afterTrack.Head != revised.Head ||
				afterTrack.Head == prepared ||
				afterSlice.Stage != "design" ||
				afterSlice.Status != "ready" ||
				afterSlice.NextRole != "implementer" {
				t.Fatalf(
					"reconciled plan: plan=%#v track=%#v slice=%#v result=%#v",
					after.Plan,
					afterTrack,
					afterSlice,
					revised,
				)
			}
			nextBase, err := preparedStateTrackBase(
				actions.repository,
				after,
				afterSlice,
			)
			if err != nil {
				t.Fatal(err)
			}
			if nextBase == prepared {
				t.Fatal("revised target reused the stale prepared base")
			}
			actionGit(
				t,
				repoPath,
				nil,
				nil,
				"update-ref",
				trackRef,
				nextBase,
				afterTrack.Head,
			)
			reprepared := readActionState(t, repository, release)
			repreparedSlice, _ := reprepared.Slice("S2")
			if repreparedSlice.PreparedBase != nextBase {
				t.Fatalf(
					"reprepared base = %q, want %q",
					repreparedSlice.PreparedBase,
					nextBase,
				)
			}
			advanceActionSlice(
				t,
				actions,
				repoPath,
				release,
				"T2",
				"S2",
				"two.txt",
				1000001864,
				"pass",
			)
			assembly, err := actions.PrepareAssembly(
				PrepareAssemblyInput{
					Release: release,
					Summary: "Prepare revised-target assembly.",
					Detail:  []byte("assembly"),
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			firstParent := strings.Fields(actionGit(
				t,
				repoPath,
				nil,
				nil,
				"rev-list",
				"--first-parent",
				assembly.Candidate,
			))
			foundRevisionAuthority := false
			for _, commit := range firstParent {
				foundRevisionAuthority =
					foundRevisionAuthority || commit == revised.Head
			}
			if assembly.Direct || !assembly.Changed ||
				!foundRevisionAuthority {
				t.Fatalf(
					"revised assembly omitted release authority: %#v history=%v",
					assembly,
					firstParent,
				)
			}
			appendActionReceipt(
				t,
				actions,
				AppendReceiptInput{
					Release:   release,
					Role:      "verifier",
					Result:    "pass",
					Summary:   "Pass revised-target assembly.",
					Detail:    []byte("fresh assembly verification"),
					Candidate: assembly.Candidate,
					CheckResults: []byte(
						"assembly checks\n",
					),
				},
			)
			if _, err := actions.MergePassedCandidate(
				MergePassedCandidateInput{
					Release: release,
					Summary: "Merge revised-target assembly.",
					Detail:  []byte("merge"),
				},
			); err != nil {
				t.Fatal(err)
			}
			complete := readActionState(t, repository, release)
			if complete.Assembly.Status != "complete" ||
				complete.Assembly.Outcome != "merged" {
				t.Fatalf(
					"revised-target assembly did not complete: %#v",
					complete.Assembly,
				)
			}
		})
	}
}

func TestPlanRevisionRejectsPreexistingProposedTrack(t *testing.T) {
	repoPath, _, actions := createActionHarness(t)
	release := "plan-existing-new-track"
	s1 := actionSlice("S1", "one.txt")
	initial := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}
	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, initial),
		Summary:   "Approve first track.", Detail: []byte("approval one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignRef := trackRef(release, "T2")
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
	actionGit(t, repoPath, nil, nil, "update-ref", foreignRef, parent)
	commitActionProduct(
		t, repoPath, parent, foreignRef,
		"foreign.txt", "foreign authority\n", 1000001875,
	)
	releaseBefore := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
	targetBefore := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	foreignBefore := actionGit(t, repoPath, nil, nil, "rev-parse", foreignRef)
	previous := recorded.Plan
	revised := []Track{
		{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
		{ID: "T2", DependsOn: []string{}, Slices: []Slice{actionSlice("S2", "two.txt")}},
	}
	_, err = actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 2, &previous, revised),
		Summary:   "Do not adopt foreign authority.", Detail: []byte("approval two"),
	})
	if ErrorCode(err) != "AMBIGUOUS_AUTHORITY" {
		t.Fatalf("pre-existing proposed track error = %v", err)
	}
	if actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release)) != releaseBefore ||
		actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main") != targetBefore ||
		actionGit(t, repoPath, nil, nil, "rev-parse", foreignRef) != foreignBefore {
		t.Fatal("rejected proposed track changed an authority ref")
	}
}

func TestAssemblyDirectReuseRequiresExactComposition(t *testing.T) {
	t.Run("one_track_two_slices", func(t *testing.T) {
		repoPath, repository, actions := createActionHarness(t)
		release := "serial-assembly"
		s1 := actionSlice("S1", "one.txt")
		s2 := actionSlice("S2", "two.txt")
		tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1, s2}}}
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
			Summary:   "Approve serial assembly.", Detail: []byte("approval one"),
		}); err != nil {
			t.Fatal(err)
		}
		advanceActionSlice(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000001900, "pass")
		finalCandidate := advanceActionSlice(
			t, actions, repoPath, release, "T1", "S2",
			"two.txt", 1000002000, "pass",
		)
		beforeAssembly, err := actions.stateFor(release)
		if err != nil {
			t.Fatal(err)
		}
		finalSlice, ok := beforeAssembly.Slice("S2")
		if !ok || finalSlice.Pass == nil {
			t.Fatal("serial track has no final PASS authority")
		}
		prepared, err := actions.PrepareAssembly(PrepareAssemblyInput{
			Release: release, Summary: "Prepare serial assembly.", Detail: []byte("assembly"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !prepared.Changed || prepared.Direct ||
			prepared.ReceiptCommit == "" {
			t.Fatalf("serial track bypassed required assembly: %#v", prepared)
		}
		preparedProduct, err := actions.repository.productTree(prepared.Candidate)
		if err != nil {
			t.Fatal(err)
		}
		finalProduct, err := actions.repository.productTree(finalCandidate)
		if err != nil {
			t.Fatal(err)
		}
		containsPass, err := actions.repository.isAncestor(
			finalSlice.Pass.OID,
			prepared.Candidate,
		)
		if err != nil {
			t.Fatal(err)
		}
		if preparedProduct != finalProduct || !containsPass {
			t.Fatalf(
				"serial assembly product/authority = %s/%t, want %s/true",
				preparedProduct,
				containsPass,
				finalProduct,
			)
		}
		track := trackRef(release, "T1")
		trackBefore := actionGit(t, repoPath, nil, nil, "rev-parse", track)
		rogue := commitActionProduct(
			t, repoPath, trackBefore, track,
			"rogue.txt", "unrecorded topology drift\n", 1000002050,
		)
		targetBefore := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
		releaseBefore := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
		if _, err := actions.MergePassedCandidate(MergePassedCandidateInput{
			Release: release, Summary: "Reject topology drift.", Detail: []byte("merge"),
		}); ErrorCode(err) != "CHANGED_CANDIDATE" {
			t.Fatalf("topology drift error = %v", err)
		}
		if actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main") != targetBefore ||
			actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release)) != releaseBefore {
			t.Fatal("topology drift moved release or target refs")
		}
		actionGit(t, repoPath, nil, nil, "update-ref", track, trackBefore, rogue)
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Role: "verifier", Result: "pass",
			Summary: "Pass serial assembly.", Detail: []byte("fresh assembly verification"),
			Candidate: prepared.Candidate, CheckResults: []byte("assembly checks\n"),
		})
		if _, err := actions.MergePassedCandidate(MergePassedCandidateInput{
			Release: release, Summary: "Merge serial assembly.", Detail: []byte("merge"),
		}); err != nil {
			t.Fatal(err)
		}
		state := readActionState(t, repository, release)
		if state.Assembly.Status != "complete" || state.Assembly.Outcome != "merged" {
			t.Fatalf("serial assembly did not complete: %#v", state.Assembly)
		}
	})

	t.Run("retained_one_slice_can_use_direct_pass", func(t *testing.T) {
		repoPath, _, actions := createActionHarness(t)
		release := "retired-assembly"
		s1 := actionSlice("S1", "one.txt")
		s2 := actionSlice("S2", "two.txt")
		initial := []Track{
			{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
			{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
		}
		recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, initial),
			Summary:   "Approve two slice identities.", Detail: []byte("approval one"),
		})
		if err != nil {
			t.Fatal(err)
		}
		finalCandidate := advanceActionSlice(
			t, actions, repoPath, release, "T1", "S1",
			"one.txt", 1000002100, "pass",
		)
		advanceActionSlice(t, actions, repoPath, release, "T2", "S2", "two.txt", 1000002200, "pass")
		previous := recorded.Plan
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(
				release, 2, &previous,
				[]Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}},
			),
			Summary: "Retire S2.", Detail: []byte("approval two"),
		}); err != nil {
			t.Fatal(err)
		}
		prepared, err := actions.PrepareAssembly(PrepareAssemblyInput{
			Release: release, Summary: "Prepare retained release.", Detail: []byte("assembly"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if prepared.Changed || !prepared.Direct ||
			prepared.Candidate != finalCandidate ||
			prepared.ReceiptCommit == "" {
			t.Fatalf("retained one-slice PASS was not direct: %#v", prepared)
		}
		merged, err := actions.MergePassedCandidate(MergePassedCandidateInput{
			Release: release, Summary: "Merge retained direct PASS.",
			Detail: []byte("merge"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if merged.ResultCommit != finalCandidate {
			t.Fatalf("retained direct merge = %#v", merged)
		}
	})
}
