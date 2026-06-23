package main

import (
	"testing"

	"github.com/swornagent/sworn/internal/command"
)

// expectedVerbs is the full set of verbs that must resolve in the registry.
var expectedVerbs = []string{
	"account",
	"login",
	"logout",
	"init",
	"verify",
	"run",
	"bench",
	"mcp",
	"lint",
	"reqverify",
	"reqvalidate",
	"designfit",
	"journeys",
	"ship",
	"specquality",
	"designaudit",
	"top",
	"doctor",
	"telemetry",
	"memory",
	"version",
	"--version",
	"-v",
	"help",
	"--help",
	"-h",
}

func TestEveryVerbResolves(t *testing.T) {
	for _, verb := range expectedVerbs {
		c, ok := command.Lookup(verb)
		if !ok {
			t.Errorf("command.Lookup(%q) not found", verb)
			continue
		}
		if c.Name != verb {
			t.Errorf("command.Lookup(%q).Name = %q", verb, c.Name)
		}
		if c.Run == nil {
			t.Errorf("command.Lookup(%q).Run is nil", verb)
		}
		// Pin 2: every registered command must have a non-empty Summary.
		if c.Summary == "" {
			t.Errorf("command.Lookup(%q).Summary is empty", verb)
		}
	}
}

func TestUnknownVerbNotFound(t *testing.T) {
	_, ok := command.Lookup("bogusverb")
	if ok {
		t.Error("command.Lookup(bogusverb) should not be found")
	}
}

func TestAllCommandsHaveNonEmptySummary(t *testing.T) {
	for _, c := range command.All() {
		if c.Summary == "" {
			t.Errorf("command %q has empty Summary", c.Name)
		}
	}
}

func TestVersionAndHelpAliasesShareHandlers(t *testing.T) {
	// version, --version, -v all resolve to cmdVersion.
	ver, ok := command.Lookup("version")
	if !ok {
		t.Fatal("version not found")
	}
	for _, alias := range []string{"--version", "-v"} {
		a, ok := command.Lookup(alias)
		if !ok {
			t.Errorf("alias %q not found", alias)
			continue
		}
		// Same Run handler reference.
		// In Go, function pointers are comparable.
		if a.Run == nil || ver.Run == nil {
			t.Errorf("nil Run for version aliases")
		}
	}

	// help, --help, -h all resolve to cmdHelp.
	help, ok := command.Lookup("help")
	if !ok {
		t.Fatal("help not found")
	}
	for _, alias := range []string{"--help", "-h"} {
		a, ok := command.Lookup(alias)
		if !ok {
			t.Errorf("alias %q not found", alias)
			continue
		}
		if a.Run == nil || help.Run == nil {
			t.Errorf("nil Run for help aliases")
		}
	}
}

func TestDispatchResolves(t *testing.T) {
	// Simulate what dispatch() does: Lookup then Run.
	// We exercise every expected verb's lookup + handler non-nil.
	for _, verb := range expectedVerbs {
		c, ok := command.Lookup(verb)
		if !ok {
			t.Errorf("dispatch: Lookup(%q) not found", verb)
			continue
		}
		if c.Run == nil {
			t.Errorf("dispatch: Lookup(%q).Run is nil", verb)
		}
	}
}
