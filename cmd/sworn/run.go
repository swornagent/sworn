package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/app"
	"github.com/swornagent/sworn/internal/engine"
)

type runApplication func(context.Context, app.Request) (app.Result, error)

type runOptions struct {
	request app.Request
	asJSON  bool
}

func runDelivery(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	application runApplication,
) int {
	options, err := parseRunOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "sworn run: %v\n", err)
		fmt.Fprintln(stderr, "usage: sworn run <run> [<work>] --config <clean-absolute-path> [--json]")
		return 2
	}
	if application == nil {
		fmt.Fprintln(stderr, "sworn run: production application is unavailable")
		return 1
	}
	result, err := application(ctx, options.request)
	if err != nil {
		fmt.Fprintf(stderr, "sworn run: %v\n", err)
		return 1
	}
	if err := writeRunResult(stdout, result, options.asJSON); err != nil {
		fmt.Fprintf(stderr, "sworn run: write output: %v\n", err)
		return 1
	}
	return 0
}

func parseRunOptions(args []string) (runOptions, error) {
	var options runOptions
	var positional []string
	configSeen := false
	jsonSeen := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--config":
			if configSeen {
				return runOptions{}, errors.New("--config may be specified only once")
			}
			configSeen = true
			index++
			if index >= len(args) || args[index] == "" {
				return runOptions{}, errors.New("--config requires a path")
			}
			options.request.ConfigPath = args[index]
		case "--json":
			if jsonSeen {
				return runOptions{}, errors.New("--json may be specified only once")
			}
			jsonSeen, options.asJSON = true, true
		default:
			if strings.HasPrefix(args[index], "-") {
				return runOptions{}, fmt.Errorf("unknown option %q", args[index])
			}
			positional = append(positional, args[index])
		}
	}
	if len(positional) < 1 || len(positional) > 2 {
		return runOptions{}, errors.New("select one run and at most one work item")
	}
	if !configSeen {
		return runOptions{}, errors.New("--config is required")
	}
	if !filepath.IsAbs(options.request.ConfigPath) ||
		filepath.Clean(options.request.ConfigPath) != options.request.ConfigPath {
		return runOptions{}, errors.New("--config requires a clean absolute path")
	}
	options.request.RunID = positional[0]
	if !engine.ValidID(options.request.RunID) {
		return runOptions{}, errors.New("run id is invalid")
	}
	if len(positional) == 2 {
		options.request.WorkID = positional[1]
		if !engine.ValidID(options.request.WorkID) {
			return runOptions{}, errors.New("work id is invalid")
		}
	}
	return options, nil
}

func writeRunResult(output io.Writer, result app.Result, asJSON bool) error {
	if err := result.Validate(); err != nil {
		return fmt.Errorf("application returned an invalid bounded run result: %w", err)
	}
	if asJSON {
		encoder := json.NewEncoder(output)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(result)
	}
	_, err := fmt.Fprintf(
		output, "run %s work %s: %s (revision %d)\n",
		result.RunID, result.WorkID, result.State, result.Revision,
	)
	return err
}
