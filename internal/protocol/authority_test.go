package protocol

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

func TestAuthorityApprovalMatchesBatonFixtureAndRoundTrips(t *testing.T) {
	snapshot, err := SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/artifacts/authority/plan-approval.json")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ParseAuthorityApproval(contents)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ReceiptID != "authority-example-release-plan-v1" ||
		receipt.PlanDigest != "sha256:5f44521823b466b350b572813c7aa8677a5e487e4eadfc8f35fde23580f5422f" ||
		len(receipt.Grants) != 5 {
		t.Fatalf("unexpected fixture receipt: %#v", receipt)
	}
	encoded, err := EncodeAuthorityApproval(receipt)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		t.Fatal(err)
	}
	if encoded.Kind != ControlReceiptSchemaVersion || encoded.Digest != CanonicalDigest(canonical) ||
		!bytes.Equal(encoded.CanonicalJSON, canonical) {
		t.Fatal("authority receipt encoding drifted from the Baton canonical fixture")
	}
}

func TestAuthorityApprovalRejectsSchemaNearMisses(t *testing.T) {
	base := `{
		"schema_version":"control-receipt-v1","kind":"authority_approval",
		"receipt_id":"approval-1","plan_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"authority_digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"source_ref":"authority:main","source_digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"grants":[{"action":"inspect","target":"workspace"}],"repository":"local:repo",
		"target_ref":"refs/heads/main","authorizer_ref":"identity:owner","approved_at":"2026-07-19T00:00:00Z"
	}`
	mutations := map[string]string{
		"case folded field": strings.Replace(base, `"receipt_id"`, `"Receipt_ID"`, 1),
		"null grant list":   strings.Replace(base, `"grants":[{"action":"inspect","target":"workspace"}]`, `"grants":null`, 1),
		"duplicate grant": strings.Replace(base,
			`"grants":[{"action":"inspect","target":"workspace"}]`,
			`"grants":[{"action":"inspect","target":"workspace"},{"target":"workspace","action":"inspect"}]`, 1),
		"unknown grant field": strings.Replace(base,
			`{"action":"inspect","target":"workspace"}`,
			`{"action":"inspect","target":"workspace","extra":true}`, 1),
		"invalid approval time": strings.Replace(base, `2026-07-19T00:00:00Z`, `2026-07-19T00:00:60Z`, 1),
	}
	for name, contents := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAuthorityApproval([]byte(contents)); err == nil {
				t.Fatal("invalid authority approval was accepted")
			}
		})
	}
}

func TestParseAuthorityGrantRetainsCanonicalBindingDefensively(t *testing.T) {
	grant, err := ParseAuthorityGrant([]byte(`{ "target": {"ref":"refs/heads/main","repository":"local:repo"}, "action":"integrate" }`))
	if err != nil {
		t.Fatal(err)
	}
	target, ok := grant.Integration()
	if !ok || grant.Action() != "integrate" || target.Repository != "local:repo" || target.Ref != "refs/heads/main" {
		t.Fatalf("unexpected parsed grant: %q %#v %t", grant.Action(), target, ok)
	}
	first := grant.CanonicalJSON()
	first[0] = '['
	if bytes.Equal(first, grant.CanonicalJSON()) {
		t.Fatal("caller mutated retained canonical grant bytes")
	}
}
