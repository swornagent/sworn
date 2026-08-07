package baton

import (
	"fmt"
	"strings"
)

const (
	// EvidenceBundleVersion identifies Sworn's own optional, non-authoritative
	// human-reviewable evidence bundle schema. It is never a lifecycle record:
	// nothing in plan admission, dispatch, the scheduler, or Merge reads it.
	EvidenceBundleVersion = "sworn.evidence-bundle/v1"
	// MaxEvidenceItems bounds how many inventoried items one bundle may name.
	MaxEvidenceItems = 128
)

var evidenceItemKinds = map[string]bool{
	"screenshot":     true,
	"recording":      true,
	"command-output": true,
	"trace":          true,
	"other":          true,
}

// evidenceVerifierResults mirrors resultsByRole["verifier"]: an evidence
// bundle only ever binds to a verifier receipt, so its declared result must
// be one a verifier receipt can actually carry.
var evidenceVerifierResults = map[string]bool{"pass": true, "fail": true, "blocked": true}

// EvidenceItem is one inventoried human-reviewable artifact: a screenshot,
// recording, command output capture, trace, or similar file. Path is a safe
// relative path; Digest is the exact sha256 of its real bytes.
type EvidenceItem struct {
	Path   string
	Kind   string
	Digest string
	Bytes  int64
}

type evidenceAdmission struct {
	raw         []byte
	digest      string
	release     string
	slice       string
	attempt     int64
	candidate   string
	productTree string
	checks      string
	result      string
	receipt     string
	items       []EvidenceItem
}

// EvidenceBundle is an immutable admission of one sworn.evidence-bundle/v1
// manifest. Admission alone proves the manifest is well-formed; it proves
// nothing about whether the binding it declares is real. Only Resolve, which
// every caller must invoke explicitly against a real State, proves that.
type EvidenceBundle struct {
	admission *evidenceAdmission
}

func (b EvidenceBundle) require() (*evidenceAdmission, error) {
	if b.admission == nil || !digestPattern.MatchString(b.admission.digest) ||
		DigestBytes(b.admission.raw) != b.admission.digest {
		return nil, recordFail("EVIDENCE_ADMISSION_REQUIRED", "operation requires one immutable parsed evidence bundle")
	}
	return b.admission, nil
}

// Digest returns the exact sha256 of the admitted manifest bytes.
func (b EvidenceBundle) Digest() string {
	if b.admission == nil {
		return ""
	}
	return b.admission.digest
}

// Items returns a copy of the bundle's inventoried items.
func (b EvidenceBundle) Items() []EvidenceItem {
	if b.admission == nil {
		return nil
	}
	return append([]EvidenceItem(nil), b.admission.items...)
}

// ParseEvidenceBundle admits one sworn.evidence-bundle/v1 manifest: bounded
// JSON binding a fixed set of human-reviewable items to one exact release,
// slice, attempt, candidate commit, product tree, checks digest, verifier
// result, and verifier receipt identity. Parsing proves only that the
// manifest is well-formed and every item path/digest/kind is structurally
// valid and unambiguous; Resolve proves the binding and item bytes are real.
func ParseEvidenceBundle(raw []byte) (EvidenceBundle, error) {
	if len(raw) > MaxEvidenceBytes {
		return EvidenceBundle{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("evidence bundle exceeds %d bytes", MaxEvidenceBytes))
	}
	value, err := strictParseJSON(raw, "evidence bundle", MaxEvidenceBytes)
	if err != nil {
		return EvidenceBundle{}, err
	}
	object, err := asObject(value, "evidence bundle")
	if err != nil {
		return EvidenceBundle{}, err
	}
	required := []string{
		"schema_version", "release", "slice", "attempt", "candidate",
		"product_tree", "checks", "result", "receipt", "items",
	}
	if err := exactKeys(object, required, nil, "evidence bundle"); err != nil {
		return EvidenceBundle{}, err
	}
	schema, err := requiredString(object["schema_version"], "evidence_bundle.schema_version", 1, 100)
	if err != nil {
		return EvidenceBundle{}, err
	}
	if schema != EvidenceBundleVersion {
		return EvidenceBundle{}, recordFail("INVALID_FIELD", "evidence_bundle.schema_version must be "+EvidenceBundleVersion)
	}
	release, err := identity(object["release"], "evidence_bundle.release")
	if err != nil {
		return EvidenceBundle{}, err
	}
	slice, err := identity(object["slice"], "evidence_bundle.slice")
	if err != nil {
		return EvidenceBundle{}, err
	}
	attempt, err := safeInteger(object["attempt"], "evidence_bundle.attempt", 1)
	if err != nil {
		return EvidenceBundle{}, err
	}
	candidate, err := objectID(object["candidate"], "evidence_bundle.candidate")
	if err != nil {
		return EvidenceBundle{}, err
	}
	productTree, err := digestString(object["product_tree"], "evidence_bundle.product_tree")
	if err != nil {
		return EvidenceBundle{}, err
	}
	checks, err := digestString(object["checks"], "evidence_bundle.checks")
	if err != nil {
		return EvidenceBundle{}, err
	}
	result, err := identity(object["result"], "evidence_bundle.result")
	if err != nil {
		return EvidenceBundle{}, err
	}
	if !evidenceVerifierResults[result] {
		return EvidenceBundle{}, recordFail("INVALID_FIELD", "evidence_bundle.result must be one verifier result")
	}
	receipt, err := objectID(object["receipt"], "evidence_bundle.receipt")
	if err != nil {
		return EvidenceBundle{}, err
	}
	rawItems, err := asArray(object["items"], "evidence_bundle.items", true, MaxEvidenceItems)
	if err != nil {
		return EvidenceBundle{}, err
	}
	items := make([]EvidenceItem, 0, len(rawItems))
	seenPaths := make(map[string]bool, len(rawItems))
	for index, rawItem := range rawItems {
		label := fmt.Sprintf("evidence_bundle.items[%d]", index)
		item, err := validateEvidenceItem(rawItem, label)
		if err != nil {
			return EvidenceBundle{}, err
		}
		if seenPaths[item.Path] {
			return EvidenceBundle{}, recordFail("DUPLICATE_IDENTITY", "evidence bundle repeats item path "+item.Path)
		}
		seenPaths[item.Path] = true
		items = append(items, item)
	}
	copied := append([]byte(nil), raw...)
	return EvidenceBundle{admission: &evidenceAdmission{
		raw: copied, digest: DigestBytes(copied),
		release: release, slice: slice, attempt: attempt, candidate: candidate,
		productTree: productTree, checks: checks, result: result, receipt: receipt,
		items: items,
	}}, nil
}

func validateEvidenceItem(value any, label string) (EvidenceItem, error) {
	object, err := asObject(value, label)
	if err != nil {
		return EvidenceItem{}, err
	}
	if err := exactKeys(object, []string{"path", "kind", "digest", "bytes"}, nil, label); err != nil {
		return EvidenceItem{}, err
	}
	path, err := repositoryPath(object["path"], label+".path")
	if err != nil {
		return EvidenceItem{}, err
	}
	if path == RecordRoot || strings.HasPrefix(path, RecordRoot+"/") {
		return EvidenceItem{}, recordFail(
			"RESERVED_RECORD_ROOT",
			label+".path cannot name reserved Baton records at "+path,
		)
	}
	kind, err := identity(object["kind"], label+".kind")
	if err != nil {
		return EvidenceItem{}, err
	}
	if !evidenceItemKinds[kind] {
		return EvidenceItem{}, recordFail("INVALID_FIELD", label+".kind is not one recognized evidence kind")
	}
	digest, err := digestString(object["digest"], label+".digest")
	if err != nil {
		return EvidenceItem{}, err
	}
	byteCount, err := safeInteger(object["bytes"], label+".bytes", 0)
	if err != nil {
		return EvidenceItem{}, err
	}
	return EvidenceItem{Path: path, Kind: kind, Digest: digest, Bytes: byteCount}, nil
}

// Resolve proves this admitted evidence bundle binds to one exact,
// already-recorded verifier receipt in state's slice history, and that every
// inventoried item's declared digest matches the caller-supplied bytes for
// its path. Callers read item bytes from wherever the bundle actually lives
// (disk, a Git tree, or elsewhere); Resolve itself performs no I/O and
// mutates nothing. Any missing, stale, tampered, or ambiguous binding fails
// closed. Resolve is never called by plan admission, dispatch, the
// scheduler, or any other write/action path; a caller must invoke it
// explicitly to read a bundle, and its absence never affects any of those
// paths.
func (b EvidenceBundle) Resolve(state State, items map[string][]byte) (ReceiptEntry, error) {
	admission, err := b.require()
	if err != nil {
		return ReceiptEntry{}, err
	}
	if state.Release != admission.release {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "evidence bundle release does not match the read state")
	}
	history, ok := state.HistoryForSlice(admission.slice)
	if !ok {
		return ReceiptEntry{}, recordFail("SLICE_NOT_FOUND", "state has no recorded history for slice "+admission.slice)
	}
	var entry *ReceiptEntry
	for index := range history.History.Entries {
		if history.History.Entries[index].OID == admission.receipt {
			entry = &history.History.Entries[index]
			break
		}
	}
	if entry == nil {
		return ReceiptEntry{}, recordFail(
			"RECEIPT_NOT_FOUND",
			"evidence bundle receipt is not in slice "+admission.slice+"'s recorded history",
		)
	}
	receipt := entry.Receipt
	if receipt.Role != "verifier" {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "evidence bundle receipt is not a verifier receipt")
	}
	if receipt.Attempt == nil || *receipt.Attempt != admission.attempt {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "evidence bundle attempt does not match the bound receipt")
	}
	if receipt.Candidate == nil || *receipt.Candidate != admission.candidate {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "evidence bundle candidate does not match the bound receipt")
	}
	if receipt.ProductTree == nil || *receipt.ProductTree != admission.productTree {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "evidence bundle product tree does not match the bound receipt")
	}
	if receipt.Checks == nil || *receipt.Checks != admission.checks {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "evidence bundle checks digest does not match the bound receipt")
	}
	if receipt.Result != admission.result {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "evidence bundle result does not match the bound receipt")
	}
	for _, item := range admission.items {
		body, ok := items[item.Path]
		if !ok {
			return ReceiptEntry{}, recordFail("EVIDENCE_ITEM_MISSING", "evidence item is missing at "+item.Path)
		}
		if int64(len(body)) != item.Bytes || DigestBytes(body) != item.Digest {
			return ReceiptEntry{}, recordFail(
				"STALE_BINDING",
				"evidence item does not match its declared digest at "+item.Path,
			)
		}
	}
	return entry.Clone(), nil
}
