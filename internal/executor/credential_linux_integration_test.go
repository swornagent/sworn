//go:build linux

package executor

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCredentialReadOnlyPersistsRefreshAndCannotWriteCandidate(t *testing.T) {
	executor, credentialPath := newCredentialReadOnlyTestExecutor(
		t, `{"auth_mode":"chatgpt","token":"before"}`,
	)
	credentialBefore, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "candidate.txt"), []byte("candidate\n"), 0o600)
	workspaceDigest, _, err := MeasureWorkspace(
		context.Background(), workspace, executor.options.Limits.InputBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	script := []byte(strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		`[ "$(cat /workspace/candidate.txt)" = "candidate" ]`,
		"if printf mutation > /workspace/forbidden 2>/dev/null; then exit 90; fi",
		`[ "$CODEX_HOME" = "/home/sworn/.codex" ]`,
		`[ "$(cat /home/sworn/.codex/auth.json)" = '{"auth_mode":"chatgpt","token":"before"}' ]`,
		`printf '%s' '{"auth_mode":"chatgpt","token":"refreshed"}' > /home/sworn/.codex/auth.json`,
		"printf 'verified-read-only\\n'",
	}, "\n") + "\n")
	executablePath := filepath.Join(t.TempDir(), "verifier")
	writeTestFile(t, executablePath, script, 0o755)

	const invocationID = "credential-read-only-verifier"
	completion, err := executor.RunCredentialReadOnly(context.Background(), Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               invocationID,
		Role:             "verifier",
		NestedSandbox:    true,
		CredentialAccess: true,
		Workspace:        workspace,
		WorkspaceDigest:  workspaceDigest,
		WorkspaceAccess:  WorkspaceReadOnly,
		ExecutableInput:  "verifier",
		Inputs: []Input{{
			Name: "verifier", Path: executablePath, Digest: digestBytes(script),
		}},
		Argv:    []string{"/inputs/verifier"},
		Network: NetworkHost,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("run credentialed read-only invocation: %v; completion=%#v", err, completion)
	}
	if completion.ExitCode != 0 || string(completion.Stdout) != "verified-read-only\n" ||
		completion.WorkspaceAccess != WorkspaceReadOnly || !completion.CredentialAccess ||
		completion.ExecutableInput != "verifier" || completion.Export != nil || len(completion.Inputs) != 1 {
		t.Fatalf("credentialed read-only completion = %#v", completion)
	}
	if bound := completion.Inputs[0]; bound.Name != "verifier" || bound.Digest != digestBytes(script) ||
		bound.Size != uint64(len(script)) {
		t.Fatalf("credentialed read-only executable binding = %#v", bound)
	}
	if observed, err := os.ReadFile(filepath.Join(workspace, "candidate.txt")); err != nil || string(observed) != "candidate\n" {
		t.Fatalf("source candidate changed: contents=%q error=%v", observed, err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "forbidden")); !os.IsNotExist(err) {
		t.Fatalf("read-only invocation wrote to source candidate: %v", err)
	}
	credentialContents, err := os.ReadFile(credentialPath)
	if err != nil || string(credentialContents) != `{"auth_mode":"chatgpt","token":"refreshed"}` {
		t.Fatalf("host credential after refresh = %q, %v", credentialContents, err)
	}
	credentialAfter, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(credentialBefore, credentialAfter) {
		t.Fatal("credentialed read-only refresh replaced the credential inode")
	}
	if _, err := os.Lstat(executor.contentBoundRuntimePath(invocationID)); !os.IsNotExist(err) {
		t.Fatalf("credentialed read-only runtime remains: %v", err)
	}
	cleanup, err := executor.ReconcileContentBound(context.Background(), invocationID)
	if err != nil || cleanup.InvocationID() != invocationID {
		t.Fatalf("reconcile credentialed read-only completion: cleanup=%#v error=%v", cleanup, err)
	}
}

func TestCredentialReadOnlyCancellationReleasesBoundaryForReuse(t *testing.T) {
	executor, _ := newCredentialReadOnlyTestExecutor(t, `{"auth_mode":"chatgpt","token":"before"}`)
	workspace, workspaceDigest := emptyTestWorkspace(t, executor)
	script := []byte(strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		`[ "$(cat /home/sworn/.codex/auth.json)" = '{"auth_mode":"chatgpt","token":"before"}' ]`,
		`if [ "${1-}" = "cancel" ]; then printf 'started\n'; sleep 60; fi`,
		"printf 'reused\\n'",
	}, "\n") + "\n")
	executablePath := filepath.Join(t.TempDir(), "verifier")
	writeTestFile(t, executablePath, script, 0o755)
	baseInvocation := Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		Role:             "verifier",
		NestedSandbox:    true,
		CredentialAccess: true,
		Workspace:        workspace,
		WorkspaceDigest:  workspaceDigest,
		WorkspaceAccess:  WorkspaceReadOnly,
		ExecutableInput:  "verifier",
		Inputs: []Input{{
			Name: "verifier", Path: executablePath, Digest: digestBytes(script),
		}},
		Network: NetworkHost,
		Timeout: 10 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	cancelledInvocation := baseInvocation
	cancelledInvocation.ID = "credential-read-only-cancel"
	cancelledInvocation.Argv = []string{"/inputs/verifier", "cancel"}
	cancelled, err := executor.RunCredentialReadOnly(ctx, cancelledInvocation)
	if err != nil || !cancelled.Cancelled || cancelled.Export != nil || !cancelled.CredentialAccess {
		t.Fatalf("credentialed read-only cancellation: completion=%#v error=%v", cancelled, err)
	}
	if _, err := os.Lstat(executor.contentBoundRuntimePath(cancelledInvocation.ID)); !os.IsNotExist(err) {
		t.Fatalf("cancelled credentialed read-only runtime remains: %v", err)
	}
	cleanup, err := executor.ReconcileContentBound(context.Background(), cancelledInvocation.ID)
	if err != nil || cleanup.InvocationID() != cancelledInvocation.ID {
		t.Fatalf("reconcile cancelled credentialed read-only run: cleanup=%#v error=%v", cleanup, err)
	}

	reusedInvocation := baseInvocation
	reusedInvocation.ID = "credential-read-only-after-cancel"
	reusedInvocation.Argv = []string{"/inputs/verifier", "reuse"}
	reused, err := executor.RunCredentialReadOnly(context.Background(), reusedInvocation)
	if err != nil || reused.ExitCode != 0 || string(reused.Stdout) != "reused\n" ||
		!reused.CredentialAccess || reused.Export != nil {
		t.Fatalf("credentialed read-only reuse: completion=%#v error=%v", reused, err)
	}
	if _, err := executor.ReconcileContentBound(context.Background(), reusedInvocation.ID); err != nil {
		t.Fatalf("reconcile reused credentialed read-only run: %v", err)
	}
}

func TestCredentialReadOnlyBusyCredentialReleasesContentOwnership(t *testing.T) {
	executor, credentialPath := newCredentialReadOnlyTestExecutor(t, `{"auth_mode":"chatgpt"}`)
	held, err := acquireCredentialFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseCredentialFile(held) })
	workspace, workspaceDigest := emptyTestWorkspace(t, executor)
	script := []byte("#!/bin/sh\nexit 0\n")
	executablePath := filepath.Join(t.TempDir(), "verifier")
	writeTestFile(t, executablePath, script, 0o755)
	const invocationID = "credential-read-only-busy"
	completion, err := executor.RunCredentialReadOnly(context.Background(), Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               invocationID,
		Role:             "verifier",
		NestedSandbox:    true,
		CredentialAccess: true,
		Workspace:        workspace,
		WorkspaceDigest:  workspaceDigest,
		WorkspaceAccess:  WorkspaceReadOnly,
		ExecutableInput:  "verifier",
		Inputs: []Input{{
			Name: "verifier", Path: executablePath, Digest: digestBytes(script),
		}},
		Argv:    []string{"/inputs/verifier"},
		Network: NetworkHost,
		Timeout: 10 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "credential file is busy") ||
		completion.InvocationID != "" || completion.Unit != "" {
		t.Fatalf("busy credentialed read-only invocation: completion=%#v error=%v", completion, err)
	}
	cleanup, err := executor.ReconcileContentBound(context.Background(), invocationID)
	if err != nil || cleanup.InvocationID() != invocationID {
		t.Fatalf("content ownership remained held after busy credential: cleanup=%#v error=%v", cleanup, err)
	}
}

func newCredentialReadOnlyTestExecutor(t *testing.T, credentialContents string) (*LinuxExecutor, string) {
	t.Helper()
	base := requireLinuxExecutor(t)
	credentialPath := newTestCredentialFile(t, credentialContents)
	options := base.options
	options.CredentialFile = credentialPath
	options.AllowCredentialFile = true
	options.AllowHostNetwork = true
	options.AllowNestedSandbox = true
	executor, err := NewLinux(options)
	if err != nil {
		t.Fatalf("configure credentialed read-only executor: %v", err)
	}
	return executor, credentialPath
}

func TestWritableCredentialFilePersistsRefreshThroughContainedService(t *testing.T) {
	base := requireWritableLinuxExecutor(t)
	credentialPath := newTestCredentialFile(t, `{"auth_mode":"chatgpt","token":"before"}`)
	before, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	options := base.options
	options.CredentialFile = credentialPath
	options.AllowCredentialFile = true
	executor, err := NewLinux(options)
	if err != nil {
		t.Fatalf("configure credential-file executor: %v", err)
	}

	workspace, workspaceDigest := emptyTestWorkspace(t, executor)
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               "credential-refresh-through-service",
		Role:             "builder",
		CredentialAccess: true,
		Workspace:        workspace,
		WorkspaceDigest:  workspaceDigest,
		WorkspaceAccess:  WorkspaceWritableExport,
		Argv: []string{
			"/usr/bin/python3", "-c", strings.Join([]string{
				"import os",
				"from pathlib import Path",
				"auth = Path('/home/sworn/.codex/auth.json')",
				"assert os.environ['CODEX_HOME'] == '/home/sworn/.codex'",
				"assert not Path('/proc/self/fd/4').exists()",
				`assert auth.read_text() == '{"auth_mode":"chatgpt","token":"before"}'`,
				`auth.write_text('{"auth_mode":"chatgpt","token":"refreshed"}')`,
				"Path('/workspace/proof').write_text('credential refresh persisted')",
			}, "; "),
		},
		Network: NetworkNone,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("run contained credential refresh: %v; completion=%#v", err, completion)
	}
	if completion.ExitCode != 0 || !completion.CredentialAccess || completion.Export == nil {
		t.Fatalf("credential refresh completion = %#v", completion)
	}
	export := *completion.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, export) })
	assertWorkspaceFile(t, filepath.Join(export.Path, "proof"), "credential refresh persisted", 0o600)

	contents, err := os.ReadFile(credentialPath)
	if err != nil || string(contents) != `{"auth_mode":"chatgpt","token":"refreshed"}` {
		t.Fatalf("host credential after refresh = %q, %v", contents, err)
	}
	after, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("contained refresh replaced the credential inode")
	}

	blindWorkspace, blindDigest := emptyTestWorkspace(t, executor)
	blind, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "credential-blind-through-service",
		Role:            "builder",
		Workspace:       blindWorkspace,
		WorkspaceDigest: blindDigest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv: []string{
			"/usr/bin/python3", "-c", strings.Join([]string{
				"import os",
				"from pathlib import Path",
				"assert 'CODEX_HOME' not in os.environ",
				"assert not Path('/home/sworn/.codex/auth.json').exists()",
				"Path('/workspace/proof').write_text('blind')",
			}, "; "),
		},
		Network: NetworkNone,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("run credential-blind contained service: %v; completion=%#v", err, blind)
	}
	if blind.ExitCode != 0 || blind.CredentialAccess || blind.Export == nil {
		t.Fatalf("credential-blind completion = %#v", blind)
	}
	blindExport := *blind.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, blindExport) })
	assertWorkspaceFile(t, filepath.Join(blindExport.Path, "proof"), "blind", 0o600)

	cancelWorkspace, cancelDigest := emptyTestWorkspace(t, executor)
	cancelContext, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cancelled, err := executor.RunWritable(cancelContext, Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               "credential-cancel-through-service",
		Role:             "builder",
		CredentialAccess: true,
		Workspace:        cancelWorkspace,
		WorkspaceDigest:  cancelDigest,
		WorkspaceAccess:  WorkspaceWritableExport,
		Argv: []string{
			"/usr/bin/python3", "-c",
			"import time; print('started', flush=True); time.sleep(60)",
		},
		Network: NetworkNone,
		Timeout: 10 * time.Second,
	})
	if err != nil || !cancelled.Cancelled || !cancelled.CredentialAccess || cancelled.Export != nil {
		t.Fatalf("credential cancellation completion=%#v, error=%v", cancelled, err)
	}
	if _, err := os.Lstat(executor.writableRuntimePath("credential-cancel-through-service")); !os.IsNotExist(err) {
		t.Fatalf("credential cancellation runtime remains: %v", err)
	}
	contents, err = os.ReadFile(credentialPath)
	if err != nil || string(contents) != `{"auth_mode":"chatgpt","token":"refreshed"}` {
		t.Fatalf("credential after cancellation = %q, %v", contents, err)
	}

	reuseWorkspace, reuseDigest := emptyTestWorkspace(t, executor)
	reused, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               "credential-reuse-after-cancel",
		Role:             "builder",
		CredentialAccess: true,
		Workspace:        reuseWorkspace,
		WorkspaceDigest:  reuseDigest,
		WorkspaceAccess:  WorkspaceWritableExport,
		Argv: []string{
			"/usr/bin/python3", "-c", strings.Join([]string{
				"from pathlib import Path",
				`assert Path('/home/sworn/.codex/auth.json').read_text() == '{"auth_mode":"chatgpt","token":"refreshed"}'`,
				"Path('/workspace/proof').write_text('reused')",
			}, "; "),
		},
		Network: NetworkNone,
		Timeout: 10 * time.Second,
	})
	if err != nil || reused.ExitCode != 0 || !reused.CredentialAccess || reused.Export == nil {
		t.Fatalf("credential reuse completion=%#v, error=%v", reused, err)
	}
	reuseExport := *reused.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, reuseExport) })
	assertWorkspaceFile(t, filepath.Join(reuseExport.Path, "proof"), "reused", 0o600)

	failureCases := []struct {
		name           string
		invocationID   string
		argv           []string
		timeout        time.Duration
		wantExitCode   int
		wantTimedOut   bool
		wantTruncated  bool
		wantQuarantine bool
	}{
		{
			name:         "nonzero target",
			invocationID: "credential-nonzero-through-service",
			argv: []string{
				"/usr/bin/python3", "-c", "raise SystemExit(7)",
			},
			timeout:        10 * time.Second,
			wantExitCode:   7,
			wantQuarantine: true,
		},
		{
			name:         "timeout",
			invocationID: "credential-timeout-through-service",
			argv: []string{
				"/usr/bin/python3", "-c", "import time; time.sleep(60)",
			},
			timeout:      300 * time.Millisecond,
			wantExitCode: engineDeathExitCode,
			wantTimedOut: true,
		},
		{
			name:         "output overflow",
			invocationID: "credential-overflow-through-service",
			argv: []string{
				"/usr/bin/python3", "-c",
				"import sys,time; sys.stdout.write('x'*100000); sys.stdout.flush(); time.sleep(60)",
			},
			timeout:       10 * time.Second,
			wantExitCode:  engineDeathExitCode,
			wantTruncated: true,
		},
	}
	for _, test := range failureCases {
		t.Run(test.name, func(t *testing.T) {
			failureWorkspace, failureDigest := emptyTestWorkspace(t, executor)
			failed, err := executor.RunWritable(context.Background(), Invocation{
				SchemaVersion:    InvocationSchemaVersion,
				ID:               test.invocationID,
				Role:             "builder",
				CredentialAccess: true,
				Workspace:        failureWorkspace,
				WorkspaceDigest:  failureDigest,
				WorkspaceAccess:  WorkspaceWritableExport,
				Argv:             test.argv,
				Network:          NetworkNone,
				Timeout:          test.timeout,
			})
			if err != nil || failed.ExitCode != test.wantExitCode || !failed.CredentialAccess ||
				failed.TimedOut != test.wantTimedOut || failed.OutputTruncated != test.wantTruncated ||
				(failed.Export != nil) != test.wantQuarantine {
				t.Fatalf("credential failure completion=%#v, error=%v", failed, err)
			}
			if failed.Export != nil {
				if err := releaseOrRemoveExport(executor, *failed.Export); err != nil {
					t.Fatalf("discard failure quarantine: %v", err)
				}
			}
			if _, err := os.Lstat(executor.writableRuntimePath(test.invocationID)); !os.IsNotExist(err) {
				t.Fatalf("credential failure runtime remains: %v", err)
			}
			assertCredentialExecutorReusable(
				t,
				executor,
				`{"auth_mode":"chatgpt","token":"refreshed"}`,
				test.invocationID+"-reuse",
			)
		})
	}
}

func assertCredentialExecutorReusable(
	t *testing.T,
	executor *LinuxExecutor,
	wantCredential string,
	invocationID string,
) {
	t.Helper()
	workspace, digest := emptyTestWorkspace(t, executor)
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               invocationID,
		Role:             "builder",
		CredentialAccess: true,
		Workspace:        workspace,
		WorkspaceDigest:  digest,
		WorkspaceAccess:  WorkspaceWritableExport,
		Argv: []string{
			"/usr/bin/python3", "-c",
			"from pathlib import Path; " +
				"assert Path('/home/sworn/.codex/auth.json').read_text() == " + strconv.Quote(wantCredential) + "; " +
				"Path('/workspace/proof').write_text('reused')",
		},
		Network: NetworkNone,
		Timeout: 10 * time.Second,
	})
	if err != nil || completion.ExitCode != 0 || !completion.CredentialAccess || completion.Export == nil {
		t.Fatalf("credential executor reuse completion=%#v, error=%v", completion, err)
	}
	if err := releaseOrRemoveExport(executor, *completion.Export); err != nil {
		t.Fatalf("discard credential reuse export: %v", err)
	}
}
