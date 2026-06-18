// Package implement — Definition of Ready gate.
//
// CheckDoR evaluates whether a slice may transition from planned to in_progress
// by composing the RTM (trace), reqverify (AC quality), and reqvalidate (human
// ratification) gates.  It fails closed — any gate that cannot be evaluated
// blocks the transition.
package implement

import (
	"context"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/reqvalidate"
	"github.com/swornagent/sworn/internal/reqverify"
	"github.com/swornagent/sworn/internal/rtm"
)

// agentVerifier adapts agent.Agent to reqverify.Verifier, allowing the
// implementer's Run() function to evaluate the requirements-verification
// gate (S04) as part of the Definition of Ready without needing a separate
// model client.
type agentVerifier struct {
	a agent.Agent
}

func (v agentVerifier) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	resp, err := v.a.Chat(ctx, []model.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPayload},
	}, nil)
	if err != nil {
		return "", 0, err
	}
	if len(resp.Choices) == 0 {
		return "", 0, fmt.Errorf("agent verifier: empty response")
	}
	return resp.Choices[0].Message.Content, 0.0, nil
}
// DoRResult captures the Definition of Ready evaluation for one slice.
// Passed is true only when every gate passes.
type DoRResult struct {
	Passed              bool
	RTMPassed           bool
	ReqverifyPassed     bool
	ReqvalidatePassed   bool
	RTMFailures         []string // human-readable failure descriptions
	ReqverifyFailures   []string // human-readable failure descriptions
	ReqvalidateFailures []string // human-readable failure descriptions
}

// CheckDoR evaluates the Definition of Ready for a specific slice.
//
// It composes three gates:
//  1. RTM (trace completeness) — via rtm.Build, filtered to the target slice
//  2. Requirements verify (AC quality) — via reqverify.Run with the given Verifier
//  3. Requirements validate (human ratification) — via reqvalidate.Run, filtered
//
// releaseDir is the absolute path to docs/release/<name>/.
// sliceID is the slice identifier (e.g. "S06-definition-of-ready").
// verifier may be nil — if nil, reqverify is skipped and ReqverifyPassed is
// reported as false with a "not evaluated" failure (fail closed).
func CheckDoR(ctx context.Context, releaseDir, sliceID string, verifier reqverify.Verifier) (*DoRResult, error) {
	result := &DoRResult{}

	// ---- 1. RTM check ----
	m, violations, err := rtm.Build(releaseDir)
	if err != nil {
		return result, fmt.Errorf("dor: rtm build: %w", err)
	}
	_ = m // we only care about violations for the target slice

	for _, v := range violations {
		if strings.Contains(v.Detail, sliceID) {
			result.RTMFailures = append(result.RTMFailures, v.String())
		}
		// orphaned_need violations don't name a specific slice, but they
		// affect the release's trace completeness. Include them when they
		// exist — the release cannot be fully traced.
		if v.Kind == "orphaned_need" {
			result.RTMFailures = append(result.RTMFailures, v.String())
		}
	}
	result.RTMPassed = len(result.RTMFailures) == 0

	// ---- 2. reqverify check ----
	if verifier == nil {
		result.ReqverifyPassed = false
		result.ReqverifyFailures = append(result.ReqverifyFailures,
			"requirements verification not evaluated (no verifier available)")
	} else {
		// Use a minimal system prompt that focuses on the quality-characteristic
		// grading. The full RequirementsVerifier prompt is loaded by the CLI;
		// here we use a lightweight version sufficient for the gate.
		const systemPrompt = `You are a requirements quality gate. Grade each acceptance criterion against ISO/IEC/IEEE 29148:2018 quality characteristics (singular, unambiguous, complete, consistent, feasible, verifiable, necessary). For each AC, respond with PASS or FAIL followed by the breached characteristic and a one-sentence reason.`
		rvReport, rvErr := reqverify.Run(ctx, releaseDir, verifier, systemPrompt)
		if rvErr != nil {
			return result, fmt.Errorf("dor: reqverify: %w", rvErr)
		}
		for _, v := range rvReport.Violations {
			if v.SliceID == sliceID {
				failure := fmt.Sprintf("%s (AC %d): %s", v.Characteristic, v.ACIndex, v.Reason)
				result.ReqverifyFailures = append(result.ReqverifyFailures, failure)
			}
		}
		result.ReqverifyPassed = len(result.ReqverifyFailures) == 0
	}

	// ---- 3. reqvalidate check ----
	rvldReport, rvldErr := reqvalidate.Run(releaseDir)
	if rvldErr != nil {
		return result, fmt.Errorf("dor: reqvalidate: %w", rvldErr)
	}
	for _, v := range rvldReport.Violations {
		if v.SliceID == sliceID {
			result.ReqvalidateFailures = append(result.ReqvalidateFailures, v.Reason)
		}
	}
	result.ReqvalidatePassed = len(result.ReqvalidateFailures) == 0

	result.Passed = result.RTMPassed && result.ReqverifyPassed && result.ReqvalidatePassed
	return result, nil
}

// DoRErrorSummary returns a human-readable error message from a failed DoRResult.
// If the result is nil or passed, it returns an empty string.
func DoRErrorSummary(result *DoRResult) string {
	if result == nil || result.Passed {
		return ""
	}
	var b strings.Builder
	b.WriteString("Definition of Ready failed:")
	if !result.RTMPassed {
		b.WriteString("\n  RTM:")
		for _, f := range result.RTMFailures {
			b.WriteString("\n    - " + f)
		}
	}
	if !result.ReqverifyPassed {
		b.WriteString("\n  Requirements verification:")
		for _, f := range result.ReqverifyFailures {
			b.WriteString("\n    - " + f)
		}
	}
	if !result.ReqvalidatePassed {
		b.WriteString("\n  Requirements validation:")
		for _, f := range result.ReqvalidateFailures {
			b.WriteString("\n    - " + f)
		}
	}
	return b.String()
}