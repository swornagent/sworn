package credentials

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateAtPreservesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"providers":{"openai":"key"},"future":{"enabled":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SetJSONAt(path, "token", "session"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"providers", "future", "token"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("field %q was lost", name)
		}
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %o, want 600", info.Mode().Perm())
	}
}

func TestUpdateAtRejectsMalformedExistingEnvelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	want := []byte(`{"providers":`)
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SetJSONAt(path, "token", "session"); err == nil {
		t.Fatal("expected malformed existing credentials to fail closed")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("malformed credentials were overwritten: got %q want %q", got, want)
	}
}

func TestDeleteAtRemovesOnlyOwnedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"providers":{"openai":"key"},"token":"session","webhook_url":"https://example.test/hook"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DeleteAt(path, "token"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["token"]; ok {
		t.Error("owned token field was not removed")
	}
	for _, name := range []string{"providers", "webhook_url"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("unowned field %q was removed", name)
		}
	}
}
