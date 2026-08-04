package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

func runApprove(args []string, stdout, stderr io.Writer) int {
	required := []string{
		"--journal", "--run", "--manifest-digest", "--project", "--release",
		"--release-ref", "--release-head", "--proposal-replay-key",
		"--plan-revision", "--prior-plan", "--plan-digest", "--target-ref",
		"--target-head", "--decision-class", "--decision", "--actor-class",
		"--actor-authority",
	}
	options, ok := parseOptionsWithOptionalValues(
		args, required, []string{"--config"}, nil, nil,
	)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn approve requires every exact approval binding; use absent for release-head and prior-plan when revision is 1")
		return 2
	}
	revision, err := strconv.ParseInt(options["--plan-revision"], 10, 64)
	if err != nil || revision < 1 {
		fmt.Fprintln(stderr, "sworn approve: plan-revision must be positive")
		return 2
	}
	releaseHead := options["--release-head"]
	priorPlan := options["--prior-plan"]
	if releaseHead == "absent" {
		releaseHead = ""
	}
	if priorPlan == "absent" {
		priorPlan = ""
	}
	command := runtimepkg.ApprovalCommand{
		SchemaVersion: runtimepkg.ApprovalCommandVersion,
		RunID:         options["--run"], ManifestDigest: options["--manifest-digest"],
		Project: options["--project"], Release: options["--release"],
		ReleaseRef: options["--release-ref"], ReleaseHead: releaseHead,
		ProposalReplayKey: options["--proposal-replay-key"],
		PlanRevision:      revision, PriorPlan: priorPlan,
		PlanDigest: options["--plan-digest"], TargetRef: options["--target-ref"],
		TargetHead: options["--target-head"], DecisionClass: options["--decision-class"],
		Decision: options["--decision"], ActorClass: options["--actor-class"],
		ActorAuthority: options["--actor-authority"],
	}
	if _, err := runtimepkg.CanonicalApprovalCommand(command); err != nil {
		writeCommandFailure(stderr, "approve", "The exact approval binding is invalid.", err)
		return 1
	}
	ctx := context.Background()
	service, factory, err := openRuntimeService(
		ctx, options["--journal"], options["--config"],
	)
	if err != nil {
		writeCommandFailure(stderr, "approve", "Could not open the saved run.", err)
		return 1
	}
	defer service.Close()
	defer factory.Close()
	result, err := service.Approve(ctx, command)
	if err != nil {
		writeCommandFailure(stderr, "approve", "The exact approval was not admitted.", err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(stderr, "sworn approve: output failed")
		return 1
	}
	return 0
}
