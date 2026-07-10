// sworn models — the discoverability counterpart to explicit-prefix
// resolution (S05, N-11): lists, per linked provider, the models actually
// available on the user's account, grouped by the resolution prefix the
// user would type. Capability annotations are sourced only from
// wire-reported metadata (internal/model/catalog.go) — no completion,
// dispatch, or probe calls are ever made (AC-04).
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/model"
)

// modelsHTTPClient is the HTTP client cmdModels passes to model.ListCatalog.
// nil (the zero value, and always in production) means ListCatalog's own
// default (http.DefaultClient). This var exists purely as a test seam: it
// lets TestModelsCommand redirect the six HTTP-based providers' real
// base-URL requests to local fixture servers while still exercising the
// real cmdModels -> model.ListCatalog dispatch path (Rule 1 reachability).
// Never set outside a test.
var modelsHTTPClient *http.Client

func init() {
	// Self-registration via init() — never edit cmd/sworn/main.go to add a
	// command (main.go's own header: "Adding a new CLI command never edits
	// this file").
	command.Register(command.Command{
		Name:    "models",
		Summary: "list models available per linked provider, grouped by prefix, with wire-reported tool-capability annotations",
		Run:     cmdModels,
	})
}

func cmdModels(args []string) int {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	provider := fs.String("provider", "", "restrict listing to one provider prefix")
	_ = fs.Parse(args)

	if *provider != "" && !validCatalogProvider(*provider) {
		fmt.Fprintf(os.Stderr, "sworn models: unknown provider %q\n", *provider)
		fmt.Fprintf(os.Stderr, "valid providers: %s\n", strings.Join(model.CatalogProviderNames(), ", "))
		return 64
	}

	results := model.ListCatalog(context.Background(), model.ProviderConfigFromEnv(), modelsHTTPClient, *provider)
	out, exitCode := renderModelsOutput(results)
	fmt.Print(out)
	return exitCode
}

// validCatalogProvider reports whether p is one of the resolution prefixes
// ListCatalog understands.
func validCatalogProvider(p string) bool {
	for _, name := range model.CatalogProviderNames() {
		if name == p {
			return true
		}
	}
	return false
}

// renderModelsOutput formats ListCatalog's results — one block per attempted
// provider, in the fixed alphabetical order ListCatalog itself iterates
// (diff-stable, AC-01), models listed with their resolution-prefixed ID and
// tools annotation. Returns the rendered text and the process exit code:
// non-zero only when at least one provider was attempted and every attempted
// provider errored (AC-03).
func renderModelsOutput(results []model.CatalogResult) (string, int) {
	var b strings.Builder
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			b.WriteString(fmt.Sprintf("%s/: error: %v\n", r.Provider, r.Err))
			failed++
			continue
		}
		b.WriteString(fmt.Sprintf("%s/ (%d models)\n", r.Provider, len(r.Models)))
		for _, m := range r.Models {
			b.WriteString(fmt.Sprintf("  %s/%s   tools: %s\n", r.Provider, m.ID, m.Tools))
		}
	}
	if len(results) > 0 && failed == len(results) {
		return b.String(), 1
	}
	return b.String(), 0
}
