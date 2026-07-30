package baton

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	ReceiptVersion = int64(1)
	ReceiptTrailer = "Baton-Receipt: "
	DetailBegin    = "Baton-Detail-Begin"
	DetailEnd      = "Baton-Detail-End"
)

var resultsByRole = map[string]map[string]bool{
	"planner":     {"approved": true, "retired": true},
	"implementer": {"designed": true, "candidate": true},
	"captain":     {"proceed": true, "revise": true, "escalate": true},
	"verifier":    {"pass": true, "fail": true, "blocked": true},
	"merge":       {"merged": true},
}

// Receipt is the canonical receipt-v1 payload carried in a commit trailer.
// Optional scalar pointers preserve field presence, which is part of the
// protocol shape.
type Receipt struct {
	Version      int64
	Release      string
	Slice        *string
	Role         string
	Result       string
	Attempt      *int64
	Plan         string
	Contract     *string
	Binds        string
	Detail       string
	Summary      string
	Target       *string
	Base         *string
	Candidate    *string
	ProductTree  *string
	Inputs       map[string]string
	Checks       *string
	ResultCommit *string
}

func (r Receipt) Clone() Receipt {
	result := r
	for source, target := range map[*string]**string{
		r.Slice: &result.Slice, r.Contract: &result.Contract, r.Target: &result.Target,
		r.Base: &result.Base, r.Candidate: &result.Candidate,
		r.ProductTree: &result.ProductTree, r.Checks: &result.Checks,
		r.ResultCommit: &result.ResultCommit,
	} {
		if source != nil {
			value := *source
			*target = &value
		}
	}
	if r.Attempt != nil {
		value := *r.Attempt
		result.Attempt = &value
	}
	if r.Inputs != nil {
		result.Inputs = make(map[string]string, len(r.Inputs))
		for key, value := range r.Inputs {
			result.Inputs[key] = value
		}
	}
	return result
}

func (r Receipt) SliceID() string {
	if r.Slice == nil {
		return ""
	}
	return *r.Slice
}

func (r Receipt) CanonicalBytes() ([]byte, error) {
	validated, err := validateReceiptMap(r.toMap())
	if err != nil {
		return nil, err
	}
	return canonicalJSON(validated.toMap())
}

func ParseReceipt(raw []byte) (Receipt, error) {
	if len(raw) > MaxReceiptBytes {
		return Receipt{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("receipt exceeds %d bytes", MaxReceiptBytes))
	}
	if bytes.ContainsAny(raw, "\r\n") {
		return Receipt{}, recordFail("INVALID_RECEIPT", "receipt must be canonical one-line JSON")
	}
	value, err := strictParseJSON(raw, "receipt", MaxReceiptBytes)
	if err != nil {
		return Receipt{}, err
	}
	object, err := asObject(value, "receipt")
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := validateReceiptMap(object)
	if err != nil {
		return Receipt{}, err
	}
	canonical, err := canonicalJSON(receipt.toMap())
	if err != nil {
		return Receipt{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return Receipt{}, recordFail("NON_CANONICAL_RECEIPT", "receipt JSON is not canonical")
	}
	return receipt, nil
}

func validateReceiptMap(value map[string]any) (Receipt, error) {
	required := []string{"version", "release", "role", "result", "plan", "binds", "detail", "summary"}
	optional := []string{
		"slice", "attempt", "contract", "target", "base", "candidate",
		"product_tree", "inputs", "checks", "result_commit",
	}
	if err := exactKeys(value, required, optional, "receipt"); err != nil {
		return Receipt{}, err
	}
	version, err := safeInteger(value["version"], "receipt.version", 1)
	if err != nil || version != ReceiptVersion {
		return Receipt{}, recordFail("INVALID_FIELD", "receipt.version must be 1")
	}
	role, err := requiredString(value["role"], "receipt.role", 1, 16)
	if err != nil {
		return Receipt{}, err
	}
	result, err := requiredString(value["result"], "receipt.result", 1, 16)
	if err != nil {
		return Receipt{}, err
	}
	if !resultsByRole[role][result] {
		return Receipt{}, recordFail("INVALID_FIELD", fmt.Sprintf("receipt result %s is invalid for %s", result, role))
	}
	release, err := identity(value["release"], "receipt.release")
	if err != nil {
		return Receipt{}, err
	}
	plan, err := objectID(value["plan"], "receipt.plan")
	if err != nil {
		return Receipt{}, err
	}
	binds, err := objectID(value["binds"], "receipt.binds")
	if err != nil {
		return Receipt{}, err
	}
	detailValue, err := digestString(value["detail"], "receipt.detail")
	if err != nil {
		return Receipt{}, err
	}
	summary, err := requiredString(value["summary"], "receipt.summary", 1, 280)
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{
		Version: ReceiptVersion, Release: release, Role: role, Result: result,
		Plan: plan, Binds: binds, Detail: detailValue, Summary: summary,
	}

	_, hasSlice := value["slice"]
	_, hasAttempt := value["attempt"]
	_, hasContract := value["contract"]
	if hasSlice != hasAttempt || hasSlice != hasContract {
		return Receipt{}, recordFail("INVALID_FIELD", "receipt slice, attempt, and contract must appear together")
	}
	if hasSlice {
		slice, err := identity(value["slice"], "receipt.slice")
		if err != nil {
			return Receipt{}, err
		}
		attempt, err := safeInteger(value["attempt"], "receipt.attempt", 1)
		if err != nil {
			return Receipt{}, err
		}
		contract, err := digestString(value["contract"], "receipt.contract")
		if err != nil {
			return Receipt{}, err
		}
		receipt.Slice, receipt.Attempt, receipt.Contract = &slice, &attempt, &contract
	} else if role == "captain" || (role == "implementer" && result == "designed") {
		return Receipt{}, recordFail("MISSING_FIELD", role+" receipt requires slice identity")
	}
	for _, item := range []struct {
		name   string
		target **string
	}{
		{"target", &receipt.Target}, {"base", &receipt.Base},
		{"candidate", &receipt.Candidate}, {"result_commit", &receipt.ResultCommit},
	} {
		if raw, ok := value[item.name]; ok {
			parsed, err := objectID(raw, "receipt."+item.name)
			if err != nil {
				return Receipt{}, err
			}
			*item.target = &parsed
		}
	}
	for _, item := range []struct {
		name   string
		target **string
	}{
		{"product_tree", &receipt.ProductTree}, {"checks", &receipt.Checks},
	} {
		if raw, ok := value[item.name]; ok {
			parsed, err := digestString(raw, "receipt."+item.name)
			if err != nil {
				return Receipt{}, err
			}
			*item.target = &parsed
		}
	}
	if raw, ok := value["inputs"]; ok {
		inputs, err := validateInputs(raw, "receipt.inputs")
		if err != nil {
			return Receipt{}, err
		}
		receipt.Inputs = inputs
	}
	if err := receipt.assertRoleFields(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func validateInputs(value any, label string) (map[string]string, error) {
	object, err := asObject(value, label)
	if err != nil {
		return nil, err
	}
	if len(object) > MaxListItems {
		return nil, recordFail("RESOURCE_LIMIT", label+" has too many inputs")
	}
	result := make(map[string]string, len(object))
	for key, value := range object {
		if _, err := identity(key, label+" key"); err != nil {
			return nil, err
		}
		parsed, err := digestString(value, label+"."+key)
		if err != nil {
			return nil, err
		}
		result[key] = parsed
	}
	return result, nil
}

func (r Receipt) assertRoleFields() error {
	present := func(name string) bool {
		switch name {
		case "slice":
			return r.Slice != nil
		case "target":
			return r.Target != nil
		case "base":
			return r.Base != nil
		case "candidate":
			return r.Candidate != nil
		case "product_tree":
			return r.ProductTree != nil
		case "inputs":
			return r.Inputs != nil
		case "checks":
			return r.Checks != nil
		case "result_commit":
			return r.ResultCommit != nil
		default:
			return false
		}
	}
	require := func(names ...string) error {
		var missing []string
		for _, name := range names {
			if !present(name) {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return recordFail("MISSING_FIELD", fmt.Sprintf("%s/%s receipt requires %s", r.Role, r.Result, strings.Join(missing, ", ")))
		}
		return nil
	}
	forbid := func(names ...string) error {
		var unexpected []string
		for _, name := range names {
			if present(name) {
				unexpected = append(unexpected, name)
			}
		}
		if len(unexpected) > 0 {
			return recordFail("INVALID_FIELD", fmt.Sprintf("%s/%s receipt forbids %s", r.Role, r.Result, strings.Join(unexpected, ", ")))
		}
		return nil
	}
	candidateEvidence := []string{"candidate", "product_tree", "inputs", "checks"}
	switch r.Role {
	case "planner":
		if r.Result == "approved" {
			if r.Slice != nil {
				return recordFail("INVALID_FIELD", "planner/approved receipt is release-scoped")
			}
			if err := require("target"); err != nil {
				return err
			}
		} else if err := require("slice"); err != nil {
			return err
		}
		return forbid("base", "candidate", "product_tree", "inputs", "checks", "result_commit")
	case "implementer":
		if r.Result == "designed" {
			if err := require("slice"); err != nil {
				return err
			}
			if (r.Base == nil) != (r.Inputs == nil) {
				return recordFail(
					"INVALID_FIELD",
					"implementer/designed receipt requires base and inputs together",
				)
			}
			return forbid("target", "candidate", "product_tree", "checks", "result_commit")
		}
		if err := require(candidateEvidence...); err != nil {
			return err
		}
		if err := forbid("result_commit"); err != nil {
			return err
		}
		if r.Slice == nil {
			return require("target", "base")
		}
		return nil
	case "captain":
		if err := require("slice"); err != nil {
			return err
		}
		return forbid("target", "base", "candidate", "product_tree", "inputs", "checks", "result_commit")
	case "verifier":
		if err := require(candidateEvidence...); err != nil {
			return err
		}
		return forbid("base", "result_commit")
	case "merge":
		if r.Slice != nil {
			return recordFail("INVALID_FIELD", "merge receipt is release-scoped")
		}
		if err := require("target", "candidate", "product_tree", "result_commit"); err != nil {
			return err
		}
		return forbid("base", "checks")
	default:
		return recordFail("INVALID_FIELD", "receipt role is invalid")
	}
}

func (r Receipt) toMap() map[string]any {
	result := map[string]any{
		"version": r.Version, "release": r.Release, "role": r.Role,
		"result": r.Result, "plan": r.Plan, "binds": r.Binds,
		"detail": r.Detail, "summary": r.Summary,
	}
	if r.Slice != nil {
		result["slice"], result["attempt"], result["contract"] = *r.Slice, *r.Attempt, *r.Contract
	}
	for _, item := range []struct {
		name  string
		value *string
	}{
		{"target", r.Target}, {"base", r.Base}, {"candidate", r.Candidate},
		{"product_tree", r.ProductTree}, {"checks", r.Checks}, {"result_commit", r.ResultCommit},
	} {
		if item.value != nil {
			result[item.name] = *item.value
		}
	}
	if r.Inputs != nil {
		inputs := make(map[string]any, len(r.Inputs))
		for key, value := range r.Inputs {
			inputs[key] = value
		}
		result["inputs"] = inputs
	}
	return result
}

type ReceiptCommit struct {
	Subject string
	Detail  []byte
	Receipt Receipt
}

func normalizeDetail(value []byte) ([]byte, error) {
	if len(value) > MaxDetailBytes {
		return nil, recordFail("RESOURCE_LIMIT", fmt.Sprintf("receipt detail exceeds %d bytes", MaxDetailBytes))
	}
	if bytes.IndexByte(value, 0) >= 0 || bytes.IndexByte(value, '\r') >= 0 {
		return nil, recordFail("INVALID_DETAIL", "receipt detail must use valid LF-only UTF-8 without NUL")
	}
	if !utf8.Valid(value) {
		return nil, recordFail("INVALID_UTF8", "receipt detail is not valid UTF-8")
	}
	if bytes.Contains(value, []byte(DetailBegin)) || bytes.Contains(value, []byte(DetailEnd)) {
		return nil, recordFail("INVALID_DETAIL", "receipt detail cannot contain marker text")
	}
	return append([]byte(nil), value...), nil
}

func RenderReceiptCommit(subject string, detail []byte, receipt Receipt) ([]byte, error) {
	if utf8.RuneCountInString(subject) < 1 || utf8.RuneCountInString(subject) > 200 ||
		strings.ContainsAny(subject, "\r\n\x00") {
		return nil, recordFail("INVALID_DETAIL", "commit subject must be one bounded line")
	}
	normalized, err := normalizeDetail(detail)
	if err != nil {
		return nil, err
	}
	next := receipt.Clone()
	next.Detail = DigestBytes(normalized)
	receiptBytes, err := next.CanonicalBytes()
	if err != nil {
		return nil, err
	}
	if len(receiptBytes) > MaxReceiptBytes {
		return nil, recordFail("RESOURCE_LIMIT", fmt.Sprintf("receipt exceeds %d bytes", MaxReceiptBytes))
	}
	var result bytes.Buffer
	result.WriteString(subject)
	result.WriteString("\n\n")
	result.WriteString(DetailBegin)
	result.WriteByte('\n')
	result.Write(normalized)
	result.WriteByte('\n')
	result.WriteString(DetailEnd)
	result.WriteString("\n\n")
	result.WriteString(ReceiptTrailer)
	result.Write(receiptBytes)
	result.WriteByte('\n')
	return result.Bytes(), nil
}

func ParseReceiptCommitMessage(raw []byte) (ReceiptCommit, error) {
	if len(raw) > MaxMessageBytes {
		return ReceiptCommit{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("receipt commit message exceeds %d bytes", MaxMessageBytes))
	}
	if bytes.IndexByte(raw, 0) >= 0 || bytes.IndexByte(raw, '\r') >= 0 {
		return ReceiptCommit{}, recordFail("INVALID_RECEIPT_COMMIT", "receipt commit message must be LF-only UTF-8 without NUL")
	}
	if !utf8.Valid(raw) {
		return ReceiptCommit{}, recordFail("INVALID_UTF8", "receipt commit message is not valid UTF-8")
	}
	if !bytes.HasSuffix(raw, []byte{'\n'}) {
		return ReceiptCommit{}, recordFail("INVALID_RECEIPT_COMMIT", "receipt commit message must end with LF")
	}
	beginToken := []byte("\n\n" + DetailBegin + "\n")
	endToken := []byte("\n" + DetailEnd + "\n\n" + ReceiptTrailer)
	begin := bytes.Index(raw, beginToken)
	end := -1
	if begin >= 0 {
		end = bytes.Index(raw[begin+len(beginToken):], endToken)
		if end >= 0 {
			end += begin + len(beginToken)
		}
	}
	if begin <= 0 || end < begin ||
		bytes.Index(raw[begin+len(beginToken):], beginToken) >= 0 ||
		bytes.Index(raw[end+len(endToken):], endToken) >= 0 {
		return ReceiptCommit{}, recordFail("INVALID_RECEIPT_COMMIT", "receipt commit message has invalid detail markers")
	}
	subject := string(raw[:begin])
	if strings.Contains(subject, "\n") || utf8.RuneCountInString(subject) > 200 {
		return ReceiptCommit{}, recordFail("INVALID_RECEIPT_COMMIT", "receipt commit subject must be one bounded line")
	}
	detail, err := normalizeDetail(raw[begin+len(beginToken) : end])
	if err != nil {
		return ReceiptCommit{}, err
	}
	trailer := raw[end+len(endToken) : len(raw)-1]
	if bytes.Contains(trailer, []byte{'\n'}) {
		return ReceiptCommit{}, recordFail("INVALID_RECEIPT_COMMIT", "receipt trailer must be the final line")
	}
	receipt, err := ParseReceipt(trailer)
	if err != nil {
		return ReceiptCommit{}, err
	}
	if receipt.Detail != DigestBytes(detail) {
		return ReceiptCommit{}, recordFail("STALE_BINDING", "receipt detail digest does not match the exact detail bytes")
	}
	return ReceiptCommit{Subject: subject, Detail: detail, Receipt: receipt}, nil
}

type HistoryEnvelope struct {
	OID        string
	Parents    []string
	Tree       string
	ParentTree string
	Message    []byte
}

type ReceiptEntry struct {
	OID     string
	Parent  string
	Tree    string
	Subject string
	Detail  []byte
	Receipt Receipt
}

func (e ReceiptEntry) Clone() ReceiptEntry {
	e.Detail = append([]byte(nil), e.Detail...)
	e.Receipt = e.Receipt.Clone()
	return e
}

func ParseReceiptHistoryEntry(value HistoryEnvelope) (ReceiptEntry, error) {
	if !objectIDPattern.MatchString(value.OID) || !objectIDPattern.MatchString(value.Tree) ||
		!objectIDPattern.MatchString(value.ParentTree) {
		return ReceiptEntry{}, recordFail("INVALID_FIELD", "history entry contains an invalid object identity")
	}
	if len(value.Parents) != 1 || !objectIDPattern.MatchString(value.Parents[0]) {
		return ReceiptEntry{}, recordFail("INVALID_HISTORY", "receipt commit must have exactly one parent")
	}
	if value.Tree != value.ParentTree {
		return ReceiptEntry{}, recordFail("PRODUCT_MUTATION", "receipt commit must be metadata-only")
	}
	parsed, err := ParseReceiptCommitMessage(value.Message)
	if err != nil {
		return ReceiptEntry{}, err
	}
	return ReceiptEntry{
		OID: value.OID, Parent: value.Parents[0], Tree: value.Tree,
		Subject: parsed.Subject, Detail: append([]byte(nil), parsed.Detail...),
		Receipt: parsed.Receipt,
	}, nil
}

func sortedInputKeys(inputs map[string]string) []string {
	result := make([]string, 0, len(inputs))
	for key := range inputs {
		result = append(result, key)
	}
	sort.Slice(result, func(i, j int) bool { return bytes.Compare([]byte(result[i]), []byte(result[j])) < 0 })
	return result
}
