package runtime

import (
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

// TestStableErrorCodeCarriesUncontainedDispatchRefusal pins the reporting path
// the driver refusal travels through on the runtime side: dispatch.go journals
// dispatch_operational_failure with stableErrorCode(invokeErr), and the driver
// contract code UNCONTAINED_DISPATCH_REFUSED must survive unchanged (it matches
// runtimeIdentityPattern, so it is never downgraded to operational_failure).
func TestStableErrorCodeCarriesUncontainedDispatchRefusal(t *testing.T) {
	err := &driver.ContractError{Code: "UNCONTAINED_DISPATCH_REFUSED"}
	if got := stableErrorCode(err); got != "UNCONTAINED_DISPATCH_REFUSED" {
		t.Fatalf("stable code = %q, want UNCONTAINED_DISPATCH_REFUSED", got)
	}
}
