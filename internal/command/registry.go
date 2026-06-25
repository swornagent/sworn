// Package command provides a process-wide command registry for the sworn CLI.
//
// Subcommands register themselves via Register (typically from an init function
// in their own cmd/sworn/<verb>.go) and the registry drives dispatch in main.
// Double-registration of a name is a programming error (panic).
//
// S51-cli-command-registry — T15-owned.
package command

import (
	"fmt"
	"sort"
	"sync"
)

// Command is a single CLI subcommand registered in the process-wide registry.
type Command struct {
	Name    string
	Summary string           // one-line description for usage listing (must be non-empty)
	Run     func(args []string) int
}

var (
	mu       sync.Mutex
	registry []Command
)

// Register records a command in the process-wide registry.
// Panics if a command with the same Name has already been registered.
func Register(c Command) {
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range registry {
		if existing.Name == c.Name {
			panic(fmt.Sprintf("command %q already registered", c.Name))
		}
	}
	registry = append(registry, c)
}

// Lookup returns the registered Command for name, and true if found.
func Lookup(name string) (Command, bool) {
	mu.Lock()
	defer mu.Unlock()
	for _, c := range registry {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// All returns all registered commands sorted alphabetically by Name.
func All() []Command {
	mu.Lock()
	defer mu.Unlock()
	sorted := make([]Command, len(registry))
	copy(sorted, registry)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}