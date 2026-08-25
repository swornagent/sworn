package baton

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// PinManifestInput admits one exact manifest and an optional source for
// reading contract bytes. When Commit is non-empty, contract bytes are read
// from that Git commit via the admitted read-only repository handle. When
// Commit is empty, contract bytes are read from the working tree rooted at
// the repository root (Repository.Root()), mirroring what an operator's
// uncommitted edit produces. In both cases the contract bytes are parsed by
// ParseSliceContract, whose recomputed canonical digest, outcome,
// dependencies, consumed products, touchpoints, and waivers replace the
// manifest's mirrored slice entries. All manifest metadata fields and all
// trailing Markdown prose are preserved; only the slice entries are
// rewritten. The result round-trips through ParsePlan before it is
// returned, so a pinned manifest is always re-admissible. Pinning an
// already-pinned manifest returns identical bytes.
type PinManifestInput struct {
	ManifestBytes []byte
	Repository    GitRepository
	Commit        string
}

// PinManifest recomputes every manifest-mirrored slice fact from contract
// bytes and returns the rewritten manifest. It derives, never relaxes: the
// six facts it writes are exactly what ResolveSliceContract's six
// STALE_BINDING checks prove, and no contract byte is edited.
func PinManifest(input PinManifestInput) ([]byte, error) {
	if input.Repository.value == nil {
		return nil, recordFail("INVALID_REPOSITORY", "one admitted Git repository is required")
	}
	parsed, err := ParsePlan(input.ManifestBytes)
	if err != nil {
		return nil, err
	}
	metadata := parsed.Metadata()
	if metadata.SchemaVersion != ManifestVersion {
		return nil, recordFail("INVALID_PLAN_FENCE", "pin requires a sworn.release-manifest/v1 manifest")
	}

	repoRoot := input.Repository.value.Root()
	contractFacts := make(map[string]sliceContractFacts)
	for _, track := range metadata.Tracks {
		for _, slice := range track.Slices {
			if slice.ContractPath == "" {
				continue
			}
			raw, err := readContractBytes(input.Repository, repoRoot, slice.ContractPath, input.Commit)
			if err != nil {
				return nil, err
			}
			contract, digest, err := ParseSliceContract(raw, slice.ID, track.ID)
			if err != nil {
				return nil, err
			}
			contractFacts[slice.ID] = sliceContractFacts{
				digest:      digest,
				outcome:     contract.Outcome,
				dependsOn:   contract.DependsOn,
				consumes:    contract.Consumes,
				touchpoints: contract.Scope.Include,
				waivers:     contract.Scope.Waivers,
			}
		}
	}

	jsonBlock, err := buildPinnedManifestJSON(metadata, contractFacts)
	if err != nil {
		return nil, err
	}
	result := append(append([]byte(manifestOpen), jsonBlock...), []byte(manifestClose+parsed.Markdown())...)

	// Prove the pinned manifest re-admits and that metadata and prose are
	// stable: pinning an already-pinned manifest returns identical bytes,
	// and the metadata fields and trailing Markdown are unchanged.
	reparsed, err := ParsePlan(result)
	if err != nil {
		return nil, err
	}
	if !metadataEqual(reparsed.Metadata(), metadata) {
		return nil, recordFail("INVALID_PLAN_REVISION", "pin changed manifest metadata")
	}
	if reparsed.Markdown() != parsed.Markdown() {
		return nil, recordFail("INVALID_PLAN_REVISION", "pin changed trailing Markdown prose")
	}
	return result, nil
}

type sliceContractFacts struct {
	digest      string
	outcome     string
	dependsOn   []string
	consumes    []string
	touchpoints []string
	waivers     []ScopeWaiver
}

// readContractBytes reads one contract's bytes from the chosen source: the
// working tree when commit is empty, or the named Git commit otherwise.
func readContractBytes(repo GitRepository, repoRoot, contractPath, commit string) ([]byte, error) {
	if commit != "" {
		body, present, err := readGitFileAt(repo, commit, contractPath)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, recordFail("CONTRACT_NOT_FOUND", "contract source is missing "+contractPath)
		}
		return body, nil
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(contractPath))
	body, err := os.ReadFile(abs)
	if err != nil {
		return nil, recordWrap("CONTRACT_NOT_FOUND", "read contract "+contractPath, err)
	}
	return body, nil
}

// buildPinnedManifestJSON encodes the manifest JSON block with derived slice
// entries. The metadata fields are carried through from the parsed manifest
// unchanged. Each slice entry's six mirrored facts (digest, outcome,
// depends_on, consumes, touchpoints, waivers) are replaced with the
// contract-derived values. Waivers is omitted when empty, matching
// parseWaivers's absent/empty equivalence. encoding/json marshals keys in
// alphabetical order, which the strict parser accepts since exactKeys is
// set-based and key order is a readability choice, not an admission
// requirement (Correction 3).
func buildPinnedManifestJSON(metadata Metadata, facts map[string]sliceContractFacts) ([]byte, error) {
	obj := map[string]any{
		"schema_version": metadata.SchemaVersion,
		"release":        metadata.Release,
		"revision":       metadata.Revision,
		"previous_plan":  nil,
		"repository":     metadata.Repository,
		"target_ref":     metadata.TargetRef,
		"approval_ref":   metadata.ApprovalRef,
	}
	if metadata.PreviousPlan != nil {
		obj["previous_plan"] = *metadata.PreviousPlan
	}

	tracks := make([]any, 0, len(metadata.Tracks))
	for _, track := range metadata.Tracks {
		slices := make([]any, 0, len(track.Slices))
		for _, slice := range track.Slices {
			f, derived := facts[slice.ID]
			// When contract facts are derived (the slice has a
			// contract_path), all six mirrored facts come strictly
			// from the contract bytes — including a nil/empty
			// waivers, which means the contract declares none and any
			// stale manifest waiver must be dropped. Per-field nil
			// fallbacks are wrong here: parseWaivers returns nil (not
			// a non-nil empty slice) for an empty waivers list, unlike
			// uniqueStringList, so a nil check cannot distinguish
			// "contract has no waivers" from "no contract was read".
			// Only slices without a contract_path (unreachable for
			// admitted manifests, since contract_path is required)
			// fall back to the manifest's own values.
			outcome := slice.Outcome
			digest, hasDigest := metadata.Contracts[slice.ID]
			if !hasDigest {
				digest = ""
			}
			dependsOn := slice.DependsOn
			consumes := slice.Consumes
			touchpoints := slice.Scope.Include
			waivers := slice.Scope.Waivers
			if derived {
				outcome = f.outcome
				digest = f.digest
				dependsOn = f.dependsOn
				consumes = f.consumes
				touchpoints = f.touchpoints
				waivers = f.waivers
			}
			entry := map[string]any{
				"id":            slice.ID,
				"outcome":       outcome,
				"contract_path": slice.ContractPath,
				"digest":        digest,
				"depends_on":    dependsOn,
				"consumes":      consumes,
				"touchpoints":   touchpoints,
			}
			if len(waivers) > 0 {
				waiverList := make([]any, 0, len(waivers))
				for _, w := range waivers {
					waiverList = append(waiverList, map[string]any{
						"package": w.Package,
						"reason":  w.Reason,
					})
				}
				entry["waivers"] = waiverList
			}
			slices = append(slices, entry)
		}
		tracks = append(tracks, map[string]any{
			"id":         track.ID,
			"depends_on": track.DependsOn,
			"slices":     slices,
		})
	}
	obj["tracks"] = tracks

	encoded, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, recordWrap("INVALID_JSON", "encode pinned manifest", err)
	}
	encoded = append(encoded, '\n')
	return encoded, nil
}

// metadataEqual compares two Metadata values for the fields pin must preserve:
// schema_version, release, revision, previous_plan, repository, target_ref,
// and approval_ref. Track structure and contracts are derived from the slice
// entries and are not compared here (they are the rewritten part).
func metadataEqual(a, b Metadata) bool {
	if a.SchemaVersion != b.SchemaVersion || a.Release != b.Release ||
		a.Revision != b.Revision || a.Repository != b.Repository ||
		a.TargetRef != b.TargetRef || a.ApprovalRef != b.ApprovalRef {
		return false
	}
	if (a.PreviousPlan == nil) != (b.PreviousPlan == nil) {
		return false
	}
	if a.PreviousPlan != nil && b.PreviousPlan != nil && *a.PreviousPlan != *b.PreviousPlan {
		return false
	}
	return true
}

// RunPlanScopeLintInput admits one exact manifest and a mode for reading
// contract bytes and running the scope lint. When Commit is non-empty,
// contracts are resolved from that Git commit via ResolveSliceContractAt and
// the lint runs against that commit via ValidatePlanScopeLintAt. When Commit
// is empty (working-tree mode), each contract_path is read from the working
// tree, proved via ResolveSliceContract (the pure-bytes prover), and the lint
// runs against the working-tree filesystem via ValidatePlanScopeLintFS.
type RunPlanScopeLintInput struct {
	ManifestBytes []byte
	Repository    GitRepository
	Commit        string
}

// LintPlanSliceResult names one slice's lint outcome.
type LintPlanSliceResult struct {
	Slice  string
	Status string
	Code   string
	Paths  []string
}

// RunPlanScopeLint parses the manifest, resolves every slice contract against
// the chosen source, and runs the recording-time scope lint. On success it
// returns one PASS result per slice. On failure it returns the structured
// UNDER_DERIVED_SCOPE result with the missing paths. The name is distinct from
// (*PackageGraph).LintPlan to avoid the receiver-collision that reads badly in
// this package.
func RunPlanScopeLint(input RunPlanScopeLintInput) ([]LintPlanSliceResult, error) {
	if input.Repository.value == nil {
		return nil, recordFail("INVALID_REPOSITORY", "one admitted Git repository is required")
	}
	parsed, err := ParsePlan(input.ManifestBytes)
	if err != nil {
		return nil, err
	}
	metadata := parsed.Metadata()
	if metadata.SchemaVersion != ManifestVersion {
		return nil, recordFail("INVALID_PLAN_FENCE", "lint requires a sworn.release-manifest/v1 manifest")
	}

	repoRoot := input.Repository.value.Root()

	// Resolve every slice contract against the chosen source so the lint
	// sees the same contract the engine would. In working-tree mode the
	// contract bytes are read from the working tree (the operator's
	// uncommitted edit) and proved via ResolveSliceContract, mirroring pin's
	// own working-tree read. In commit mode the S1 digest-addressed
	// resolution path is used via ResolveSliceContractAt.
	for _, track := range metadata.Tracks {
		for _, slice := range track.Slices {
			if slice.ContractPath == "" {
				continue
			}
			if input.Commit != "" {
				if _, err := parsed.ResolveSliceContractAt(input.Repository, slice.ID, input.Commit); err != nil {
					return nil, err
				}
			} else {
				raw, err := readContractBytes(input.Repository, repoRoot, slice.ContractPath, "")
				if err != nil {
					return nil, err
				}
				if _, err := parsed.ResolveSliceContract(slice.ID, raw); err != nil {
					return nil, err
				}
			}
		}
	}

	// Run the scope lint against the chosen source.
	var lintErr error
	if input.Commit != "" {
		lintErr = ValidatePlanScopeLintAt(input.Repository, parsed, input.Commit)
	} else {
		lintErr = ValidatePlanScopeLintFS(os.DirFS(repoRoot), parsed)
	}
	if lintErr != nil {
		return lintFailureResults(parsed, lintErr), lintErr
	}

	results := make([]LintPlanSliceResult, 0)
	for _, track := range metadata.Tracks {
		for _, slice := range track.Slices {
			results = append(results, LintPlanSliceResult{
				Slice:  slice.ID,
				Status: "PASS",
			})
		}
	}
	return results, nil
}

func lintFailureResults(parsed Plan, lintErr error) []LintPlanSliceResult {
	var recErr *RecordError
	if errors.As(lintErr, &recErr) && len(recErr.Paths) > 0 {
		return []LintPlanSliceResult{{
			Slice:  "",
			Status: "FAIL",
			Code:   recErr.Code,
			Paths:  recErr.Paths,
		}}
	}
	metadata := parsed.Metadata()
	results := make([]LintPlanSliceResult, 0)
	for _, track := range metadata.Tracks {
		for _, slice := range track.Slices {
			results = append(results, LintPlanSliceResult{
				Slice:  slice.ID,
				Status: "FAIL",
				Code:   ErrorCode(lintErr),
			})
		}
	}
	return results
}
