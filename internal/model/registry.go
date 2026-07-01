package model

// driverWithCapabilities pairs a driver name with its capability bitmask.
// Used by the capabilities registry for human-readable output (sworn capabilities).
type driverWithCapabilities struct {
	Name         string
	Capabilities Capability
}

// capabilityRegistry maps provider prefix → capabilities for every driver
// registered in NewClient / FromEnv. This is a thin discoverability layer;
// the canonical capability check is the CapabilityProvider interface method.
var capabilityRegistry = []driverWithCapabilities{
	{Name: "openai", Capabilities: CapVerify | CapChat | CapStructuredOutput},
	{Name: "openai-responses", Capabilities: CapVerify | CapChat | CapStructuredOutput},
	{Name: "anthropic", Capabilities: CapVerify | CapChat},
	{Name: "claude-cli", Capabilities: CapVerify | CapChat}, {Name: "azure", Capabilities: CapVerify},
	{Name: "bedrock", Capabilities: CapVerify},
	{Name: "google", Capabilities: CapVerify},
	{Name: "vertex", Capabilities: CapVerify},
	{Name: "oci", Capabilities: CapVerify},
	{Name: "ollama", Capabilities: CapVerify},
	{Name: "deepseek", Capabilities: CapVerify | CapChat | CapStructuredOutput},
	{Name: "groq", Capabilities: CapVerify | CapChat},
	{Name: "mistral", Capabilities: CapVerify | CapChat},
	{Name: "openrouter", Capabilities: CapVerify | CapChat},
	{Name: "cloudflare", Capabilities: CapVerify | CapChat},
	{Name: "github", Capabilities: CapVerify | CapChat},
}

// CapabilityRegistry returns a read-only snapshot of every known driver and its
// capabilities. Callers (e.g. the sworn capabilities subcommand) can format this
// for human-readable output without importing individual driver types.
func CapabilityRegistry() []driverWithCapabilities {
	return capabilityRegistry
}

// HasChat reports whether the driver named by provider prefix supports the
// Chat capability, as recorded in the registry.
func HasChat(provider string) bool {
	for _, d := range capabilityRegistry {
		if d.Name == provider {
			return d.Capabilities&CapChat != 0
		}
	}
	return false
}
