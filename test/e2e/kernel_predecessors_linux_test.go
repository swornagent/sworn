//go:build linux

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// A1(i). Each preceding slice must have an independently verified
// real-built-binary result bound to its exact candidate, and this slice's base
// must be exactly the last of those products.
//
// This file proves that binding by executing it rather than transcribing it.
// It reads only ordinary Git history -- commit messages and trees reachable
// from HEAD -- and never opens the reserved record path. The product tree
// identity is recomputed here independently, from `git ls-tree -r` with the
// reserved record root excluded, so a receipt whose recorded product_tree does
// not describe its own candidate cannot pass.

// kernelReleaseName is the release whose predecessor chain this file binds.
const kernelReleaseName = "2026-08-07-sworn-native-delivery-kernel"

// kernelReceipt is the machine-readable half of one Baton-Receipt trailer.
type kernelReceipt struct {
	Commit      string
	Release     string            `json:"release"`
	Slice       string            `json:"slice"`
	Role        string            `json:"role"`
	Result      string            `json:"result"`
	Attempt     int64             `json:"attempt"`
	Candidate   string            `json:"candidate"`
	ProductTree string            `json:"product_tree"`
	Contract    string            `json:"contract"`
	Plan        string            `json:"plan"`
	Base        string            `json:"base"`
	Binds       string            `json:"binds"`
	Inputs      map[string]string `json:"inputs"`
}

// hostGit runs one read-only Git command in the repository this test source
// lives in. It never writes.
func hostGit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := exec.Command(e2eGit, append([]string{"-C", moduleRoot(t)}, args...)...)
	command.Env = cleanEnvironment(map[string]string{"LANG": "C", "LC_ALL": "C"})
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

// kernelReceiptsFromHistory reads every Baton-Receipt trailer reachable from
// HEAD for one release, newest last.
func kernelReceiptsFromHistory(t *testing.T, release string) []kernelReceipt {
	t.Helper()
	// %x00 asks Git to emit a NUL between records; the argument itself stays
	// plain text, which exec requires.
	const separator = "\x00"
	raw, err := hostGit(t, "log", "--reverse", "--format=%x00%H%n%B", "HEAD")
	if err != nil {
		t.Skipf("predecessor history is unavailable here: %v", err)
	}
	var receipts []kernelReceipt
	for _, block := range strings.Split(raw, separator) {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		newline := strings.Index(block, "\n")
		if newline < 0 {
			continue
		}
		commit := strings.TrimSpace(block[:newline])
		for _, line := range strings.Split(block[newline+1:], "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "Baton-Receipt: ") {
				continue
			}
			var receipt kernelReceipt
			if json.Unmarshal(
				[]byte(strings.TrimPrefix(line, "Baton-Receipt: ")), &receipt,
			) != nil {
				continue
			}
			if receipt.Release != release {
				continue
			}
			receipt.Commit = commit
			receipts = append(receipts, receipt)
		}
	}
	return receipts
}

// kernelProductTree recomputes one commit's product tree identity
// independently: every entry of the recursive tree except the reserved record
// root, hashed as path\0mode\0type\0oid\n in path order. This is deliberately
// not a call into the product's own helper -- an independent recomputation is
// what makes a wrong recorded product_tree detectable.
func kernelProductTree(t *testing.T, commit string) string {
	t.Helper()
	raw, err := hostGit(t, "ls-tree", "-r", "-z", commit)
	if err != nil {
		t.Fatal(err)
	}
	type entry struct{ path, mode, kind, oid string }
	var entries []entry
	for _, record := range strings.Split(raw, "\x00") {
		if record == "" {
			continue
		}
		metadata, path, found := strings.Cut(record, "\t")
		if !found {
			t.Fatalf("ls-tree record = %q", record)
		}
		fields := strings.Fields(metadata)
		if len(fields) != 3 {
			t.Fatalf("ls-tree metadata = %q", metadata)
		}
		if path == baton.RecordRoot ||
			strings.HasPrefix(path, baton.RecordRoot+"/") ||
			path == baton.LegacyRecordRoot ||
			strings.HasPrefix(path, baton.LegacyRecordRoot+"/") {
			continue
		}
		entries = append(entries, entry{path, fields[0], fields[1], fields[2]})
	}
	if len(entries) == 0 {
		t.Fatalf("commit %s has no product entries", commit)
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].path < entries[right].path
	})
	hasher := sha256.New()
	for _, item := range entries {
		fmt.Fprintf(
			hasher, "%s\x00%s\x00%s\x00%s\n", item.path, item.mode, item.kind, item.oid,
		)
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

// TestKernelPredecessorProductsAreExactlyBound is the executable form of
// A1(i). For each slice this one consumes transitively it proves, from real
// Git objects: one verifier PASS receipt exists, that receipt is a
// metadata-only child of the exact candidate it names, the candidate's
// recomputed product tree equals the product_tree the receipt records, each
// slice's recorded inputs equal its predecessor's passed product tree, and
// this slice's own base carries exactly the last passed product.
func TestKernelPredecessorProductsAreExactlyBound(t *testing.T) {
	t.Parallel()
	receipts := kernelReceiptsFromHistory(t, kernelReleaseName)
	if len(receipts) == 0 {
		t.Skipf(
			"no %s receipts are reachable from HEAD; "+
				"this binding is only assertable on the delivering branch",
			kernelReleaseName,
		)
	}

	predecessors := []struct{ slice, consumes string }{
		{"S1-native-authority", ""},
		{"S2-slice-artifacts", "S1-native-authority"},
		{"S3-semantic-loop", "S2-slice-artifacts"},
	}
	passed := make(map[string]kernelReceipt, len(predecessors))
	for _, predecessor := range predecessors {
		var found []kernelReceipt
		for _, receipt := range receipts {
			if receipt.Slice == predecessor.slice &&
				receipt.Role == "verifier" && receipt.Result == "pass" {
				found = append(found, receipt)
			}
		}
		if len(found) != 1 {
			t.Fatalf(
				"%s has %d verifier PASS receipts, want exactly one",
				predecessor.slice, len(found),
			)
		}
		receipt := found[0]
		if receipt.Candidate == "" || receipt.ProductTree == "" ||
			receipt.Contract == "" {
			t.Fatalf("%s PASS receipt = %#v", predecessor.slice, receipt)
		}

		// The verdict is a metadata-only child of the exact candidate it
		// names: same tree, and the candidate is its parent.
		receiptTree, err := hostGit(t, "rev-parse", receipt.Commit+"^{tree}")
		if err != nil {
			t.Fatal(err)
		}
		candidateTree, err := hostGit(t, "rev-parse", receipt.Candidate+"^{tree}")
		if err != nil {
			t.Fatal(err)
		}
		if receiptTree != candidateTree {
			t.Fatalf(
				"%s verdict %s does not carry the candidate %s tree",
				predecessor.slice, receipt.Commit, receipt.Candidate,
			)
		}
		if _, err := hostGit(
			t, "merge-base", "--is-ancestor", receipt.Candidate, receipt.Commit,
		); err != nil {
			t.Fatalf(
				"%s candidate %s is not an ancestor of its verdict %s",
				predecessor.slice, receipt.Candidate, receipt.Commit,
			)
		}
		// Tree equality plus ancestry is the whole claim: whatever records were
		// appended between the candidate and its verdict changed no product
		// byte, so the verdict was reached over exactly the reviewed tree.
		if changed, err := hostGit(
			t, "diff", "--name-only", receipt.Candidate, receipt.Commit,
		); err != nil || changed != "" {
			t.Fatalf(
				"%s changed %q between candidate %s and verdict %s (err=%v)",
				predecessor.slice, changed, receipt.Candidate, receipt.Commit, err,
			)
		}

		// The recorded product identity actually describes that candidate.
		if recomputed := kernelProductTree(t, receipt.Candidate); recomputed !=
			receipt.ProductTree {
			t.Fatalf(
				"%s product_tree recorded %s, recomputed %s",
				predecessor.slice, receipt.ProductTree, recomputed,
			)
		}

		// The verified result is real history under this slice's own base.
		if _, err := hostGit(
			t, "merge-base", "--is-ancestor", receipt.Commit, "HEAD",
		); err != nil {
			t.Fatalf(
				"%s PASS %s is not an ancestor of this slice's base",
				predecessor.slice, receipt.Commit,
			)
		}

		// Consumed inputs are the predecessor's exact passed product.
		if predecessor.consumes != "" {
			previous, ok := passed[predecessor.consumes]
			if !ok {
				t.Fatalf("%s consumed %s before it passed",
					predecessor.slice, predecessor.consumes)
			}
			if receipt.Inputs[predecessor.consumes] != previous.ProductTree {
				t.Fatalf(
					"%s inputs[%s] = %q, want the passed product %q",
					predecessor.slice, predecessor.consumes,
					receipt.Inputs[predecessor.consumes], previous.ProductTree,
				)
			}
		}
		passed[predecessor.slice] = receipt
	}

	// A1(i) closing clause: this slice's exact base is the last passed
	// product, so nothing entered the composition between S3 and S4.
	last := passed["S3-semantic-loop"]
	baseTree := kernelProductTree(t, last.Commit)
	if baseTree != last.ProductTree {
		t.Fatalf(
			"S3 PASS commit product tree %s != its recorded product %s",
			baseTree, last.ProductTree,
		)
	}

	// Every S4 record in this history must bind that same consumed product,
	// so the composition this slice builds on is the verified one.
	bound := 0
	for _, receipt := range receipts {
		if receipt.Slice != "S4-kernel-proof" {
			continue
		}
		if value, present := receipt.Inputs["S3-semantic-loop"]; present {
			bound++
			if value != last.ProductTree {
				t.Fatalf(
					"S4 %s/%s binds S3 product %q, want %q",
					receipt.Role, receipt.Result, value, last.ProductTree,
				)
			}
		}
	}
	if bound == 0 {
		t.Fatal("no S4 record binds the consumed S3 product")
	}
}
