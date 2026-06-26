package main

import "github.com/swornagent/sworn/internal/command"

func init() {
	// Register every top-level verb already present on release-wt.
	// T15-owned — this is the only file that needs editing when a new verb
	// enters the repository. Once per-file self-registration is adopted, each
	// cmd/sworn/<verb>.go registers its own verb via its own init().

	command.Register(command.Command{
		Name:    "init",
		Summary: "bootstrap SwornAgent in a repo",
		Run:     cmdInit,
	})
	command.Register(command.Command{
		Name:    "verify",
		Summary: "emit a JSON verdict (PASS/FAIL/BLOCKED) against a spec + diff",
		Run:     cmdVerify,
	})
	command.Register(command.Command{
		Name:    "run",
		Summary: "execute the full turnkey loop: implement → verify → retry/escalate",
		Run:     cmdRun,
	})
	command.Register(command.Command{
		Name:    "bench",
		Summary: "run a model benchmark against a task set",
		Run:     cmdBench,
	})
	command.Register(command.Command{
		Name:    "mcp",
		Summary: "MCP transport for AI planning + operations",
		Run:     cmdMcp,
	})
	command.Register(command.Command{
		Name:    "lint",
		Summary: "check a release for structural problems (ac | trace)",
		Run:     cmdLint,
	})
	command.Register(command.Command{
		Name:    "reqverify",
		Summary: "grade acceptance criteria quality (ISO 29148)",
		Run:     cmdReqverify,
	})
	command.Register(command.Command{
		Name:    "reqvalidate",
		Summary: "check requirements validation records",
		Run:     cmdReqvalidate,
	})
	command.Register(command.Command{
		Name:    "designfit",
		Summary: "design-fit gate for high-stakes choices",
		Run:     cmdDesignfit,
	})
	command.Register(command.Command{
		Name:    "journeys",
		Summary: "draft and validate customer journeys",
		Run:     cmdJourneys,
	})
	command.Register(command.Command{
		Name:    "ship",
		Summary: "human-walkthrough attestation gate",
		Run:     cmdShip,
	})
	command.Register(command.Command{
		Name:    "specquality",
		Summary: "compute soundness + completeness metrics for a release",
		Run:     cmdSpecquality,
	})
	command.Register(command.Command{
		Name:    "designaudit",
		Summary: "design conformance audit against the project design system",
		Run:     cmdDesignaudit,
	})
	command.Register(command.Command{
		Name:    "top",
		Summary: "read-only evidence surface for the active release",
		Run:     cmdTop,
	})
	command.Register(command.Command{
		Name:    "doctor",
		Summary: "run health checks on the SwornAgent installation",
		Run:     cmdDoctor,
	})
	command.Register(command.Command{
		Name:    "telemetry",
		Summary: "manage anonymous usage telemetry (on | off | status)",
		Run:     cmdTelemetry,
	})
	command.Register(command.Command{
		Name:    "memory",
		Summary: "manage the SwornAgent memory store",
		Run:     cmdMemory,
	})
	command.Register(command.Command{
		Name:    "regress",
		Summary: "run full test suite regression against a merged release worktree",
		Run:     cmdRegress,
	})
	command.Register(command.Command{
		Name:    "llm-check",
		Summary: "run an LLM-based quality check against a slice",
		Run:     cmdLLMCheck,
	})

	// version and help are registered as aliases (multiple names → same handler).
	command.Register(command.Command{
		Name: "version", Summary: "print sworn binary and baton-protocol versions",
		Run: cmdVersion,
	})
	command.Register(command.Command{
		Name:    "--version",
		Summary: "print sworn binary and baton-protocol versions",
		Run:     cmdVersion,
	})
	command.Register(command.Command{
		Name:    "-v",
		Summary: "print sworn binary and baton-protocol versions",
		Run:     cmdVersion,
	})
	command.Register(command.Command{
		Name:    "help",
		Summary: "print usage and command listing",
		Run:     cmdHelp,
	})
	command.Register(command.Command{
		Name:    "--help",
		Summary: "print usage and command listing",
		Run:     cmdHelp,
	})
	command.Register(command.Command{
		Name:    "-h",
		Summary: "print usage and command listing",
		Run:     cmdHelp,
	})
}
