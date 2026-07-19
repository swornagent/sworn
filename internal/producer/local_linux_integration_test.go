//go:build linux

package producer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/store"
)

const producerShimSentinel = "__sworn_producer_shim_test__"

func TestProducerExecutorShimProcess(t *testing.T) {
	index := -1
	for candidate, argument := range os.Args {
		if argument == producerShimSentinel {
			index = candidate
			break
		}
	}
	if index < 0 {
		return
	}
	os.Exit(executor.RunShim(os.Args[index+1:], os.Stdin, os.Stdout, os.Stderr))
}

func TestLocalProducerUsesRealContainedBoundary(t *testing.T) {
	ctx := context.Background()
	runner := requireProducerLinuxExecutor(t)
	repository, candidate, checked := prepareProducerCandidate(t)
	control, err := store.Open(ctx, filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	definitionBytes, err := protocol.EncodeCanonical(LocalCheckDefinition{
		SchemaVersion: LocalCheckDefinitionSchemaVersion,
		Argv: []string{
			"/usr/bin/python3", "-c",
			"from pathlib import Path; assert Path('value.txt').read_text() == 'candidate\\n'; print('measured')",
		},
		WorkingDirectory: ".", TimeoutSeconds: 10,
		Evidence: EvidenceDefinition{
			ID: "real-check", AcceptanceIDs: []string{"AC1"}, Boundary: "component",
			Observed: "The real contained check exited successfully.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := control.PutArtifact(ctx, "application/json", definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunLocal(ctx, runner, control, Request{
		CheckID: "real", RunID: "real-check-run",
		Definition: protocol.Artifact{Ref: digest, MediaType: "application/json", Digest: digest},
		Repository: repository, Candidate: candidate, Workspace: checked,
	})
	if err != nil || result.Check == nil || result.Evidence == nil {
		t.Fatalf("real RunLocal = %#v, %v", result, err)
	}
	_, receiptBytes, err := control.Artifact(ctx, result.Receipt.Digest)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := protocol.ParseLocalCheckReceipt(receiptBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, stdout, err := control.Artifact(ctx, receipt.Stdout.Digest)
	if err != nil || string(stdout) != "measured\n" {
		t.Fatalf("real check stdout = %q, %v", stdout, err)
	}
}

func requireProducerLinuxExecutor(t *testing.T) *executor.LinuxExecutor {
	t.Helper()
	runtimeRoot := t.TempDir()
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	limits := executor.DefaultLimits()
	limits.Runtime = 30 * time.Second
	limits.MemoryBytes = 256 << 20
	limits.Tasks = 64
	limits.FileBytes = 16 << 20
	limits.TempBytes = 32 << 20
	limits.HomeBytes = 16 << 20
	limits.InputBytes = 32 << 20
	limits.WorkspaceBytes = 32 << 20
	limits.StdoutBytes = 32 << 10
	limits.StderrBytes = 32 << 10
	runner, err := executor.NewLinux(executor.Options{
		RuntimeRoot: runtimeRoot,
		ShimArgv: []string{
			testBinary, "-test.run=^TestProducerExecutorShimProcess$", "--", producerShimSentinel,
		},
		Limits: limits,
	})
	if err == nil {
		_, err = runner.Probe(context.Background())
	}
	if err != nil {
		if os.Getenv("SWORN_REQUIRE_LINUX_EXECUTOR") == "1" {
			t.Fatalf("required Linux executor capability: %v", err)
		}
		t.Skipf("Linux executor capability unavailable: %v", err)
	}
	return runner
}

func ExampleLocalCheckDefinition() {
	definition := LocalCheckDefinition{
		SchemaVersion:    LocalCheckDefinitionSchemaVersion,
		Argv:             []string{"/usr/local/go/bin/go", "test", "./..."},
		WorkingDirectory: ".", TimeoutSeconds: 120,
		Evidence: EvidenceDefinition{
			ID: "tests", AcceptanceIDs: []string{"AC1"}, Boundary: "component",
			Observed: "The registered project check passed over the fresh candidate.",
		},
	}
	encoded, _ := protocol.EncodeCanonical(definition)
	fmt.Println(strings.Contains(string(encoded), "/usr/local/go/bin/go"))
	// Output: true
}
