package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/swornagent/sworn/internal/driver"
)

const driverReadinessSchemaVersion = "sworn.driver-readiness/v1"
const driverProbeSchemaVersion = "sworn.driver-probe/v1"

type driverReadinessOutput struct {
	SchemaVersion       string                 `json:"schema_version"`
	Command             string                 `json:"command"`
	ConfigurationDigest string                 `json:"configuration_digest"`
	Reports             []driver.ProfileReport `json:"reports"`
	// LiveCall states plainly whether this command made a live provider
	// call: false for inspect/doctor, true for certify. It rides every
	// readiness output, not only doctor's, so a reader never has to infer
	// it from the command name alone.
	LiveCall bool `json:"live_call"`
	// MissingFamilies, MissingSurfaces and RosterNote are set only for a
	// not-ready --all report: the single production roster declaration
	// (driver.ProductionRequiredFamilies/Surfaces) names exactly what the
	// built registry is missing (S4-lane-live-probe A4).
	MissingFamilies []driver.ProfileFamily  `json:"missing_families,omitempty"`
	MissingSurfaces []driver.ProfileSurface `json:"missing_surfaces,omitempty"`
	RosterNote      string                  `json:"roster_note,omitempty"`
}

type driverProbeOutput struct {
	SchemaVersion       string                `json:"schema_version"`
	Profile             string                `json:"profile"`
	Model               string                `json:"model"`
	Family              driver.ProfileFamily  `json:"family"`
	Surface             driver.ProfileSurface `json:"surface,omitempty"`
	AdapterID           string                `json:"adapter_id"`
	AdapterVersion      string                `json:"adapter_version"`
	ConfigurationDigest string                `json:"configuration_digest"`
	Ready               bool                  `json:"ready"`
	Code                string                `json:"code"`
	Message             string                `json:"message,omitempty"`
	RequestID           string                `json:"request_id,omitempty"`
	LatencyMillis       int64                 `json:"latency_ms"`
	LiveCall            bool                  `json:"live_call"`
	// CLICompatibility is present for native CLI lanes only.
	CLICompatibility *driver.NativeCLICompatibility `json:"cli_compatibility,omitempty"`
}

type driverCommandOptions struct {
	config  string
	profile string
	model   string
	all     bool
}

type driverProbeOptions struct {
	config  string
	profile string
	model   string
	json    bool
}

func runDriver(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "probe" {
		return runDriverProbe(args[1:], stdout, stderr)
	}
	command, options, ok := parseDriverCommand(args)
	if !ok {
		fmt.Fprintln(
			stderr,
			"usage: sworn driver inspect|doctor|certify --config ABS --json "+
				"(--profile PROFILE --model MODEL | --all)\n"+
				"       sworn driver probe --config ABS --profile PROFILE "+
				"--model MODEL [--json]",
		)
		return 2
	}

	loaded, err := driver.LoadDriverConfig(options.config)
	if err != nil {
		writeCommandFailure(
			stderr,
			"driver "+command,
			"Could not read the AI connection configuration.",
			&driverConfigError{err: err},
		)
		return 1
	}
	factory, err := driver.NewProductionDriverFactory(loaded)
	if err != nil {
		writeCommandFailure(
			stderr,
			"driver "+command,
			"Could not prepare the configured AI connections.",
			err,
		)
		return 1
	}
	defer factory.Close()

	profiles := []string{options.profile}
	if options.all {
		profiles = loaded.Profiles()
	}
	factoryOptions, err := driverCheckOptions(loaded, factory, profiles)
	if err != nil {
		writeCommandFailure(
			stderr,
			"driver "+command,
			"Could not resolve the native CLI the AI connection"+
				" configuration names.",
			err,
		)
		return 1
	}
	var registry driver.ConfiguredDriverRegistry
	if options.all {
		registry, err = loaded.BuildAllRegistry(factoryOptions)
	} else {
		registry, err = loaded.BuildRegistry(profiles, factoryOptions)
	}
	if err != nil {
		// Only a genuinely unknown profile is a "not found" (sworn#267):
		// every other build failure - an inadmissible adapter, a family
		// missing from --all's production roster - must not masquerade as
		// one, or the operator debugs the wrong thing.
		message := "The AI connection configuration could not be built" +
			" into a driver registry."
		if commandErrorCode(err) == "UNKNOWN_PROFILE" {
			message = "Could not find that profile and model" +
				" in the AI connection configuration."
		}
		writeCommandFailure(stderr, "driver "+command, message, err)
		return 1
	}

	reports, ready, missingFamilies, missingSurfaces := driverReports(
		context.Background(),
		command,
		registry,
		options,
	)
	output := driverReadinessOutput{
		SchemaVersion:       driverReadinessSchemaVersion,
		Command:             command,
		ConfigurationDigest: registry.ConfigurationDigest(),
		Reports:             reports,
		LiveCall:            command == "certify",
		MissingFamilies:     missingFamilies,
		MissingSurfaces:     missingSurfaces,
	}
	if len(missingFamilies) > 0 || len(missingSurfaces) > 0 {
		output.RosterNote = driver.ProductionRosterNote
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(stderr, "sworn driver %s: output failed\n", command)
		return 1
	}
	if !ready {
		return 1
	}
	return 0
}

// driverCheckOptions binds every run-snapshot native adapter the checked
// profiles use to a preview of its host CLI, resolved the way a run started
// now would snapshot it, so the readiness commands check and report that CLI.
func driverCheckOptions(
	loaded driver.LoadedDriverConfig,
	factory *driver.ProductionDriverFactory,
	profiles []string,
) (driver.DriverFactoryOptions, error) {
	options := factory.Options()
	previews, err := loaded.PreviewNativeCLISnapshots(
		context.Background(),
		profiles,
	)
	if err != nil {
		return driver.DriverFactoryOptions{}, err
	}
	options.NativeCLISnapshots = previews
	return options, nil
}

func parseDriverCommand(
	args []string,
) (string, driverCommandOptions, bool) {
	if len(args) == 0 {
		return "", driverCommandOptions{}, false
	}
	command := args[0]
	switch command {
	case "inspect", "doctor", "certify":
	default:
		return "", driverCommandOptions{}, false
	}
	var options driverCommandOptions
	seen := make(map[string]struct{})
	jsonOutput := false
	for index := 1; index < len(args); index++ {
		name := args[index]
		if _, duplicate := seen[name]; duplicate {
			return "", driverCommandOptions{}, false
		}
		seen[name] = struct{}{}
		switch name {
		case "--all":
			options.all = true
		case "--json":
			jsonOutput = true
		case "--config", "--profile", "--model":
			if index+1 >= len(args) || args[index+1] == "" ||
				len(args[index+1]) >= 2 && args[index+1][:2] == "--" {
				return "", driverCommandOptions{}, false
			}
			index++
			switch name {
			case "--config":
				options.config = args[index]
			case "--profile":
				options.profile = args[index]
			case "--model":
				options.model = args[index]
			}
		default:
			return "", driverCommandOptions{}, false
		}
	}
	profileMode := options.profile != "" && options.model != ""
	if options.config == "" || !jsonOutput ||
		options.all == profileMode ||
		(options.all && (options.profile != "" || options.model != "")) ||
		(!options.all && !profileMode) {
		return "", driverCommandOptions{}, false
	}
	return command, options, true
}

func driverReports(
	ctx context.Context,
	command string,
	registry driver.ConfiguredDriverRegistry,
	options driverCommandOptions,
) ([]driver.ProfileReport, bool, []driver.ProfileFamily, []driver.ProfileSurface) {
	certifications := registry.Certifications()
	if !options.all {
		configured := false
		for _, certification := range certifications {
			if certification.Profile == options.profile &&
				certification.Model == options.model {
				configured = true
				break
			}
		}
		report := runDriverCheck(
			ctx,
			command,
			registry,
			options.profile,
			options.model,
		)
		if !configured {
			report.State = driver.ReadinessNotCertified
			report.Code = "model_not_configured"
		}
		if report.Family == driver.ProfileFake {
			report.State = driver.ReadinessFail
			report.Code = "fake_not_production"
		}
		return []driver.ProfileReport{report},
			configured && report.State == driver.ReadinessPass &&
				report.Family != driver.ProfileFake,
			nil, nil
	}

	reports := make([]driver.ProfileReport, 0, len(certifications))
	seen := make(map[string]struct{}, len(certifications))
	ready := true
	for _, certification := range certifications {
		key := certification.Profile + "\x00" + certification.Model
		if _, duplicate := seen[key]; duplicate {
			ready = false
			continue
		}
		seen[key] = struct{}{}
		report := runDriverCheck(
			ctx,
			command,
			registry,
			certification.Profile,
			certification.Model,
		)
		if report.Family == driver.ProfileFake {
			continue
		}
		if report.State != driver.ReadinessPass ||
			report.Family != certification.Family ||
			report.Surface != certification.Surface {
			ready = false
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(left, right int) bool {
		if reports[left].Profile != reports[right].Profile {
			return reports[left].Profile < reports[right].Profile
		}
		if reports[left].Model != reports[right].Model {
			return reports[left].Model < reports[right].Model
		}
		if reports[left].Family != reports[right].Family {
			return reports[left].Family < reports[right].Family
		}
		return reports[left].Surface < reports[right].Surface
	})
	complete, missingFamilies, missingSurfaces := completeProductionReadiness(reports)
	return reports, ready && complete, missingFamilies, missingSurfaces
}

func runDriverCheck(
	ctx context.Context,
	command string,
	registry driver.ConfiguredDriverRegistry,
	profile string,
	model string,
) driver.ProfileReport {
	switch command {
	case "inspect":
		return registry.Inspect(ctx, profile, model)
	case "doctor":
		return registry.Doctor(ctx, profile, model)
	default:
		return registry.Certify(ctx, profile, model)
	}
}

// completeProductionReadiness reads the single production roster
// declaration (driver.ProductionRequiredFamilies/Surfaces,
// S4-lane-live-probe A4) instead of its own copy, so a registry that builds
// can never then be reported not ready for a family or surface the build
// did not require. It reports readiness plus exactly what is missing, in
// roster order, so the CLI can name it.
func completeProductionReadiness(
	reports []driver.ProfileReport,
) (bool, []driver.ProfileFamily, []driver.ProfileSurface) {
	presentFamilies := make(map[driver.ProfileFamily]bool)
	presentSurfaces := make(map[driver.ProfileSurface]bool)
	allPass := true
	for _, report := range reports {
		if report.State != driver.ReadinessPass {
			allPass = false
		}
		presentFamilies[report.Family] = true
		if report.Surface != "" {
			presentSurfaces[report.Surface] = true
		}
	}
	missingFamilies, missingSurfaces := driver.MissingProductionMembers(
		presentFamilies, presentSurfaces,
	)
	ready := allPass && len(missingFamilies) == 0 && len(missingSurfaces) == 0
	return ready, missingFamilies, missingSurfaces
}

func parseDriverProbeCommand(args []string) (driverProbeOptions, bool) {
	var options driverProbeOptions
	seen := make(map[string]struct{})
	for index := 0; index < len(args); index++ {
		name := args[index]
		if _, duplicate := seen[name]; duplicate {
			return driverProbeOptions{}, false
		}
		seen[name] = struct{}{}
		switch name {
		case "--json":
			options.json = true
		case "--config", "--profile", "--model":
			if index+1 >= len(args) || args[index+1] == "" ||
				len(args[index+1]) >= 2 && args[index+1][:2] == "--" {
				return driverProbeOptions{}, false
			}
			index++
			switch name {
			case "--config":
				options.config = args[index]
			case "--profile":
				options.profile = args[index]
			case "--model":
				options.model = args[index]
			}
		default:
			return driverProbeOptions{}, false
		}
	}
	if options.config == "" || options.profile == "" || options.model == "" {
		return driverProbeOptions{}, false
	}
	return options, true
}

// runDriverProbe implements `sworn driver probe`: one minimal live request
// for an explicitly named profile and model (S4-lane-live-probe A1). Unlike
// inspect/doctor/certify, --json is optional; the default is a human-
// readable line carrying the same typed code, message, request id and
// latency the JSON form carries.
func runDriverProbe(args []string, stdout, stderr io.Writer) int {
	options, ok := parseDriverProbeCommand(args)
	if !ok {
		fmt.Fprintln(
			stderr,
			"usage: sworn driver probe --config ABS --profile PROFILE "+
				"--model MODEL [--json]",
		)
		return 2
	}
	loaded, err := driver.LoadDriverConfig(options.config)
	if err != nil {
		writeCommandFailure(
			stderr,
			"driver probe",
			"Could not read the AI connection configuration.",
			&driverConfigError{err: err},
		)
		return 1
	}
	factory, err := driver.NewProductionDriverFactory(loaded)
	if err != nil {
		writeCommandFailure(
			stderr,
			"driver probe",
			"Could not prepare the configured AI connections.",
			err,
		)
		return 1
	}
	defer factory.Close()
	factoryOptions, err := driverCheckOptions(
		loaded,
		factory,
		[]string{options.profile},
	)
	if err != nil {
		writeCommandFailure(
			stderr,
			"driver probe",
			"Could not resolve the native CLI the AI connection"+
				" configuration names.",
			err,
		)
		return 1
	}
	registry, err := loaded.BuildRegistry(
		[]string{options.profile},
		factoryOptions,
	)
	if err != nil {
		message := "The AI connection configuration could not be built" +
			" into a driver registry."
		if commandErrorCode(err) == "UNKNOWN_PROFILE" {
			message = "Could not find that profile and model" +
				" in the AI connection configuration."
		}
		writeCommandFailure(stderr, "driver probe", message, err)
		return 1
	}
	result, err := driver.ProbeLane(
		context.Background(), registry, options.profile, options.model,
	)
	if err != nil {
		message := "Could not send the lane probe."
		if commandErrorCode(err) == "UNKNOWN_PROFILE" {
			message = "Could not find that profile and model" +
				" in the AI connection configuration."
		}
		writeCommandFailure(stderr, "driver probe", message, err)
		return 1
	}
	output := driverProbeOutput{
		SchemaVersion:       driverProbeSchemaVersion,
		Profile:             result.Profile,
		Model:               result.Model,
		Family:              result.Family,
		Surface:             result.Surface,
		AdapterID:           result.AdapterID,
		AdapterVersion:      result.AdapterVersion,
		ConfigurationDigest: result.ConfigurationDigest,
		Ready:               result.Ready,
		Code:                result.Code,
		Message:             result.Message,
		RequestID:           result.RequestID,
		LatencyMillis:       result.LatencyMillis,
		LiveCall:            true,
		CLICompatibility:    result.CLICompatibility,
	}
	if options.json {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			fmt.Fprintln(stderr, "sworn driver probe: output failed")
			return 1
		}
	} else {
		writeDriverProbeText(stdout, output)
	}
	if !result.Ready {
		return 1
	}
	return 0
}

func writeDriverProbeText(out io.Writer, output driverProbeOutput) {
	if output.Ready {
		fmt.Fprintf(
			out,
			"sworn driver probe: %s / %s is admitting requests (%dms).\n",
			output.Profile, output.Model, output.LatencyMillis,
		)
	} else {
		fmt.Fprintf(
			out,
			"sworn driver probe: %s / %s is not admitting requests (%dms).\n",
			output.Profile, output.Model, output.LatencyMillis,
		)
	}
	fmt.Fprintf(out, "Technical code: %s\n", output.Code)
	if output.Message != "" {
		fmt.Fprintf(out, "Provider message: %s\n", output.Message)
	}
	if output.RequestID != "" {
		fmt.Fprintf(out, "Provider request id: %s\n", output.RequestID)
	}
	if output.CLICompatibility != nil {
		fmt.Fprintf(
			out,
			"Native CLI: %s (%s)\n",
			output.CLICompatibility.CLIVersion,
			output.CLICompatibility.Status,
		)
	}
}
