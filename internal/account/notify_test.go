package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotifyWebhook(t *testing.T) {
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		var event NotifyEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("decode payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received.Store(event)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, nil)
	event := NotifyEvent{
		Release:           "test-release",
		Track:             "T1",
		SliceID:           "S01-fail",
		State:             "failed_verification",
		ViolationsSummary: "3 violation(s) found",
		WorktreePath:      "/tmp/test-worktree",
		Timestamp:         "2026-07-01T00:00:00Z",
	}

	n.Notify(context.Background(), event)

	got, ok := received.Load().(NotifyEvent)
	if !ok {
		t.Fatal("no webhook payload received")
	}
	if got.Release != "test-release" {
		t.Errorf("Release = %q, want %q", got.Release, "test-release")
	}
	if got.Track != "T1" {
		t.Errorf("Track = %q, want %q", got.Track, "T1")
	}
	if got.SliceID != "S01-fail" {
		t.Errorf("SliceID = %q, want %q", got.SliceID, "S01-fail")
	}
	if got.State != "failed_verification" {
		t.Errorf("State = %q, want %q", got.State, "failed_verification")
	}
	if got.ViolationsSummary != "3 violation(s) found" {
		t.Errorf("ViolationsSummary = %q, want %q", got.ViolationsSummary, "3 violation(s) found")
	}
	if got.WorktreePath != "/tmp/test-worktree" {
		t.Errorf("WorktreePath = %q, want %q", got.WorktreePath, "/tmp/test-worktree")
	}
	if got.Timestamp != "2026-07-01T00:00:00Z" {
		t.Errorf("Timestamp = %q, want %q", got.Timestamp, "2026-07-01T00:00:00Z")
	}
}

func TestNotifyRetryOnFailure(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, nil)
	event := NotifyEvent{
		Release: "test-release",
		Track:   "T1",
		SliceID: "S01-fail",
		State:   "failed_verification",
	}

	// Notify should not error even though all attempts fail.
	n.Notify(context.Background(), event)

	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestNotifyNoOp(t *testing.T) {
	// No webhook URL, no credentials — should be a complete no-op.
	n := NewNotifier("", nil)
	event := NotifyEvent{
		Release: "test-release",
		SliceID: "S01-fail",
		State:   "failed_verification",
	}

	// This must not panic, must not make any network call.
	n.Notify(context.Background(), event)
	// Test passes if no panic.
}

func TestNotifyNoOp_NilNotifier(t *testing.T) {
	var n *Notifier
	event := NotifyEvent{
		Release: "test-release",
		SliceID: "S01-fail",
		State:   "failed_verification",
	}
	// Nil receiver must not panic.
	n.Notify(context.Background(), event)
}

func TestNotifyWithAccount(t *testing.T) {
	var webhookCalled atomic.Bool
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	var apiCalled atomic.Bool
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization Bearer test-token, got %s", r.Header.Get("Authorization"))
		}
		apiCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer apiSrv.Close()

	// Override the proxy host for the test.
	t.Setenv("SWORN_PROXY_URL", apiSrv.URL)

	creds := &Credentials{
		Token:     "test-token",
		Email:     "test@example.com",
		Tier:      "free",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	n := NewNotifier(webhookSrv.URL, creds)
	event := NotifyEvent{
		Release: "test-release",
		Track:   "T1",
		SliceID: "S01-fail",
		State:   "failed_verification",
	}

	n.Notify(context.Background(), event)

	if !webhookCalled.Load() {
		t.Error("webhook was not called")
	}
	if !apiCalled.Load() {
		t.Error("SwornAgent API was not called")
	}
}

func TestNotifyWithAccount_ExpiredToken(t *testing.T) {
	var apiCalled atomic.Bool
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer apiSrv.Close()

	t.Setenv("SWORN_PROXY_URL", apiSrv.URL)

	// Expired token — IsLoggedIn should return false.
	creds := &Credentials{
		Token:     "expired-token",
		Email:     "test@example.com",
		Tier:      "free",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	n := NewNotifier("", creds)
	event := NotifyEvent{
		Release: "test-release",
		SliceID: "S01-fail",
		State:   "failed_verification",
	}

	n.Notify(context.Background(), event)

	if apiCalled.Load() {
		t.Error("API was called with expired token — should be skipped")
	}
}

func TestNotifyWebhook_TimeoutContext(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	event := NotifyEvent{
		Release: "test-release",
		SliceID: "S01-fail",
		State:   "failed_verification",
	}

	// Should not panic even with cancelled context.
	n.Notify(ctx, event)
}

// writeProofJSON writes a minimal proof-v1 proof.json fixture carrying the
// given not_delivered entries into sliceDir/proof.json.
func writeProofJSON(t *testing.T, sliceDir string, notDelivered []string) {
	t.Helper()
	payload := struct {
		NotDelivered []string `json:"not_delivered"`
	}{NotDelivered: notDelivered}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sliceDir, "proof.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestViolationsSummary_FromFile is AC-01's integration test: ViolationsSummary
// reads proof.json.not_delivered directly — no proof.md regex-scrape remains.
func TestViolationsSummary_FromFile(t *testing.T) {
	sliceDir := t.TempDir()

	// No proof.json at all — should fall back.
	got := ViolationsSummary(sliceDir, 3)
	if got != "3 violation(s) found" {
		t.Errorf("got %q, want '3 violation(s) found'", got)
	}

	// Write a proof.json with not_delivered violations.
	writeProofJSON(t, sliceDir, []string{
		"Missing reachability artefact in proof bundle",
		"Test coverage below threshold",
		"Design TL;DR not reviewed",
	})

	got = ViolationsSummary(sliceDir, 3)
	if got != "Missing reachability artefact in proof bundle" {
		t.Errorf("got %q, want 'Missing reachability artefact in proof bundle'", got)
	}

	// proof.json present but not_delivered empty.
	writeProofJSON(t, sliceDir, nil)

	got = ViolationsSummary(sliceDir, 3)
	if got != "3 violation(s) found" {
		t.Errorf("got %q, want '3 violation(s) found'", got)
	}

	// No violations, count 0.
	got = ViolationsSummary(sliceDir, 0)
	if got != "verification failed" {
		t.Errorf("got %q, want 'verification failed'", got)
	}

	// A decoy proof.md sitting alongside proof.json must be ignored entirely
	// (AC-01: "instead of", not "in addition to").
	decoy := "# Proof Bundle\n\n## Violations\n1. LEGACY-SCRAPE-MARKER should never surface\n"
	if err := os.WriteFile(filepath.Join(sliceDir, "proof.md"), []byte(decoy), 0644); err != nil {
		t.Fatal(err)
	}
	writeProofJSON(t, sliceDir, []string{"the real violation"})
	got = ViolationsSummary(sliceDir, 1)
	if got != "the real violation" {
		t.Errorf("got %q, want 'the real violation' (proof.md decoy must be ignored)", got)
	}
}

func TestViolationsSummary_Truncation(t *testing.T) {
	sliceDir := t.TempDir()

	long := ""
	for i := 0; i < 250; i++ {
		long += "x"
	}
	writeProofJSON(t, sliceDir, []string{long})

	got := ViolationsSummary(sliceDir, 0)
	if len(got) > 200 {
		t.Errorf("summary length %d exceeds max 200: %q", len(got), got)
	}
	if len(got) < 197 {
		t.Errorf("summary not truncated near boundary: len=%d", len(got))
	}
}

// TestViolationsSummary_MalformedProofJSONFallsBack proves an unparseable
// proof.json is treated the same as an absent one (matches the established
// internal/mcp/context.go readProofViolations precedent) — no panic, no
// proof.md fallback.
func TestViolationsSummary_MalformedProofJSONFallsBack(t *testing.T) {
	sliceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sliceDir, "proof.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	got := ViolationsSummary(sliceDir, 2)
	if got != "2 violation(s) found" {
		t.Errorf("got %q, want '2 violation(s) found'", got)
	}
}

func TestNotifyEvent_JSONShape(t *testing.T) {
	event := NotifyEvent{
		Release:           "test-release",
		Track:             "T1",
		SliceID:           "S01-fail",
		State:             "failed_verification",
		ViolationsSummary: "1. Missing reachability artefact",
		WorktreePath:      "/tmp/worktree",
		Timestamp:         "2026-07-01T00:00:00Z",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	required := []string{"release", "track", "slice_id", "state", "violations_summary", "worktree_path", "timestamp"}
	for _, key := range required {
		if _, ok := decoded[key]; !ok {
			t.Errorf("missing required JSON key: %s", key)
		}
	}
}

func TestNotify_TimestampDefault(t *testing.T) {
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event NotifyEvent
		json.NewDecoder(r.Body).Decode(&event)
		received.Store(event)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, nil)
	event := NotifyEvent{
		Release: "test",
		SliceID: "S01",
		State:   "failed_verification",
		// Timestamp left empty — should be filled in.
	}

	n.Notify(context.Background(), event)

	got, ok := received.Load().(NotifyEvent)
	if !ok {
		t.Fatal("no payload received")
	}
	if got.Timestamp == "" {
		t.Error("Timestamp was not filled in")
	}
	// Must be a valid RFC3339 time.
	if _, err := time.Parse(time.RFC3339, got.Timestamp); err != nil {
		t.Errorf("Timestamp %q is not valid RFC3339: %v", got.Timestamp, err)
	}
}
