package baton

import "testing"

func failScopeOID() string { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }
func failScopeDigest() string {
	return "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}

// verifierFailScopeReceipt builds an otherwise-valid verifier receipt, minus
// FailScope, which the caller sets to exercise the additive field.
func verifierFailScopeReceipt(result string) Receipt {
	slice, attempt, contract := "S1", int64(2), failScopeDigest()
	candidate, productTree, checks := failScopeOID(), failScopeDigest(), failScopeDigest()
	return Receipt{
		Version: ReceiptVersion, Release: "release", Role: "verifier", Result: result,
		Plan: failScopeOID(), Binds: failScopeOID(), Detail: failScopeDigest(), Summary: "summary",
		Slice: &slice, Attempt: &attempt, Contract: &contract,
		Candidate: &candidate, ProductTree: &productTree, Checks: &checks,
		Inputs: map[string]string{},
	}
}

// captainFailScopeReceipt builds an otherwise-valid captain receipt, minus
// FailScope, which the caller sets to prove the field is forbidden outside
// a verifier/fail receipt.
func captainFailScopeReceipt() Receipt {
	slice, attempt, contract := "S1", int64(2), failScopeDigest()
	return Receipt{
		Version: ReceiptVersion, Release: "release", Role: "captain", Result: "proceed",
		Plan: failScopeOID(), Binds: failScopeOID(), Detail: failScopeDigest(), Summary: "summary",
		Slice: &slice, Attempt: &attempt, Contract: &contract,
	}
}

func TestReceiptFailScopeRoundTripsAndIsGatedToVerifierFail(t *testing.T) {
	t.Parallel()
	evidence := "evidence"
	receipt := verifierFailScopeReceipt("fail")
	receipt.FailScope = &evidence

	canonical, err := receipt.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseReceipt(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.FailScope == nil || *parsed.FailScope != "evidence" {
		t.Fatalf("fail_scope round trip = %v", parsed.FailScope)
	}
	roundTripped, err := parsed.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(roundTripped) != string(canonical) {
		t.Fatalf("canonical bytes not stable:\n%s\nvs\n%s", roundTripped, canonical)
	}
	cloned := parsed.Clone()
	if cloned.FailScope == parsed.FailScope || *cloned.FailScope != *parsed.FailScope {
		t.Fatalf("Clone did not deep-copy FailScope: %#v vs %#v", cloned.FailScope, parsed.FailScope)
	}
}

func TestReceiptFailScopeRejectedOutsideVerifierFail(t *testing.T) {
	t.Parallel()
	evidence := "evidence"
	tests := map[string]Receipt{
		"verifier/pass":    func() Receipt { r := verifierFailScopeReceipt("pass"); r.FailScope = &evidence; return r }(),
		"verifier/blocked": func() Receipt { r := verifierFailScopeReceipt("blocked"); r.FailScope = &evidence; return r }(),
		"captain/proceed":  func() Receipt { r := captainFailScopeReceipt(); r.FailScope = &evidence; return r }(),
	}
	for name, bad := range tests {
		name, bad := name, bad
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := bad.CanonicalBytes(); ErrorCode(err) != "INVALID_FIELD" {
				t.Fatalf("%s: error = %v", name, err)
			}
		})
	}
}

func TestReceiptFailScopeRejectsUnknownLiteral(t *testing.T) {
	t.Parallel()
	code := "code"
	receipt := verifierFailScopeReceipt("fail")
	receipt.FailScope = &code
	if _, err := receipt.CanonicalBytes(); ErrorCode(err) != "INVALID_FIELD" {
		t.Fatalf("error = %v", err)
	}
}
