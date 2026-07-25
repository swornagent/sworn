package baton

import (
	"bytes"
	"fmt"
	"reflect"
)

// TransitionResult is one closed Baton responsibility result.
type TransitionResult string

const (
	DesignWritten TransitionResult = "DESIGN_WRITTEN"
	Proceed       TransitionResult = "PROCEED"
	Revise        TransitionResult = "REVISE"
	Escalate      TransitionResult = "ESCALATE"
	Implemented   TransitionResult = "IMPLEMENTED"
	Pass          TransitionResult = "PASS"
	Fail          TransitionResult = "FAIL"
	Blocked       TransitionResult = "BLOCKED"
	Merged        TransitionResult = "MERGED"
	Materialize   TransitionResult = "MATERIALIZE"
	Rebound       TransitionResult = "REBOUND"
	NoVerdict     TransitionResult = "NO_VERDICT"
)

func validTransitionResult(result TransitionResult) bool {
	return oneOf(string(result),
		string(DesignWritten), string(Proceed), string(Revise), string(Escalate),
		string(Implemented), string(Pass), string(Fail), string(Blocked),
		string(Merged), string(Materialize), string(Rebound), string(NoVerdict),
	)
}

// ValidateTransition validates the closed RC2 lifecycle table. It is
// structural only; action admission additionally requires trusted evidence.
func ValidateTransition(previous, next Status, result TransitionResult) error {
	previousAdmission, err := previous.require()
	if err != nil {
		return err
	}
	nextAdmission, err := next.require()
	if err != nil {
		return err
	}
	if !validTransitionResult(result) {
		return recordFail("UNKNOWN_TRANSITION_RESULT", "unknown responsibility result "+string(result))
	}
	before, after := previousAdmission.view, nextAdmission.view
	if before.Stage == "merge" && before.Status == "complete" {
		if result == NoVerdict && bytes.Equal(previousAdmission.raw, nextAdmission.raw) {
			return nil
		}
		return recordFail("TERMINAL_REWRITE", "terminal status identity and outcome are write-once")
	}
	if result == NoVerdict {
		if !bytes.Equal(previousAdmission.raw, nextAdmission.raw) {
			return recordFail("IMMUTABLE_BINDING_CHANGED", "durable status after runner failure changed")
		}
		return nil
	}
	switch result {
	case Materialize:
		return validateMaterializeTransition(before, after)
	case Rebound:
		return validateReboundTransition(before, after)
	case DesignWritten:
		return validateDesignWrittenTransition(before, after)
	case Proceed, Revise, Escalate:
		return validateCaptainTransition(before, after, result)
	case Implemented:
		return validateImplementedTransition(before, after)
	case Pass, Fail, Blocked:
		return validateVerificationTransition(before, after, result)
	case Merged:
		return validateMergedTransition(before, after)
	default:
		return recordFail("UNKNOWN_TRANSITION_RESULT", "unknown responsibility result "+string(result))
	}
}

func transitionSame(left, right any, label string) error {
	if !reflect.DeepEqual(left, right) {
		return recordFail("IMMUTABLE_BINDING_CHANGED", label+" changed across the transition")
	}
	return nil
}

func transitionDifferent(left, right any, label string) error {
	if reflect.DeepEqual(left, right) {
		return recordFail("EVIDENCE_NOT_REFRESHED", label+" must change across the transition")
	}
	return nil
}

func requireTransitionProjection(value StatusView, expected, label string) error {
	if observed := projection(value); observed != expected {
		return recordFail("INVALID_TRANSITION", fmt.Sprintf("%s must be %s, observed %s", label, expected, observed))
	}
	return nil
}

func assertIdentity(previous, next StatusView, allowAuthority, allowTarget bool) error {
	pairs := []struct {
		left  any
		right any
		label string
	}{
		{previous.Schema, next.Schema, "status.$schema"},
		{previous.SchemaVersion, next.SchemaVersion, "status.schema_version"},
		{previous.Kind, next.Kind, "status.kind"},
		{previous.Release, next.Release, "status.release"},
		{previous.WorkID, next.WorkID, "status.work_id"},
		{previous.TrackID, next.TrackID, "status.track_id"},
		{previous.OwnerRef, next.OwnerRef, "status.owner_ref"},
	}
	if !allowAuthority {
		pairs = append(pairs, struct {
			left  any
			right any
			label string
		}{previous.AuthorityRef, next.AuthorityRef, "status.authority_ref"})
	}
	if !allowTarget {
		pairs = append(pairs, struct {
			left  any
			right any
			label string
		}{previous.TargetRef, next.TargetRef, "status.target_ref"})
	}
	for _, pair := range pairs {
		if err := transitionSame(pair.left, pair.right, pair.label); err != nil {
			return err
		}
	}
	return nil
}

func assertNormalBindings(previous, next StatusView, allowAuthority, allowTarget bool) error {
	if err := assertIdentity(previous, next, allowAuthority, allowTarget); err != nil {
		return err
	}
	if err := transitionSame(previous.Plan, next.Plan, "status.plan"); err != nil {
		return err
	}
	return transitionSame(previous.Materialization, next.Materialization, "status.materialization")
}

func validateDesignWrittenTransition(previous, next StatusView) error {
	if previous.Kind != "work" {
		return recordFail("INVALID_TRANSITION", "only work has a design gate")
	}
	if err := requireTransitionProjection(previous, "design/ready/implementer", "DESIGN_WRITTEN source"); err != nil {
		return err
	}
	if err := requireTransitionProjection(next, "design/ready/captain", "DESIGN_WRITTEN result"); err != nil {
		return err
	}
	if err := assertNormalBindings(previous, next, false, false); err != nil {
		return err
	}
	switch previous.Outcome {
	case "none":
		if previous.Design != nil || previous.Captain != nil {
			return recordFail("UNEXPECTED_TRANSITION_FIELD", "initial design source contains gates")
		}
	case "revise":
		if previous.Captain == nil || previous.Captain.Outcome != "revise" || previous.Design == nil || next.Design == nil {
			return recordFail("INVALID_TRANSITION", "a repeated design must follow Captain REVISE")
		}
		if err := transitionDifferent(previous.Design.Digest, next.Design.Digest, "design digest after REVISE"); err != nil {
			return err
		}
		if err := transitionDifferent(previous.Design.ProducerInvocation, next.Design.ProducerInvocation, "design producer invocation after REVISE"); err != nil {
			return err
		}
	default:
		return recordFail("INVALID_TRANSITION", "DESIGN_WRITTEN source has an invalid outcome")
	}
	if next.Outcome != "none" || next.Captain != nil || next.Proof != nil || next.Verification != nil ||
		next.Merge != nil || next.Blocker != nil {
		return recordFail("INVALID_TRANSITION", "DESIGN_WRITTEN result retains an invalid field")
	}
	return nil
}

func validateCaptainTransition(previous, next StatusView, result TransitionResult) error {
	if previous.Kind != "work" {
		return recordFail("INVALID_TRANSITION", "only work has a Captain gate")
	}
	if err := requireTransitionProjection(previous, "design/ready/captain", string(result)+" source"); err != nil {
		return err
	}
	if err := assertNormalBindings(previous, next, false, false); err != nil {
		return err
	}
	if err := transitionSame(previous.Design, next.Design, "status.design"); err != nil {
		return err
	}
	if previous.Captain != nil || previous.Proof != nil || previous.Verification != nil || previous.Merge != nil || previous.Blocker != nil {
		return recordFail("UNEXPECTED_TRANSITION_FIELD", string(result)+" source retains later evidence")
	}
	if next.Captain == nil || next.Captain.Outcome != stringsLower(string(result)) {
		return recordFail("INVALID_TRANSITION", string(result)+" requires a matching Captain outcome")
	}
	switch result {
	case Proceed:
		if err := requireTransitionProjection(next, "implement/ready/implementer", "PROCEED result"); err != nil {
			return err
		}
		if next.Outcome != "proceed" || next.Proof != nil || next.Verification != nil || next.Merge != nil || next.Blocker != nil {
			return recordFail("INVALID_TRANSITION", "PROCEED result has invalid evidence")
		}
	case Revise:
		if err := requireTransitionProjection(next, "design/ready/implementer", "REVISE result"); err != nil {
			return err
		}
		if next.Outcome != "revise" || next.Proof != nil || next.Verification != nil || next.Merge != nil || next.Blocker != nil {
			return recordFail("INVALID_TRANSITION", "REVISE result has invalid evidence")
		}
	case Escalate:
		if err := requireTransitionProjection(next, "design/blocked/planner", "ESCALATE result"); err != nil {
			return err
		}
		if next.Outcome != "escalate" || next.Proof != nil || next.Verification != nil || next.Merge != nil {
			return recordFail("INVALID_TRANSITION", "ESCALATE result has invalid evidence")
		}
	}
	return nil
}

func validateImplementedTransition(previous, next StatusView) error {
	if previous.Kind != "work" {
		return recordFail("INVALID_TRANSITION", "IMPLEMENTED applies only to work")
	}
	if err := requireTransitionProjection(previous, "implement/ready/implementer", "IMPLEMENTED source"); err != nil {
		return err
	}
	if err := requireTransitionProjection(next, "verify/ready/verifier", "IMPLEMENTED result"); err != nil {
		return err
	}
	if err := assertNormalBindings(previous, next, false, false); err != nil {
		return err
	}
	for _, gate := range []struct {
		left, right any
		label       string
	}{{previous.Design, next.Design, "status.design"}, {previous.Captain, next.Captain, "status.captain"}} {
		if err := transitionSame(gate.left, gate.right, gate.label); err != nil {
			return err
		}
	}
	if previous.Captain == nil || previous.Captain.Outcome != "proceed" {
		return recordFail("INVALID_TRANSITION", "IMPLEMENTED requires a current Captain PROCEED")
	}
	switch previous.Outcome {
	case "proceed":
		if previous.Proof != nil || previous.Verification != nil || previous.Merge != nil || previous.Blocker != nil {
			return recordFail("UNEXPECTED_TRANSITION_FIELD", "first implementation source retains evidence")
		}
	case "fail":
		if previous.Proof == nil || previous.Verification == nil || previous.Verification.Outcome != "fail" ||
			next.Proof == nil {
			return recordFail("INVALID_TRANSITION", "repair must follow Verifier FAIL")
		}
		if err := transitionDifferent(previous.Proof.Digest, next.Proof.Digest, "proof digest after FAIL"); err != nil {
			return err
		}
		if err := transitionDifferent(previous.Proof.ProducerInvocation, next.Proof.ProducerInvocation, "proof producer after FAIL"); err != nil {
			return err
		}
	default:
		return recordFail("INVALID_TRANSITION", "IMPLEMENTED source must follow PROCEED or FAIL")
	}
	if next.Outcome != "none" || next.Verification != nil || next.Merge != nil || next.Blocker != nil {
		return recordFail("INVALID_TRANSITION", "IMPLEMENTED result retains invalid evidence")
	}
	return nil
}

func validateVerificationTransition(previous, next StatusView, result TransitionResult) error {
	if err := requireTransitionProjection(previous, "verify/ready/verifier", string(result)+" source"); err != nil {
		return err
	}
	if err := assertNormalBindings(previous, next, false, false); err != nil {
		return err
	}
	for _, gate := range []struct {
		left, right any
		label       string
	}{{previous.Design, next.Design, "status.design"}, {previous.Captain, next.Captain, "status.captain"}, {previous.Proof, next.Proof, "status.proof"}} {
		if err := transitionSame(gate.left, gate.right, gate.label); err != nil {
			return err
		}
	}
	if previous.Verification != nil || previous.Merge != nil || previous.Blocker != nil ||
		next.Verification == nil || next.Verification.Outcome != stringsLower(string(result)) {
		return recordFail("INVALID_TRANSITION", string(result)+" has invalid Verifier evidence")
	}
	switch result {
	case Pass:
		if err := requireTransitionProjection(next, "merge/ready/merge", "PASS result"); err != nil {
			return err
		}
		if next.Outcome != "pass" || next.Merge != nil || next.Blocker != nil {
			return recordFail("INVALID_TRANSITION", "PASS result has invalid fields")
		}
	case Fail:
		if previous.Kind == "assembly" {
			if err := requireTransitionProjection(next, "verify/ready/planner", "assembly FAIL result"); err != nil {
				return err
			}
		} else if err := requireTransitionProjection(next, "implement/ready/implementer", "FAIL result"); err != nil {
			return err
		}
		if next.Outcome != "fail" || next.Merge != nil || next.Blocker != nil {
			return recordFail("INVALID_TRANSITION", "FAIL result has invalid fields")
		}
	case Blocked:
		if err := requireTransitionProjection(next, "verify/blocked/planner", "BLOCKED result"); err != nil {
			return err
		}
		if next.Outcome != "blocked" || next.Merge != nil {
			return recordFail("INVALID_TRANSITION", "BLOCKED result has invalid fields")
		}
	}
	return nil
}

func validateMergedTransition(previous, next StatusView) error {
	if err := requireTransitionProjection(previous, "merge/ready/merge", "MERGED source"); err != nil {
		return err
	}
	if err := requireTransitionProjection(next, "merge/complete/none", "MERGED result"); err != nil {
		return err
	}
	if err := assertNormalBindings(previous, next, previous.Kind == "work", false); err != nil {
		return err
	}
	for _, gate := range []struct {
		left, right any
		label       string
	}{{previous.Design, next.Design, "status.design"}, {previous.Captain, next.Captain, "status.captain"}, {previous.Proof, next.Proof, "status.proof"}, {previous.Verification, next.Verification, "status.verification"}} {
		if err := transitionSame(gate.left, gate.right, gate.label); err != nil {
			return err
		}
	}
	if previous.Merge != nil || previous.Blocker != nil || next.Outcome != "merged" || next.Merge == nil || next.Merge.Outcome != "merged" {
		return recordFail("INVALID_TRANSITION", "MERGED result must contain the deterministic Merge binding")
	}
	releaseRef := "refs/heads/release-wt/" + previous.Release
	if previous.Kind == "work" {
		if previous.AuthorityRef != previous.OwnerRef || next.AuthorityRef != releaseRef {
			return recordFail("INVALID_AUTHORITY_TRANSFER", "work Merge transfers authority from its track to release-wt")
		}
	} else if next.AuthorityRef != releaseRef {
		return recordFail("INVALID_AUTHORITY_TRANSFER", "assembly authority remains release-wt")
	}
	return nil
}

func validateMaterializeTransition(previous, next StatusView) error {
	if previous.Kind != "work" || next.Kind != "work" {
		return recordFail("INVALID_TRANSITION", "MATERIALIZE applies only to planned work")
	}
	if previous.Stage == "merge" && previous.Status == "complete" {
		return recordFail("TERMINAL_REWRITE", "terminal work cannot be materialised")
	}
	if err := requireTransitionProjection(previous, "design/ready/implementer", "MATERIALIZE source"); err != nil {
		return err
	}
	if previous.Outcome != "none" || previous.Materialization != nil || previous.Design != nil || previous.Captain != nil ||
		previous.Proof != nil || previous.Verification != nil || previous.Merge != nil || previous.Blocker != nil {
		return recordFail("INVALID_TRANSITION", "MATERIALIZE requires a pristine release baseline")
	}
	if err := assertIdentity(previous, next, true, false); err != nil {
		return err
	}
	releaseRef := "refs/heads/release-wt/" + previous.Release
	if previous.AuthorityRef != releaseRef || next.AuthorityRef != previous.OwnerRef || next.Materialization == nil {
		return recordFail("INVALID_AUTHORITY_TRANSFER", "MATERIALIZE transfers release baseline authority to the exact owner ref")
	}
	beforeCopy, afterCopy := previous, next
	beforeCopy.AuthorityRef = ""
	afterCopy.AuthorityRef = ""
	afterCopy.Materialization = nil
	return transitionSame(beforeCopy, afterCopy, "materialised durable projection")
}

func validateReboundTransition(previous, next StatusView) error {
	if previous.Kind != "work" || next.Kind != "work" {
		return recordFail("INVALID_TRANSITION", "REBOUND applies only to non-terminal work")
	}
	if previous.Stage == "merge" && previous.Status == "complete" {
		return recordFail("TERMINAL_REWRITE", "terminal work cannot be rebound")
	}
	releaseRef := "refs/heads/release-wt/" + previous.Release
	if previous.AuthorityRef != releaseRef || previous.Materialization != nil ||
		projection(previous) != "design/ready/implementer" || previous.Outcome != "none" ||
		previous.Blocker != nil || previous.Design != nil || previous.Captain != nil ||
		previous.Proof != nil || previous.Verification != nil || previous.Merge != nil {
		return recordFail("MATERIALIZED_REBOUND", "REBOUND applies only to a pristine unmaterialized release baseline")
	}
	if err := assertIdentity(previous, next, false, true); err != nil {
		return err
	}
	if reflect.DeepEqual(previous.Plan, next.Plan) {
		return recordFail("REPLAN_NOT_CHANGED", "REBOUND requires a new plan or approval binding")
	}
	if next.Materialization != nil {
		return recordFail("MATERIALIZED_REBOUND", "REBOUND result cannot retain materialization")
	}
	if err := requireTransitionProjection(next, "design/ready/implementer", "REBOUND result"); err != nil {
		return err
	}
	if next.Outcome != "none" || next.Blocker != nil || next.Design != nil || next.Captain != nil ||
		next.Proof != nil || next.Verification != nil || next.Merge != nil {
		return recordFail("INVALID_TRANSITION", "REBOUND result is not pristine")
	}
	return nil
}

func stringsLower(value string) string {
	result := make([]byte, len(value))
	for index := range value {
		if value[index] >= 'A' && value[index] <= 'Z' {
			result[index] = value[index] + ('a' - 'A')
		} else {
			result[index] = value[index]
		}
	}
	return string(result)
}
