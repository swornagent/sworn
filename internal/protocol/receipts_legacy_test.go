package protocol

import (
	"bytes"
	"strings"
	"testing"
)

// legacyForm rewrites a current-vocabulary receipt commit message into the
// exact form the pre-rename binary wrote (sworn#316 changed the markers).
func legacyForm(message []byte) []byte {
	message = bytes.ReplaceAll(message, []byte(DetailBegin), []byte(legacyDetailBegin))
	message = bytes.ReplaceAll(message, []byte(DetailEnd), []byte(legacyDetailEnd))
	return bytes.ReplaceAll(message, []byte(ReceiptTrailer), []byte(legacyReceiptTrailer))
}

func TestParseReceiptCommitMessageReadsPreRenameMarkers(t *testing.T) {
	slice := "S1"
	attempt := int64(1)
	contract := "sha256:" + strings.Repeat("a", 64)
	receipt := Receipt{
		Version: ReceiptVersion, Release: "2026-09-11-phased-evidence",
		Slice: &slice, Attempt: &attempt, Contract: &contract,
		Role: "lead", Result: "proceed",
		Plan: strings.Repeat("1", 40), Binds: strings.Repeat("2", 40),
		Summary: "PROCEED with the design as submitted.",
	}
	current, err := RenderReceiptCommit("sworn(release/S1): lead proceed", []byte("the reasoning"), receipt)
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyForm(current)
	// The pre-rename binary also named the Lead role "captain".
	legacy = bytes.ReplaceAll(legacy, []byte(`"role":"lead"`), []byte(`"role":"captain"`))
	if bytes.Equal(legacy, current) || !bytes.Contains(legacy, []byte("Baton-Receipt: ")) {
		t.Fatalf("legacy rewrite did not change the message:\n%s", legacy)
	}
	for name, message := range map[string][]byte{"current": current, "legacy": legacy} {
		if !hasReceiptTrailer(message) {
			t.Fatalf("%s message not recognised as a receipt", name)
		}
		parsed, err := ParseReceiptCommitMessage(message)
		if err != nil {
			t.Fatalf("%s message: %v", name, err)
		}
		if parsed.Receipt.Role != "lead" || parsed.Receipt.Result != "proceed" ||
			string(parsed.Detail) != "the reasoning" || parsed.Subject != "sworn(release/S1): lead proceed" {
			t.Fatalf("%s message parsed as %+v", name, parsed)
		}
	}
	if hasReceiptTrailer([]byte("docs: mention Protocol-Receipt in prose\n")) {
		t.Fatal("a trailer mentioned mid-line must not mark a receipt")
	}
}

func TestParseReceiptCommitMessageRejectsMixedMarkers(t *testing.T) {
	target := strings.Repeat("3", 40)
	receipt := Receipt{
		Version: ReceiptVersion, Release: "release", Role: "planner", Result: "approved",
		Plan: strings.Repeat("1", 40), Binds: strings.Repeat("2", 40), Summary: "approve plan",
		Target: &target,
	}
	current, err := RenderReceiptCommit("approve plan", []byte("approval"), receipt)
	if err != nil {
		t.Fatal(err)
	}
	mixed := bytes.ReplaceAll(current, []byte(DetailBegin), []byte(legacyDetailBegin))
	if _, err := ParseReceiptCommitMessage(mixed); err == nil {
		t.Fatal("a message mixing marker vocabularies parsed")
	}
}

// A release whose revision-1 approval was recorded by the pre-rename binary
// must still be readable and revisable: this is the exact history that
// refused HISTORY_BOUNDARY_MISSING on 2026-09-18 (sworn#320).
func TestReleaseHistoryRecordedBeforeTheRenameStaysReadable(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "pre-rename"
	approved, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release),
		Summary:   "Approve revision 1.",
		Detail:    []byte("approval"),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := actionGit(t, repoPath, nil, nil, "cat-file", "commit", approved.Head)
	headerEnd := strings.Index(raw, "\n\n")
	if headerEnd < 0 {
		t.Fatalf("approval commit has no message: %q", raw)
	}
	message := []byte(raw[headerEnd+2:])
	if !bytes.HasSuffix(message, []byte("\n")) {
		message = append(message, '\n')
	}
	parent := strings.TrimSpace(actionGit(t, repoPath, nil, nil, "rev-parse", approved.Head+"^"))
	rewritten, err := actions.repository.prepareMetadata(parent, legacyForm(message))
	if err != nil {
		t.Fatal(err)
	}
	ref := releaseRef(release)
	actionGit(t, repoPath, nil, nil, "update-ref", ref, rewritten.Commit, approved.Head)

	state := readActionState(t, repository, release)
	if state.Plan.Metadata.Revision != 1 || state.Plan.OID != approved.Plan {
		t.Fatalf("pre-rename approval not read: revision %d plan %s (want %s)",
			state.Plan.Metadata.Revision, state.Plan.OID, approved.Plan)
	}
}
