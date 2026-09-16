package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/protocol"
)

const (
	LeadDelegationVersion        = "sworn.lead-delegation/v1"
	LeadPlanPolicyVersion        = "sworn.lead-plan-policy/v1"
	LeadPlanReviewResponsibility = "lead_plan_review"
	LeadDelegationActorClass     = "external_authorizer"
	MaxLeadDelegationBytes       = 64 * 1024
	MaxLeadDecisionRules         = 2
	MaxLeadFieldRules            = 256
	MaxLeadValuesPerField        = 32
	MaxLeadDeltaOperations       = 128
	MaxLeadPlanRevision          = 1024
	MaxLeadPlannerAttempts       = 16
	// The role-neutral driver retry contract durably supports three attempts.
	// Lead authority must not admit a larger limit than the real dispatch
	// boundary can spend and recover.
	MaxLeadAttemptsPerProposal = 3
	MaxLeadDecisions           = 256
	MaxLeadReplans             = 64
)

type LeadLineageAnchor struct {
	State        string `json:"state"`
	PlanOID      string `json:"plan_oid"`
	PlanRevision int64  `json:"plan_revision"`
	ReleaseHead  string `json:"release_head"`
}

type LeadDecisionRule struct {
	DecisionClass   string   `json:"decision_class"`
	AllowedOutcomes []string `json:"allowed_outcomes"`
}

type LeadDelegationLimits struct {
	MinimumPlanRevision               int64 `json:"minimum_plan_revision"`
	MaximumPlanRevision               int64 `json:"maximum_plan_revision"`
	MaximumPlannerAttemptsPerRevision int64 `json:"maximum_planner_attempts_per_revision"`
	MaximumLeadAttemptsPerProposal    int64 `json:"maximum_lead_attempts_per_proposal"`
	MaximumTotalLeadDecisions         int64 `json:"maximum_total_lead_decisions"`
	ReplanBudget                      int64 `json:"replan_budget"`
}

type LeadFieldRule struct {
	JSONPointer         string   `json:"json_pointer"`
	AllowedValueDigests []string `json:"allowed_value_digests"`
}

type LeadDeltaOperation struct {
	Operation   string `json:"operation"`
	JSONPointer string `json:"json_pointer"`
	FromDigest  string `json:"from_digest"`
	ToDigest    string `json:"to_digest"`
}

type LeadDeltaRules struct {
	MaximumOperations int64                `json:"maximum_operations"`
	AllowedOperations []LeadDeltaOperation `json:"allowed_operations"`
}

type LeadPlanPolicy struct {
	SchemaVersion           string          `json:"schema_version"`
	AuthorityClass          string          `json:"authority_class"`
	ProtocolChange          bool            `json:"protocol_change"`
	DestructiveOperations   bool            `json:"destructive_operations"`
	HighStakesAuthorization bool            `json:"high_stakes_authorization"`
	InitialShapeDigest      string          `json:"initial_shape_digest"`
	FieldRules              []LeadFieldRule `json:"field_rules"`
	DeltaRules              LeadDeltaRules  `json:"delta_rules"`
}

type LeadDelegation struct {
	SchemaVersion        string               `json:"schema_version"`
	RunID                string               `json:"run_id"`
	ManifestDigest       string               `json:"manifest_digest"`
	Project              string               `json:"project"`
	Release              string               `json:"release"`
	ReleaseRef           string               `json:"release_ref"`
	ReleaseLineageAnchor LeadLineageAnchor    `json:"release_lineage_anchor"`
	TargetRef            string               `json:"target_ref"`
	TargetHead           string               `json:"target_head"`
	DelegationEpoch      int64                `json:"delegation_epoch"`
	DelegateRole         string               `json:"delegate_role"`
	Responsibility       string               `json:"responsibility"`
	DecisionRules        []LeadDecisionRule   `json:"decision_rules"`
	Limits               LeadDelegationLimits `json:"limits"`
	PlanRules            LeadPlanPolicy       `json:"plan_rules"`
}

type AdmittedLeadDelegation struct {
	Envelope LeadDelegation
	Bytes    []byte
	Digest   string
}

func ParseLeadDelegation(body []byte) (AdmittedLeadDelegation, error) {
	if len(body) == 0 || len(body) > MaxLeadDelegationBytes || body[len(body)-1] != '\n' ||
		rejectDuplicateJSONKeys(body) != nil {
		return AdmittedLeadDelegation{}, runtimeFail("INVALID_LEAD_DELEGATION", nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var envelope LeadDelegation
	if decoder.Decode(&envelope) != nil || requireJSONEOF(decoder) != nil ||
		validateLeadDelegation(envelope) != nil {
		return AdmittedLeadDelegation{}, runtimeFail("INVALID_LEAD_DELEGATION", nil)
	}
	canonical, err := json.Marshal(envelope)
	canonical = append(canonical, '\n')
	if err != nil || !bytes.Equal(canonical, body) {
		return AdmittedLeadDelegation{}, runtimeFail("NONCANONICAL_LEAD_DELEGATION", nil)
	}
	return AdmittedLeadDelegation{
		Envelope: envelope, Bytes: append([]byte(nil), body...), Digest: sha256Digest(body),
	}, nil
}

func CanonicalLeadDelegation(envelope LeadDelegation) ([]byte, error) {
	if err := validateLeadDelegation(envelope); err != nil {
		return nil, err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, runtimeFail("INVALID_LEAD_DELEGATION", err)
	}
	return append(body, '\n'), nil
}

func validateLeadDelegation(value LeadDelegation) error {
	if value.SchemaVersion != LeadDelegationVersion ||
		!runtimeIdentityPattern.MatchString(value.RunID) ||
		!runtimeDigestPattern.MatchString(value.ManifestDigest) ||
		!boundedLeadText(value.Project, 128) || !boundedLeadText(value.Release, 128) ||
		value.ReleaseRef != "refs/heads/release-wt/"+value.Release ||
		!strings.HasPrefix(value.TargetRef, "refs/heads/") || !validGitObjectID(value.TargetHead) ||
		value.DelegationEpoch < 1 || value.DelegateRole != "lead" ||
		value.Responsibility != LeadPlanReviewResponsibility {
		return runtimeFail("INVALID_LEAD_DELEGATION", nil)
	}
	a := value.ReleaseLineageAnchor
	if (a.State == "absent" && (a.PlanOID != "" || a.PlanRevision != 0 || a.ReleaseHead != "")) ||
		(a.State == "present" && (!validGitObjectID(a.PlanOID) || a.PlanRevision < 1 || !validGitObjectID(a.ReleaseHead))) ||
		(a.State != "absent" && a.State != "present") {
		return runtimeFail("INVALID_LEAD_DELEGATION", nil)
	}
	if len(value.DecisionRules) < 1 || len(value.DecisionRules) > MaxLeadDecisionRules {
		return runtimeFail("INVALID_LEAD_DELEGATION", nil)
	}
	previousClass := ""
	for _, rule := range value.DecisionRules {
		if (rule.DecisionClass != PlannerProposalClass && rule.DecisionClass != PlannerReplanClass) ||
			rule.DecisionClass <= previousClass || len(rule.AllowedOutcomes) == 0 || len(rule.AllowedOutcomes) > 3 ||
			!sortedUniqueLeadOutcomes(rule.AllowedOutcomes) {
			return runtimeFail("INVALID_LEAD_DELEGATION", nil)
		}
		previousClass = rule.DecisionClass
	}
	l := value.Limits
	if l.MinimumPlanRevision < 1 || l.MaximumPlanRevision < l.MinimumPlanRevision || l.MaximumPlanRevision > MaxLeadPlanRevision ||
		l.MaximumPlannerAttemptsPerRevision < 1 || l.MaximumPlannerAttemptsPerRevision > MaxLeadPlannerAttempts ||
		l.MaximumLeadAttemptsPerProposal < 1 || l.MaximumLeadAttemptsPerProposal > MaxLeadAttemptsPerProposal ||
		l.MaximumTotalLeadDecisions < 1 || l.MaximumTotalLeadDecisions > MaxLeadDecisions ||
		l.ReplanBudget < 0 || l.ReplanBudget > MaxLeadReplans || validateLeadPlanPolicy(value.PlanRules) != nil {
		return runtimeFail("INVALID_LEAD_DELEGATION", nil)
	}
	return nil
}

func validateLeadPlanPolicy(policy LeadPlanPolicy) error {
	if policy.SchemaVersion != LeadPlanPolicyVersion || policy.AuthorityClass != "ordinary_delivery" ||
		policy.ProtocolChange || policy.DestructiveOperations || policy.HighStakesAuthorization ||
		!runtimeDigestPattern.MatchString(policy.InitialShapeDigest) || len(policy.FieldRules) > MaxLeadFieldRules ||
		policy.DeltaRules.MaximumOperations < 0 || policy.DeltaRules.MaximumOperations > MaxLeadDeltaOperations ||
		len(policy.DeltaRules.AllowedOperations) > MaxLeadDeltaOperations ||
		int64(len(policy.DeltaRules.AllowedOperations)) > policy.DeltaRules.MaximumOperations {
		return runtimeFail("INVALID_LEAD_PLAN_POLICY", nil)
	}
	previous := ""
	for _, rule := range policy.FieldRules {
		if !validExactJSONPointer(rule.JSONPointer) || rule.JSONPointer <= previous || len(rule.AllowedValueDigests) == 0 ||
			len(rule.AllowedValueDigests) > MaxLeadValuesPerField || !sortedUniqueDigests(rule.AllowedValueDigests) {
			return runtimeFail("INVALID_LEAD_PLAN_POLICY", nil)
		}
		previous = rule.JSONPointer
	}
	previous = ""
	for _, operation := range policy.DeltaRules.AllowedOperations {
		key := operation.Operation + "\x00" + operation.JSONPointer + "\x00" + operation.FromDigest + "\x00" + operation.ToDigest
		if operation.Operation != "replace" || !validExactJSONPointer(operation.JSONPointer) ||
			!runtimeDigestPattern.MatchString(operation.FromDigest) || !runtimeDigestPattern.MatchString(operation.ToDigest) ||
			operation.FromDigest == operation.ToDigest || key <= previous {
			return runtimeFail("INVALID_LEAD_PLAN_POLICY", nil)
		}
		previous = key
	}
	return nil
}

func boundedLeadText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func sortedUniqueLeadOutcomes(values []string) bool {
	previous := ""
	for _, value := range values {
		if (value != "escalate" && value != "proceed" && value != "revise") || value <= previous {
			return false
		}
		previous = value
	}
	return true
}

func sortedUniqueDigests(values []string) bool {
	previous := ""
	for _, value := range values {
		if !runtimeDigestPattern.MatchString(value) || value <= previous {
			return false
		}
		previous = value
	}
	return true
}

func validExactJSONPointer(value string) bool {
	if value == "" || len(value) > 512 || value[0] != '/' || strings.ContainsAny(value, "*\\[](){}?+") {
		return false
	}
	for _, part := range strings.Split(value[1:], "/") {
		if part == "" || strings.Contains(part, "~") {
			return false
		}
	}
	return true
}

type leadPlanShape struct {
	SchemaVersion string           `json:"schema_version"`
	Repository    string           `json:"repository"`
	TargetRef     string           `json:"target_ref"`
	Tracks        []leadTrackShape `json:"tracks"`
	ContractKeys  []string         `json:"contract_keys"`
}
type leadTrackShape struct {
	ID        string           `json:"id"`
	DependsOn []string         `json:"depends_on"`
	Slices    []leadSliceShape `json:"slices"`
}
type leadSliceShape struct {
	ID              string   `json:"id"`
	DependsOn       []string `json:"depends_on"`
	Consumes        []string `json:"consumes"`
	AcceptanceIDs   []string `json:"acceptance_ids"`
	IncludeCount    int      `json:"include_count"`
	ExcludeCount    int      `json:"exclude_count"`
	CheckCount      int      `json:"check_count"`
	ConstraintCount int      `json:"constraint_count"`
}

func LeadPlanStructuralProjection(plan protocol.Plan) ([]byte, string, error) {
	metadata := plan.Metadata()
	if !protocol.IsAdmittedPlanVersion(metadata.SchemaVersion) || len(metadata.Tracks) > 64 {
		return nil, "", runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
	}
	shape := leadPlanShape{SchemaVersion: "sworn.lead-plan-structure/v1", Repository: metadata.Repository, TargetRef: metadata.TargetRef, Tracks: make([]leadTrackShape, len(metadata.Tracks)), ContractKeys: make([]string, 0, len(metadata.Contracts))}
	for key := range metadata.Contracts {
		shape.ContractKeys = append(shape.ContractKeys, key)
	}
	sort.Strings(shape.ContractKeys)
	for i, track := range metadata.Tracks {
		if len(track.Slices) > 128 || len(track.DependsOn) > 64 {
			return nil, "", runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
		}
		shape.Tracks[i] = leadTrackShape{ID: track.ID, DependsOn: append([]string(nil), track.DependsOn...), Slices: make([]leadSliceShape, len(track.Slices))}
		for j, slice := range track.Slices {
			if len(slice.Acceptance) > 128 || len(slice.Scope.Include) > 128 || len(slice.Scope.Exclude) > 128 || len(slice.Checks) > 128 || len(slice.Constraints) > 128 || len(slice.DependsOn) > 128 || len(slice.Consumes) > 128 {
				return nil, "", runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
			}
			ids := make([]string, len(slice.Acceptance))
			for k, criterion := range slice.Acceptance {
				ids[k] = criterion.ID
			}
			shape.Tracks[i].Slices[j] = leadSliceShape{ID: slice.ID, DependsOn: append([]string(nil), slice.DependsOn...), Consumes: append([]string(nil), slice.Consumes...), AcceptanceIDs: ids, IncludeCount: len(slice.Scope.Include), ExcludeCount: len(slice.Scope.Exclude), CheckCount: len(slice.Checks), ConstraintCount: len(slice.Constraints)}
		}
	}
	body, err := json.Marshal(shape)
	if err != nil {
		return nil, "", runtimeFail("LEAD_PLAN_POLICY_REFUSED", err)
	}
	return body, sha256Digest(body), nil
}

func leadPlanLeaves(plan protocol.Plan) (map[string]string, error) {
	metadata := plan.Metadata()
	metadata.Revision = 0
	metadata.PreviousPlan = nil
	metadata.ApprovalRef = ""
	body, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decodeErr := decoder.Decode(&value); decodeErr != nil {
		return nil, decodeErr
	}
	result := make(map[string]string)
	var walk func(string, any) error
	walk = func(pointer string, current any) error {
		switch typed := current.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if err := walk(pointer+"/"+key, typed[key]); err != nil {
					return err
				}
			}
		case []any:
			for index, child := range typed {
				if err := walk(fmt.Sprintf("%s/%d", pointer, index), child); err != nil {
					return err
				}
			}
		default:
			canonical, marshalErr := json.Marshal(typed)
			if marshalErr != nil {
				return marshalErr
			}
			result[pointer] = sha256Digest(canonical)
		}
		return nil
	}
	if err := walk("", value); err != nil {
		return nil, err
	}
	delete(result, "/revision")
	delete(result, "/previous_plan")
	delete(result, "/approval_ref")
	return result, nil
}

// LeadPlanValueProjection returns the exact finite leaf bindings an
// external authorizer may choose from when constructing a closed plan policy.
// The returned map is a copy and excludes proposal-specific lineage fields.
func LeadPlanValueProjection(plan protocol.Plan) (map[string]string, error) {
	values, err := leadPlanLeaves(plan)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(values))
	for pointer, digest := range values {
		result[pointer] = digest
	}
	return result, nil
}

func ValidateLeadPlanPolicy(policy LeadPlanPolicy, proposal protocol.Plan, prior *protocol.Plan) error {
	if validateLeadPlanPolicy(policy) != nil {
		return runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
	}
	_, shapeDigest, err := LeadPlanStructuralProjection(proposal)
	if err != nil || shapeDigest != policy.InitialShapeDigest {
		return runtimeFail("LEAD_PLAN_POLICY_REFUSED", err)
	}
	leaves, err := leadPlanLeaves(proposal)
	if err != nil {
		return runtimeFail("LEAD_PLAN_POLICY_REFUSED", err)
	}
	rules := make(map[string]map[string]bool, len(policy.FieldRules))
	for _, rule := range policy.FieldRules {
		allowed := make(map[string]bool, len(rule.AllowedValueDigests))
		for _, digest := range rule.AllowedValueDigests {
			allowed[digest] = true
		}
		rules[rule.JSONPointer] = allowed
	}
	if len(leaves) == 0 || len(rules) == 0 {
		return runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
	}
	for pointer, digest := range leaves {
		if !rules[pointer][digest] {
			return runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
		}
	}
	if prior == nil {
		return nil
	}
	before, err := leadPlanLeaves(*prior)
	if err != nil {
		return runtimeFail("LEAD_PLAN_POLICY_REFUSED", err)
	}
	changes := make([]LeadDeltaOperation, 0)
	for pointer, from := range before {
		if to, ok := leaves[pointer]; !ok || to != from {
			changes = append(changes, LeadDeltaOperation{Operation: "replace", JSONPointer: pointer, FromDigest: from, ToDigest: to})
		}
	}
	for pointer := range leaves {
		if _, ok := before[pointer]; !ok {
			return runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		return a.Operation+"\x00"+a.JSONPointer+"\x00"+a.FromDigest+"\x00"+a.ToDigest < b.Operation+"\x00"+b.JSONPointer+"\x00"+b.FromDigest+"\x00"+b.ToDigest
	})
	if int64(len(changes)) > policy.DeltaRules.MaximumOperations {
		return runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
	}
	allowed := make(map[string]bool, len(policy.DeltaRules.AllowedOperations))
	for _, op := range policy.DeltaRules.AllowedOperations {
		allowed[op.Operation+"\x00"+op.JSONPointer+"\x00"+op.FromDigest+"\x00"+op.ToDigest] = true
	}
	for _, op := range changes {
		if !allowed[op.Operation+"\x00"+op.JSONPointer+"\x00"+op.FromDigest+"\x00"+op.ToDigest] {
			return runtimeFail("LEAD_PLAN_POLICY_REFUSED", nil)
		}
	}
	return nil
}
