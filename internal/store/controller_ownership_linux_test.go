//go:build linux

package store

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	storeOwnershipHelperMode  = "SWORN_STORE_OWNERSHIP_HELPER_MODE"
	storeOwnershipHelperPath  = "SWORN_STORE_OWNERSHIP_HELPER_PATH"
	storeOwnershipHelperOwner = "SWORN_STORE_OWNERSHIP_HELPER_OWNER"
)

func TestStoreControllerOwnershipPhasesContentionAndRelease(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	firstStore := openTestStore(t, path)
	secondStore := openTestStore(t, path)
	t.Cleanup(func() {
		_ = secondStore.Close()
		_ = firstStore.Close()
	})

	first, err := firstStore.AcquireControllerOwnership("controller-first")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ValidateRecovery(firstStore, "controller-first"); err != nil {
		t.Fatalf("validate recovery ownership: %v", err)
	}
	for name, check := range map[string]func() error{
		"wrong Store": func() error {
			return first.ValidateRecovery(secondStore, "controller-first")
		},
		"wrong owner": func() error {
			return first.ValidateRecovery(firstStore, "controller-foreign")
		},
		"nil": func() error {
			return (*ControllerOwnership)(nil).ValidateRecovery(firstStore, "controller-first")
		},
		"zero": func() error {
			return (&ControllerOwnership{}).ValidateRecovery(firstStore, "controller-first")
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(); !errors.Is(err, ErrInvalidControllerOwnership) {
				t.Fatalf("foreign validation error = %v", err)
			}
		})
	}
	if err := first.ValidateActive(firstStore, "controller-first"); !errors.Is(err, ErrInvalidControllerOwnership) {
		t.Fatalf("recovery ownership validated active: %v", err)
	}
	if second, err := secondStore.AcquireControllerOwnership("controller-second"); second != nil || !errors.Is(err, ErrControllerOwnershipUnavailable) {
		t.Fatalf("same-inode contender ownership=%#v, error=%v", second, err)
	}
	if err := first.Activate(ctx, firstStore, "controller-first"); err != nil {
		t.Fatal(err)
	}
	if err := first.ValidateActive(firstStore, "controller-first"); err != nil {
		t.Fatalf("validate active ownership: %v", err)
	}
	if err := first.Activate(ctx, firstStore, "controller-first"); err != nil {
		t.Fatalf("idempotent activation: %v", err)
	}

	foreignCopy := *first
	if err := foreignCopy.ValidateActive(firstStore, "controller-first"); !errors.Is(err, ErrInvalidControllerOwnership) {
		t.Fatalf("copied ownership validation error = %v", err)
	}
	if err := foreignCopy.Close(); !errors.Is(err, ErrInvalidControllerOwnership) {
		t.Fatalf("copied ownership close error = %v", err)
	}
	if err := first.ValidateActive(firstStore, "controller-first"); err != nil {
		t.Fatalf("copied close disturbed exact owner: %v", err)
	}
	if err := firstStore.Close(); err == nil || !strings.Contains(err.Error(), "active controller ownership") {
		t.Fatalf("Store.Close with active ownership error = %v", err)
	}
	if err := first.ValidateActive(firstStore, "controller-first"); err != nil {
		t.Fatalf("failed Store.Close disturbed ownership: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("idempotent ownership close: %v", err)
	}
	if err := first.ValidateActive(firstStore, "controller-first"); !errors.Is(err, ErrInvalidControllerOwnership) {
		t.Fatalf("released ownership validation error = %v", err)
	}

	second, err := secondStore.AcquireControllerOwnership("controller-second")
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreControllerOwnershipActivationRequiresCompleteRecovery(t *testing.T) {
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO runs (
			run_id, delivery_id, repository_id, target_ref, plan_digest,
			revision, phase, terminal, state_json, created_at_us, updated_at_us
		) VALUES ('run-recovery', 'delivery-recovery', 'repository-recovery',
		          'refs/heads/recovery', ?, 0, 'planned', 0, CAST('{}' AS BLOB), 1, 1)`,
		"sha256:"+strings.Repeat("a", 64),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES ('command-recovery', 'run-recovery', 'test.effect', 0, ?,
		          CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 1)`,
		"sha256:"+strings.Repeat("b", 64),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state,
			attempt, owner_id, receipt_json, last_error, created_at_us,
			started_at_us, completed_at_us
		) VALUES ('effect-recovery', 'run-recovery', 'command-recovery', 0,
		          'check.local', CAST('{}' AS BLOB), 'running', 1,
		          'stopped-worker', NULL, NULL, 1, 1, NULL)`,
	); err != nil {
		t.Fatal(err)
	}
	ownership, err := control.AcquireControllerOwnership("controller-recovery")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ownership.Close() })
	if err := ownership.Activate(ctx, control, "controller-recovery"); err == nil ||
		!strings.Contains(err.Error(), "1 unresolved effects") {
		t.Fatalf("activation with running effect error = %v", err)
	}
	if err := ownership.ValidateRecovery(control, "controller-recovery"); err != nil {
		t.Fatalf("failed activation left recovery phase: %v", err)
	}
	if recovered, err := control.RecoverControlledInterruptedEffects(
		ctx, ownership, "controller-recovery", "test controller stopped",
	); err != nil || recovered != 1 {
		t.Fatalf("recover interrupted effects = %d, %v", recovered, err)
	}
	if err := ownership.Activate(ctx, control, "controller-recovery"); err == nil ||
		!strings.Contains(err.Error(), "1 unresolved effects") {
		t.Fatalf("activation with unknown effect error = %v", err)
	}
}

func TestStoreControllerOwnershipRejectsPathReplacementBeforeAndAfterAcquire(t *testing.T) {
	t.Run("before acquire", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "control.db")
		control := openTestStore(t, path)
		t.Cleanup(func() { _ = control.Close() })
		replaceStorePath(t, path)
		if ownership, err := control.AcquireControllerOwnership("controller-replaced"); ownership != nil || err == nil || !strings.Contains(err.Error(), "was replaced") {
			t.Fatalf("replacement ownership=%#v, error=%v", ownership, err)
		}
	})

	t.Run("after acquire", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "control.db")
		control := openTestStore(t, path)
		t.Cleanup(func() { _ = control.Close() })
		ownership, err := control.AcquireControllerOwnership("controller-replaced")
		if err != nil {
			t.Fatal(err)
		}
		replaceStorePath(t, path)
		if err := ownership.ValidateRecovery(control, "controller-replaced"); !errors.Is(err, ErrInvalidControllerOwnership) || !strings.Contains(err.Error(), "was replaced") {
			t.Fatalf("replacement validation error = %v", err)
		}
		if err := ownership.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStoreControllerOwnershipRejectsParentReplacementAndHardLinks(t *testing.T) {
	t.Run("parent replacement", func(t *testing.T) {
		root := t.TempDir()
		parent := filepath.Join(root, "state")
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(parent, "control.db")
		control := openTestStore(t, path)
		t.Cleanup(func() { _ = control.Close() })
		retained := filepath.Join(root, "state-retained")
		if err := os.Rename(parent, retained); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		if ownership, err := control.AcquireControllerOwnership("controller-parent"); ownership != nil || err == nil || !strings.Contains(err.Error(), "parent") ||
			!strings.Contains(err.Error(), "replaced") {
			t.Fatalf("parent replacement ownership=%#v, error=%v", ownership, err)
		}
	})

	t.Run("hard link", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "control.db")
		control := openTestStore(t, path)
		t.Cleanup(func() { _ = control.Close() })
		alias := filepath.Join(directory, "alias.db")
		if err := os.Link(path, alias); err != nil {
			t.Fatal(err)
		}
		if ownership, err := control.AcquireControllerOwnership("controller-linked"); ownership != nil || err == nil || !strings.Contains(err.Error(), "hard links") {
			t.Fatalf("hard-link ownership=%#v, error=%v", ownership, err)
		}
		if err := os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		ownership, err := control.AcquireControllerOwnership("controller-unlinked")
		if err != nil {
			t.Fatalf("acquire after hard-link removal: %v", err)
		}
		if err := ownership.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStoreControllerOwnershipRejectsUnsafeParentPermissions(t *testing.T) {
	t.Run("before acquire", func(t *testing.T) {
		parent := t.TempDir()
		path := filepath.Join(parent, "control.db")
		control := openTestStore(t, path)
		t.Cleanup(func() { _ = control.Close() })
		if err := os.Chmod(parent, 0o777); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

		ownership, err := control.AcquireControllerOwnership("controller-unsafe-parent")
		if ownership != nil || err == nil || !strings.Contains(err.Error(), "group and world write bits") {
			t.Fatalf("unsafe-parent ownership=%#v, error=%v", ownership, err)
		}
	})

	t.Run("after acquire", func(t *testing.T) {
		parent := t.TempDir()
		path := filepath.Join(parent, "control.db")
		control := openTestStore(t, path)
		t.Cleanup(func() { _ = control.Close() })
		ownership, err := control.AcquireControllerOwnership("controller-parent-drift")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ownership.Close() })
		if err := os.Chmod(parent, 0o777); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

		if err := ownership.ValidateRecovery(control, "controller-parent-drift"); !errors.Is(err, ErrInvalidControllerOwnership) ||
			!strings.Contains(err.Error(), "group and world write bits") {
			t.Fatalf("unsafe-parent validation error = %v", err)
		}
		if err := os.Chmod(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := ownership.ValidateRecovery(control, "controller-parent-drift"); err != nil {
			t.Fatalf("validation after restoring safe parent permissions: %v", err)
		}
	})
}

func TestStoreControllerOwnershipIndependentProcessesContendAndCrashRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	control := openTestStore(t, path)
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	holder := storeOwnershipHelperCommand(t, "hold", path, "controller-holder")
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	holder.Stderr = &stderr
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	holderLive := true
	t.Cleanup(func() {
		if holderLive {
			_ = holder.Process.Kill()
			_ = holder.Wait()
		}
	})

	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
			return
		}
		ready <- ""
	}()
	select {
	case line := <-ready:
		if line != "READY" {
			t.Fatalf("holder readiness = %q, stderr=%q", line, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ownership holder")
	}

	output, err := storeOwnershipHelperCommand(t, "attempt", path, "controller-contender").CombinedOutput()
	if err != nil || !strings.Contains(string(output), "CONTENDED") {
		t.Fatalf("independent contender output=%q, error=%v", output, err)
	}
	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := holder.Wait(); err == nil {
		t.Fatal("SIGKILLed holder unexpectedly exited successfully")
	}
	holderLive = false

	output, err = storeOwnershipHelperCommand(t, "attempt", path, "controller-successor").CombinedOutput()
	if err != nil || !strings.Contains(string(output), "ACQUIRED") {
		t.Fatalf("post-crash successor output=%q, error=%v", output, err)
	}
}

func TestStoreControllerOwnershipProcessHelper(t *testing.T) {
	mode := os.Getenv(storeOwnershipHelperMode)
	if mode == "" {
		return
	}
	path := os.Getenv(storeOwnershipHelperPath)
	ownerID := os.Getenv(storeOwnershipHelperOwner)
	control, err := OpenConfigured(context.Background(), path, ControlConfiguration{
		LocalCheckRuntimeManifestDigest: "sha256:" + strings.Repeat("e", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	ownership, err := control.AcquireControllerOwnership(ownerID)
	if mode == "attempt" && errors.Is(err, ErrControllerOwnershipUnavailable) {
		_ = control.Close()
		fmt.Println("CONTENDED")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Activate(context.Background(), control, ownerID); err != nil {
		t.Fatal(err)
	}
	if mode == "attempt" {
		if err := ownership.Close(); err != nil {
			t.Fatal(err)
		}
		if err := control.Close(); err != nil {
			t.Fatal(err)
		}
		fmt.Println("ACQUIRED")
		return
	}
	if mode != "hold" {
		t.Fatalf("unknown helper mode %q", mode)
	}
	if _, err := fmt.Fprintln(os.Stdout, "READY"); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func storeOwnershipHelperCommand(t *testing.T, mode, path, ownerID string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestStoreControllerOwnershipProcessHelper$")
	command.Env = append(os.Environ(),
		storeOwnershipHelperMode+"="+mode,
		storeOwnershipHelperPath+"="+path,
		storeOwnershipHelperOwner+"="+ownerID,
	)
	return command
}

func replaceStorePath(t *testing.T, path string) {
	t.Helper()
	retained := path + ".retained"
	if err := os.Rename(path, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
}
