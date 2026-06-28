package baton

import (
	"strings"
	"testing"
)

// validPayload is a minimal conformant status.json.
var validPayload = []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "verification": { "result": "pending" }
}`)

func TestValidate_UnknownSchema(t *testing.T) {
	err := Validate("nonexistent-v1", []byte(`{}`))
	if err == nil {
		t.Fatal("want error for unknown schema, got nil")
	}
	if !strings.Contains(err.Error(), "unknown schema") {
		t.Errorf("want 'unknown schema', got: %v", err)
	}
}

func TestValidate_ValidPayload(t *testing.T) {
	if err := Validate("slice-status-v1", validPayload); err != nil {
		t.Errorf("valid payload: want nil, got %v", err)
	}
}

func TestValidate_EmptyObject(t *testing.T) {
	err := Validate("slice-status-v1", []byte(`{}`))
	if err == nil {
		t.Fatal("empty object: want error, got nil")
	}
	if !strings.Contains(err.Error(), "empty object") {
		t.Errorf("want 'empty object', got: %v", err)
	}
}

func TestValidate_MissingSliceID(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "verification": { "result": "pending" }
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("missing slice_id: want error, got nil")
	}
	if !strings.Contains(err.Error(), "slice_id") {
		t.Errorf("want error mentioning slice_id, got: %v", err)
	}
}

func TestValidate_MissingRelease(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "verification": { "result": "pending" }
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("missing release: want error, got nil")
	}
	if !strings.Contains(err.Error(), "release") {
		t.Errorf("want error mentioning release, got: %v", err)
	}
}

func TestValidate_MissingTrack(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "release": "2026-06-27-conformance-foundation",
  "state": "in_progress",
  "verification": { "result": "pending" }
}`)
	// track is OPTIONAL: single-slice `sworn run` writes an empty track, and the
	// canonical slice-status-v1 required set is [slice_id, release, state,
	// verification] — not track. Missing track must validate cleanly.
	// (2026-06-28: previously asserted track was required, which broke every
	// single-slice run and ~28 internal/run tests.)
	if err := Validate("slice-status-v1", payload); err != nil {
		t.Fatalf("missing track must be allowed (optional), got error: %v", err)
	}
}

func TestValidate_MissingState(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "verification": { "result": "pending" }
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("missing state: want error, got nil")
	}
	if !strings.Contains(err.Error(), "state") {
		t.Errorf("want error mentioning state, got: %v", err)
	}
}

func TestValidate_InvalidState(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "state": "bogus",
  "verification": { "result": "pending" }
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("invalid state: want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown state") {
		t.Errorf("want 'unknown state', got: %v", err)
	}
}

func TestValidate_EmptyStringField(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "verification": { "result": "pending" }
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("empty slice_id: want error, got nil")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("want 'non-empty', got: %v", err)
	}
}

func TestValidate_MissingVerification(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "state": "in_progress"
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("missing verification: want error, got nil")
	}
	if !strings.Contains(err.Error(), "verification") {
		t.Errorf("want error mentioning verification, got: %v", err)
	}
}

func TestValidate_MissingVerificationResult(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "verification": {}
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("missing verification.result: want error, got nil")
	}
	if !strings.Contains(err.Error(), "verification.result") {
		t.Errorf("want error mentioning verification.result, got: %v", err)
	}
}

func TestValidate_WrongSchemaURI(t *testing.T) {
	payload := []byte(`{
  "$schema": "https://example.com/schemas/baton/slice-status-v1.json",
  "slice_id": "S13-schema-embed-validate",
  "release": "2026-06-27-conformance-foundation",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "verification": { "result": "pending" }
}`)
	err := Validate("slice-status-v1", payload)
	if err == nil {
		t.Fatal("wrong $schema: want error, got nil")
	}
	if !strings.Contains(err.Error(), "$schema") {
		t.Errorf("want error mentioning $schema, got: %v", err)
	}
}

func TestValidate_InvalidJSON(t *testing.T) {
	err := Validate("slice-status-v1", []byte(`not json`))
	if err == nil {
		t.Fatal("invalid JSON: want error, got nil")
	}
}