package driver

import (
	"reflect"
	"strings"
	"testing"
)

// validManifestPlanBytes returns one complete, canonically admissible
// sworn.release-manifest/v1 body with a real slice contract digest, proving
// the driver's plan admission accepts both canonical schemas through the
// same baton.ParsePlan delegate.
func validManifestPlanBytes() []byte {
	return []byte("```sworn-release-manifest-v1\n" + `{
  "schema_version": "sworn.release-manifest/v1",
  "release": "fixture",
  "revision": 1,
  "previous_plan": null,
  "repository": "fixture/repo",
  "target_ref": "refs/heads/main",
  "approval_ref": "fixture://approval/fixture/1",
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
}` + "\n```\n# Manifest\n")
}

func TestNewPlanBytesAcceptsBothCanonicalSchemasAndDelegatesRejection(t *testing.T) {
	t.Parallel()
	for name, body := range map[string][]byte{
		"legacy baton.plan/v2":      validPlanBytes(),
		"sworn.release-manifest/v1": validManifestPlanBytes(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewPlanBytes(body); err != nil {
				t.Fatalf("NewPlanBytes(%s) = %v", name, err)
			}
		})
	}

	for name, body := range map[string][]byte{
		"unknown fence":  []byte("```baton-plan-v3\n{}\n```\n# Plan\n"),
		"malformed json": []byte("```baton-plan-v2\nnot json\n```\n# Plan\n"),
		"minimal object missing required fields": []byte(
			"```baton-plan-v2\n{\"schema_version\":\"baton.plan/v2\"}\n```\n# Plan\n",
		),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewPlanBytes(body); !IsCode(err, "INVALID_PLAN_BYTES") {
				t.Fatalf("NewPlanBytes(%s) err = %v, want INVALID_PLAN_BYTES", name, err)
			}
		})
	}
}

func TestPlannerProposalSubmissionAcceptsManifestFormat(t *testing.T) {
	t.Parallel()
	permission, request := permissionFixture(t, RolePlanner, PlannerProposal)
	submission := Submission{
		SchemaVersion:  SubmissionSchemaVersion,
		InvocationID:   request.InvocationID,
		Responsibility: PlannerProposal,
		Summary:        "Compact responsibility summary padded so every floored responsibility this shared fixture drives clears the submission content floor for permission and correction-accounting coverage.",
		Detail:         "Bounded LF-only detail padded so every floored responsibility this shared fixture drives clears the detail content floor while exercising submission-permission acceptance and rejection coverage across these tests.\n",
	}
	var err error
	submission.Plan, err = NewPlanBytes(validManifestPlanBytes())
	if err != nil {
		t.Fatal(err)
	}
	body, err := EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newSubmissionServer(permission)
	if err != nil {
		t.Fatal(err)
	}
	seal, _, err := server.Submit(body)
	if err != nil || !seal.Accepted {
		t.Fatalf("seal = %#v, err = %v", seal, err)
	}
}

// TestSubmissionWireEnvelopeIsClosedAndSingleFile guards the Captain
// correction that phase 2 must not add a second Plan-shaped field or any
// archive/envelope to Submission beyond the one sanctioned exception: the
// planner_proposal-only Contracts map that lets a proposal carry new
// contract files beside its plan bytes (sworn#210). It stays exactly seven
// fields plus that one map, with exactly one ExactBytes-typed plan field and
// one ExactBytes-typed checks field.
func TestSubmissionWireEnvelopeIsClosedAndSingleFile(t *testing.T) {
	t.Parallel()
	fields := reflect.VisibleFields(reflect.TypeFor[Submission]())
	names := make([]string, len(fields))
	exactBytesFields := 0
	for i, field := range fields {
		names[i] = field.Name
		if field.Type == reflect.TypeFor[*ExactBytes]() {
			exactBytesFields++
		}
	}
	wantNames := []string{
		"SchemaVersion", "InvocationID", "Responsibility", "Summary", "Detail", "Plan", "Checks", "Decision", "Contracts",
	}
	if strings.Join(names, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("Submission fields = %v, want exactly %v", names, wantNames)
	}
	if exactBytesFields != 2 {
		t.Fatalf("Submission has %d *ExactBytes fields, want exactly 2 (Plan, Checks)", exactBytesFields)
	}
	contractsField, ok := reflect.TypeFor[Submission]().FieldByName("Contracts")
	if !ok || contractsField.Type != reflect.TypeFor[map[string]*ExactBytes]() {
		t.Fatalf("Contracts field = %#v, want map[string]*ExactBytes", contractsField)
	}
}
