package protocol

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"testing"
)

const (
	standardPlanDigest      = "sha256:5f44521823b466b350b572813c7aa8677a5e487e4eadfc8f35fde23580f5422f"
	standardAuthorityDigest = "sha256:20d9d443a98f0a43d64e4eaffdb29bf111c1a00f7c42847094a5a57e81d8da4b"
	standardContractDigest  = "sha256:3636fadbe95f88831a30d05044113459932e813fb36d4b30cee623663e219a94"
)

func TestParseDeliveryPlanBindsCompleteCanonicalFixture(t *testing.T) {
	t.Parallel()

	contents := standardPlanFixture(t)
	plan, err := ParseDeliveryPlan(contents)
	if err != nil {
		t.Fatalf("ParseDeliveryPlan() error = %v", err)
	}
	record := plan.Record()
	if record.Kind != DeliveryPlanSchemaVersion || record.Digest != standardPlanDigest {
		t.Fatalf("record = (%q, %q), want (%q, %q)", record.Kind, record.Digest, DeliveryPlanSchemaVersion, standardPlanDigest)
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(record.CanonicalJSON, canonical) || len(canonical) != 1167 {
		t.Fatalf("canonical plan binding differs (length %d)", len(canonical))
	}
	if plan.DeliveryID() != "example-release" ||
		plan.Outcome() != "Expose a health endpoint that reports the assembled service as ready." ||
		plan.CreatedAt() != "2026-07-19T00:00:00Z" {
		t.Fatalf("plan facts = %q, %q, %q", plan.DeliveryID(), plan.Outcome(), plan.CreatedAt())
	}
	if plan.Policy() != (PlanPolicy{
		Ref:    "examples/assurance-policy.json",
		Digest: "sha256:7a97154ed556cf6821be212cc8a8b97268e4bb74e5d6286e0337f157e00c2a23",
	}) {
		t.Fatalf("policy = %#v", plan.Policy())
	}
	if plan.Target() != (PlanTarget{Repository: "local:example", Ref: "refs/heads/main"}) {
		t.Fatalf("target = %#v", plan.Target())
	}
	authority := plan.Authority()
	if authority.SourceRef != "examples/authority-source.json" ||
		authority.Digest != standardAuthorityDigest || len(authority.Grants) != 5 {
		t.Fatalf("authority = %#v", authority)
	}
	workspaceTarget, workspaceIntegration := authority.Grants[0].Integration()
	integrationTarget, hasIntegration := authority.Grants[4].Integration()
	if authority.Grants[0].Action() != "inspect" || workspaceIntegration || workspaceTarget != (PlanTarget{}) ||
		authority.Grants[4].Action() != "integrate" || !hasIntegration || integrationTarget != plan.Target() {
		t.Fatalf("authority grants = %#v", authority.Grants)
	}
	if got := string(authority.Grants[0].CanonicalJSON()); got != `{"action":"inspect","target":"workspace"}` {
		t.Fatalf("canonical inspect grant = %q", got)
	}
	if (PlanGrant{}).CanonicalJSON() != nil {
		t.Fatal("caller-constructed grant acquired a canonical binding")
	}
	if ids := plan.WorkIDs(); !slices.Equal(ids, []string{"health-endpoint"}) {
		t.Fatalf("work ids = %#v", ids)
	}
	contract, ok := plan.Work("health-endpoint")
	if !ok || contract.Digest() != standardContractDigest {
		t.Fatalf("contract = %#v, %t, digest %q", contract, ok, contract.Digest())
	}
	view := contract.View()
	if view.ID != "health-endpoint" || view.Outcome != "The running service answers GET /health with a ready response." ||
		!slices.Equal(view.Scope.Include, []string{"src", "tests"}) ||
		!slices.Equal(view.Scope.Exclude, []string{"vendor"}) || len(view.Acceptance) != 1 ||
		view.Acceptance[0] != (PlanAcceptance{
			ID: "AC1", Criterion: "Starting the assembled service and requesting GET /health returns HTTP 200 with {\"status\":\"ready\"}.",
			EvidenceLevel: "assembled",
		}) || len(view.DependsOn) != 0 || view.Assurance.Profile != "standard" ||
		len(view.Assurance.Packs) != 0 || !slices.Equal(view.Constraints, []string{"Do not add a new runtime dependency."}) {
		t.Fatalf("work view = %#v", view)
	}
}

func TestExactPlanViewsAndBytesAreDefensiveCopies(t *testing.T) {
	t.Parallel()

	plan, err := ParseDeliveryPlan(standardPlanFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	record := plan.Record()
	record.CanonicalJSON[0] = '['
	if plan.Record().Digest != standardPlanDigest || plan.Record().CanonicalJSON[0] != '{' {
		t.Fatal("record mutation changed exact plan")
	}
	ids := plan.WorkIDs()
	ids[0] = "changed"
	if plan.WorkIDs()[0] != "health-endpoint" {
		t.Fatal("work id mutation changed exact plan")
	}
	authority := plan.Authority()
	authority.Grants[0] = PlanGrant{}
	canonicalGrant := authority.Grants[0].CanonicalJSON()
	if canonicalGrant != nil {
		t.Fatal("zero replacement grant retained canonical bytes")
	}
	freshAuthority := plan.Authority()
	freshIntegration, ok := freshAuthority.Grants[4].Integration()
	if freshAuthority.Grants[0].Action() != "inspect" || !ok || freshIntegration.Repository != "local:example" {
		t.Fatal("authority view mutation changed exact plan")
	}
	canonicalGrant = freshAuthority.Grants[0].CanonicalJSON()
	canonicalGrant[0] = '['
	if fresh := plan.Authority().Grants[0].CanonicalJSON(); fresh[0] != '{' {
		t.Fatal("canonical grant mutation changed exact plan")
	}
	contract, _ := plan.Work("health-endpoint")
	view := contract.View()
	view.Scope.Include[0] = "changed"
	view.Acceptance[0].Criterion = "changed"
	view.Assurance.Packs = append(view.Assurance.Packs, "changed@1")
	view.Constraints[0] = "changed"
	fresh := contract.View()
	if fresh.Scope.Include[0] != "src" || strings.HasPrefix(fresh.Acceptance[0].Criterion, "changed") ||
		len(fresh.Assurance.Packs) != 0 || fresh.Constraints[0] != "Do not add a new runtime dependency." {
		t.Fatal("work view mutation changed exact plan")
	}
	zeroRecord := (ExactPlan{}).Record()
	if zeroRecord.Kind != "" || zeroRecord.Digest != "" || zeroRecord.CanonicalJSON != nil ||
		(ExactPlan{}).DeliveryID() != "" || len((ExactPlan{}).WorkIDs()) != 0 {
		t.Fatal("zero exact plan exposed facts")
	}
	if _, ok := (ExactPlan{}).Work("anything"); ok || (ExactWorkContract{}).Digest() != "" ||
		(ExactWorkContract{}).View().ID != "" {
		t.Fatal("zero exact contract exposed facts")
	}
}

func TestParseDeliveryPlanPreservesFullWorkFactsAndOrder(t *testing.T) {
	t.Parallel()

	contents := mutateStandardPlan(t, func(plan map[string]any) {
		first := planWork(plan, 0)
		first["assurance"] = map[string]any{
			"profile": "assured", "packs": []any{"security@1"}, "risk_tags": []any{"security"},
		}
		first["constraints"] = []any{"Keep compatibility.", "Retain audit data."}
		second := cloneJSONValue(t, first).(map[string]any)
		second["id"] = "follow-up"
		second["outcome"] = "Confirm the dependent behavior."
		second["acceptance"] = []any{map[string]any{
			"id": "AC2", "criterion": "The dependent behavior is confirmed.", "evidence_level": "live",
		}}
		second["depends_on"] = []any{"health-endpoint"}
		second["assurance"] = map[string]any{"profile": "standard", "packs": []any{}, "risk_tags": []any{"follow-up"}}
		second["constraints"] = []any{}
		plan["work"] = append(plan["work"].([]any), second)
	})
	plan, err := ParseDeliveryPlan(contents)
	if err != nil {
		t.Fatalf("ParseDeliveryPlan() error = %v", err)
	}
	if !slices.Equal(plan.WorkIDs(), []string{"health-endpoint", "follow-up"}) {
		t.Fatalf("work order = %#v", plan.WorkIDs())
	}
	first, _ := plan.Work("health-endpoint")
	firstView := first.View()
	if firstView.Assurance.Profile != "assured" ||
		!slices.Equal(firstView.Assurance.Packs, []string{"security@1"}) ||
		!slices.Equal(firstView.Assurance.RiskTags, []string{"security"}) ||
		!slices.Equal(firstView.Constraints, []string{"Keep compatibility.", "Retain audit data."}) {
		t.Fatalf("first work facts = %#v", firstView)
	}
	second, _ := plan.Work("follow-up")
	secondView := second.View()
	if !slices.Equal(secondView.DependsOn, []string{"health-endpoint"}) ||
		secondView.Acceptance[0].EvidenceLevel != "live" ||
		!slices.Equal(secondView.Assurance.RiskTags, []string{"follow-up"}) || secondView.Constraints == nil {
		t.Fatalf("second work facts = %#v", secondView)
	}
}

func TestDeliveryPlanDigestDomainsUseCompleteCanonicalObjects(t *testing.T) {
	t.Parallel()

	base, err := ParseDeliveryPlan(standardPlanFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	baseWork, _ := base.Work("health-endpoint")

	planOnly, err := ParseDeliveryPlan(mutateStandardPlan(t, func(plan map[string]any) {
		plan["outcome"] = "A different delivery outcome."
	}))
	if err != nil {
		t.Fatal(err)
	}
	planOnlyWork, _ := planOnly.Work("health-endpoint")
	if planOnly.Record().Digest == base.Record().Digest ||
		planOnly.Authority().Digest != base.Authority().Digest || planOnlyWork.Digest() != baseWork.Digest() {
		t.Fatal("plan-only fact escaped or contaminated its digest domain")
	}

	contractChange, err := ParseDeliveryPlan(mutateStandardPlan(t, func(plan map[string]any) {
		acceptance := planWork(plan, 0)["acceptance"].([]any)[0].(map[string]any)
		acceptance["criterion"] = "A different complete criterion."
	}))
	if err != nil {
		t.Fatal(err)
	}
	changedWork, _ := contractChange.Work("health-endpoint")
	if contractChange.Record().Digest == base.Record().Digest || changedWork.Digest() == baseWork.Digest() ||
		contractChange.Authority().Digest != base.Authority().Digest {
		t.Fatal("contract fact escaped or contaminated its digest domain")
	}

	authorityChange, err := ParseDeliveryPlan(mutateStandardPlan(t, func(plan map[string]any) {
		grants := plan["authority"].(map[string]any)["grants"].([]any)
		grants[0], grants[1] = grants[1], grants[0]
	}))
	if err != nil {
		t.Fatal(err)
	}
	authorityWork, _ := authorityChange.Work("health-endpoint")
	if authorityChange.Record().Digest == base.Record().Digest ||
		authorityChange.Authority().Digest == base.Authority().Digest || authorityWork.Digest() != baseWork.Digest() {
		t.Fatal("ordered authority fact escaped or contaminated its digest domain")
	}

	var decoded any
	if err := json.Unmarshal(standardPlanFixture(t), &decoded); err != nil {
		t.Fatal(err)
	}
	reordered, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := ParseDeliveryPlan(reordered)
	if err != nil {
		t.Fatal(err)
	}
	equivalentWork, _ := equivalent.Work("health-endpoint")
	if equivalent.Record().Digest != base.Record().Digest ||
		equivalent.Authority().Digest != base.Authority().Digest || equivalentWork.Digest() != baseWork.Digest() {
		t.Fatal("non-semantic JSON layout changed exact digests")
	}
}

func TestParseDeliveryPlanEnforcesSchemaAndGraphRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"unknown top property", func(plan map[string]any) {
			plan["Schema_Version"] = plan["schema_version"]
			delete(plan, "schema_version")
		}},
		{"unknown nested property", func(plan map[string]any) { plan["authority"].(map[string]any)["extra"] = true }},
		{"null optional constraints", func(plan map[string]any) { planWork(plan, 0)["constraints"] = nil }},
		{"null optional risk tags", func(plan map[string]any) { planWork(plan, 0)["assurance"].(map[string]any)["risk_tags"] = nil }},
		{"null required excludes", func(plan map[string]any) { planWork(plan, 0)["scope"].(map[string]any)["exclude"] = nil }},
		{"empty include", func(plan map[string]any) { planWork(plan, 0)["scope"].(map[string]any)["include"] = []any{} }},
		{"duplicate include", func(plan map[string]any) {
			planWork(plan, 0)["scope"].(map[string]any)["include"] = []any{"src", "src"}
		}},
		{"newline path", func(plan map[string]any) {
			planWork(plan, 0)["scope"].(map[string]any)["include"] = []any{"src\nchild"}
		}},
		{"standard with pack", func(plan map[string]any) {
			planWork(plan, 0)["assurance"].(map[string]any)["packs"] = []any{"security@1"}
		}},
		{"assured without pack", func(plan map[string]any) { planWork(plan, 0)["assurance"].(map[string]any)["profile"] = "assured" }},
		{"duplicate grant", func(plan map[string]any) {
			authority := plan["authority"].(map[string]any)
			grants := authority["grants"].([]any)
			authority["grants"] = append(grants, cloneJSONValue(t, grants[0]))
		}},
		{"invalid workspace grant target", func(plan map[string]any) {
			plan["authority"].(map[string]any)["grants"].([]any)[0].(map[string]any)["target"] = "other"
		}},
		{"integration target drift", func(plan map[string]any) {
			grants := plan["authority"].(map[string]any)["grants"].([]any)
			grants[len(grants)-1].(map[string]any)["target"].(map[string]any)["ref"] = "refs/heads/other"
		}},
		{"duplicate work", func(plan map[string]any) {
			plan["work"] = append(plan["work"].([]any), cloneJSONValue(t, planWork(plan, 0)))
		}},
		{"duplicate acceptance across work", func(plan map[string]any) {
			second := cloneJSONValue(t, planWork(plan, 0)).(map[string]any)
			second["id"] = "second"
			plan["work"] = append(plan["work"].([]any), second)
		}},
		{"unknown dependency", func(plan map[string]any) { planWork(plan, 0)["depends_on"] = []any{"missing"} }},
		{"dependency cycle", func(plan map[string]any) { planWork(plan, 0)["depends_on"] = []any{"health-endpoint"} }},
		{"leap second", func(plan map[string]any) { plan["created_at"] = "1990-12-31T23:59:60Z" }},
		{"whitespace constraint", func(plan map[string]any) { planWork(plan, 0)["constraints"] = []any{" \t "} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if plan, err := ParseDeliveryPlan(mutateStandardPlan(t, test.mutate)); err == nil {
				t.Fatalf("ParseDeliveryPlan() = %#v, want error", plan)
			}
		})
	}
}

func TestParseDeliveryPlanAcceptsSchemaValidBoundaryCases(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"lowercase date-time", func(plan map[string]any) { plan["created_at"] = "2026-07-19t00:00:00z" }},
		{"same include and exclude", func(plan map[string]any) {
			scope := planWork(plan, 0)["scope"].(map[string]any)
			scope["include"] = []any{"."}
			scope["exclude"] = []any{"."}
		}},
		{"no integration grant", func(plan map[string]any) {
			authority := plan["authority"].(map[string]any)
			grants := authority["grants"].([]any)
			authority["grants"] = grants[:len(grants)-1]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseDeliveryPlan(mutateStandardPlan(t, test.mutate)); err != nil {
				t.Fatalf("ParseDeliveryPlan() error = %v", err)
			}
		})
	}
}

func TestParseDeliveryPlanRejectsStrictJSONAndByteLimitViolations(t *testing.T) {
	t.Parallel()

	duplicateName := bytes.Replace(
		standardPlanFixture(t),
		[]byte(`"schema_version": "delivery-plan-v1",`),
		[]byte(`"schema_version": "delivery-plan-v1", "schema_version": "delivery-plan-v1",`),
		1,
	)
	if _, err := ParseDeliveryPlan(duplicateName); err == nil || !strings.Contains(err.Error(), "duplicate object name") {
		t.Fatalf("duplicate-name error = %v", err)
	}
	tooLarge := make([]byte, MaximumDeliveryPlanBytes+1)
	if _, err := ParseDeliveryPlan(tooLarge); err == nil || !strings.Contains(err.Error(), "byte ceiling") {
		t.Fatalf("byte-ceiling error = %v", err)
	}
}

func standardPlanFixture(t testing.TB) []byte {
	t.Helper()
	snapshot, err := SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/standard-plan.json")
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func mutateStandardPlan(t testing.TB, mutate func(map[string]any)) []byte {
	t.Helper()
	var plan map[string]any
	if err := json.Unmarshal(standardPlanFixture(t), &plan); err != nil {
		t.Fatal(err)
	}
	mutate(plan)
	contents, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func planWork(plan map[string]any, index int) map[string]any {
	return plan["work"].([]any)[index].(map[string]any)
}

func cloneJSONValue(t testing.TB, value any) any {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned any
	if err := json.Unmarshal(contents, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
