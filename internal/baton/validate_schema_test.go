package baton

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton/schemas"
)

const exactV015BoardSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://baton.sawy3r.net/schemas/board-v1.json",
  "title": "Baton Release Board",
  "description": "Machine-readable structure of one release board: tracks, slices, dependency edges, narrow shared-touchpoint declarations, and the vertical trace — a PURE PLAN artefact. All runtime state is DERIVED from git refs, never persisted here (track-mode invariant 5): worktree paths are conventional (computed from repo basename + release + track-id), and a track's state is computed from ref existence + merge-base ancestry against release-wt. Slice runtime state lives in each slice's status.json (slice-status-v1); the board references slices by id and never duplicates their state.",
  "type": "object",
  "additionalProperties": false,
  "required": ["release", "tracks"],
  "$defs": {
    "id_token": {
      "type": "string",
      "description": "A clean identifier token. Disallows whitespace and ':' so a newline-fusion defect (e.g. a track id that absorbed a following 'slices:' key) fails validation by construction.",
      "pattern": "^[A-Za-z0-9][A-Za-z0-9._-]*$",
      "minLength": 1
    }
  },
  "properties": {
    "$schema": {
      "type": "string",
      "const": "https://baton.sawy3r.net/schemas/board-v1.json"
    },
    "release": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name"],
      "properties": {
        "name": {
          "type": "string",
          "minLength": 1,
          "description": "Release identifier, e.g. 2026-06-19-safe-parallelism."
        },
        "target_version": {
          "type": "string",
          "description": "Semver the release ships, e.g. v0.1.0."
        },
        "integration_branch": {
          "type": "string",
          "description": "The release integration base branch, e.g. release/v0.1.0."
        },
        "vertical_trace": {
          "type": "object",
          "additionalProperties": false,
          "description": "The golden-thread top: the benefit every slice traces up to, and the optional higher objective it serves. Grouped so the trace can grow without a breaking change.",
          "required": ["benefit"],
          "properties": {
            "benefit": {
              "type": "string",
              "description": "The user/business benefit this release delivers. The RTM gate threads every slice up to this."
            },
            "org_objective": {
              "type": "string",
              "description": "Optional higher objective the benefit serves. The solo/small-team floor has none; absence is valid."
            }
          }
        }
      }
    },
    "tracks": {
      "type": "array",
      "description": "Touchpoint-disjoint tracks. Order is informational; dependencies are explicit via depends_on.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "slices"],
        "properties": {
          "id": {
            "$ref": "#/$defs/id_token",
            "description": "Track id, e.g. T14-baton-integration."
          },
          "slices": {
            "type": "array",
            "description": "Slice ids in this track, in implementation order. Each slice's own status.json (slice-status-v1) holds its runtime state; the track's own state (planned/in_progress/merged) is DERIVED from git refs, not stored here (invariant 5).",
            "items": { "$ref": "#/$defs/id_token" }
          },
          "depends_on": {
            "type": "array",
            "description": "Track ids that must merge before this track.",
            "items": { "$ref": "#/$defs/id_token" }
          }
        }
      }
    },
    "shared_touchpoints": {
      "type": "object",
      "description": "Narrow machine-readable exceptions to path-level track disjointness, keyed by exact repository-relative path. Omission is equivalent to an empty object for older plans. Each value maps every contributing track id to its distinct non-empty region or symbol. This declaration licenses only Git's conflict-free canonical three-way composition; it never licenses manual conflict resolution.",
      "propertyNames": {
        "minLength": 1,
        "pattern": "^(?!/)(?!.*(?:^|/)\\.\\.?($|/)).+$"
      },
      "additionalProperties": {
        "type": "object",
        "minProperties": 2,
        "description": "Every track permitted to contribute to this path, keyed uniquely by track id. Region strings must be unique within the declaration.",
        "propertyNames": { "$ref": "#/$defs/id_token" },
        "additionalProperties": {
          "type": "string",
          "minLength": 1
        }
      }
    }
  }
}
`

// TestValidateSchema_Compiles confirms every embedded schema compiles under a
// real draft-2020-12 evaluator (the legacy hand-rolled validator never did).
func TestValidateSchema_Compiles(t *testing.T) {
	for name := range schemas.SchemaMap {
		if _, err := CompiledSchema(name); err != nil {
			t.Errorf("schema %q failed to compile: %v", name, err)
		}
	}
}

// TestValidateSchema_GoodAndBad proves real validation accepts a conformant
// slice-status payload and rejects a malformed one (missing required field).
func TestValidateSchema_GoodAndBad(t *testing.T) {
	good := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01-x", "release": "r1", "state": "planned",
		"start_commit": null,
		"maintainability": {"state": "pending", "cycle": 0, "implementation_head": null, "rollback_slice_id": null, "reports": [], "adjudication": null},
		"verification": {"result": "pending"}
	}`
	if err := ValidateSchema("slice-status-v1", []byte(good)); err != nil {
		t.Errorf("good payload rejected: %v", err)
	}

	bad := `{"slice_id": "S01-x"}` // missing required release/state/verification
	if err := ValidateSchema("slice-status-v1", []byte(bad)); err == nil {
		t.Error("malformed payload accepted — real validation not enforcing required fields")
	}

	if err := ValidateSchema("no-such-schema", []byte(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "unknown schema") {
		t.Errorf("unknown schema should error, got %v", err)
	}
}

// TestValidateSchema_VerifierVerdict proves the ADR-0011 keystone schema enforces
// its core contract: the verdict enum is real, and a FAIL/BLOCKED verdict MUST
// carry at least one violation (the allOf conditional that structurally prevents
// a verifier from failing a slice without citing why).
func TestValidateSchema_VerifierVerdict(t *testing.T) {
	pass := `{"schema_version": 1, "verdict": "PASS", "rationale": "all checks satisfied"}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(pass)); err != nil {
		t.Errorf("valid PASS verdict rejected: %v", err)
	}

	failWithViolations := `{"schema_version": 1, "verdict": "FAIL", "rationale": "AC3 unmet",
		"violations": [{"gate": "adversarial", "description": "AC3 not satisfied"}]}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(failWithViolations)); err != nil {
		t.Errorf("valid FAIL+violations verdict rejected: %v", err)
	}

	failNoViolations := `{"schema_version": 1, "verdict": "FAIL", "rationale": "vague"}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(failNoViolations)); err == nil {
		t.Error("FAIL without violations accepted — allOf conditional not enforced")
	}

	badEnum := `{"schema_version": 1, "verdict": "MAYBE", "rationale": "x"}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(badEnum)); err == nil {
		t.Error("out-of-enum verdict accepted — verdict enum not enforced")
	}
}

// TestValidateSchema_EffortComplexity proves the #36 effort_complexity field is
// enforced by real draft-2020-12 evaluation on BOTH spec-v1 (planner-canonical)
// and slice-status-v1 (implementer mirror): a conformant rating validates, an
// off-enum axis is rejected, and a rating missing a required axis is rejected.
func TestValidateSchema_EffortComplexity(t *testing.T) {
	// v0.10.0 spec-v1: additionalProperties:false, schema_version retired, and
	// slice_id/user_outcome/covers_needs/acceptance_criteria/in_scope/out_of_scope
	// all required.
	specGood := `{
		"$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
		"slice_id": "S01", "release": "r1",
		"user_outcome": "the thing works", "covers_needs": ["N-01"],
		"in_scope": ["do the thing"], "out_of_scope": ["not the other thing"],
		"acceptance_criteria": [{"id": "AC-01", "text": "the thing shall work"}],
		"effort_complexity": {"effort": "high", "complexity": "low", "quadrant": "grind"}
	}`
	if err := ValidateSchema("spec-v1", []byte(specGood)); err != nil {
		t.Errorf("good spec rating rejected: %v", err)
	}

	specBadEnum := `{
		"$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
		"slice_id": "S01", "release": "r1",
		"user_outcome": "the thing works", "covers_needs": ["N-01"],
		"in_scope": ["do the thing"], "out_of_scope": ["not the other thing"],
		"acceptance_criteria": [{"id": "AC-01", "text": "the thing shall work"}],
		"effort_complexity": {"effort": "medium", "complexity": "low", "quadrant": "grind"}
	}`
	if err := ValidateSchema("spec-v1", []byte(specBadEnum)); err == nil {
		t.Error("off-enum effort accepted — schema enum not enforced")
	}

	statusGood := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01", "release": "r1", "state": "planned",
		"start_commit": null,
		"maintainability": {"state": "pending", "cycle": 0, "implementation_head": null, "rollback_slice_id": null, "reports": [], "adjudication": null},
		"verification": {"result": "pending"},
		"effort_complexity": {"effort": "low", "complexity": "high", "quadrant": "puzzle", "confirmed_by_implementer": true}
	}`
	if err := ValidateSchema("slice-status-v1", []byte(statusGood)); err != nil {
		t.Errorf("good status rating rejected: %v", err)
	}

	statusBadMissing := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01", "release": "r1", "state": "planned",
		"verification": {"result": "pending"},
		"effort_complexity": {"effort": "low"}
	}`
	if err := ValidateSchema("slice-status-v1", []byte(statusBadMissing)); err == nil {
		t.Error("rating missing required complexity/quadrant accepted")
	}
}

func TestCompileV015BoardSchemaWithoutSemanticWeakening(t *testing.T) {
	digest := sha256.Sum256([]byte(exactV015BoardSchema))
	if got, want := fmt.Sprintf("%x", digest), "1122cbf7fb8fd2de62d2e54667ed00ec9ef12bf52970c5362a0fcdfcffbfaae5"; got != want {
		t.Fatalf("exact v0.15.1 board-v1 fixture digest = %s, want %s", got, want)
	}

	schema, err := compileSchemaBytes("board-v1", []byte(exactV015BoardSchema))
	if err != nil {
		t.Fatalf("compile exact v0.15.1 board-v1 bytes: %v", err)
	}

	accepted := []string{"a", ".a", "a..b", "a//b", "a/", ".github/workflows/ci.yml"}
	rejected := []string{
		"/a", ".", "..", "a/./b", "a/../b", "a/.", "a/..",
		"a\nb", "\n", "a\rb", "a\u2028b", "a\u2029b", "a\n",
	}
	validate := func(path string) error {
		return schema.Validate(map[string]any{
			"release": map[string]any{"name": "r1"},
			"tracks":  []any{},
			"shared_touchpoints": map[string]any{
				path: map[string]any{"T1": "first", "T2": "second"},
			},
		})
	}
	for _, path := range accepted {
		if err := validate(path); err != nil {
			t.Errorf("path %q rejected: %v", path, err)
		}
	}
	for _, path := range rejected {
		if err := validate(path); err == nil {
			t.Errorf("path %q accepted, want rejection", path)
		}
	}
}
