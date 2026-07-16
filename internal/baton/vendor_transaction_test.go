package baton

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestVendorTransactionFailureRestoresPrimaryWorktree(t *testing.T) {
	probeRepo := newVendorTransactionRepo(t)
	probePlan := fullVendorTestPlan(t, probeRepo)
	applyPositions := len(probePlan.changed)
	for failIndex := 0; failIndex < applyPositions; failIndex++ {
		t.Run(fmt.Sprintf("apply-%d", failIndex), func(t *testing.T) {
			repo := newVendorTransactionRepo(t)
			plan := fullVendorTestPlan(t, repo)
			applyCount := 0
			plan.fileOps = &vendorFileOps{
				replace: func(repo vendorRepository, rel string, content []byte, mode os.FileMode) error {
					current := applyCount
					applyCount++
					if current == 0 {
						_, recovery, loadErr := loadVendorRecovery(repo.root)
						if loadErr != nil || recovery == nil {
							t.Fatalf("first replacement began without validated recovery authority: recovery=%v err=%v", recovery, loadErr)
						}
					}
					if err := atomicReplaceVendorFile(repo, rel, content, mode); err != nil {
						return err
					}
					if current == failIndex {
						return fmt.Errorf("injected apply failure %d", current)
					}
					return nil
				},
				restore: restoreVendorOriginal,
			}

			err := applyVendorTransaction(repo, plan)
			var txErr *vendorError
			if !errors.As(err, &txErr) || txErr.class != "apply-failed" {
				t.Fatalf("apply error = %v, want class apply-failed", err)
			}
			for _, candidate := range plan.candidates {
				if err := verifyVendorOriginal(repo, candidate); err != nil {
					t.Errorf("%s not restored: %v", candidate.path, err)
				}
			}
			assertNoVendorTemps(t, repo.root)
			if _, err := os.Lstat(repo.recoveryRoot); !os.IsNotExist(err) {
				t.Fatalf("successful rollback left recovery material: %v", err)
			}
		})
	}

	for rollbackIndex := 0; rollbackIndex < applyPositions; rollbackIndex++ {
		t.Run(fmt.Sprintf("rollback-%d-publishes-authority", rollbackIndex), func(t *testing.T) {
			repo := newVendorTransactionRepo(t)
			plan := fullVendorTestPlan(t, repo)
			applyCount := 0
			// The apply fault is injected after the final atomic replacement,
			// so reverse rollback visits the complete mapped-plus-VERSION plan.
			// Fail every rollback position in turn and require restart authority.
			failedRestorePath := plan.candidates[len(plan.changed)-1-rollbackIndex].path
			plan.fileOps = &vendorFileOps{
				replace: func(repo vendorRepository, rel string, content []byte, mode os.FileMode) error {
					current := applyCount
					applyCount++
					if err := atomicReplaceVendorFile(repo, rel, content, mode); err != nil {
						return err
					}
					if current == len(plan.changed)-1 {
						return fmt.Errorf("injected apply failure")
					}
					return nil
				},
				restore: func(repo vendorRepository, candidate vendorCandidate) error {
					if candidate.path == failedRestorePath {
						return fmt.Errorf("injected rollback failure")
					}
					return restoreVendorOriginal(repo, candidate)
				},
			}

			err := applyVendorTransaction(repo, plan)
			var txErr *vendorError
			if !errors.As(err, &txErr) || txErr.class != "rollback-incomplete" {
				t.Fatalf("apply error = %v, want class rollback-incomplete", err)
			}
			if txErr.recovery == "" {
				t.Fatal("rollback-incomplete error omitted recovery location")
			}
			loadedRepo, recovery, err := loadVendorRecovery(repo.root)
			if err != nil {
				t.Fatalf("load recovery: %v", err)
			}
			if recovery == nil {
				t.Fatal("rollback-incomplete did not publish recovery authority")
			}
			if err := recoverVendorTransaction(loadedRepo, recovery); err != nil {
				t.Fatalf("fresh recovery: %v", err)
			}
			for _, candidate := range plan.candidates {
				// The normative recovery tuple records destination bytes, mode,
				// and existence, not pre-existing empty parent directories.
				candidate.original.missingParents = nil
				if err := verifyVendorOriginal(repo, candidate); err != nil {
					t.Fatalf("fresh recovery did not restore %s: %v", candidate.path, err)
				}
			}
			if _, err := os.Lstat(repo.recoveryRoot); !os.IsNotExist(err) {
				t.Fatalf("successful recovery retained authority: %v", err)
			}
		})
	}
}

func TestVendorRecoveryRecordRejectsUntrustedMaterial(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(t *testing.T, repo vendorRepository, transactionDir string)
	}{
		{
			name: "snapshot bytes",
			tamper: func(t *testing.T, _ vendorRepository, transactionDir string) {
				t.Helper()
				snapshot := firstVendorSnapshot(t, transactionDir)
				if err := os.WriteFile(snapshot, []byte("tampered\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing snapshot",
			tamper: func(t *testing.T, _ vendorRepository, transactionDir string) {
				t.Helper()
				if err := os.Remove(firstVendorSnapshot(t, transactionDir)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked snapshot",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				snapshot := firstVendorSnapshot(t, transactionDir)
				outside := filepath.Join(repo.root, "outside-snapshot")
				if err := os.WriteFile(outside, []byte("old\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(snapshot); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, snapshot); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "snapshot mode drift",
			tamper: func(t *testing.T, _ vendorRepository, transactionDir string) {
				t.Helper()
				if err := os.Chmod(firstVendorSnapshot(t, transactionDir), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "sentinel mode drift",
			tamper: func(t *testing.T, repo vendorRepository, _ string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(repo.recoveryRoot, vendorRecoverySentinelName), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked sentinel",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				sentinel := filepath.Join(repo.recoveryRoot, vendorRecoverySentinelName)
				if err := os.Remove(sentinel); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(transactionDir, vendorRecoveryManifestName), sentinel); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "manifest mode drift",
			tamper: func(t *testing.T, _ vendorRepository, transactionDir string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(transactionDir, vendorRecoveryManifestName), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "transaction mode drift",
			tamper: func(t *testing.T, _ vendorRepository, transactionDir string) {
				t.Helper()
				if err := os.Chmod(transactionDir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "snapshots directory mode drift",
			tamper: func(t *testing.T, _ vendorRepository, transactionDir string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(transactionDir, "snapshots"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "foreign transaction material",
			tamper: func(t *testing.T, _ vendorRepository, transactionDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(transactionDir, "foreign"), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "foreign recovery-root material",
			tamper: func(t *testing.T, repo vendorRepository, _ string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repo.recoveryRoot, "foreign"), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "recovery-root mode drift",
			tamper: func(t *testing.T, repo vendorRepository, _ string) {
				t.Helper()
				if err := os.Chmod(repo.recoveryRoot, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked recovery ancestor",
			tamper: func(t *testing.T, repo vendorRepository, _ string) {
				t.Helper()
				swornDir := filepath.Join(repo.gitAdmin, "sworn")
				realDir := filepath.Join(repo.gitAdmin, "sworn-real")
				if err := os.Rename(swornDir, realDir); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(realDir, swornDir); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked destination",
			tamper: func(t *testing.T, repo vendorRepository, _ string) {
				t.Helper()
				paths := make([]string, 0, len(batonFileMappings))
				for _, mapping := range batonFileMappings {
					paths = append(paths, filepath.ToSlash(mapping.Dest))
				}
				sort.Strings(paths)
				destination := filepath.Join(repo.root, filepath.FromSlash(paths[len(paths)-1]))
				if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(repo.root, "foreign-destination")
				if err := os.WriteFile(outside, []byte("foreign\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, destination); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "sentinel traversal",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				transactionID := filepath.Base(transactionDir)
				sentinel := vendorRecoverySentinel{
					RecordVersion:     1,
					TransactionSHA256: "sha256:" + transactionID,
					RecoveryDirectory: filepath.Join(repo.recoveryRoot, "..", transactionID),
				}
				data, err := marshalVendorControlJSON(sentinel)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(repo.recoveryRoot, vendorRecoverySentinelName), data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "sentinel self-cancelling traversal",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				transactionID := filepath.Base(transactionDir)
				separator := string(filepath.Separator)
				sentinel := vendorRecoverySentinel{
					RecordVersion:     1,
					TransactionSHA256: "sha256:" + transactionID,
					RecoveryDirectory: transactionDir + separator + ".." + separator + transactionID,
				}
				data, err := marshalVendorControlJSON(sentinel)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(repo.recoveryRoot, vendorRecoverySentinelName), data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "repository identity drift",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				rewriteVendorRecoveryManifest(t, repo, transactionDir, func(manifest *vendorRecoveryManifest) {
					manifest.RepositoryRoot += "-foreign"
				})
			},
		},
		{
			name: "Git-admin identity drift",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				rewriteVendorRecoveryManifest(t, repo, transactionDir, func(manifest *vendorRecoveryManifest) {
					manifest.GitAdminDirectory += "-foreign"
				})
			},
		},
		{
			name: "traversing destination",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				rewriteVendorRecoveryManifest(t, repo, transactionDir, func(manifest *vendorRecoveryManifest) {
					manifest.Destinations[0].Path = "../outside"
				})
			},
		},
		{
			name: "duplicate destination",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				rewriteVendorRecoveryManifest(t, repo, transactionDir, func(manifest *vendorRecoveryManifest) {
					manifest.Destinations[1].Path = manifest.Destinations[0].Path
				})
			},
		},
		{
			name: "missing destination tuple",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				rewriteVendorRecoveryManifest(t, repo, transactionDir, func(manifest *vendorRecoveryManifest) {
					manifest.Destinations = manifest.Destinations[:len(manifest.Destinations)-1]
				})
			},
		},
		{
			name: "missing VERSION tuple",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				rewriteVendorRecoveryManifest(t, repo, transactionDir, func(manifest *vendorRecoveryManifest) {
					for i, destination := range manifest.Destinations {
						if destination.Path == upstreamVersionPath {
							manifest.Destinations = append(manifest.Destinations[:i], manifest.Destinations[i+1:]...)
							return
						}
					}
					t.Fatal("recovery fixture omitted VERSION before tamper")
				})
			},
		},
		{
			name: "missing installer archive tuple",
			tamper: func(t *testing.T, repo vendorRepository, transactionDir string) {
				t.Helper()
				rewriteVendorRecoveryManifest(t, repo, transactionDir, func(manifest *vendorRecoveryManifest) {
					for i, destination := range manifest.Destinations {
						if destination.Path == installerArchivePath {
							manifest.Destinations = append(manifest.Destinations[:i], manifest.Destinations[i+1:]...)
							return
						}
					}
					t.Fatal("recovery fixture omitted installer archive before tamper")
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, transactionDir, changedPath, changedBytes := publishVendorTestRecovery(t)
			test.tamper(t, repo, transactionDir)

			before, err := os.ReadFile(filepath.Join(repo.root, filepath.FromSlash(changedPath)))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, changedBytes) {
				t.Fatal("test setup did not leave a changed destination")
			}
			_, recovery, err := loadVendorRecovery(repo.root)
			if err == nil || recovery != nil {
				t.Fatalf("tampered recovery load = (%v, %v), want fail-closed error", recovery, err)
			}
			after, readErr := os.ReadFile(filepath.Join(repo.root, filepath.FromSlash(changedPath)))
			if readErr != nil || !bytes.Equal(after, before) {
				t.Fatalf("tampered recovery touched destination: read=%v before=%q after=%q", readErr, before, after)
			}
			if _, statErr := os.Lstat(filepath.Join(repo.recoveryRoot, vendorRecoverySentinelName)); statErr != nil {
				t.Fatalf("tampered recovery authority was removed: %v", statErr)
			}
		})
	}
}

func TestVendorRepositoryUsesCurrentWorktreeGitAdmin(t *testing.T) {
	base := t.TempDir()
	repoRoot := filepath.Join(base, "linked")
	gitAdmin := filepath.Join(base, "main.git", "worktrees", "linked")
	if err := os.MkdirAll(gitAdmin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".git"), []byte("gitdir: "+gitAdmin+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo, recovery, err := loadVendorRecovery(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if recovery != nil {
		t.Fatal("new linked worktree unexpectedly has recovery material")
	}
	wantAdmin, err := filepath.EvalSymlinks(gitAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if repo.gitAdmin != wantAdmin {
		t.Fatalf("gitAdmin = %q, want current-worktree admin %q", repo.gitAdmin, wantAdmin)
	}
	if !strings.HasPrefix(repo.recoveryRoot, wantAdmin+string(filepath.Separator)) {
		t.Fatalf("recovery root %q is outside current-worktree admin %q", repo.recoveryRoot, wantAdmin)
	}
	if strings.HasPrefix(repo.recoveryRoot, filepath.Join(repoRoot, ".git")) {
		t.Fatalf("recovery root incorrectly used linked-worktree .git file: %q", repo.recoveryRoot)
	}
}

func TestVendorFreshInvocationRecoversOnlyAndRequiresRerun(t *testing.T) {
	repo, _, changedPath, _ := publishVendorTestRecovery(t)

	result, err := Vendor(VendorOpts{
		SourceDir: t.TempDir(), // deliberately invalid if ordinary planning runs
		RepoRoot:  repo.root,
	})
	if result != nil {
		t.Fatalf("recovery-only invocation returned vendor result: %#v", result)
	}
	var txErr *vendorError
	if !errors.As(err, &txErr) || txErr.class != "recovered-rerun-required" {
		t.Fatalf("recovery-only error = %v, want recovered-rerun-required", err)
	}
	got, readErr := os.ReadFile(filepath.Join(repo.root, filepath.FromSlash(changedPath)))
	if readErr != nil || string(got) != "original bytes\n" {
		t.Fatalf("fresh invocation did not restore original: read=%v got=%q", readErr, got)
	}
	if _, statErr := os.Lstat(repo.recoveryRoot); !os.IsNotExist(statErr) {
		t.Fatalf("fresh recovery retained authority: %v", statErr)
	}
}

func TestVendorRecoveryAuthorityRetiresAtomically(t *testing.T) {
	repo, _, changedPath, changedBytes := publishVendorTestRecovery(t)
	retiredRoot := filepath.Join(filepath.Dir(repo.recoveryRoot), ".baton-vendor-retired-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := os.Rename(repo.recoveryRoot, retiredRoot); err != nil {
		t.Fatal(err)
	}
	_, recovery, err := loadVendorRecovery(repo.root)
	var txErr *vendorError
	if !errors.As(err, &txErr) || txErr.class != "recovery-debris-cleaned-rerun-required" || recovery != nil {
		t.Fatalf("load after atomic authority retirement = (%v, %v), want debris-cleaned rerun", recovery, err)
	}
	got, err := os.ReadFile(filepath.Join(repo.root, filepath.FromSlash(changedPath)))
	if err != nil || !bytes.Equal(got, changedBytes) {
		t.Fatalf("retired cleanup debris restored primary bytes: read=%v got=%q", err, got)
	}
	if _, err := os.Lstat(retiredRoot); !os.IsNotExist(err) {
		t.Fatalf("fresh invocation retained payload-bearing cleanup debris: %v", err)
	}
}

func TestVendorRecoveryStagingDebrisIsScrubbedBeforeOrdinaryWrite(t *testing.T) {
	repo := newVendorTransactionRepo(t)
	if err := ensureVendorRecoveryParentPath(repo); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(filepath.Dir(repo.recoveryRoot), ".baton-vendor-staging-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "payload-snapshot"), []byte("private original bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, recovery, err := loadVendorRecovery(repo.root)
	var txErr *vendorError
	if !errors.As(err, &txErr) || txErr.class != "recovery-debris-cleaned-rerun-required" || recovery != nil {
		t.Fatalf("load staging debris = (%v, %v), want debris-cleaned rerun", recovery, err)
	}
	if _, err := os.Lstat(staging); !os.IsNotExist(err) {
		t.Fatalf("fresh invocation retained staged payload bytes: %v", err)
	}
}

type vendorTestCandidate struct {
	path    string
	desired []byte
}

func newVendorTransactionRepo(t *testing.T) vendorRepository {
	t.Helper()
	root := t.TempDir()
	makeVendorGitAdmin(t, root)
	repo, recovery, err := loadVendorRecovery(root)
	if err != nil {
		t.Fatal(err)
	}
	if recovery != nil {
		t.Fatal("new repo unexpectedly has recovery material")
	}
	return repo
}

func vendorTestPlan(t *testing.T, repo vendorRepository, inputs []vendorTestCandidate) *vendorPlan {
	t.Helper()
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].path < inputs[j].path })
	plan := &vendorPlan{candidates: make([]vendorCandidate, 0, len(inputs))}
	for _, input := range inputs {
		original, err := snapshotVendorOriginal(repo, input.path)
		if err != nil {
			t.Fatal(err)
		}
		plan.candidates = append(plan.candidates, vendorCandidate{
			path:        input.path,
			desired:     append([]byte(nil), input.desired...),
			desiredMode: 0o644,
			original:    original,
		})
		plan.changed = append(plan.changed, len(plan.candidates)-1)
	}
	return plan
}

func fullVendorTestPlan(t *testing.T, repo vendorRepository) *vendorPlan {
	t.Helper()
	versionBytes := []byte("baton-protocol: v0.13.1\nvendored: 2026-07-14\nupstream-sha: old\nupstream-digest: sha256:old\n")
	writeVendorTestFile(t, repo.root, upstreamVersionPath, versionBytes, 0o600)
	inputs := make([]vendorTestCandidate, 0, len(batonFileMappings)+2)
	for _, mapping := range batonFileMappings {
		inputs = append(inputs, vendorTestCandidate{
			path:    filepath.ToSlash(mapping.Dest),
			desired: []byte("candidate for " + filepath.ToSlash(mapping.Dest) + "\n"),
		})
	}
	inputs = append(inputs, vendorTestCandidate{
		path:    upstreamVersionPath,
		desired: []byte("baton-protocol: v0.15.1\nvendored: 2026-07-16\nupstream-sha: new\nupstream-digest: sha256:new\n"),
	})
	inputs = append(inputs, vendorTestCandidate{
		path:    installerArchivePath,
		desired: []byte("installer archive candidate\n"),
	})
	return vendorTestPlan(t, repo, inputs)
}

func publishVendorTestRecovery(t *testing.T) (vendorRepository, string, string, []byte) {
	t.Helper()
	repo := newVendorTransactionRepo(t)
	firstPath := filepath.ToSlash(batonFileMappings[0].Dest)
	writeVendorTestFile(t, repo.root, firstPath, []byte("original bytes\n"), 0o640)
	plan := fullVendorTestPlan(t, repo)
	for i := range plan.candidates {
		original, err := snapshotVendorOriginal(repo, plan.candidates[i].path)
		if err != nil {
			t.Fatal(err)
		}
		plan.candidates[i].original = original
	}
	changedBytes := []byte("non-authoritative changed bytes\n")
	if err := atomicReplaceVendorFile(repo, firstPath, changedBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	recovery, err := publishVendorRecovery(repo, plan)
	if err != nil {
		t.Fatal(err)
	}
	return repo, recovery.transactionDir, firstPath, changedBytes
}

func rewriteVendorRecoveryManifest(t *testing.T, repo vendorRepository, oldTransactionDir string, mutate func(*vendorRecoveryManifest)) {
	t.Helper()
	manifestPath := filepath.Join(oldTransactionDir, vendorRecoveryManifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest vendorRecoveryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	data, err = marshalVendorControlJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	newID := hex.EncodeToString(digest[:])
	newTransactionDir := filepath.Join(repo.recoveryRoot, newID)
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldTransactionDir, newTransactionDir); err != nil {
		t.Fatal(err)
	}
	sentinel := vendorRecoverySentinel{
		RecordVersion:     1,
		TransactionSHA256: "sha256:" + newID,
		RecoveryDirectory: newTransactionDir,
	}
	sentinelBytes, err := marshalVendorControlJSON(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.recoveryRoot, vendorRecoverySentinelName), sentinelBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

func firstVendorSnapshot(t *testing.T, transactionDir string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(transactionDir, "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("recovery fixture has no snapshot")
	}
	return filepath.Join(transactionDir, "snapshots", entries[0].Name())
}

func writeVendorTestFile(t *testing.T, root, rel string, data []byte, mode os.FileMode) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, data, mode.Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(abs, mode.Perm()); err != nil {
		t.Fatal(err)
	}
}

func assertNoVendorTemps(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".sworn-baton-vendor-") || strings.HasPrefix(entry.Name(), ".sworn-baton-control-") {
			t.Errorf("temporary transaction file remains: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
