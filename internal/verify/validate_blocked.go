package verify

import (
	"encoding/json"
	"fmt"
	"os"
)

// SliceStatus holds only the fields needed to validate a BLOCKED verdict.
type SliceStatus struct {
	State        string              `json:"state"`
	Verification VerificationSummary `json:"verification"`
}

// VerificationSummary holds the verdict fields from status.json.
type VerificationSummary struct {
	Result     string   `json:"result"`
	Violations []string `json:"violations"`
}

// ValidateBlockedViolations reads a slice's status.json and returns an error if
// the verdict is BLOCKED but violations is empty. Returns nil for non-BLOCKED
// verdicts or BLOCKED with populated violations. The error message names the
// slice so the caller can surface it in human-readable output.
func ValidateBlockedViolations(statusPath string) error {
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return fmt.Errorf("ValidateBlockedViolations: cannot read %s: %w", statusPath, err)
	}
	var s SliceStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("ValidateBlockedViolations: %s is not valid JSON: %w", statusPath, err)
	}
	if s.Verification.Result == "blocked" && len(s.Verification.Violations) == 0 {
		return fmt.Errorf("BLOCKED verdict with empty violations in %s: a BLOCKED verdict must record the concrete defect in verification.violations", statusPath)
	}
	return nil
}