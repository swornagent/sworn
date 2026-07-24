// Command batongolden verifies the compiled Baton admission used by Sworn.
// Lifecycle vectors are added by W1 from Baton's independent reference.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/swornagent/sworn/internal/baton"
)

const goldenSchema = "sworn.baton-golden-admission/v1"

type verification struct {
	Schema   string         `json:"schema"`
	Identity baton.Identity `json:"identity"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "verify" {
		fmt.Fprintln(stderr, "usage: batongolden verify")
		return 2
	}
	pkg, err := baton.Load()
	if err != nil {
		fmt.Fprintf(stderr, "batongolden: %v\n", err)
		return 1
	}
	identity, err := pkg.Identity()
	if err != nil {
		fmt.Fprintf(stderr, "batongolden: %v\n", err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(verification{
		Schema:   goldenSchema,
		Identity: identity,
	}); err != nil {
		fmt.Fprintf(stderr, "batongolden: write verification: %v\n", err)
		return 1
	}
	return 0
}
