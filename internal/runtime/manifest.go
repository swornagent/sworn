package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
)

const (
	ManifestVersion  = "sworn.runtime-manifest/v1"
	MaxManifestBytes = 2 * 1024 * 1024
)

var (
	runtimeIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	runtimeDigestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	repositoryPattern      = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	markerPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
)

type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return "runtime: " + e.Code
	}
	return "runtime: " + e.Code + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

func runtimeFail(code string, err error) error { return &Error{Code: code, Err: err} }

func IsCode(err error, code string) bool {
	var runtimeErr *Error
	return errors.As(err, &runtimeErr) && runtimeErr.Code == code
}

type ApprovalPolicy struct {
	Repository          string   `json:"repository"`
	Issue               int64    `json:"issue"`
	Marker              string   `json:"marker"`
	AllowedAuthorIDs    []int64  `json:"allowed_author_ids"`
	AllowedAssociations []string `json:"allowed_associations"`
}

type FakeDriverConfig struct {
	Executable string `json:"executable"`
	Digest     string `json:"digest"`
	AdapterKey string `json:"adapter_key"`
	Profile    string `json:"profile"`
}

type ScriptedSubmissions struct {
	PlannerProposal           string `json:"planner_proposal"`
	ImplementerDesign         string `json:"implementer_design"`
	CaptainReview             string `json:"captain_review"`
	ImplementerImplementation string `json:"implementer_implementation"`
	WorkVerification          string `json:"work_verification"`
	AssemblyVerification      string `json:"assembly_verification"`
}

func (s ScriptedSubmissions) forResponsibility(
	responsibility driver.Responsibility,
) string {
	switch responsibility {
	case driver.PlannerProposal:
		return s.PlannerProposal
	case driver.ImplementerDesign:
		return s.ImplementerDesign
	case driver.CaptainReview:
		return s.CaptainReview
	case driver.ImplementerImplementation:
		return s.ImplementerImplementation
	case driver.WorkVerification:
		return s.WorkVerification
	case driver.AssemblyVerification:
		return s.AssemblyVerification
	default:
		return ""
	}
}

type Manifest struct {
	SchemaVersion string                `json:"schema_version"`
	RunID         string                `json:"run_id"`
	Repository    string                `json:"repository"`
	Release       string                `json:"release"`
	TargetRef     string                `json:"target_ref"`
	Intent        string                `json:"intent"`
	ActiveTrack   string                `json:"active_track"`
	ActiveSlice   string                `json:"active_slice"`
	Approval      ApprovalPolicy        `json:"approval"`
	Driver        FakeDriverConfig      `json:"driver"`
	Roles         driver.RoleSelections `json:"roles"`
	Limits        driver.Limits         `json:"limits"`
	Submissions   ScriptedSubmissions   `json:"scripted_submissions"`
}

type admittedManifest struct {
	value  Manifest
	raw    []byte
	digest string
}

func ParseManifest(body []byte) (Manifest, error) {
	admitted, err := admitManifest(body)
	if err != nil {
		return Manifest{}, err
	}
	return admitted.value, nil
}

func admitManifest(body []byte) (admittedManifest, error) {
	if len(body) < 2 || len(body) > MaxManifestBytes ||
		body[len(body)-1] != '\n' || !utf8.Valid(body) ||
		bytes.ContainsRune(body, '\r') {
		return admittedManifest{}, runtimeFail("INVALID_MANIFEST", nil)
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return admittedManifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return admittedManifest{}, runtimeFail("INVALID_MANIFEST", nil)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return admittedManifest{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return admittedManifest{}, err
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(append(canonical, '\n'), body) {
		return admittedManifest{}, runtimeFail("NONCANONICAL_MANIFEST", nil)
	}
	raw := append([]byte(nil), body...)
	return admittedManifest{
		value: manifest, raw: raw, digest: sha256Digest(raw),
	}, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != ManifestVersion {
		return runtimeFail("INVALID_MANIFEST_VERSION", nil)
	}
	for label, value := range map[string]string{
		"run": manifest.RunID, "release": manifest.Release,
		"track": manifest.ActiveTrack, "slice": manifest.ActiveSlice,
	} {
		if !runtimeIdentityPattern.MatchString(value) {
			return runtimeFail("INVALID_"+strings.ToUpper(label), nil)
		}
	}
	if !filepath.IsAbs(manifest.Repository) ||
		filepath.Clean(manifest.Repository) != manifest.Repository {
		return runtimeFail("INVALID_REPOSITORY", nil)
	}
	if err := gitx.ValidateHeadRef(manifest.TargetRef); err != nil ||
		!strings.HasPrefix(manifest.TargetRef, "refs/heads/") {
		return runtimeFail("INVALID_TARGET_REF", nil)
	}
	if strings.TrimSpace(manifest.Intent) == "" ||
		len([]byte(manifest.Intent)) > 8_192 ||
		!utf8.ValidString(manifest.Intent) ||
		strings.ContainsAny(manifest.Intent, "\x00\r") {
		return runtimeFail("INVALID_INTENT", nil)
	}
	if err := validateApprovalPolicy(manifest.Approval); err != nil {
		return err
	}
	if !filepath.IsAbs(manifest.Driver.Executable) ||
		!runtimeDigestPattern.MatchString(manifest.Driver.Digest) ||
		!runtimeIdentityPattern.MatchString(manifest.Driver.AdapterKey) ||
		!runtimeIdentityPattern.MatchString(manifest.Driver.Profile) {
		return runtimeFail("INVALID_DRIVER", nil)
	}
	if err := driver.ValidateRoleSelections(manifest.Roles); err != nil {
		return runtimeFail("INVALID_ROLES", nil)
	}
	for _, role := range []driver.RoleSelection{
		manifest.Roles.Planner,
		manifest.Roles.Implementer,
		manifest.Roles.Captain,
		manifest.Roles.Verifier,
	} {
		if role.Profile != manifest.Driver.Profile {
			return runtimeFail("INVALID_ROLES", nil)
		}
	}
	if manifest.Limits.TimeoutMillis < 1 ||
		manifest.Limits.TimeoutMillis > driver.MaxTimeoutMillis ||
		manifest.Limits.OutputBytes < 1 ||
		manifest.Limits.OutputBytes > driver.MaxProviderOutputBytes {
		return runtimeFail("INVALID_LIMITS", nil)
	}
	for _, responsibility := range []driver.Responsibility{
		driver.PlannerProposal,
		driver.ImplementerDesign,
		driver.CaptainReview,
		driver.ImplementerImplementation,
		driver.WorkVerification,
		driver.AssemblyVerification,
	} {
		encoded := manifest.Submissions.forResponsibility(responsibility)
		if encoded == "" || len(encoded) > 3*driver.MaxSubmissionBytes {
			return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
		}
		body, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || base64.StdEncoding.EncodeToString(body) != encoded {
			return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
		}
		submission, err := driver.DecodeSubmission(body)
		if err != nil || submission.Responsibility != responsibility ||
			submission.InvocationID != invocationID(manifest.RunID, responsibility) {
			return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
		}
	}
	return nil
}

func validateApprovalPolicy(policy ApprovalPolicy) error {
	if !repositoryPattern.MatchString(policy.Repository) ||
		policy.Issue < 1 || policy.Issue > driver.MaxSafeInteger ||
		!markerPattern.MatchString(policy.Marker) ||
		len(policy.AllowedAuthorIDs) == 0 ||
		len(policy.AllowedAuthorIDs) > 64 ||
		len(policy.AllowedAssociations) == 0 ||
		len(policy.AllowedAssociations) > 8 {
		return runtimeFail("INVALID_APPROVAL_POLICY", nil)
	}
	for index, id := range policy.AllowedAuthorIDs {
		if id < 1 || id > driver.MaxSafeInteger ||
			(index > 0 && id <= policy.AllowedAuthorIDs[index-1]) {
			return runtimeFail("INVALID_APPROVAL_POLICY", nil)
		}
	}
	allowed := map[string]bool{
		"COLLABORATOR": true,
		"MEMBER":       true,
		"OWNER":        true,
	}
	for index, association := range policy.AllowedAssociations {
		if !allowed[association] ||
			(index > 0 && association <= policy.AllowedAssociations[index-1]) {
			return runtimeFail("INVALID_APPROVAL_POLICY", nil)
		}
	}
	return nil
}

func invocationID(runID string, responsibility driver.Responsibility) string {
	return runID + "/" + string(responsibility) + "/1"
}

func sha256Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil {
		return runtimeFail("INVALID_MANIFEST", nil)
	}
	return requireJSONEOF(decoder)
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not text")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("duplicate object key")
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("array is not closed")
		}
	default:
		return errors.New("unexpected delimiter")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return runtimeFail("INVALID_MANIFEST", nil)
	}
	return nil
}

func canonicalManifest(manifest Manifest) ([]byte, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return nil, runtimeFail("INVALID_MANIFEST", nil)
	}
	return append(body, '\n'), nil
}

func containsInt64(sortedValues []int64, value int64) bool {
	index := sort.Search(len(sortedValues), func(index int) bool {
		return sortedValues[index] >= value
	})
	return index < len(sortedValues) && sortedValues[index] == value
}

func containsString(sortedValues []string, value string) bool {
	index := sort.SearchStrings(sortedValues, value)
	return index < len(sortedValues) && sortedValues[index] == value
}
