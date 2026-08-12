package baton

import (
	"strings"
	"testing"
)

// TestVerifyAssetDirectsVerifierToReadHostBoundaryEvidence is the A5 proof
// that the role asset pins move in this same candidate and that the verifier
// instructions tell a verifier to read the engine's recorded host-boundary
// evidence for a declared containment-requiring check instead of re-running
// it, never returning PASS when that evidence is missing or failed.
func TestVerifyAssetDirectsVerifierToReadHostBoundaryEvidence(t *testing.T) {
	pkg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	body, err := pkg.ReadAsset("operations/baton-verify.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"READ the engine's recorded\n   host-boundary evidence",
		"Never execute a declared host check",
		"recorded as anything other than a pass",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("baton-verify.md is missing %q", required)
		}
	}
	if !strings.Contains(text, "host-evidence.json") {
		t.Fatalf("baton-verify.md does not name the projected host evidence input")
	}
	// The moved pin is part of this candidate: the admitted operation digest
	// must equal the exact bytes shipped here.
	release := readReleaseFile(t)
	manifest := readAssetManifest(t)
	for _, operation := range release.Operations {
		if operation.Name != "baton-verify" {
			continue
		}
		if digest(body) != operation.SHA256 {
			t.Fatalf("baton-verify asset digest %s != operation pin %s", digest(body), operation.SHA256)
		}
		for _, entry := range manifest.Assets {
			if entry.Path == "operations/baton-verify.md" &&
				(entry.SHA256 != operation.SHA256 ||
					int64(len(body)) != entry.Size) {
				t.Fatalf("baton-verify manifest pin does not match shipped bytes")
			}
		}
	}
}
