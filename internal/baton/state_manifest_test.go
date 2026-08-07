package baton

import (
	"encoding/json"
	"testing"
)

// TestReadStateManifestRereadsContractsFromCapturedTarget proves the
// repository plan/release discovery owner (ReadState/readState, via
// planAt+resolveManifestContracts) does not merely trust an admitted
// sworn.release-manifest/v1 manifest's declared digests: every read
// re-resolves every declared contract_path against the exact captured
// target ref head, using the same ParsePlan/ResolveSliceContract authority
// RecordPlanRevision already uses at write time.
func TestReadStateManifestRereadsContractsFromCapturedTarget(t *testing.T) {
	t.Parallel()
	repoPath, repository, actions := createActionHarness(t)
	release := "manifest-read-atomic"
	contractPath := "contracts/S1.json"
	contractRaw := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	withContract := prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: contractRaw})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContract, base)

	planBytes := manifestActionPlanBytes(t, release, contractPath, "one.txt", digest, []any{})
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes, ContractTree: withContract,
		Summary: "Approve manifest revision.", Detail: []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("record plan revision: result = %#v, err = %v", result, err)
	}

	state := readActionState(t, repository, release)
	if state.Plan.Metadata.SchemaVersion != ManifestVersion {
		t.Fatalf("schema_version = %q, want %q", state.Plan.Metadata.SchemaVersion, ManifestVersion)
	}
	if state.Plan.Metadata.Contracts["S1"] != digest {
		t.Fatalf("contract digest = %q, want %q", state.Plan.Metadata.Contracts["S1"], digest)
	}

	// Reread again: a normal state read is idempotent and keeps resolving
	// cleanly against the same unchanged target tree.
	again := readActionState(t, repository, release)
	if again.Plan.Metadata.SchemaVersion != ManifestVersion {
		t.Fatal("second read lost manifest identity")
	}
}

// TestReadStateManifestFailsClosedOnDriftedContract proves discovery fails
// closed, on every subsequent read, if the target ref's committed contract
// content drifts away from the manifest's declaration after admission --
// not only at RecordPlanRevision time. This is what makes repository
// discovery itself fail-closed, independent of write-time admission.
func TestReadStateManifestFailsClosedOnDriftedContract(t *testing.T) {
	t.Parallel()
	contractPath := "contracts/S1.json"
	contractRaw := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	substituted := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	var substitutedValue map[string]any
	if err := json.Unmarshal(substituted, &substitutedValue); err != nil {
		t.Fatal(err)
	}
	substitutedValue["outcome"] = "Deliver a different outcome after admission."
	substitutedRaw := manifestContractRaw(t, substitutedValue)

	cases := []struct {
		name string
		// mutateTree receives the original pre-contract base commit (whose
		// tree has no contract file at all) and the admitted withContract
		// commit, and returns a new commit descended from withContract that
		// drifts the target ref away from what was admitted.
		mutateTree func(t *testing.T, repoPath, base, withContract string) string
		code       string
	}{
		{
			name: "contract_deleted",
			mutateTree: func(t *testing.T, repoPath, base, withContract string) string {
				bareTree := actionGit(t, repoPath, nil, nil, "rev-parse", base+"^{tree}")
				return actionGit(t, repoPath, []byte("remove contract\n"), nil, "commit-tree", bareTree, "-p", withContract)
			},
			code: "CONTRACT_NOT_FOUND",
		},
		{
			name: "contract_substituted",
			mutateTree: func(t *testing.T, repoPath, base, withContract string) string {
				return prepareActionContractTree(t, repoPath, withContract, map[string][]byte{contractPath: substitutedRaw})
			},
			code: "STALE_BINDING",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoPath, repository, actions := createActionHarness(t)
			release := "manifest-read-drift-" + test.name
			base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
			withContract := prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: contractRaw})
			actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContract, base)

			planBytes := manifestActionPlanBytes(t, release, contractPath, "one.txt", digest, []any{})
			result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: planBytes, ContractTree: withContract,
				Summary: "Approve manifest revision.", Detail: []byte("approval"),
			})
			if err != nil || !result.Changed {
				t.Fatalf("record plan revision: result = %#v, err = %v", result, err)
			}

			// A real state read still succeeds against the unmoved target.
			if _, err := ReadState(UseGitRepository(repository), release, inertActionResolver); err != nil {
				t.Fatalf("read before drift: %v", err)
			}

			drifted := test.mutateTree(t, repoPath, base, withContract)
			actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", drifted, withContract)

			_, err = ReadState(UseGitRepository(repository), release, inertActionResolver)
			if code := ErrorCode(err); code != test.code {
				t.Fatalf("code = %q (%v), want %q", code, err, test.code)
			}
		})
	}
}
