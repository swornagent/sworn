//go:build linux

package e2e

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// The Sworn conformance profile declares what must be proven. This file is
// where it is actually proven, and the registry below is the only thing that
// can certify a declared (case, surface) pair: an anchor is recorded at the
// moment a real binary was observed behaving as the case describes. Prose, a
// tool-list snapshot, a schema parse, a unit test, or a command exit status
// register nothing and therefore certify nothing.
var (
	swornConformanceMutex   sync.Mutex
	swornConformanceAnchors = map[swornruntime.ConformanceObligation][]string{}
)

// recordSwornConformance registers one executed real-binary anchor for one
// declared (case, surface) pair. It fails the calling test if the pair is not
// declared by the Sworn-owned profile, so a test cannot invent coverage for a
// case the product does not promise.
func recordSwornConformance(t *testing.T, caseID, surface, anchor string) {
	t.Helper()
	if anchor == "" {
		t.Fatalf("conformance anchor for %s/%s is empty", caseID, surface)
	}
	profile, err := swornruntime.LoadConformanceProfile()
	if err != nil {
		t.Fatal(err)
	}
	declared := false
	for _, obligation := range profile.Obligations() {
		if obligation.Case == caseID && obligation.Surface == surface {
			declared = true
			break
		}
	}
	if !declared {
		t.Fatalf(
			"conformance case %q does not declare surface %q", caseID, surface,
		)
	}
	obligation := swornruntime.ConformanceObligation{Case: caseID, Surface: surface}
	swornConformanceMutex.Lock()
	defer swornConformanceMutex.Unlock()
	swornConformanceAnchors[obligation] = append(
		swornConformanceAnchors[obligation], anchor,
	)
}

// certifySwornConformance is the executable gate. Every declared (case,
// surface) pair must have at least one anchor that a passing real-binary test
// registered during this run; otherwise the package fails.
//
// A filtered run (-run, or -short) cannot see the whole suite, so it reports
// nothing rather than a false failure. An unfiltered run is the certifying
// one.
func certifySwornConformance() error {
	if filter := flag.Lookup("test.run"); filter != nil &&
		filter.Value.String() != "" {
		return nil
	}
	if short := flag.Lookup("test.short"); short != nil &&
		short.Value.String() == "true" {
		return nil
	}
	profile, err := swornruntime.LoadConformanceProfile()
	if err != nil {
		return err
	}
	swornConformanceMutex.Lock()
	defer swornConformanceMutex.Unlock()
	var missing []string
	results := make([]map[string]any, 0, len(profile.Obligations()))
	for _, obligation := range profile.Obligations() {
		anchors := swornConformanceAnchors[obligation]
		if len(anchors) == 0 {
			missing = append(
				missing, obligation.Case+"@"+obligation.Surface,
			)
			continue
		}
		sorted := append([]string(nil), anchors...)
		sort.Strings(sorted)
		results = append(results, map[string]any{
			"case": obligation.Case, "surface": obligation.Surface,
			"status": "PASS", "anchors": sorted,
		})
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"sworn conformance profile %s is not certified: no real-binary anchor for %s",
			swornruntime.ConformanceDigest(), strings.Join(missing, ", "),
		)
	}
	evidence, err := json.Marshal(map[string]any{
		"schema_version": "sworn.conformance-result/v1",
		"profile":        profile.Name,
		"profile_digest": swornruntime.ConformanceDigest(),
		"results":        results,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "sworn conformance evidence: %s\n", evidence)
	return nil
}

// TestSwornConformanceProfileIsWellFormed proves the profile itself admits and
// that it still declares every surface A6 names. This is a declaration check
// only; it certifies nothing.
func TestSwornConformanceProfileIsWellFormed(t *testing.T) {
	t.Parallel()
	profile, err := swornruntime.LoadConformanceProfile()
	if err != nil {
		t.Fatal(err)
	}
	declared := make(map[string]bool, len(profile.Surfaces))
	for _, surface := range profile.Surfaces {
		declared[surface.ID] = true
	}
	for _, required := range []string{
		surfaceInstalledSkillMCP, surfaceDirectMCP, surfaceTUI,
		surfaceCLI, surfaceConfiguredDriver,
	} {
		if !declared[required] {
			t.Fatalf("profile does not declare surface %q", required)
		}
	}
	if len(profile.Obligations()) < len(profile.Cases) {
		t.Fatalf("profile obligations = %d", len(profile.Obligations()))
	}
}

const (
	surfaceInstalledSkillMCP = "installed_skill_mcp"
	surfaceDirectMCP         = "direct_mcp"
	surfaceTUI               = "tui"
	surfaceCLI               = "cli"
	surfaceConfiguredDriver  = "configured_driver"

	caseReadParity         = "run-state-read-parity"
	caseTurnVisibility     = "open-human-turn-visibility-parity"
	caseAnswerAdmittedOnce = "human-turn-answer-admitted-once"
	caseStaleAnswerRefused = "stale-answer-refused"
	caseUnavailableRefused = "unavailable-transition-refused"
	caseRestartRecovery    = "restart-recovers-one-continuation"
)
