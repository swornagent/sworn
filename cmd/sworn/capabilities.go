// sworn capabilities — render the driver registry enumeration: every
// registered driver with its prefixes, deprecated aliases, role set,
// availability in this environment, and which prefixes currently resolve
// through the sworn proxy (sworn#69 visibility). No model dispatch happens:
// the probes are PATH lookups, key-presence checks, and a credentials-file
// read (S05-driver-registry AC-05).
//
// This verb replaces the hand-maintained capabilityRegistry table that
// previously lived in internal/model/registry.go — the driver registry
// (internal/driver/registry) is the single authority now (AC-01).
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/driver/registry"
	"github.com/swornagent/sworn/internal/model"
)

func init() {
	command.Register(command.Command{
		Name:    "capabilities",
		Summary: "list registered drivers: prefixes, roles, availability, proxy routing",
		Run:     cmdCapabilities,
	})
}

func cmdCapabilities(_ []string) int {
	infos := registry.Default(model.ProviderConfigFromEnv()).Drivers()
	fmt.Print(renderCapabilities(infos))
	return 0
}

// renderCapabilities formats the enumeration as one block per driver,
// deterministic order (sorted by driver name) so output is diff-stable.
func renderCapabilities(infos []registry.Info) string {
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })

	var b strings.Builder
	b.WriteString("registered drivers (resolution: explicit prefix -> driver, no fallback):\n\n")
	for _, info := range infos {
		b.WriteString(info.Name)
		b.WriteString("\n")

		prefixes := make([]string, 0, len(info.Prefixes))
		for _, p := range info.Prefixes {
			prefixes = append(prefixes, p+"/")
		}
		var aliases []string
		for alias, canonical := range info.DeprecatedAliases {
			aliases = append(aliases, fmt.Sprintf("%s/ (deprecated alias of %s/)", alias, canonical))
		}
		sort.Strings(aliases)
		b.WriteString("  prefixes:  " + strings.Join(append(prefixes, aliases...), ", ") + "\n")
		b.WriteString("  roles:     " + info.Roles.String() + "\n")

		avail := "no"
		if info.Available {
			avail = "yes"
		}
		if info.Detail != "" {
			avail += " — " + info.Detail
		}
		b.WriteString("  available: " + avail + "\n")

		if len(info.ViaProxy) > 0 {
			var via []string
			for _, p := range info.ViaProxy {
				via = append(via, p+"/")
			}
			b.WriteString("  via proxy: " + strings.Join(via, ", ") + " (sworn login active; set SWORN_DIRECT=1 for direct routing)\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("prefix semantics (sworn#31): openai/ = Responses API; openai-completions/ = legacy chat/completions; openai-responses/ = deprecated alias of openai/, kept for one release.\n")
	return b.String()
}
