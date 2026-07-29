package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	ManifestVersion   = "sworn.runtime-manifest/v2"
	MaxManifestBytes  = 2 * 1024 * 1024
	MaxParallelTracks = 8
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
	AllowedAuthorIDs    []int64  `json:"allowed_author_ids"`
	AllowedAssociations []string `json:"allowed_associations"`
}

type FakeDriverConfig struct {
	Executable string `json:"executable"`
	Digest     string `json:"digest"`
	AdapterKey string `json:"adapter_key"`
	Profile    string `json:"profile"`
}

type ScriptedAttempt struct {
	Slice          string                `json:"slice"`
	Responsibility driver.Responsibility `json:"responsibility"`
	BatonAttempt   int64                 `json:"baton_attempt"`
	Epoch          int64                 `json:"epoch"`
	Try            int64                 `json:"try"`
	Behavior       string                `json:"behavior"`
	Submission     string                `json:"submission,omitempty"`
}

func (m Manifest) script(slice string, responsibility driver.Responsibility, batonAttempt, epoch, try int64) (ScriptedAttempt, bool) {
	for _, script := range m.Scripts {
		if script.Slice == slice && script.Responsibility == responsibility &&
			script.BatonAttempt == batonAttempt && script.Epoch == epoch && script.Try == try {
			return script, true
		}
	}
	return ScriptedAttempt{}, false
}

type Manifest struct {
	SchemaVersion      string                `json:"schema_version"`
	RunID              string                `json:"run_id"`
	Repository         string                `json:"repository"`
	Release            string                `json:"release"`
	TargetRef          string                `json:"target_ref"`
	Intent             string                `json:"intent"`
	MaxParallelTracks  int                   `json:"max_parallel_tracks"`
	Approval           ApprovalPolicy        `json:"approval"`
	Driver             *FakeDriverConfig     `json:"driver,omitempty"`
	DriverConfigDigest string                `json:"driver_config_digest,omitempty"`
	Roles              driver.RoleSelections `json:"roles"`
	Limits             driver.Limits         `json:"limits"`
	Scripts            []ScriptedAttempt     `json:"scripted_attempts,omitempty"`
}

func (m Manifest) production() bool {
	return m.Driver == nil && m.DriverConfigDigest != ""
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
	if manifest.MaxParallelTracks < 1 || manifest.MaxParallelTracks > MaxParallelTracks {
		return runtimeFail("INVALID_PARALLELISM", nil)
	}
	if err := validateApprovalPolicy(manifest.Approval); err != nil {
		return err
	}
	if err := driver.ValidateRoleSelections(manifest.Roles); err != nil {
		return runtimeFail("INVALID_ROLES", nil)
	}
	if manifest.Limits.TimeoutMillis < 1 ||
		manifest.Limits.TimeoutMillis > driver.MaxTimeoutMillis ||
		manifest.Limits.OutputBytes < 1 ||
		manifest.Limits.OutputBytes > driver.MaxProviderOutputBytes {
		return runtimeFail("INVALID_LIMITS", nil)
	}
	switch {
	case manifest.Driver != nil && manifest.DriverConfigDigest == "":
		if !filepath.IsAbs(manifest.Driver.Executable) ||
			!runtimeDigestPattern.MatchString(manifest.Driver.Digest) ||
			!runtimeIdentityPattern.MatchString(manifest.Driver.AdapterKey) ||
			!runtimeIdentityPattern.MatchString(manifest.Driver.Profile) {
			return runtimeFail("INVALID_DRIVER", nil)
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
		return validateScriptedAttempts(manifest)
	case manifest.Driver == nil && manifest.DriverConfigDigest != "":
		if !runtimeDigestPattern.MatchString(manifest.DriverConfigDigest) {
			return runtimeFail("INVALID_DRIVER_CONFIG_DIGEST", nil)
		}
		if manifest.Scripts != nil {
			return runtimeFail("INVALID_MANIFEST_VARIANT", nil)
		}
		return nil
	default:
		return runtimeFail("INVALID_MANIFEST_VARIANT", nil)
	}
}

func validateScriptedAttempts(manifest Manifest) error {
	if len(manifest.Scripts) == 0 || len(manifest.Scripts) > 4096 {
		return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
	}
	previous := ""
	hasInitialPlan := false
	for _, script := range manifest.Scripts {
		if script.Slice != "" && !runtimeIdentityPattern.MatchString(script.Slice) ||
			script.BatonAttempt < 1 || script.Epoch < 1 || script.Try < 1 || script.Try > 3 {
			return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
		}
		key := fmt.Sprintf("%s/%s/%020d/%020d/%d", script.Responsibility,
			script.Slice, script.BatonAttempt, script.Epoch, script.Try)
		if key <= previous {
			return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
		}
		previous = key
		switch script.Behavior {
		case "submit":
			if script.Submission == "" || len(script.Submission) > 3*driver.MaxSubmissionBytes {
				return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
			}
			body, err := base64.StdEncoding.Strict().DecodeString(script.Submission)
			if err != nil || base64.StdEncoding.EncodeToString(body) != script.Submission {
				return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
			}
			submission, err := driver.DecodeSubmission(body)
			if err != nil || submission.Responsibility != script.Responsibility ||
				submission.InvocationID != invocationID(manifest.RunID, script) {
				return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
			}
		case "none", "usage_unavailable", "block", "attempt_workspace_write", "malformed_submission_frame":
			if script.Submission != "" {
				return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
			}
		default:
			return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
		}
		hasInitialPlan = hasInitialPlan || script.Responsibility == driver.PlannerProposal &&
			script.BatonAttempt == 1 && script.Epoch == 1 && script.Try == 1
	}
	if !hasInitialPlan {
		return runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
	}
	return nil
}

func validateApprovalPolicy(policy ApprovalPolicy) error {
	if !repositoryPattern.MatchString(policy.Repository) ||
		policy.Issue < 1 || policy.Issue > driver.MaxSafeInteger ||
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

func invocationID(runID string, script ScriptedAttempt) string {
	return dispatchInvocationID(runID, dispatchCoordinates{
		Slice:          script.Slice,
		Responsibility: script.Responsibility,
		BatonAttempt:   script.BatonAttempt,
		Epoch:          script.Epoch,
		Try:            script.Try,
	})
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
