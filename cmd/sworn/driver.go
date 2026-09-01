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

type driverReadinessOutput struct {
	SchemaVersion       string                 `json:"schema_version"`
	Command             string                 `json:"command"`
	ConfigurationDigest string                 `json:"configuration_digest"`
	Reports             []driver.ProfileReport `json:"reports"`
}

type driverCommandOptions struct {
	config  string
	profile string
	model   string
	all     bool
}

func runDriver(args []string, stdout, stderr io.Writer) int {
	command, options, ok := parseDriverCommand(args)
	if !ok {
		fmt.Fprintln(
			stderr,
			"usage: sworn driver inspect|doctor|certify --config ABS --json "+
				"(--profile PROFILE --model MODEL | --all)",
		)
		return 2
	}

	loaded, err := driver.LoadDriverConfig(options.config)
	if err != nil {
		writeCommandFailure(
			stderr,
			"driver "+command,
			"Could not read the AI connection configuration.",
			err,
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

	var registry driver.ConfiguredDriverRegistry
	if options.all {
		registry, err = loaded.BuildAllRegistry(factory.Options())
	} else {
		registry, err = loaded.BuildRegistry(
			[]string{options.profile},
			factory.Options(),
		)
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

	reports, ready := driverReports(
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
) ([]driver.ProfileReport, bool) {
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
				report.Family != driver.ProfileFake
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
	return reports, ready && completeProductionReadiness(reports)
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

func completeProductionReadiness(reports []driver.ProfileReport) bool {
	families := make(map[driver.ProfileFamily]bool)
	surfaces := make(map[driver.ProfileSurface]bool)
	for _, report := range reports {
		if report.State != driver.ReadinessPass {
			return false
		}
		families[report.Family] = true
		if report.Surface != "" {
			surfaces[report.Surface] = true
		}
	}
	for _, family := range []driver.ProfileFamily{
		driver.ProfileCodex,
		driver.ProfileClaude,
		driver.ProfileOpenAIHTTP,
		driver.ProfileDeepSeek,
		driver.ProfileGemini,
		driver.ProfileBedrock,
	} {
		if !families[family] {
			return false
		}
	}
	return surfaces[driver.ProfileSurfaceBedrockRuntimeConverse] &&
		surfaces[driver.ProfileSurfaceBedrockMantleChat]
}
