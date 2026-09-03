package driver

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func validClaudeTestConfig(t *testing.T) NativeAdapterConfig {
	t.Helper()
	return nativePinModeTestConfig(
		t, NativePinModeExact, ClaudeCLIVersion, ClaudeCLIDigest,
		ClaudeCredentialTarget, ClaudeCLIVersion+" (Claude Code)",
	)
}

func validCodexTestConfig(t *testing.T) NativeAdapterConfig {
	t.Helper()
	config := validClaudeTestConfig(t)
	config.Key = "a-codex"
	config.ID = "sworn.codex"
	config.Family = ProfileCodex
	config.CLIVersion = CodexCLIVersion
	config.CLI.Digest = CodexCLIDigest
	config.CredentialTarget = CodexCredentialTarget
	config.VersionOutput = "codex-cli " + CodexCLIVersion
	config.CredentialRefs = []string{"codex-file"}
	return config
}

func dummyResolver(context.Context, string) (string, error) {
	return "", nil
}

func TestNativeAdmissionDetail19TermsClosedVocabulary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configure  func(t *testing.T, c *NativeAdapterConfig)
		wantDetail string
		testDirect bool // test validateNativeConfig directly in addition to NewNativeAdapter
	}{
		// 1. cli_identity
		{
			name: "1. cli_identity",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.CLI.Path = "/nonexistent/path/to/binary"
			},
			wantDetail: "cli_identity",
			testDirect: true,
		},
		// 2. cli_admission_bounds
		{
			name: "2. cli_admission_bounds (zero credential bytes)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.MaxCredentialBytes = 0
			},
			wantDetail: "cli_admission_bounds",
			testDirect: true,
		},
		{
			name: "2. cli_admission_bounds (excess credential bytes)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.MaxCredentialBytes = 1_048_577
			},
			wantDetail: "cli_admission_bounds",
			testDirect: true,
		},
		{
			name: "2. cli_admission_bounds (empty version output)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.VersionOutput = ""
			},
			wantDetail: "cli_admission_bounds",
			testDirect: true,
		},
		{
			name: "2. cli_admission_bounds (oversized version output)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.VersionOutput = strings.Repeat("a", 257)
			},
			wantDetail: "cli_admission_bounds",
			testDirect: true,
		},
		// 3. pin_mode
		{
			name: "3. pin_mode",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.PinMode = "invalid_pin_mode"
			},
			wantDetail: "pin_mode",
			testDirect: true,
		},
		// 4. credential_target
		{
			name: "4. credential_target (claude with codex target)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.CredentialTarget = CodexCredentialTarget
			},
			wantDetail: "credential_target",
			testDirect: true,
		},
		// 5. version
		{
			name: "5. version (claude exact version mismatch)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.CLIVersion = "9.9.9"
			},
			wantDetail: "version",
			testDirect: true,
		},
		{
			name: "5. version (claude minor mismatch)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.PinMode = NativePinModeMinor
				c.CLIVersion = "9.9.9"
			},
			wantDetail: "version",
			testDirect: true,
		},
		// 6. version_output
		{
			name: "6. version_output (claude exact output mismatch)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.VersionOutput = "bad version output"
			},
			wantDetail: "version_output",
			testDirect: true,
		},
		{
			name: "6. version_output (claude minor output mismatch)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.PinMode = NativePinModeMinor
				c.VersionOutput = "bad version output"
			},
			wantDetail: "version_output",
			testDirect: true,
		},
		// 7. digest
		{
			name: "7. digest (claude CLI digest mismatch)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.CLI.Digest = "sha256:" + strings.Repeat("0", 64)
			},
			wantDetail: "digest",
			testDirect: true,
		},
		// 8. family
		{
			name: "8. family (unknown family)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.Family = "unknown-family"
			},
			wantDetail: "family",
			testDirect: true,
		},
		// 9. toolchain_root
		{
			name: "9. toolchain_root",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RuntimeFiles = append(c.RuntimeFiles, PinnedRuntimeFile{
					Path:   c.RuntimeFiles[0].Path,
					Target: "/usr/bin/some-override",
					Digest: c.RuntimeFiles[0].Digest,
				})
			},
			wantDetail: "toolchain_root",
			testDirect: true,
		},
		// 10. runtime_file
		{
			name: "10. runtime_file (empty runtime files)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RuntimeFiles = nil
			},
			wantDetail: "runtime_file",
			testDirect: true,
		},
		{
			name: "10. runtime_file (empty required targets)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RequiredRuntimeTargets = nil
			},
			wantDetail: "runtime_file",
			testDirect: true,
		},
		// 11. runtime_file_shape
		{
			name: "11. runtime_file_shape (collision with guest workspace)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RuntimeFiles[0].Target = GuestWorkspacePath
			},
			wantDetail: "runtime_file_shape",
			testDirect: true,
		},
		{
			name: "11. runtime_file_shape (target beneath /home/sworn)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RuntimeFiles[0].Target = "/home/sworn/data"
			},
			wantDetail: "runtime_file_shape",
			testDirect: true,
		},
		{
			name: "11. runtime_file_shape (relative path)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RuntimeFiles[0].Path = "relative/path"
			},
			wantDetail: "runtime_file_shape",
			testDirect: true,
		},
		// 12. runtime_file_digest
		{
			name: "12. runtime_file_digest",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RuntimeFiles[0].Digest = "not-a-valid-sha256-digest"
			},
			wantDetail: "runtime_file_digest",
			testDirect: true,
		},
		// 13. runtime_file_duplicate
		{
			name: "13. runtime_file_duplicate (duplicate in runtime files)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				dup := c.RuntimeFiles[0]
				c.RuntimeFiles = append(c.RuntimeFiles, dup)
			},
			wantDetail: "runtime_file_duplicate",
			testDirect: true,
		},
		{
			name: "13. runtime_file_duplicate (duplicate in required targets)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RequiredRuntimeTargets = append(c.RequiredRuntimeTargets, c.RequiredRuntimeTargets[0])
			},
			wantDetail: "runtime_file_duplicate",
			testDirect: true,
		},
		// 14. runtime_file_missing
		{
			name: "14. runtime_file_missing",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.RequiredRuntimeTargets = append(c.RequiredRuntimeTargets, "/etc/extra-required-target")
			},
			wantDetail: "runtime_file_missing",
			testDirect: true,
		},
		// 15. trust_anchor
		{
			name: "15. trust_anchor (missing /etc/hosts)",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				filtered := make([]PinnedRuntimeFile, 0, len(c.RuntimeFiles))
				for _, rf := range c.RuntimeFiles {
					if rf.Target != "/etc/hosts" {
						filtered = append(filtered, rf)
					}
				}
				c.RuntimeFiles = filtered
				// also remove from required so it doesn't fail on runtime_file_missing first
				reqFiltered := make([]string, 0, len(c.RequiredRuntimeTargets))
				for _, req := range c.RequiredRuntimeTargets {
					if req != "/etc/hosts" {
						reqFiltered = append(reqFiltered, req)
					}
				}
				c.RequiredRuntimeTargets = reqFiltered
			},
			wantDetail: "trust_anchor",
			testDirect: true,
		},
		// 16. adapter_key
		{
			name: "16. adapter_key",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.Key = "invalid key"
			},
			wantDetail: "adapter_key",
			testDirect: false,
		},
		// 17. adapter_id
		{
			name: "17. adapter_id",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.ID = "invalid ID"
			},
			wantDetail: "adapter_id",
			testDirect: false,
		},
		// 18. adapter_version
		{
			name: "18. adapter_version",
			configure: func(t *testing.T, c *NativeAdapterConfig) {
				c.Version = "invalid.version.format!"
			},
			wantDetail: "adapter_version",
			testDirect: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validClaudeTestConfig(t)
			tc.configure(t, &cfg)

			if tc.testDirect {
				err := validateNativeConfig(cfg)
				if err == nil {
					t.Fatalf("validateNativeConfig expected error with detail %q, got nil", tc.wantDetail)
				}
				var contractErr *ContractError
				if !errors.As(err, &contractErr) {
					t.Fatalf("expected *ContractError, got %T: %v", err, err)
				}
				if contractErr.Code != "NATIVE_NOT_CERTIFIED" {
					t.Fatalf("expected code NATIVE_NOT_CERTIFIED, got %q", contractErr.Code)
				}
				if contractErr.Detail != tc.wantDetail {
					t.Fatalf("expected detail %q, got %q", tc.wantDetail, contractErr.Detail)
				}
			}

			// NewNativeAdapter wraps or surfaces INVALID_ADAPTER with the detail
			_, err := NewNativeAdapter(cfg, dummyResolver)
			if err == nil {
				t.Fatalf("NewNativeAdapter expected error with detail %q, got nil", tc.wantDetail)
			}
			var contractErr *ContractError
			if !errors.As(err, &contractErr) {
				t.Fatalf("expected *ContractError, got %T: %v", err, err)
			}
			if contractErr.Code != "INVALID_ADAPTER" {
				t.Fatalf("expected code INVALID_ADAPTER, got %q", contractErr.Code)
			}
			if contractErr.Detail != tc.wantDetail {
				t.Fatalf("expected detail %q, got %q", tc.wantDetail, contractErr.Detail)
			}
		})
	}

	// 19. credential_resolver
	t.Run("19. credential_resolver (nil resolver)", func(t *testing.T) {
		cfg := validClaudeTestConfig(t)
		_, err := NewNativeAdapter(cfg, nil)
		if err == nil {
			t.Fatal("NewNativeAdapter expected error for nil resolver, got nil")
		}
		var contractErr *ContractError
		if !errors.As(err, &contractErr) {
			t.Fatalf("expected *ContractError, got %T: %v", err, err)
		}
		if contractErr.Code != "INVALID_ADAPTER" {
			t.Fatalf("expected code INVALID_ADAPTER, got %q", contractErr.Code)
		}
		if contractErr.Detail != "credential_resolver" {
			t.Fatalf("expected detail %q, got %q", "credential_resolver", contractErr.Detail)
		}
	})

	// Also verify Codex conditions for 4, 5, 6, 7
	t.Run("Codex credential_target mismatch", func(t *testing.T) {
		cfg := validCodexTestConfig(t)
		cfg.CredentialTarget = ClaudeCredentialTarget
		err := validateNativeConfig(cfg)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) || contractErr.Detail != "credential_target" {
			t.Fatalf("expected detail credential_target, got %v", err)
		}
	})

	t.Run("Codex version mismatch", func(t *testing.T) {
		cfg := validCodexTestConfig(t)
		cfg.CLIVersion = "9.9.9"
		err := validateNativeConfig(cfg)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) || contractErr.Detail != "version" {
			t.Fatalf("expected detail version, got %v", err)
		}
	})

	t.Run("Codex version_output mismatch", func(t *testing.T) {
		cfg := validCodexTestConfig(t)
		cfg.VersionOutput = "bad output"
		err := validateNativeConfig(cfg)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) || contractErr.Detail != "version_output" {
			t.Fatalf("expected detail version_output, got %v", err)
		}
	})

	t.Run("Codex digest mismatch", func(t *testing.T) {
		cfg := validCodexTestConfig(t)
		cfg.CLI.Digest = "sha256:" + strings.Repeat("0", 64)
		err := validateNativeConfig(cfg)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) || contractErr.Detail != "digest" {
			t.Fatalf("expected detail digest, got %v", err)
		}
	})
}
