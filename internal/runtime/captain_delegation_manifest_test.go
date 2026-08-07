package runtime

import (
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// runtimeManifestPlan builds one complete, canonically admissible
// sworn.release-manifest/v1 plan, mirroring runtimePlan's legacy fixture, to
// prove Captain's structural projection recognizes both admitted schemas
// rather than hardcoding baton.PlanVersion.
func runtimeManifestPlan(t *testing.T, release, repository, target string) baton.Plan {
	t.Helper()
	body := []byte("```sworn-release-manifest-v1\n" + `{
  "schema_version": "sworn.release-manifest/v1",
  "release": "` + release + `",
  "revision": 1,
  "previous_plan": null,
  "repository": "` + repository + `",
  "target_ref": "` + target + `",
  "approval_ref": "operator://` + release + `/1",
  "tracks": [
    {
      "id": "T1",
      "depends_on": [],
      "slices": [
        {
          "id": "S1",
          "outcome": "Deliver S1.",
          "contract_path": "contracts/S1.json",
          "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["one.txt"]
        }
      ]
    }
  ]
}` + "\n```\n\nFixture manifest.\n")
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestCaptainPlanStructuralProjectionAcceptsBothCanonicalSchemas(t *testing.T) {
	t.Parallel()
	_, legacyPlan := runtimePlan(t, "release-legacy", "acme-repo", "refs/heads/main", "legacy")
	manifestPlan := runtimeManifestPlan(t, "release-manifest", "acme-repo", "refs/heads/main")

	for name, plan := range map[string]baton.Plan{
		"legacy baton.plan/v2":      legacyPlan,
		"sworn.release-manifest/v1": manifestPlan,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := CaptainPlanStructuralProjection(plan); err != nil {
				t.Fatalf("CaptainPlanStructuralProjection(%s) = %v", name, err)
			}
		})
	}
}

func TestIsAdmittedPlanVersionRecognizesExactlyBothCanonicalSchemas(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]bool{
		baton.PlanVersion:            true,
		baton.ManifestVersion:        true,
		"baton.plan/v3":              false,
		"":                           false,
		baton.PlanVersion + "-extra": false,
	} {
		if got := baton.IsAdmittedPlanVersion(value); got != want {
			t.Fatalf("IsAdmittedPlanVersion(%q) = %v, want %v", value, got, want)
		}
	}
}
