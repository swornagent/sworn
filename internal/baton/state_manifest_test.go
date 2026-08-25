package baton

import (
	"encoding/json"
	"testing"
)

// TestReadStateManifestRereadsContractsFromReleaseHead proves the
// repository plan/release discovery owner (ReadState/readState, via
// planAt+resolveManifestContracts) does not merely trust an admitted
// sworn.release-manifest/v1 manifest's declared digests: every read
// re-resolves every declared contract from the digest-addressed store in
// the record root at the exact captured release head (the record commit
// lineage, which always contains the record root), using the same
// ParsePlan/ResolveSliceContract authority RecordPlanRevision already uses
// at write time.
func TestReadStateManifestRereadsContractsFromReleaseHead(t *testing.T) {
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
	// cleanly from the release head's digest store.
	again := readActionState(t, repository, release)
	if again.Plan.Metadata.SchemaVersion != ManifestVersion {
		t.Fatal("second read lost manifest identity")
	}
}

// TestReadStateManifestResolvesFromDigestStoreNotTargetPath proves that
// resolution reads contract bytes from the digest-addressed store at the
// release head, not from the target path: even when the target path's
// contract bytes are substituted (stale), resolution succeeds because the
// digest store has the correct bytes.
func TestReadStateManifestResolvesFromDigestStoreNotTargetPath(t *testing.T) {
	t.Parallel()
	repoPath, repository, actions := createActionHarness(t)
	release := "manifest-digest-store"
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

	// Substitute the target-path contract bytes after recording. The digest
	// store at the release head still has the correct bytes, so resolution
	// succeeds despite the target-path drift.
	substituted := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	var substitutedValue map[string]any
	if err := json.Unmarshal(substituted, &substitutedValue); err != nil {
		t.Fatal(err)
	}
	substitutedValue["outcome"] = "Deliver a different outcome after admission."
	substitutedRaw := manifestContractRaw(t, substitutedValue)
	drifted := prepareActionContractTree(t, repoPath, withContract, map[string][]byte{contractPath: substitutedRaw})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", drifted, withContract)

	// Resolution succeeds from the digest store despite the target-path drift.
	state := readActionState(t, repository, release)
	if state.Plan.Metadata.SchemaVersion != ManifestVersion {
		t.Fatalf("schema_version = %q, want %q", state.Plan.Metadata.SchemaVersion, ManifestVersion)
	}
	if state.Plan.Metadata.Contracts["S1"] != digest {
		t.Fatalf("contract digest = %q, want %q", state.Plan.Metadata.Contracts["S1"], digest)
	}
	_ = result
}

// TestReadStateManifestResolvesFromDigestStoreWithMovedPath proves A1's
// "from a moved path" pin: the contract_path in the manifest is changed to a
// new location (the file physically moved), but the digest is unchanged and
// the digest-addressed store has the bytes. Resolution succeeds from the
// digest store regardless of where the path points.
func TestReadStateManifestResolvesFromDigestStoreWithMovedPath(t *testing.T) {
	t.Parallel()
	repoPath, repository, actions := createActionHarness(t)
	release := "manifest-moved-path"
	contractPath := "contracts/S1.json"
	movedPath := "contracts/moved/S1.json"
	contractRaw := manifestContractRaw(t, manifestContractBody("S1", "one.txt"))
	_, digest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	base := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
	withContract := prepareActionContractTree(t, repoPath, base, map[string][]byte{contractPath: contractRaw})
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", withContract, base)

	// Record with the original contract path.
	planBytes := manifestActionPlanBytes(t, release, contractPath, "one.txt", digest, []any{})
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes, ContractTree: withContract,
		Summary: "Approve manifest revision.", Detail: []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("record plan revision: result = %#v, err = %v", result, err)
	}

	// Move the contract file to a new path in the target tree. The digest
	// store at the release head still has the bytes, so resolution succeeds
	// regardless of where the target-path points.
	withMoved := prepareActionContractTree(t, repoPath, withContract, map[string][]byte{movedPath: contractRaw})
	// Also remove the old path.
	bareTree := actionGit(t, repoPath, nil, nil, "rev-parse", withMoved+"^{tree}")
	indexEnv := []string{"GIT_INDEX_FILE=" + t.TempDir() + "/index"}
	actionGit(t, repoPath, nil, indexEnv, "read-tree", bareTree)
	actionGit(t, repoPath, nil, indexEnv, "update-index", "--force-remove", "--", contractPath)
	newTree := actionGit(t, repoPath, nil, indexEnv, "write-tree")
	movedCommit := actionGit(t, repoPath, []byte("move contract\n"), nil, "commit-tree", newTree, "-p", withMoved)
	actionGit(t, repoPath, nil, nil, "update-ref", "refs/heads/main", movedCommit, withContract)

	// Resolution succeeds from the digest store despite the moved path.
	state := readActionState(t, repository, release)
	if state.Plan.Metadata.SchemaVersion != ManifestVersion {
		t.Fatalf("schema_version = %q, want %q", state.Plan.Metadata.SchemaVersion, ManifestVersion)
	}
	if state.Plan.Metadata.Contracts["S1"] != digest {
		t.Fatalf("contract digest = %q, want %q", state.Plan.Metadata.Contracts["S1"], digest)
	}
	_ = result
}

// TestReadStateManifestFailsClosedOnDriftedDigestStore proves discovery
// fails closed, on every subsequent read, if the digest-addressed contract
// store in the record root at the release head is substituted with stale
// bytes after admission. The digest store is the authority; the target path
// is provenance. A substituted digest store fails closed with STALE_BINDING
// even though the target path still has the correct bytes — this proves the
// digest store is the resolution authority, not the target path.
func TestReadStateManifestFailsClosedOnDriftedDigestStore(t *testing.T) {
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

	t.Run("digest_store_substituted", func(t *testing.T) {
		t.Parallel()
		repoPath, repository, actions := createActionHarness(t)
		release := "manifest-read-drift-digest-store-substituted"
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

		// A real state read still succeeds against the unmoved release head.
		if _, err := ReadState(UseGitRepository(repository), release, inertActionResolver); err != nil {
			t.Fatalf("read before drift: %v", err)
		}

		// Substitute the digest store at the release head with stale bytes.
		// The target path still has the correct bytes, but the digest store
		// is the authority, so resolution fails with STALE_BINDING.
		storePath := contractStorePath(RecordRoot, release, digest)
		driftedRelease := prepareActionContractTree(t, repoPath, result.Head, map[string][]byte{storePath: substitutedRaw})
		actionGit(t, repoPath, nil, nil, "update-ref", releaseRef(release), driftedRelease, result.Head)

		_, err = ReadState(UseGitRepository(repository), release, inertActionResolver)
		if code := ErrorCode(err); code != "STALE_BINDING" {
			t.Fatalf("code = %q (%v), want STALE_BINDING", code, err)
		}
	})
}

// TestReadStateManifestFallsBackToTargetPathWhenDigestStoreMissing proves
// the backward-compatibility fallback: when the digest-addressed store is
// absent at the release head (as for releases recorded before this slice),
// resolution falls back to path-keyed reading from the target head and
// succeeds.
func TestReadStateManifestFallsBackToTargetPathWhenDigestStoreMissing(t *testing.T) {
	t.Parallel()
	repoPath, repository, actions := createActionHarness(t)
	release := "manifest-fallback-target-path"
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

	// Remove the digest store from the release head, simulating a legacy
	// release recorded before this slice.
	storePath := contractStorePath(RecordRoot, release, digest)
	indexEnv := []string{"GIT_INDEX_FILE=" + t.TempDir() + "/index"}
	actionGit(t, repoPath, nil, indexEnv, "read-tree", result.Head+"^{tree}")
	actionGit(t, repoPath, nil, indexEnv, "update-index", "--force-remove", "--", storePath)
	newTree := actionGit(t, repoPath, nil, indexEnv, "write-tree")
	driftedRelease := actionGit(t, repoPath, []byte("remove digest store\n"), nil, "commit-tree", newTree, "-p", result.Head)
	actionGit(t, repoPath, nil, nil, "update-ref", releaseRef(release), driftedRelease, result.Head)

	// Resolution falls back to the target path and succeeds.
	state := readActionState(t, repository, release)
	if state.Plan.Metadata.SchemaVersion != ManifestVersion {
		t.Fatalf("schema_version = %q, want %q", state.Plan.Metadata.SchemaVersion, ManifestVersion)
	}
	if state.Plan.Metadata.Contracts["S1"] != digest {
		t.Fatalf("contract digest = %q, want %q", state.Plan.Metadata.Contracts["S1"], digest)
	}
}