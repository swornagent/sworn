package protocol

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SubmissionSchemaVersion = "submission-v1"
	MaximumSubmissionBytes  = 1 << 20
)

var (
	protocolIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	packIDPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}@[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)
	digestPattern     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	oidPattern        = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	mediaTypePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]{0,126}/[a-z0-9][a-z0-9!#$&^_.+-]{0,126}$`)
	refCharacters     = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	recordTimePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[Tt](\d{2}):(\d{2}):(\d{2})(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$`)
)

// ValidID reports whether a value fits Baton's common record-identifier
// profile. It lets engine-owned producers reject bad identities before
// performing or persisting work.
func ValidID(value string) bool { return protocolIDPattern.MatchString(value) }

type Artifact struct {
	Ref       string `json:"ref"`
	MediaType string `json:"media_type"`
	Digest    string `json:"digest"`
}

type BuilderRun struct {
	RunID       string `json:"run_id"`
	Agent       string `json:"agent"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

type GitPoint struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Commit     string `json:"commit"`
}

type CandidatePoint struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Tree       string `json:"tree"`
}

type Assurance struct {
	Profile      string   `json:"profile"`
	Packs        []string `json:"packs"`
	PolicyRef    string   `json:"policy_ref"`
	PolicyDigest string   `json:"policy_digest"`
}

type Environment struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

type Check struct {
	ID            string      `json:"id"`
	Outcome       string      `json:"outcome"`
	RunID         string      `json:"run_id"`
	CandidateTree string      `json:"candidate_tree"`
	Environment   Environment `json:"environment"`
	StartedAt     string      `json:"started_at"`
	CompletedAt   string      `json:"completed_at"`
	ExitCode      *int        `json:"exit_code,omitempty"`
	Receipt       Artifact    `json:"receipt"`
	Command       string      `json:"command,omitempty"`
}

type Evidence struct {
	ID            string      `json:"id"`
	AcceptanceIDs []string    `json:"acceptance_ids,omitempty"`
	PackIDs       []string    `json:"pack_ids,omitempty"`
	Kind          string      `json:"kind"`
	Boundary      string      `json:"boundary"`
	Environment   Environment `json:"environment"`
	UsesMocks     bool        `json:"uses_mocks"`
	ProducerRunID string      `json:"producer_run_id"`
	CandidateTree string      `json:"candidate_tree"`
	CapturedAt    string      `json:"captured_at"`
	Artifact      Artifact    `json:"artifact"`
	Observed      string      `json:"observed"`
	Notes         string      `json:"notes,omitempty"`
}

type Submission struct {
	SchemaVersion    string         `json:"schema_version"`
	SubmissionID     string         `json:"submission_id"`
	DeliveryID       string         `json:"delivery_id"`
	WorkID           string         `json:"work_id"`
	Attempt          int64          `json:"attempt"`
	CreatedAt        string         `json:"created_at"`
	PlanDigest       string         `json:"plan_digest"`
	ContractDigest   string         `json:"contract_digest"`
	AuthorityReceipt Artifact       `json:"authority_receipt"`
	Builder          BuilderRun     `json:"builder"`
	Base             GitPoint       `json:"base"`
	Candidate        CandidatePoint `json:"candidate"`
	Assurance        Assurance      `json:"assurance"`
	ChangedPaths     []string       `json:"changed_paths"`
	Checks           []Check        `json:"checks"`
	Evidence         []Evidence     `json:"evidence"`
}

type EncodedRecord struct {
	Kind          string
	CanonicalJSON []byte
	Digest        string
}

// EncodeSubmission validates engine-owned cross-field invariants and returns
// the exact immutable Baton record bytes and digest.
func EncodeSubmission(submission Submission) (EncodedRecord, error) {
	if err := validateSubmission(submission); err != nil {
		return EncodedRecord{}, err
	}
	canonical, err := EncodeCanonical(submission)
	if err != nil {
		return EncodedRecord{}, fmt.Errorf("canonicalize submission: %w", err)
	}
	if len(canonical) > MaximumSubmissionBytes {
		return EncodedRecord{}, errors.New("submission exceeds byte ceiling")
	}
	return EncodedRecord{
		Kind:          SubmissionSchemaVersion,
		CanonicalJSON: canonical,
		Digest:        CanonicalDigest(canonical),
	}, nil
}

func validateSubmission(submission Submission) error {
	if submission.SchemaVersion != SubmissionSchemaVersion {
		return fmt.Errorf("unknown submission schema %q", submission.SchemaVersion)
	}
	for name, value := range map[string]string{
		"submission":  submission.SubmissionID,
		"delivery":    submission.DeliveryID,
		"work":        submission.WorkID,
		"builder run": submission.Builder.RunID,
	} {
		if !protocolIDPattern.MatchString(value) {
			return fmt.Errorf("invalid %s id %q", name, value)
		}
	}
	if submission.Attempt < 1 || submission.Attempt > maximumSafeInteger {
		return errors.New("submission attempt is outside the interoperable range")
	}
	createdAt, err := parseRecordTime(submission.CreatedAt, "submission creation")
	if err != nil {
		return err
	}
	for name, digest := range map[string]string{
		"plan":     submission.PlanDigest,
		"contract": submission.ContractDigest,
		"policy":   submission.Assurance.PolicyDigest,
	} {
		if !digestPattern.MatchString(digest) {
			return fmt.Errorf("invalid %s digest", name)
		}
	}
	if err := validateArtifact(submission.AuthorityReceipt, "authority receipt"); err != nil {
		return err
	}
	if !nonEmpty(submission.Builder.Agent) {
		return errors.New("builder agent is required")
	}
	builderStart, err := parseRecordTime(submission.Builder.StartedAt, "builder start")
	if err != nil {
		return err
	}
	builderEnd, err := parseRecordTime(submission.Builder.CompletedAt, "builder completion")
	if err != nil {
		return err
	}
	if builderEnd.Before(builderStart) || createdAt.Before(builderEnd) {
		return errors.New("builder timestamps are outside the submission window")
	}
	if !nonEmpty(submission.Base.Repository) || submission.Base.Repository != submission.Candidate.Repository {
		return errors.New("base and candidate require one repository identity")
	}
	if !validBranchRef(submission.Base.Ref) {
		return fmt.Errorf("invalid base ref %q", submission.Base.Ref)
	}
	for name, oid := range map[string]string{
		"base commit":      submission.Base.Commit,
		"candidate commit": submission.Candidate.Commit,
		"candidate tree":   submission.Candidate.Tree,
	} {
		if !oidPattern.MatchString(oid) {
			return fmt.Errorf("invalid %s", name)
		}
	}
	if err := validateAssurance(submission.Assurance); err != nil {
		return err
	}
	if submission.ChangedPaths == nil {
		return errors.New("changed paths must be an array")
	}
	if !slices.IsSorted(submission.ChangedPaths) {
		return errors.New("changed paths must be sorted")
	}
	if duplicateStrings(submission.ChangedPaths) {
		return errors.New("changed paths contain duplicates")
	}
	for _, path := range submission.ChangedPaths {
		if !validGitPath(path) {
			return fmt.Errorf("invalid changed path %q", path)
		}
	}
	if len(submission.Checks) == 0 {
		return errors.New("submission requires at least one check")
	}
	if len(submission.Evidence) == 0 {
		return errors.New("submission requires at least one evidence item")
	}
	checkIDs := make(map[string]struct{}, len(submission.Checks))
	runs := make(map[string]Check, len(submission.Checks))
	for _, check := range submission.Checks {
		if err := validateCheck(check, submission, builderEnd, createdAt); err != nil {
			return err
		}
		if _, exists := checkIDs[check.ID]; exists {
			return fmt.Errorf("duplicate check id %q", check.ID)
		}
		checkIDs[check.ID] = struct{}{}
		if check.RunID == submission.Builder.RunID {
			return fmt.Errorf("check %q reuses the builder run", check.ID)
		}
		if _, exists := runs[check.RunID]; exists {
			return fmt.Errorf("duplicate check run id %q", check.RunID)
		}
		runs[check.RunID] = check
	}
	evidenceIDs := make(map[string]struct{}, len(submission.Evidence))
	selectedPacks := make(map[string]struct{}, len(submission.Assurance.Packs))
	for _, packID := range submission.Assurance.Packs {
		selectedPacks[packID] = struct{}{}
	}
	for _, evidence := range submission.Evidence {
		if err := validateEvidence(evidence, submission, runs, selectedPacks, createdAt); err != nil {
			return err
		}
		if _, exists := evidenceIDs[evidence.ID]; exists {
			return fmt.Errorf("duplicate evidence id %q", evidence.ID)
		}
		evidenceIDs[evidence.ID] = struct{}{}
	}
	return nil
}

func validateAssurance(assurance Assurance) error {
	if !nonEmpty(assurance.PolicyRef) || !digestPattern.MatchString(assurance.PolicyDigest) {
		return errors.New("assurance requires a policy reference and digest")
	}
	if assurance.Packs == nil {
		return errors.New("assurance packs must be an array")
	}
	if duplicateStrings(assurance.Packs) || !slices.IsSorted(assurance.Packs) {
		return errors.New("assurance packs must be unique and sorted")
	}
	for _, packID := range assurance.Packs {
		if !packIDPattern.MatchString(packID) {
			return fmt.Errorf("invalid assurance pack id %q", packID)
		}
	}
	switch assurance.Profile {
	case "standard":
		if len(assurance.Packs) != 0 {
			return errors.New("standard assurance cannot select packs")
		}
	case "assured":
		if len(assurance.Packs) == 0 {
			return errors.New("assured submission requires a pack")
		}
	default:
		return fmt.Errorf("invalid assurance profile %q", assurance.Profile)
	}
	return nil
}

func validateCheck(check Check, submission Submission, builderEnd, createdAt recordTime) error {
	if !protocolIDPattern.MatchString(check.ID) || !protocolIDPattern.MatchString(check.RunID) {
		return errors.New("check requires valid check and run ids")
	}
	if check.Outcome != "pass" && check.Outcome != "fail" && check.Outcome != "skipped" {
		return fmt.Errorf("check %q has invalid outcome %q", check.ID, check.Outcome)
	}
	if check.ExitCode != nil && (*check.ExitCode < -2_147_483_648 || *check.ExitCode > 2_147_483_647) {
		return fmt.Errorf("check %q exit code is outside Baton's int32 range", check.ID)
	}
	if check.CandidateTree != submission.Candidate.Tree {
		return fmt.Errorf("check %q does not bind the candidate tree", check.ID)
	}
	if err := validateEnvironment(check.Environment); err != nil {
		return fmt.Errorf("check %q: %w", check.ID, err)
	}
	startedAt, err := parseRecordTime(check.StartedAt, "check start")
	if err != nil {
		return err
	}
	completedAt, err := parseRecordTime(check.CompletedAt, "check completion")
	if err != nil {
		return err
	}
	if startedAt.Before(builderEnd) || completedAt.Before(startedAt) || createdAt.Before(completedAt) {
		return fmt.Errorf("check %q timestamps are outside its producer window", check.ID)
	}
	if err := validateArtifact(check.Receipt, "check receipt"); err != nil {
		return fmt.Errorf("check %q: %w", check.ID, err)
	}
	if check.Command != "" && !nonEmpty(check.Command) {
		return fmt.Errorf("check %q has an empty command", check.ID)
	}
	return nil
}

func validateEvidence(
	evidence Evidence,
	submission Submission,
	runs map[string]Check,
	selectedPacks map[string]struct{},
	createdAt recordTime,
) error {
	if !protocolIDPattern.MatchString(evidence.ID) {
		return fmt.Errorf("invalid evidence id %q", evidence.ID)
	}
	if len(evidence.AcceptanceIDs) == 0 && len(evidence.PackIDs) == 0 {
		return fmt.Errorf("evidence %q is not linked to acceptance or pack ids", evidence.ID)
	}
	for label, values := range map[string][]string{
		"acceptance": evidence.AcceptanceIDs,
		"pack":       evidence.PackIDs,
	} {
		if duplicateStrings(values) || !slices.IsSorted(values) {
			return fmt.Errorf("evidence %q %s ids must be unique and sorted", evidence.ID, label)
		}
		for _, value := range values {
			valid := protocolIDPattern.MatchString(value)
			if label == "pack" {
				valid = packIDPattern.MatchString(value)
				_, selected := selectedPacks[value]
				valid = valid && selected
			}
			if !valid {
				return fmt.Errorf("evidence %q has invalid or unselected %s id %q", evidence.ID, label, value)
			}
		}
	}
	if evidence.Kind != "test" && evidence.Kind != "observation" && evidence.Kind != "artifact" && evidence.Kind != "attestation" {
		return fmt.Errorf("evidence %q has invalid kind %q", evidence.ID, evidence.Kind)
	}
	if evidence.Boundary != "component" && evidence.Boundary != "assembled" && evidence.Boundary != "live" {
		return fmt.Errorf("evidence %q has invalid boundary %q", evidence.ID, evidence.Boundary)
	}
	if evidence.UsesMocks && evidence.Boundary != "component" {
		return fmt.Errorf("evidence %q uses mocks above component boundary", evidence.ID)
	}
	if err := validateEnvironment(evidence.Environment); err != nil {
		return fmt.Errorf("evidence %q: %w", evidence.ID, err)
	}
	producer, exists := runs[evidence.ProducerRunID]
	if !exists {
		return fmt.Errorf("evidence %q names unknown producer run %q", evidence.ID, evidence.ProducerRunID)
	}
	if evidence.CandidateTree != submission.Candidate.Tree {
		return fmt.Errorf("evidence %q does not bind the candidate tree", evidence.ID)
	}
	capturedAt, err := parseRecordTime(evidence.CapturedAt, "evidence capture")
	if err != nil {
		return err
	}
	producerStart, _ := parseRecordTime(producer.StartedAt, "producer start")
	producerEnd, _ := parseRecordTime(producer.CompletedAt, "producer completion")
	if capturedAt.Before(producerStart) || capturedAt.After(producerEnd) || capturedAt.After(createdAt) {
		return fmt.Errorf("evidence %q capture is outside its producer window", evidence.ID)
	}
	if err := validateArtifact(evidence.Artifact, "evidence artifact"); err != nil {
		return fmt.Errorf("evidence %q: %w", evidence.ID, err)
	}
	if !nonEmpty(evidence.Observed) || (evidence.Notes != "" && !nonEmpty(evidence.Notes)) {
		return fmt.Errorf("evidence %q requires a non-empty observation", evidence.ID)
	}
	return nil
}

func validateArtifact(artifact Artifact, label string) error {
	if !nonEmpty(artifact.Ref) || !mediaTypePattern.MatchString(artifact.MediaType) || !digestPattern.MatchString(artifact.Digest) {
		return fmt.Errorf("%s requires a valid reference, media type, and digest", label)
	}
	return nil
}

// ValidateArtifactContent applies Baton's media-type and strict I-JSON rules
// to exact artifact bytes. JSON artifacts are validated but never reserialized;
// their digest continues to cover the raw bytes supplied by the producer.
func ValidateArtifactContent(mediaType string, contents []byte) error {
	if !mediaTypePattern.MatchString(mediaType) {
		return fmt.Errorf("invalid artifact media type %q", mediaType)
	}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		if _, err := CanonicalizeJSON(contents); err != nil {
			return fmt.Errorf("invalid strict JSON artifact: %w", err)
		}
	}
	return nil
}

func validateEnvironment(environment Environment) error {
	switch environment.Kind {
	case "local", "ci", "sandbox", "staging", "production":
	default:
		return fmt.Errorf("invalid environment kind %q", environment.Kind)
	}
	if !nonEmpty(environment.Ref) {
		return errors.New("environment reference is required")
	}
	return nil
}

type recordTime struct {
	wholeSeconds int64
	fraction     string
}

func (left recordTime) Before(right recordTime) bool { return left.compare(right) < 0 }
func (left recordTime) After(right recordTime) bool  { return left.compare(right) > 0 }

func (left recordTime) compare(right recordTime) int {
	if left.wholeSeconds < right.wholeSeconds {
		return -1
	}
	if left.wholeSeconds > right.wholeSeconds {
		return 1
	}
	length := max(len(left.fraction), len(right.fraction))
	for index := 0; index < length; index++ {
		leftDigit, rightDigit := byte('0'), byte('0')
		if index < len(left.fraction) {
			leftDigit = left.fraction[index]
		}
		if index < len(right.fraction) {
			rightDigit = right.fraction[index]
		}
		if leftDigit < rightDigit {
			return -1
		}
		if leftDigit > rightDigit {
			return 1
		}
	}
	return 0
}

func parseRecordTime(value, label string) (recordTime, error) {
	parts := recordTimePattern.FindStringSubmatch(value)
	invalid := func() (recordTime, error) {
		return recordTime{}, fmt.Errorf("invalid %s time %q", label, value)
	}
	if parts == nil {
		return invalid()
	}
	year, _ := strconv.Atoi(parts[1][:4])
	hour, _ := strconv.Atoi(parts[2])
	minute, _ := strconv.Atoi(parts[3])
	second, _ := strconv.Atoi(parts[4])
	if year == 0 || hour > 23 || minute > 59 || second > 59 {
		return invalid()
	}
	zone := parts[6]
	if zone == "z" {
		zone = "Z"
	}
	if zone != "Z" {
		zoneHour, _ := strconv.Atoi(zone[1:3])
		zoneMinute, _ := strconv.Atoi(zone[4:6])
		if zoneHour > 23 || zoneMinute > 59 {
			return invalid()
		}
	}
	base := parts[1] + "T" + parts[2] + ":" + parts[3] + ":" + parts[4] + zone
	parsed, err := time.Parse("2006-01-02T15:04:05Z07:00", base)
	if err != nil {
		return invalid()
	}
	fraction := strings.TrimRight(strings.TrimPrefix(parts[5], "."), "0")
	return recordTime{wholeSeconds: parsed.Unix(), fraction: fraction}, nil
}

func nonEmpty(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != ""
}

func duplicateStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func validBranchRef(value string) bool {
	if !strings.HasPrefix(value, "refs/heads/") || !refCharacters.MatchString(value) ||
		strings.Contains(value, "//") || strings.Contains(value, "..") {
		return false
	}
	branch := strings.TrimPrefix(value, "refs/heads/")
	if branch == "" || !isASCIIAlphaNumeric(branch[0]) {
		return false
	}
	last := branch[len(branch)-1]
	if !isASCIIAlphaNumeric(last) && last != '_' && last != '-' {
		return false
	}
	for _, segment := range strings.Split(branch, "/") {
		if strings.HasPrefix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return false
		}
	}
	return true
}

func validGitPath(path string) bool {
	if path == "" || path == "." || !utf8.ValidString(path) || strings.HasPrefix(path, "/") ||
		strings.HasSuffix(path, "/") || strings.Contains(path, "\x00") ||
		strings.Contains(path, "//") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}
