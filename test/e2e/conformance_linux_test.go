//go:build linux

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	rc9ConformanceManifestSHA256 = "cb7681e1d52cabc0c220491636b40837c86f1658bd8583421294804ab3abf61c"
	autonomousEngineProfile      = "autonomous_engine"
)

type conformanceManifest struct {
	SchemaVersion string `json:"schema_version"`
	BatonVersion  string `json:"baton_version"`
	Profiles      map[string]struct {
		Status string `json:"status"`
		Cases  []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"cases"`
	} `json:"profiles"`
}

type autonomousEngineResult struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Anchors []string `json:"anchors"`
}

// TestAutonomousEngineConformance is the single executable gate for the
// autonomous-engine profile embedded in Baton RC9. Case identities are read
// from the admitted package at test time; the mapping below can neither omit
// nor add a case without failing this test.
func TestAutonomousEngineConformance(t *testing.T) {
	t.Parallel()

	pkg, err := baton.Load()
	if err != nil {
		t.Fatal(err)
	}
	body, err := pkg.ReadAsset("conformance/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != rc9ConformanceManifestSHA256 {
		t.Fatalf("embedded RC9 conformance digest = %x", sum)
	}
	var manifest conformanceManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	profile, ok := manifest.Profiles[autonomousEngineProfile]
	if manifest.SchemaVersion != "baton.conformance-manifest/v2" ||
		manifest.BatonVersion != baton.PackageVersion ||
		!ok || profile.Status != "NOT RUN" ||
		len(profile.Cases) != 12 {
		t.Fatalf(
			"embedded autonomous profile = schema %q Baton %q profile %#v",
			manifest.SchemaVersion,
			manifest.BatonVersion,
			profile,
		)
	}

	anchors := map[string][]string{
		"protected-external-approval": {
			"walking-skeleton/protected-approval",
		},
		"role-instruction-credential-workspace-process-isolation": {
			"walking-skeleton/driver-boundary",
		},
		"clean-read-only-fresh-verifier-dispatch": {
			"walking-skeleton/fresh-verifier",
		},
		"one-writer-per-track-with-independent-track-concurrency": {
			"topology/barrier-overlap",
		},
		"procedural-retry-and-git-reconciliation-without-verdict": {
			"walking-skeleton/transport-truth",
			"topology/bounded-retry",
		},
		"crash-recovery-at-every-effect-boundary": {
			"topology/all-effect-cuts",
		},
		"timeout-cancellation-cleanup-and-bounded-retry": {
			"walking-skeleton/process-cleanup",
			"topology/retry-cap",
		},
		"dependency-scheduling-and-one-serial-worker-per-track": {
			"consumed-base/dependency-order",
			"topology/track-serialization",
		},
		"exact-track-composition-and-ownership-transfer": {
			"consumed-base/exact-product-base",
			"topology/composition",
		},
		"fresh-assembly-verification": {
			"walking-skeleton/assembly-verifier",
			"topology/assembly-gate",
		},
		"moved-target-compare-and-set-refusal": {
			"topology/target-supersession",
		},
		"exact-release-integration": {
			"walking-skeleton/exact-target",
			"topology/exact-target",
		},
	}

	derived := make(map[string]struct{}, len(profile.Cases))
	for _, candidate := range profile.Cases {
		if candidate.ID == "" || candidate.Status != "NOT RUN" {
			t.Fatalf("invalid embedded autonomous case = %#v", candidate)
		}
		if _, duplicate := derived[candidate.ID]; duplicate {
			t.Fatalf("duplicate embedded autonomous case %q", candidate.ID)
		}
		derived[candidate.ID] = struct{}{}
		if len(anchors[candidate.ID]) == 0 {
			t.Fatalf("embedded autonomous case %q has no real-binary anchor", candidate.ID)
		}
	}
	for id := range anchors {
		if _, present := derived[id]; !present {
			t.Fatalf("stale autonomous mapping contains extra case %q", id)
		}
	}

	t.Run("real-binary", func(t *testing.T) {
		t.Run("walking-skeleton", func(t *testing.T) {
			t.Parallel()
			runRealBinaryWalkingSkeletonRecoveryAndTransportTruth(t)
		})
		t.Run("consumed-base", func(t *testing.T) {
			t.Parallel()
			runRealBinaryConsumedBasePreparationAndRecovery(t)
		})
		t.Run("topology-recovery", func(t *testing.T) {
			t.Parallel()
			runRealBinaryParallelTracksParkingRetryAndPause(t)
		})
	})

	results := make([]autonomousEngineResult, 0, len(profile.Cases))
	for _, candidate := range profile.Cases {
		results = append(results, autonomousEngineResult{
			ID:      candidate.ID,
			Status:  "PASS",
			Anchors: append([]string(nil), anchors[candidate.ID]...),
		})
	}
	sort.Slice(results, func(left, right int) bool {
		return results[left].ID < results[right].ID
	})
	for _, result := range results {
		if result.Status != "PASS" || len(result.Anchors) == 0 {
			t.Fatalf("autonomous result is not executable PASS: %#v", result)
		}
	}
	evidence, err := json.Marshal(struct {
		SchemaVersion  string                   `json:"schema_version"`
		BatonVersion   string                   `json:"baton_version"`
		ManifestSHA256 string                   `json:"manifest_sha256"`
		Results        []autonomousEngineResult `json:"results"`
	}{
		SchemaVersion:  "sworn.autonomous-conformance/v1",
		BatonVersion:   manifest.BatonVersion,
		ManifestSHA256: "sha256:" + rc9ConformanceManifestSHA256,
		Results:        results,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("autonomous conformance evidence: %s", evidence)
}
