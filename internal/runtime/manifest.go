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
	"time"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
)

const (
	ManifestVersionV2 = "sworn.runtime-manifest/v2"
	ManifestVersionV3 = "sworn.runtime-manifest/v3"
	ManifestVersionV4 = "sworn.runtime-manifest/v4"
	ManifestVersion   = "sworn.runtime-manifest/v5"
	MaxManifestBytes  = 2 * 1024 * 1024
	MaxParallelTracks = 8
)

var (
	runtimeIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	runtimeDigestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
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
	if errors.As(err, &runtimeErr) && runtimeErr.Code == code {
		return true
	}
	var gitErr *gitx.Error
	if errors.As(err, &gitErr) && gitErr.Code == code {
		return true
	}
	return false
}

type OwnerTransitionError struct {
	ExpiresAt time.Time
}

func (e *OwnerTransitionError) Error() string {
	if e == nil || e.ExpiresAt.IsZero() {
		return "owner lease active"
	}
	return fmt.Sprintf("owner lease active until %s", e.ExpiresAt.UTC().Format(time.RFC3339))
}

func OwnerLeaseExpiry(err error) (time.Time, bool) {
	var transition *OwnerTransitionError
	if errors.As(err, &transition) && transition != nil && !transition.ExpiresAt.IsZero() {
		return transition.ExpiresAt, true
	}
	return time.Time{}, false
}

type ProjectAuthority struct {
	Project                     string  `json:"project"`
	ExternalAuthorizer          string  `json:"external_authorizer"`
	BootstrapApprovedPlanDigest *string `json:"bootstrap_approved_plan_digest,omitempty"`
}

type FakeDriverConfig struct {
	Executable string `json:"executable"`
	Digest     string `json:"digest"`
	AdapterKey string `json:"adapter_key"`
	Profile    string `json:"profile"`
}

type AutomationSelections struct {
	Recovery driver.ModelSelection `json:"recovery"`
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
	GitIdentity        gitx.Identity         `json:"git_identity"`
	Intent             string                `json:"intent"`
	MaxParallelTracks  int                   `json:"max_parallel_tracks"`
	Authority          ProjectAuthority      `json:"authority"`
	Driver             *FakeDriverConfig     `json:"driver,omitempty"`
	DriverConfigDigest string                `json:"driver_config_digest,omitempty"`
	Roles              driver.RoleSelections `json:"roles"`
	Automation         *AutomationSelections `json:"automation,omitempty"`
	Limits             driver.Limits         `json:"limits"`
	Scripts            []ScriptedAttempt     `json:"scripted_attempts,omitempty"`
}

func (m Manifest) production() bool {
	return m.Driver == nil && m.DriverConfigDigest != ""
}

func (m Manifest) EffectiveDegradationBudget() int64 {
	return m.Limits.EffectiveDegradationBudget()
}

func (m Manifest) recoverySelection() (driver.ModelSelection, bool) {
	if m.SchemaVersion != ManifestVersion || m.Automation == nil {
		return driver.ModelSelection{}, false
	}
	return m.Automation.Recovery, true
}

type admittedManifest struct {
	value         Manifest
	raw           []byte
	digest        string
	legacyVersion string
}

func admitStoredManifest(body []byte) (admittedManifest, error) {
	version, err := classifyManifestVersion(body)
	if err != nil {
		return admittedManifest{}, err
	}
	if version == ManifestVersionV2 || version == ManifestVersionV3 || version == ManifestVersionV4 {
		return admittedManifest{
			raw: append([]byte(nil), body...), digest: sha256Digest(body),
			legacyVersion: version,
		}, nil
	}
	return admitManifest(body)
}

func ParseManifest(body []byte) (Manifest, error) {
	admitted, err := admitManifest(body)
	if err != nil {
		return Manifest{}, err
	}
	return admitted.value, nil
}

func admitManifest(body []byte) (admittedManifest, error) {
	version, err := classifyManifestVersion(body)
	if err != nil {
		return admittedManifest{}, err
	}
	if version == ManifestVersionV2 || version == ManifestVersionV3 || version == ManifestVersionV4 {
		return admittedManifest{}, runtimeFail("MIGRATION_REQUIRED", nil)
	}
	if version != ManifestVersion {
		return admittedManifest{}, runtimeFail("INVALID_MANIFEST_VERSION", nil)
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

// ClassifyManifestVersion performs the bounded, authority-free inspection used
// by read-only discovery. Legacy approval fields are never decoded or trusted.
func ClassifyManifestVersion(body []byte) (string, error) {
	return classifyManifestVersion(body)
}

type ManifestIdentity struct {
	SchemaVersion string
	Repository    string
	Release       string
}

// InspectManifestIdentity reads only the project binding needed for read-only
// discovery. In particular it never decodes legacy approval authority.
func InspectManifestIdentity(body []byte) (ManifestIdentity, error) {
	version, err := classifyManifestVersion(body)
	if err != nil {
		return ManifestIdentity{}, err
	}
	var wire struct {
		Repository string `json:"repository"`
		Release    string `json:"release"`
	}
	if json.Unmarshal(body, &wire) != nil ||
		!filepath.IsAbs(wire.Repository) ||
		filepath.Clean(wire.Repository) != wire.Repository ||
		!runtimeIdentityPattern.MatchString(wire.Release) {
		return ManifestIdentity{}, runtimeFail("INVALID_MANIFEST", nil)
	}
	return ManifestIdentity{
		SchemaVersion: version,
		Repository:    wire.Repository,
		Release:       wire.Release,
	}, nil
}

func classifyManifestVersion(body []byte) (string, error) {
	if len(body) < 2 || len(body) > MaxManifestBytes ||
		body[len(body)-1] != '\n' || !utf8.Valid(body) ||
		bytes.ContainsRune(body, '\r') {
		return "", runtimeFail("INVALID_MANIFEST", nil)
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := decoder.Decode(&envelope); err != nil || envelope.SchemaVersion == "" {
		return "", runtimeFail("INVALID_MANIFEST", nil)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return "", err
	}
	switch envelope.SchemaVersion {
	case ManifestVersionV2, ManifestVersionV3, ManifestVersionV4, ManifestVersion:
		return envelope.SchemaVersion, nil
	default:
		return "", runtimeFail("INVALID_MANIFEST_VERSION", nil)
	}
}

func validateManifest(manifest Manifest) error {
	switch manifest.SchemaVersion {
	case ManifestVersion:
		if manifest.Automation == nil ||
			driver.ValidateModelSelection(
				manifest.Automation.Recovery,
			) != nil {
			return runtimeFail("INVALID_AUTOMATION", nil)
		}
	default:
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
	if err := gitx.ValidateIdentity(manifest.GitIdentity); err != nil {
		return runtimeFail("INVALID_GIT_IDENTITY", err)
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
	if err := validateProjectAuthority(manifest.Authority); err != nil {
		return err
	}
	if err := driver.ValidateRoleSelections(manifest.Roles); err != nil {
		return runtimeFail("INVALID_ROLES", nil)
	}
	if manifest.Limits.TimeoutMillis < 1 ||
		manifest.Limits.TimeoutMillis > driver.MaxTimeoutMillis ||
		manifest.Limits.OutputBytes < 1 ||
		manifest.Limits.OutputBytes > driver.MaxProviderOutputBytes ||
		manifest.Limits.DegradationBudget < 0 ||
		manifest.Limits.DegradationBudget > driver.MaxDegradationBudget {
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
		if recovery, enabled := manifest.recoverySelection(); enabled &&
			recovery.Profile != manifest.Driver.Profile {
			return runtimeFail("INVALID_AUTOMATION", nil)
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

func validateProjectAuthority(authority ProjectAuthority) error {
	// Both names are deliberately the same small identifier language. This
	// excludes URLs, accounts, credentials and provider-qualified identities.
	if !runtimeIdentityPattern.MatchString(authority.Project) ||
		!runtimeIdentityPattern.MatchString(authority.ExternalAuthorizer) {
		return runtimeFail("INVALID_AUTHORITY", nil)
	}
	if authority.BootstrapApprovedPlanDigest != nil &&
		!runtimeDigestPattern.MatchString(*authority.BootstrapApprovedPlanDigest) {
		return runtimeFail("INVALID_AUTHORITY", nil)
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

func containsControlCharacter(value string) bool {
	for _, r := range value {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}

func containsString(sortedValues []string, value string) bool {
	index := sort.SearchStrings(sortedValues, value)
	return index < len(sortedValues) && sortedValues[index] == value
}
