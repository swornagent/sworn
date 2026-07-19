package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/swornagent/sworn/internal/repo"
)

const (
	DeliveryPlanSchemaVersion = "delivery-plan-v1"
	MaximumDeliveryPlanBytes  = 1 << 20
)

// ExactPlan is an immutable delivery-plan capability minted only after the
// complete Baton plan has passed strict JSON, schema, and graph validation.
// Its canonical record and derived digests always come from the original
// complete objects, never from a reduced projection.
type ExactPlan struct {
	data *exactPlanData
}

// ExactWorkContract is an immutable work fragment tied to its exact parent
// plan. It proves structure, not approval, policy resolution, dependency
// readiness, or authority.
type ExactWorkContract struct {
	plan  *exactPlanData
	index int
}

type PlanTarget struct {
	Repository string
	Ref        string
}

type PlanPolicy struct {
	Ref    string
	Digest string
}

// PlanGrant is an immutable validated grant extracted from an ExactPlan.
type PlanGrant struct {
	action         string
	integration    PlanTarget
	hasIntegration bool
	canonical      []byte
}

func (grant PlanGrant) Action() string { return grant.action }

// Integration returns the target and true for an integrate grant. Workspace
// grants return the zero target and false.
func (grant PlanGrant) Integration() (PlanTarget, bool) {
	return grant.integration, grant.hasIntegration
}

// CanonicalJSON returns the validated grant's exact RFC 8785 bytes. A grant
// constructed by a caller rather than extracted from an ExactPlan has no such
// binding and returns nil.
func (grant PlanGrant) CanonicalJSON() []byte {
	return append([]byte(nil), grant.canonical...)
}

type PlanAcceptance struct {
	ID            string
	Criterion     string
	EvidenceLevel string
}

type PlanAssurance struct {
	Profile  string
	Packs    []string
	RiskTags []string
}

type PlanWorkView struct {
	ID          string
	Outcome     string
	Scope       repo.Scope
	Acceptance  []PlanAcceptance
	DependsOn   []string
	Assurance   PlanAssurance
	Constraints []string
}

type PlanAuthorityView struct {
	SourceRef string
	Digest    string
	Grants    []PlanGrant
}

type exactPlanData struct {
	record     EncodedRecord
	deliveryID string
	outcome    string
	createdAt  string
	policy     PlanPolicy
	target     PlanTarget
	authority  exactPlanAuthority
	work       []exactPlanWork
	workIndex  map[string]int
}

type exactPlanAuthority struct {
	sourceRef string
	digest    string
	grants    []PlanGrant
}

type exactPlanWork struct {
	view   PlanWorkView
	digest string
}

type rawPlanObject map[string]json.RawMessage

// ParseDeliveryPlan validates a complete Baton delivery-plan-v1 record and
// returns an opaque capability bound to its RFC 8785 canonical bytes.
func ParseDeliveryPlan(contents []byte) (ExactPlan, error) {
	if len(contents) > MaximumDeliveryPlanBytes {
		return ExactPlan{}, errors.New("delivery plan exceeds byte ceiling")
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		return ExactPlan{}, fmt.Errorf("delivery plan is not strict I-JSON: %w", err)
	}
	if len(canonical) > MaximumDeliveryPlanBytes {
		return ExactPlan{}, errors.New("canonical delivery plan exceeds byte ceiling")
	}
	root, err := decodePlanObject(canonical, "delivery plan", []string{
		"schema_version", "delivery_id", "outcome", "created_at", "assurance_policy",
		"target", "authority", "work",
	}, nil)
	if err != nil {
		return ExactPlan{}, err
	}

	schemaVersion, err := decodePlanString(root["schema_version"], "delivery plan schema_version")
	if err != nil {
		return ExactPlan{}, err
	}
	if schemaVersion != DeliveryPlanSchemaVersion {
		return ExactPlan{}, fmt.Errorf("unknown delivery plan schema %q", schemaVersion)
	}
	deliveryID, err := decodePlanString(root["delivery_id"], "delivery plan delivery_id")
	if err != nil || !ValidID(deliveryID) {
		return ExactPlan{}, errors.New("delivery plan has an invalid delivery_id")
	}
	outcome, err := decodePlanString(root["outcome"], "delivery plan outcome")
	if err != nil || !nonEmpty(outcome) {
		return ExactPlan{}, errors.New("delivery plan requires a non-empty outcome")
	}
	createdAt, err := decodePlanString(root["created_at"], "delivery plan created_at")
	if err != nil {
		return ExactPlan{}, err
	}
	if _, err := parseRecordTime(createdAt, "delivery plan creation"); err != nil {
		return ExactPlan{}, err
	}

	policy, err := parsePlanPolicy(root["assurance_policy"])
	if err != nil {
		return ExactPlan{}, err
	}
	target, err := parsePlanTarget(root["target"], "delivery plan target")
	if err != nil {
		return ExactPlan{}, err
	}
	authority, err := parsePlanAuthority(root["authority"], target)
	if err != nil {
		return ExactPlan{}, err
	}
	workRaw, err := decodePlanArray(root["work"], "delivery plan work")
	if err != nil {
		return ExactPlan{}, err
	}
	if len(workRaw) == 0 {
		return ExactPlan{}, errors.New("delivery plan requires at least one work contract")
	}

	work := make([]exactPlanWork, 0, len(workRaw))
	workIndex := make(map[string]int, len(workRaw))
	acceptanceIDs := make(map[string]struct{})
	for index, raw := range workRaw {
		parsed, err := parsePlanWork(raw, index)
		if err != nil {
			return ExactPlan{}, err
		}
		if _, exists := workIndex[parsed.view.ID]; exists {
			return ExactPlan{}, fmt.Errorf("delivery plan contains duplicate work id %q", parsed.view.ID)
		}
		workIndex[parsed.view.ID] = index
		for _, acceptance := range parsed.view.Acceptance {
			if _, exists := acceptanceIDs[acceptance.ID]; exists {
				return ExactPlan{}, fmt.Errorf("delivery plan contains duplicate acceptance id %q", acceptance.ID)
			}
			acceptanceIDs[acceptance.ID] = struct{}{}
		}
		work = append(work, parsed)
	}
	if err := validatePlanDependencies(work, workIndex); err != nil {
		return ExactPlan{}, err
	}

	record := EncodedRecord{
		Kind:          DeliveryPlanSchemaVersion,
		CanonicalJSON: append([]byte(nil), canonical...),
		Digest:        CanonicalDigest(canonical),
	}
	data := &exactPlanData{
		record: record, deliveryID: deliveryID, outcome: outcome, createdAt: createdAt,
		policy: policy, target: target, authority: authority, work: work, workIndex: workIndex,
	}
	return ExactPlan{data: data}, nil
}

func parsePlanPolicy(raw json.RawMessage) (PlanPolicy, error) {
	object, err := decodePlanObject(raw, "delivery plan assurance_policy", []string{"ref", "digest"}, nil)
	if err != nil {
		return PlanPolicy{}, err
	}
	ref, err := decodePlanString(object["ref"], "delivery plan assurance_policy ref")
	if err != nil || !nonEmpty(ref) {
		return PlanPolicy{}, errors.New("delivery plan has an invalid assurance policy ref")
	}
	digest, err := decodePlanString(object["digest"], "delivery plan assurance_policy digest")
	if err != nil || !digestPattern.MatchString(digest) {
		return PlanPolicy{}, errors.New("delivery plan has an invalid assurance policy digest")
	}
	return PlanPolicy{Ref: ref, Digest: digest}, nil
}

func parsePlanTarget(raw json.RawMessage, label string) (PlanTarget, error) {
	object, err := decodePlanObject(raw, label, []string{"repository", "ref"}, nil)
	if err != nil {
		return PlanTarget{}, err
	}
	repository, err := decodePlanString(object["repository"], label+" repository")
	if err != nil || !nonEmpty(repository) {
		return PlanTarget{}, fmt.Errorf("%s has an invalid repository", label)
	}
	ref, err := decodePlanString(object["ref"], label+" ref")
	if err != nil || !validBranchRef(ref) {
		return PlanTarget{}, fmt.Errorf("%s has an invalid branch ref", label)
	}
	return PlanTarget{Repository: repository, Ref: ref}, nil
}

func parsePlanAuthority(raw json.RawMessage, target PlanTarget) (exactPlanAuthority, error) {
	object, err := decodePlanObject(raw, "delivery plan authority", []string{"ref", "grants"}, nil)
	if err != nil {
		return exactPlanAuthority{}, err
	}
	ref, err := decodePlanString(object["ref"], "delivery plan authority ref")
	if err != nil || !nonEmpty(ref) {
		return exactPlanAuthority{}, errors.New("delivery plan has an invalid authority ref")
	}
	grantRaw, err := decodePlanArray(object["grants"], "delivery plan authority grants")
	if err != nil {
		return exactPlanAuthority{}, err
	}
	if len(grantRaw) == 0 {
		return exactPlanAuthority{}, errors.New("delivery plan authority requires at least one grant")
	}
	grants := make([]PlanGrant, 0, len(grantRaw))
	seen := make(map[string]struct{}, len(grantRaw))
	for index, rawGrant := range grantRaw {
		parsed, err := ParseAuthorityGrant(rawGrant)
		if err != nil {
			return exactPlanAuthority{}, fmt.Errorf("delivery plan authority grant %d: %w", index, err)
		}
		canonicalGrant := parsed.CanonicalJSON()
		if _, exists := seen[string(canonicalGrant)]; exists {
			return exactPlanAuthority{}, errors.New("delivery plan authority contains duplicate grants")
		}
		seen[string(canonicalGrant)] = struct{}{}
		integration, hasIntegration := parsed.Integration()
		if hasIntegration && integration != target {
			return exactPlanAuthority{}, errors.New("delivery plan integration grant does not match its target")
		}
		grants = append(grants, PlanGrant{
			action: parsed.Action(), integration: integration,
			hasIntegration: hasIntegration, canonical: canonicalGrant,
		})
	}
	canonicalAuthority, err := CanonicalizeJSON(raw)
	if err != nil {
		return exactPlanAuthority{}, fmt.Errorf("canonicalize delivery plan authority: %w", err)
	}
	return exactPlanAuthority{
		sourceRef: ref,
		digest:    CanonicalDigest(canonicalAuthority),
		grants:    grants,
	}, nil
}

func parsePlanWork(raw json.RawMessage, index int) (exactPlanWork, error) {
	label := fmt.Sprintf("delivery plan work %d", index)
	object, err := decodePlanObject(raw, label, []string{
		"id", "outcome", "scope", "acceptance", "depends_on", "assurance",
	}, []string{"constraints"})
	if err != nil {
		return exactPlanWork{}, err
	}
	id, err := decodePlanString(object["id"], label+" id")
	if err != nil || !ValidID(id) {
		return exactPlanWork{}, fmt.Errorf("%s has an invalid id", label)
	}
	outcome, err := decodePlanString(object["outcome"], label+" outcome")
	if err != nil || !nonEmpty(outcome) {
		return exactPlanWork{}, fmt.Errorf("%s requires a non-empty outcome", label)
	}
	scope, err := parsePlanScope(object["scope"], label+" scope")
	if err != nil {
		return exactPlanWork{}, err
	}
	acceptance, err := parsePlanAcceptance(object["acceptance"], label+" acceptance")
	if err != nil {
		return exactPlanWork{}, err
	}
	dependsOn, err := parsePlanStringSet(object["depends_on"], label+" depends_on", 0, ValidID)
	if err != nil {
		return exactPlanWork{}, err
	}
	assurance, err := parsePlanAssurance(object["assurance"], label+" assurance")
	if err != nil {
		return exactPlanWork{}, err
	}
	var constraints []string
	if rawConstraints, exists := object["constraints"]; exists {
		constraints, err = parsePlanStringSet(rawConstraints, label+" constraints", 0, nonEmpty)
		if err != nil {
			return exactPlanWork{}, err
		}
	}
	canonical, err := CanonicalizeJSON(raw)
	if err != nil {
		return exactPlanWork{}, fmt.Errorf("canonicalize %s: %w", label, err)
	}
	return exactPlanWork{
		view: PlanWorkView{
			ID: id, Outcome: outcome, Scope: scope, Acceptance: acceptance,
			DependsOn: dependsOn, Assurance: assurance, Constraints: constraints,
		},
		digest: CanonicalDigest(canonical),
	}, nil
}

func parsePlanScope(raw json.RawMessage, label string) (repo.Scope, error) {
	object, err := decodePlanObject(raw, label, []string{"include", "exclude"}, nil)
	if err != nil {
		return repo.Scope{}, err
	}
	include, err := parsePlanStringSet(object["include"], label+" include", 1, func(string) bool { return true })
	if err != nil {
		return repo.Scope{}, err
	}
	exclude, err := parsePlanStringSet(object["exclude"], label+" exclude", 0, func(string) bool { return true })
	if err != nil {
		return repo.Scope{}, err
	}
	scope := repo.Scope{Include: include, Exclude: exclude}
	if err := scope.Validate(); err != nil {
		return repo.Scope{}, fmt.Errorf("%s: %w", label, err)
	}
	return scope, nil
}

func parsePlanAcceptance(raw json.RawMessage, label string) ([]PlanAcceptance, error) {
	items, err := decodePlanArray(raw, label)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%s requires at least one item", label)
	}
	result := make([]PlanAcceptance, 0, len(items))
	for index, item := range items {
		itemLabel := fmt.Sprintf("%s %d", label, index)
		object, err := decodePlanObject(item, itemLabel, []string{"id", "criterion", "evidence_level"}, nil)
		if err != nil {
			return nil, err
		}
		id, err := decodePlanString(object["id"], itemLabel+" id")
		if err != nil || !ValidID(id) {
			return nil, fmt.Errorf("%s has an invalid id", itemLabel)
		}
		criterion, err := decodePlanString(object["criterion"], itemLabel+" criterion")
		if err != nil || !nonEmpty(criterion) {
			return nil, fmt.Errorf("%s requires a non-empty criterion", itemLabel)
		}
		level, err := decodePlanString(object["evidence_level"], itemLabel+" evidence_level")
		if err != nil || (level != "component" && level != "assembled" && level != "live") {
			return nil, fmt.Errorf("%s has an invalid evidence level", itemLabel)
		}
		result = append(result, PlanAcceptance{ID: id, Criterion: criterion, EvidenceLevel: level})
	}
	return result, nil
}

func parsePlanAssurance(raw json.RawMessage, label string) (PlanAssurance, error) {
	object, err := decodePlanObject(raw, label, []string{"profile", "packs"}, []string{"risk_tags"})
	if err != nil {
		return PlanAssurance{}, err
	}
	profile, err := decodePlanString(object["profile"], label+" profile")
	if err != nil || (profile != "standard" && profile != "assured") {
		return PlanAssurance{}, fmt.Errorf("%s has an invalid profile", label)
	}
	packs, err := parsePlanStringSet(object["packs"], label+" packs", 0, func(value string) bool {
		return packIDPattern.MatchString(value)
	})
	if err != nil {
		return PlanAssurance{}, err
	}
	if profile == "standard" && len(packs) != 0 {
		return PlanAssurance{}, fmt.Errorf("%s standard profile cannot select packs", label)
	}
	if profile == "assured" && len(packs) == 0 {
		return PlanAssurance{}, fmt.Errorf("%s assured profile requires a pack", label)
	}
	var riskTags []string
	if rawRiskTags, exists := object["risk_tags"]; exists {
		riskTags, err = parsePlanStringSet(rawRiskTags, label+" risk_tags", 0, ValidID)
		if err != nil {
			return PlanAssurance{}, err
		}
	}
	return PlanAssurance{Profile: profile, Packs: packs, RiskTags: riskTags}, nil
}

func validatePlanDependencies(work []exactPlanWork, workIndex map[string]int) error {
	indegree := make([]int, len(work))
	dependents := make([][]int, len(work))
	for index, contract := range work {
		indegree[index] = len(contract.view.DependsOn)
		for _, dependency := range contract.view.DependsOn {
			dependencyIndex, exists := workIndex[dependency]
			if !exists {
				return fmt.Errorf("work %q depends on unknown work %q", contract.view.ID, dependency)
			}
			dependents[dependencyIndex] = append(dependents[dependencyIndex], index)
		}
	}
	ready := make([]int, 0, len(work))
	for index, count := range indegree {
		if count == 0 {
			ready = append(ready, index)
		}
	}
	processed := 0
	for len(ready) > 0 {
		index := ready[0]
		ready = ready[1:]
		processed++
		for _, dependent := range dependents[index] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if processed != len(work) {
		return errors.New("delivery plan work dependency graph contains a cycle")
	}
	return nil
}

func parsePlanStringSet(
	raw json.RawMessage,
	label string,
	minimum int,
	valid func(string) bool,
) ([]string, error) {
	items, err := decodePlanArray(raw, label)
	if err != nil {
		return nil, err
	}
	if len(items) < minimum {
		return nil, fmt.Errorf("%s requires at least %d item(s)", label, minimum)
	}
	values := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		value, err := decodePlanString(item, fmt.Sprintf("%s %d", label, index))
		if err != nil || !valid(value) {
			return nil, fmt.Errorf("%s contains an invalid item", label)
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("%s contains duplicate items", label)
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values, nil
}

func decodePlanObject(
	raw []byte,
	label string,
	required []string,
	optional []string,
) (rawPlanObject, error) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, fmt.Errorf("%s must be an object", label)
	}
	var object rawPlanObject
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, name := range required {
		allowed[name] = struct{}{}
		if _, exists := object[name]; !exists {
			return nil, fmt.Errorf("%s is missing required property %q", label, name)
		}
	}
	for _, name := range optional {
		allowed[name] = struct{}{}
	}
	unknown := make([]string, 0)
	for name := range object {
		if _, exists := allowed[name]; !exists {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%s has unknown property %q", label, unknown[0])
	}
	return object, nil
}

func decodePlanArray(raw []byte, label string) ([]json.RawMessage, error) {
	if len(raw) == 0 || raw[0] != '[' {
		return nil, fmt.Errorf("%s must be an array", label)
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	return array, nil
}

func decodePlanString(raw []byte, label string) (string, error) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", fmt.Errorf("%s must be a string", label)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("decode %s: %w", label, err)
	}
	return value, nil
}

func (plan ExactPlan) Record() EncodedRecord {
	if plan.data == nil {
		return EncodedRecord{}
	}
	record := plan.data.record
	record.CanonicalJSON = append([]byte(nil), record.CanonicalJSON...)
	return record
}

func (plan ExactPlan) DeliveryID() string {
	if plan.data == nil {
		return ""
	}
	return plan.data.deliveryID
}

func (plan ExactPlan) Outcome() string {
	if plan.data == nil {
		return ""
	}
	return plan.data.outcome
}

func (plan ExactPlan) CreatedAt() string {
	if plan.data == nil {
		return ""
	}
	return plan.data.createdAt
}

func (plan ExactPlan) Target() PlanTarget {
	if plan.data == nil {
		return PlanTarget{}
	}
	return plan.data.target
}

func (plan ExactPlan) Policy() PlanPolicy {
	if plan.data == nil {
		return PlanPolicy{}
	}
	return plan.data.policy
}

func (plan ExactPlan) Authority() PlanAuthorityView {
	if plan.data == nil {
		return PlanAuthorityView{}
	}
	view := PlanAuthorityView{
		SourceRef: plan.data.authority.sourceRef,
		Digest:    plan.data.authority.digest,
		Grants:    make([]PlanGrant, 0, len(plan.data.authority.grants)),
	}
	for _, grant := range plan.data.authority.grants {
		cloned := grant
		cloned.canonical = append([]byte(nil), grant.canonical...)
		view.Grants = append(view.Grants, cloned)
	}
	return view
}

func (plan ExactPlan) WorkIDs() []string {
	if plan.data == nil {
		return nil
	}
	ids := make([]string, len(plan.data.work))
	for index := range plan.data.work {
		ids[index] = plan.data.work[index].view.ID
	}
	return ids
}

func (plan ExactPlan) Work(id string) (ExactWorkContract, bool) {
	if plan.data == nil {
		return ExactWorkContract{}, false
	}
	index, exists := plan.data.workIndex[id]
	if !exists {
		return ExactWorkContract{}, false
	}
	return ExactWorkContract{plan: plan.data, index: index}, true
}

func (contract ExactWorkContract) Digest() string {
	if !contract.valid() {
		return ""
	}
	return contract.plan.work[contract.index].digest
}

func (contract ExactWorkContract) View() PlanWorkView {
	if !contract.valid() {
		return PlanWorkView{}
	}
	return clonePlanWorkView(contract.plan.work[contract.index].view)
}

func (contract ExactWorkContract) valid() bool {
	return contract.plan != nil && contract.index >= 0 && contract.index < len(contract.plan.work)
}

func clonePlanWorkView(view PlanWorkView) PlanWorkView {
	view.Scope.Include = slices.Clone(view.Scope.Include)
	view.Scope.Exclude = slices.Clone(view.Scope.Exclude)
	view.Acceptance = slices.Clone(view.Acceptance)
	view.DependsOn = slices.Clone(view.DependsOn)
	view.Assurance.Packs = slices.Clone(view.Assurance.Packs)
	view.Assurance.RiskTags = slices.Clone(view.Assurance.RiskTags)
	view.Constraints = slices.Clone(view.Constraints)
	return view
}
