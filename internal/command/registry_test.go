package command_test

import (
	"testing"

	"github.com/swornagent/sworn/internal/command"
)

func TestRegisterAndLookup(t *testing.T) {
	// Clear registry before test (registry is process-global).
	// We can't clear it directly, but we can check that registered commands resolve.
	c := command.Command{Name: "test-foo", Summary: "foo command", Run: func(_ []string) int { return 99 }}
	command.Register(c)

	got, ok := command.Lookup("test-foo")
	if !ok {
		t.Fatal("Lookup(test-foo) not found after Register")
	}
	if got.Name != "test-foo" {
		t.Errorf("Name = %q, want %q", got.Name, "test-foo")
	}
	if got.Summary != "foo command" {
		t.Errorf("Summary = %q, want %q", got.Summary, "foo command")
	}
	// Verify Run identity by calling it.
	if exit := got.Run(nil); exit != 99 {
		t.Errorf("Run() = %d, want 99", exit)
	}
}

func TestLookupNotFound(t *testing.T) {
	_, ok := command.Lookup("nonexistent-xyz")
	if ok {
		t.Error("Lookup(nonexistent) should not be found")
	}
}

func TestAllSorted(t *testing.T) {
	// Register a few commands in unsorted order.
	command.Register(command.Command{Name: "zulu", Summary: "z"})
	command.Register(command.Command{Name: "alpha", Summary: "a"})
	command.Register(command.Command{Name: "mike", Summary: "m"})

	all := command.All()
	if len(all) < 3 {
		t.Fatalf("All() returned %d commands, want at least 3", len(all))
	}

	// Find the three we just registered within All().
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, c.Name)
	}

	// The full slice must be sorted.
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("All() not sorted: names[%d]=%q < names[%d]=%q", i, names[i], i-1, names[i-1])
		}
	}

	// Verify all three registered commands are present.
	for _, want := range []string{"alpha", "mike", "zulu"} {
		found := false
		for _, c := range all {
			if c.Name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("All() missing %q", want)
		}
	}
}

func TestDuplicatePanics(t *testing.T) {
	command.Register(command.Command{Name: "dup-test", Summary: "first"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration, but none occurred")
		}
	}()
	command.Register(command.Command{Name: "dup-test", Summary: "second"})
}