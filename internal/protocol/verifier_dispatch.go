package protocol

import (
	"errors"
	"fmt"
)

const VerifierDispatchKind = "verifier_dispatch"

// VerifierDispatch is Baton's closed verifier_dispatch control receipt. It
// describes the review materialization; verifier profile and process facts
// remain in Sworn's internal effect closure rather than widening this schema.
type VerifierDispatch struct {
	SchemaVersion             string         `json:"schema_version"`
	Kind                      string         `json:"kind"`
	DispatchID                string         `json:"dispatch_id"`
	Role                      string         `json:"role"`
	SubmissionDigest          string         `json:"submission_digest"`
	Candidate                 CandidatePoint `json:"candidate"`
	Workspace                 string         `json:"workspace"`
	FreshContext              bool           `json:"fresh_context"`
	BuilderTranscriptIncluded bool           `json:"builder_transcript_included"`
	TargetRefWritable         bool           `json:"target_ref_writable"`
	RemotesPresent            bool           `json:"remotes_present"`
	WriteCredentialsPresent   bool           `json:"write_credentials_present"`
	CreatedAt                 string         `json:"created_at"`
}

// VerifierDispatchInput contains only variable engine-owned facts. Submission
// and candidate binding are derived from the exact submission capability; the
// builder below stamps every Baton discriminator and isolation constant.
type VerifierDispatchInput struct {
	Submission ExactSubmission
	DispatchID string
	Workspace  string
	CreatedAt  string
}

// ParseVerifierDispatch strictly validates exact control-receipt bytes. The
// receipt's isolation booleans are claims until the later effect closure proves
// them against the actual materialization and process profile.
func ParseVerifierDispatch(contents []byte) (VerifierDispatch, error) {
	var dispatch VerifierDispatch
	if err := decodeExactJSONShape(
		contents, MaximumControlReceiptBytes, "verifier dispatch", &dispatch,
	); err != nil {
		return VerifierDispatch{}, err
	}
	if err := validateVerifierDispatch(dispatch); err != nil {
		return VerifierDispatch{}, err
	}
	return dispatch, nil
}

// BuildVerifierDispatch stamps and encodes an engine-owned canonical Baton
// control receipt. Callers cannot choose role or isolation booleans.
func BuildVerifierDispatch(input VerifierDispatchInput) (EncodedRecord, error) {
	if !input.Submission.valid() {
		return EncodedRecord{}, errors.New("verifier dispatch requires an exact submission capability")
	}
	submission := input.Submission.View()
	submissionCreated, _ := parseRecordTime(submission.CreatedAt, "submission creation")
	dispatchCreated, err := parseRecordTime(input.CreatedAt, "dispatch creation")
	if err != nil {
		return EncodedRecord{}, err
	}
	if dispatchCreated.Before(submissionCreated) {
		return EncodedRecord{}, errors.New("verifier dispatch precedes its submission")
	}
	if input.DispatchID == submission.Builder.RunID {
		return EncodedRecord{}, errors.New("verifier dispatch reuses the builder run identity")
	}
	dispatch := VerifierDispatch{
		SchemaVersion:             ControlReceiptSchemaVersion,
		Kind:                      VerifierDispatchKind,
		DispatchID:                input.DispatchID,
		Role:                      "verifier",
		SubmissionDigest:          input.Submission.Record().Digest,
		Candidate:                 submission.Candidate,
		Workspace:                 input.Workspace,
		FreshContext:              true,
		BuilderTranscriptIncluded: false,
		TargetRefWritable:         false,
		RemotesPresent:            false,
		WriteCredentialsPresent:   false,
		CreatedAt:                 input.CreatedAt,
	}
	return encodeVerifierDispatch(dispatch)
}

func encodeVerifierDispatch(dispatch VerifierDispatch) (EncodedRecord, error) {
	if err := validateVerifierDispatch(dispatch); err != nil {
		return EncodedRecord{}, err
	}
	canonical, err := EncodeCanonical(dispatch)
	if err != nil {
		return EncodedRecord{}, fmt.Errorf("canonicalize verifier dispatch: %w", err)
	}
	if len(canonical) > MaximumControlReceiptBytes {
		return EncodedRecord{}, errors.New("verifier dispatch exceeds byte ceiling")
	}
	if _, err := ParseVerifierDispatch(canonical); err != nil {
		return EncodedRecord{}, err
	}
	return EncodedRecord{
		Kind: ControlReceiptSchemaVersion, CanonicalJSON: canonical, Digest: CanonicalDigest(canonical),
	}, nil
}

func validateVerifierDispatch(dispatch VerifierDispatch) error {
	if dispatch.SchemaVersion != ControlReceiptSchemaVersion || dispatch.Kind != VerifierDispatchKind ||
		dispatch.Role != "verifier" {
		return errors.New("artifact is not a Baton verifier dispatch")
	}
	if !ValidID(dispatch.DispatchID) || !ValidDigest(dispatch.SubmissionDigest) {
		return errors.New("verifier dispatch has an invalid identity or submission digest")
	}
	if err := validateCandidatePoint(dispatch.Candidate, "verifier dispatch candidate"); err != nil {
		return err
	}
	if !ValidNonEmpty(dispatch.Workspace) || !ValidDateTime(dispatch.CreatedAt) {
		return errors.New("verifier dispatch has an invalid workspace or creation time")
	}
	if !dispatch.FreshContext || dispatch.BuilderTranscriptIncluded || dispatch.TargetRefWritable ||
		dispatch.RemotesPresent || dispatch.WriteCredentialsPresent {
		return errors.New("verifier dispatch does not describe an isolated fresh review")
	}
	return nil
}

func validateCandidatePoint(candidate CandidatePoint, label string) error {
	if !ValidNonEmpty(candidate.Repository) || !oidPattern.MatchString(candidate.Commit) ||
		!oidPattern.MatchString(candidate.Tree) {
		return fmt.Errorf("%s is invalid", label)
	}
	return nil
}
