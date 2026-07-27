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

func advanceActionSlice(
	t *testing.T,
	actions *Actions,
	repoPath, release, track, slice, file string,
	timestamp int64,
	verdict string,
) string {
	t.Helper()
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "implementer", Result: "designed",
		Summary: "Design " + slice + ".", Detail: []byte("design " + slice),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "captain", Result: "proceed",
		Summary: "Proceed " + slice + ".", Detail: []byte("review " + slice),
	})
	ref := "refs/heads/track/" + release + "/" + track
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidate := commitActionProduct(t, repoPath, parent, ref, file, slice+"\n", timestamp)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "implementer", Result: "candidate",
		Summary: "Candidate " + slice + ".", Detail: []byte("implementation " + slice),
		Candidate: candidate, CheckResults: []byte("checks " + slice + "\n"),
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

func TestUnavailableConsumedPassWaitsWithoutClaimingChange(t *testing.T) {
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
	state := readActionState(t, repository, release)
	slice, _ := state.Slice("S2")
	if slice.Candidate == nil || slice.Pass != nil || slice.Retained ||
		slice.Stage != "verify" || slice.Status != "waiting" ||
		slice.NextRole != "none" || slice.Outcome != "none" ||
		slice.StaleReason != "" {
		t.Fatalf("unavailable consumed PASS claimed a change: %#v", slice)
	}
}

func TestPinlessCrossPlanStagesResetOnlyWhenTheyConsume(t *testing.T) {
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
			revision, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(release, 2, &previous, tracks),
				Summary:   "Approve unchanged revision.", Detail: []byte("approval two"),
			})
			if err != nil {
				t.Fatal(err)
			}
			state := readActionState(t, repository, release)
			slice, _ := state.Slice("S2")
			if test.consumes {
				if slice.Retained || slice.Stage != "design" ||
					slice.NextRole != "implementer" || slice.Outcome != "none" ||
					slice.Attempt != 2 || slice.CurrentReceipt == nil ||
					slice.CurrentReceipt.OID != revision.ReceiptCommit {
					t.Fatalf("consuming pinless stage was retained: %#v", slice)
				}
				return
			}
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
				t.Fatalf("non-consuming stage was not retained: %#v", slice)
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
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidate := commitActionProduct(t, repoPath, parent, ref, "two.txt", "S2 repaired\n", 1000001400)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: "S2", Role: "implementer", Result: "candidate",
		Summary: "Repair S2.", Detail: []byte("repair S2"),
		Candidate: candidate, CheckResults: []byte("repair checks\n"),
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
		prepared, err := actions.PrepareAssembly(PrepareAssemblyInput{
			Release: release, Summary: "Prepare serial assembly.", Detail: []byte("assembly"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if prepared.Changed || !prepared.Direct ||
			prepared.Candidate != finalCandidate ||
			prepared.ReceiptCommit == "" {
			t.Fatalf("serial final PASS was not reused directly: %#v", prepared)
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
		}); ErrorCode(err) != "CHANGED_OWNER_HEAD" {
			t.Fatalf("topology drift error = %v", err)
		}
		if actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main") != targetBefore ||
			actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release)) != releaseBefore {
			t.Fatal("topology drift moved release or target refs")
		}
		actionGit(t, repoPath, nil, nil, "update-ref", track, trackBefore, rogue)
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

	t.Run("release_history_divergence_requires_assembly_verification", func(t *testing.T) {
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
		if !prepared.Changed || prepared.Direct ||
			prepared.Candidate == finalCandidate ||
			prepared.ReceiptCommit == "" {
			t.Fatalf("divergent release history reused the work PASS: %#v", prepared)
		}
		_, err = actions.MergePassedCandidate(MergePassedCandidateInput{
			Release: release, Summary: "Do not merge without assembly PASS.",
			Detail: []byte("merge"),
		})
		if ErrorCode(err) != "ASSEMBLY_PASS_REQUIRED" {
			t.Fatalf("merge-before-assembly-pass error = %v", err)
		}
	})
}
