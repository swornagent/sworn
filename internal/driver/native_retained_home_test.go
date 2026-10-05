//go:build linux

package driver

import (
	"os"
	"path/filepath"
	"testing"
)

// codexCredentialFixture has the shape of a ChatGPT-sign-in Codex auth.json.
const codexCredentialFixture = `{
  "OPENAI_API_KEY": null,
  "tokens": {
    "id_token": "fixture-id-token-aaaaaaaaaaaaaaaa",
    "access_token": "fixture-access-token-bbbbbbbbbbbb",
    "refresh_token": "fixture-refresh-token-cccccccccccc",
    "account_id": "0f6b1c2d-3e4f-4a5b-8c6d-7e8f9a0b1c2d"
  },
  "last_refresh": "2026-10-05T01:02:03.456789Z",
  "workspace_account_id": "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
}`

// The retained-home scan must let the CLI keep the account identifier it
// records in its own session state (Codex 0.160.0 writes tokens.account_id
// into the rollout metadata), while every token, and any credential field it
// does not name, still counts as a leak.
func TestNativeRetainedHomeScanExemptsOnlyNamedAccountIdentifier(t *testing.T) {
	setNativeMemoryRootEnv(t)
	cases := []struct {
		name   string
		family ProfileFamily
		home   string
		leak   bool
	}{
		{
			name:   "codex account id in rollout metadata",
			family: ProfileCodex,
			home: `{"type":"session_meta","payload":{"account_id":` +
				`"0f6b1c2d-3e4f-4a5b-8c6d-7e8f9a0b1c2d"}}`,
			leak: false,
		},
		{
			name:   "codex access token",
			family: ProfileCodex,
			home:   `{"auth":"fixture-access-token-bbbbbbbbbbbb"}`,
			leak:   true,
		},
		{
			name:   "codex unnamed field",
			family: ProfileCodex,
			home:   `{"seen":"9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"}`,
			leak:   true,
		},
		{
			name:   "account id outside the codex family",
			family: ProfileClaude,
			home:   `{"seen":"0f6b1c2d-3e4f-4a5b-8c6d-7e8f9a0b1c2d"}`,
			leak:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, err := newNativeContinuationState(NativeAdapterConfig{
				Family:           tc.family,
				CredentialTarget: CodexCredentialTarget,
			}, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer state.closeContinuation()
			state.mu.Lock()
			root := state.root
			state.mu.Unlock()
			dir := filepath.Join(root, "home", ".codex", "sessions")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(
				filepath.Join(dir, "rollout.jsonl"),
				[]byte(tc.home),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			if got := state.containsAny(
				[]byte(codexCredentialFixture),
			); got != tc.leak {
				t.Fatalf("containsAny = %v, want %v", got, tc.leak)
			}
		})
	}
}
