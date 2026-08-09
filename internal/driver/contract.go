package driver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	DriverContractVersion        = "sworn.driver/v1"
	RequestSchemaVersion         = "sworn.driver-request/v1"
	ResultSchemaVersion          = "sworn.driver-result/v1"
	OperationVersion             = "baton.operation/v2"
	GuestWorkspacePath           = "/workspace"
	GuestInputPath               = "/sworn/inputs"
	MaxRequestBytes              = 1_048_576
	MaxInstructionBytes          = 262_144
	MaxProviderOutputBytes       = 1_048_576
	MaxInputs                    = 256
	MaxTimeoutMillis             = 86_400_000
	MaxSafeInteger         int64 = 9_007_199_254_740_991
)

var (
	identityPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)
	driverIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	versionPattern        = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)
	digestPattern         = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ContractError carries a stable code and never request, model, stderr, or secret bytes.
type ContractError struct {
	Code string
}

func (e *ContractError) Error() string { return "driver contract: " + e.Code }
func fail(code string) error           { return &ContractError{Code: code} }
func IsCode(err error, code string) bool {
	var contractErr *ContractError
	return errors.As(err, &contractErr) && contractErr.Code == code
}

type Role string

const (
	RolePlanner     Role = "planner"
	RoleImplementer Role = "implementer"
	RoleCaptain     Role = "captain"
	RoleVerifier    Role = "verifier"
)

func (r Role) valid() bool {
	return r == RolePlanner || r == RoleImplementer || r == RoleCaptain ||
		r == RoleVerifier
}

var operationForRole = map[Role]string{
	RolePlanner:     "baton-plan",
	RoleImplementer: "baton-implement",
	RoleCaptain:     "baton-design-review",
	RoleVerifier:    "baton-verify",
}

type Operation struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Digest       string `json:"digest"`
	Instructions string `json:"instructions"`
}
type PackageIdentity struct {
	Version        string `json:"version"`
	ManifestSHA256 string `json:"manifest_sha256"`
}
type WorkspaceAccess string

const (
	ReadOnly  WorkspaceAccess = "read_only"
	ReadWrite WorkspaceAccess = "read_write"
)

type Workspace struct {
	Path   string          `json:"path"`
	Access WorkspaceAccess `json:"access"`
}
type Input struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
}
type Limits struct {
	TimeoutMillis int64 `json:"timeout_ms"`
	OutputBytes   int64 `json:"output_bytes"`
}
type Request struct {
	SchemaVersion string    `json:"schema_version"`
	InvocationID  string    `json:"invocation_id"`
	Role          Role      `json:"role"`
	Operation     Operation `json:"operation"`
	Profile       string    `json:"profile"`
	Model         string    `json:"model"`
	Workspace     Workspace `json:"workspace"`
	Inputs        []Input   `json:"inputs"`
	// FreshContext describes a well-formed request. Execution routes decide
	// whether a nonfresh request has the continuation authority it requires.
	FreshContext bool   `json:"fresh_context"`
	Limits       Limits `json:"limits"`
}
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	// CacheReadTokens and CacheWriteTokens are the normalized cache-accounting
	// pair surfaced from provider responses. Each side is nil (omitted) when
	// the provider vocabulary reports only one side (Gemini and the Responses
	// API report reads only); a nil side is honest absence, never a measured
	// zero.
	CacheReadTokens  *int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64 `json:"cache_write_tokens,omitempty"`
}
type TransportStatus string

const (
	Completed      TransportStatus = "completed"
	TransportError TransportStatus = "transport_error"
	TimedOut       TransportStatus = "timeout"
	Cancelled      TransportStatus = "cancelled"
	RunnerError    TransportStatus = "runner_error"
)

func (s TransportStatus) valid() bool {
	return s == Completed || s == TransportError || s == TimedOut ||
		s == Cancelled || s == RunnerError
}

type Result struct {
	SchemaVersion   string           `json:"schema_version"`
	InvocationID    string           `json:"invocation_id"`
	AdapterID       string           `json:"adapter_id"`
	AdapterVersion  string           `json:"adapter_version"`
	ObservedModel   string           `json:"observed_model"`
	DurationMillis  int64            `json:"duration_ms"`
	TransportStatus TransportStatus  `json:"transport_status"`
	Usage           *Usage           `json:"usage,omitempty"`
	Cost            *CostObservation `json:"cost,omitempty"`
}
type ResultBinding struct {
	InvocationID, AdapterID, AdapterVersion string
	Model                                   string
	BindModel                               bool
}
type DriverInfo struct {
	ContractVersion string `json:"contract_version"`
	AdapterID       string `json:"adapter_id"`
	AdapterVersion  string `json:"adapter_version"`
}
type DriverInfoBinding struct {
	AdapterID, AdapterVersion string
}

func ValidateDriverInfo(info DriverInfo, expected DriverInfoBinding) error {
	if info.ContractVersion != DriverContractVersion {
		return fail("INVALID_VERSION")
	}
	if !driverIdentityPattern.MatchString(info.AdapterID) ||
		!versionPattern.MatchString(info.AdapterVersion) {
		return fail("INVALID_DRIVER")
	}
	if expected.AdapterID != "" && info.AdapterID != expected.AdapterID {
		return fail("DRIVER_BINDING_MISMATCH")
	}
	if expected.AdapterVersion != "" && info.AdapterVersion != expected.AdapterVersion {
		return fail("DRIVER_BINDING_MISMATCH")
	}
	return nil
}
func EncodeDriverInfo(info DriverInfo) ([]byte, error) {
	return encodeValidated(ValidateDriverInfo(info, DriverInfoBinding{}), info)
}
func DecodeDriverInfo(body []byte, expected DriverInfoBinding) (DriverInfo, error) {
	var info DriverInfo
	if _, err := decodeTyped(
		body,
		4_096,
		[]string{"contract_version", "adapter_id", "adapter_version"},
		nil,
		&info,
	); err != nil {
		return DriverInfo{}, err
	}
	if err := ValidateDriverInfo(info, expected); err != nil {
		return DriverInfo{}, err
	}
	canonical, err := EncodeDriverInfo(info)
	if err != nil {
		return DriverInfo{}, err
	}
	if !bytes.Equal(canonical, body) {
		return DriverInfo{}, fail("NONCANONICAL_JSON")
	}
	return info, nil
}
func CanonicalOperation(role Role) (Operation, error) {
	id, ok := operationForRole[role]
	if !ok {
		return Operation{}, fail("INVALID_ROLE")
	}
	pkg, _, err := admittedPackage()
	if err != nil {
		return Operation{}, fail("INVALID_PACKAGE")
	}
	body, err := pkg.ReadAsset("operations/" + id + ".md")
	if err != nil {
		return Operation{}, fail("INVALID_PACKAGE")
	}
	if len(body) == 0 || len(body) > MaxInstructionBytes || !utf8.Valid(body) ||
		body[len(body)-1] != '\n' || bytes.ContainsRune(body, '\r') {
		return Operation{}, fail("INVALID_PACKAGE")
	}
	return Operation{
		ID:           id,
		Version:      OperationVersion,
		Digest:       Digest(body),
		Instructions: string(body),
	}, nil
}

// admittedPackage loads Sworn's own embedded role-asset bundle. Admission is
// self-consistency only: the compiled bundle must match its own recorded
// digests. It never requires a separately installed, tagged, checked-out, or
// certified external Baton release.
func admittedPackage() (baton.Package, PackageIdentity, error) {
	pkg, err := baton.Load()
	if err != nil {
		return baton.Package{}, PackageIdentity{}, err
	}
	identity, err := pkg.Identity()
	if err != nil ||
		identity.RoleAssetsVersion != baton.RoleAssetsVersion ||
		identity.ManifestSHA256 != baton.ManifestSHA256 {
		return baton.Package{}, PackageIdentity{}, fail("INVALID_PACKAGE")
	}
	return pkg, PackageIdentity{
		Version:        identity.RoleAssetsVersion,
		ManifestSHA256: identity.ManifestSHA256,
	}, nil
}
func NewRequest(
	invocationID string,
	role Role,
	profile string,
	model string,
	workspace Workspace,
	inputs []Input,
	freshContext bool,
	limits Limits,
) (Request, error) {
	operation, err := CanonicalOperation(role)
	if err != nil {
		return Request{}, err
	}
	request := Request{
		SchemaVersion: RequestSchemaVersion,
		InvocationID:  invocationID,
		Role:          role,
		Operation:     operation,
		Profile:       profile,
		Model:         model,
		Workspace:     workspace,
		Inputs:        make([]Input, len(inputs)),
		FreshContext:  freshContext,
		Limits:        limits,
	}
	copy(request.Inputs, inputs)
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}
func ValidateRequest(request Request) error {
	if request.SchemaVersion != RequestSchemaVersion {
		return fail("INVALID_VERSION")
	}
	if err := validateIdentity(request.InvocationID); err != nil {
		return err
	}
	if !request.Role.valid() {
		return fail("INVALID_ROLE")
	}
	canonical, err := CanonicalOperation(request.Role)
	if err != nil {
		return err
	}
	if request.Operation != canonical {
		if request.Operation.ID != canonical.ID {
			return fail("OPERATION_ROLE_MISMATCH")
		}
		return fail("STALE_OPERATION")
	}
	if !providerKeyPattern.MatchString(request.Profile) {
		return fail("INVALID_PROFILE")
	}
	if validateText(request.Model, 500, false) != nil {
		return fail("INVALID_MODEL")
	}
	if err := validateWorkspace(request.Workspace); err != nil {
		return err
	}
	if request.Role == RoleVerifier &&
		request.Workspace.Access != ReadOnly {
		return fail("INVALID_VERIFIER")
	}
	if request.Inputs == nil {
		return fail("INVALID_FIELD")
	}
	if len(request.Inputs) > MaxInputs {
		return fail("RESOURCE_LIMIT")
	}
	names := make(map[string]struct{}, len(request.Inputs))
	paths := make(map[string]struct{}, len(request.Inputs))
	for _, input := range request.Inputs {
		if !driverIdentityPattern.MatchString(input.Name) {
			return fail("INVALID_INPUT")
		}
		if err := validateRepositoryPath(input.Path); err != nil {
			return err
		}
		if !digestPattern.MatchString(input.Digest) {
			return fail("INVALID_DIGEST")
		}
		if _, exists := names[input.Name]; exists {
			return fail("DUPLICATE_INPUT")
		}
		if _, exists := paths[input.Path]; exists {
			return fail("DUPLICATE_INPUT")
		}
		names[input.Name] = struct{}{}
		paths[input.Path] = struct{}{}
	}
	if request.Limits.TimeoutMillis < 1 || request.Limits.TimeoutMillis > MaxTimeoutMillis {
		return fail("INVALID_LIMIT")
	}
	if request.Limits.OutputBytes < 1 || request.Limits.OutputBytes > MaxProviderOutputBytes {
		return fail("INVALID_LIMIT")
	}
	body, err := json.Marshal(request)
	if err != nil || len(body)+1 > MaxRequestBytes {
		return fail("RESOURCE_LIMIT")
	}
	return nil
}
func EncodeRequest(request Request) ([]byte, error) {
	return encodeValidated(ValidateRequest(request), request)
}
func DecodeRequest(body []byte) (Request, error) {
	var request Request
	if _, err := decodeTyped(
		body,
		MaxRequestBytes,
		[]string{
			"schema_version", "invocation_id", "role", "operation", "profile", "model",
			"workspace", "inputs", "fresh_context", "limits",
		},
		nil,
		&request,
	); err != nil {
		return Request{}, err
	}
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	canonical, err := EncodeRequest(request)
	if err != nil {
		return Request{}, err
	}
	if !bytes.Equal(canonical, body) {
		return Request{}, fail("NONCANONICAL_JSON")
	}
	return request, nil
}
func ValidateResult(result Result, expected ResultBinding) error {
	if result.SchemaVersion != ResultSchemaVersion {
		return fail("INVALID_VERSION")
	}
	if err := validateIdentity(result.InvocationID); err != nil {
		return err
	}
	if !driverIdentityPattern.MatchString(result.AdapterID) ||
		!versionPattern.MatchString(result.AdapterVersion) {
		return fail("INVALID_DRIVER")
	}
	if err := validateText(result.ObservedModel, 500, false); err != nil {
		return fail("INVALID_MODEL")
	}
	if result.DurationMillis < 0 || result.DurationMillis > MaxSafeInteger {
		return fail("INVALID_FIELD")
	}
	if !result.TransportStatus.valid() {
		return fail("INVALID_TRANSPORT_STATUS")
	}
	if result.Usage != nil {
		if result.Usage.InputTokens < 0 || result.Usage.InputTokens > MaxSafeInteger ||
			result.Usage.OutputTokens < 0 || result.Usage.OutputTokens > MaxSafeInteger {
			return fail("INVALID_USAGE")
		}
		if result.Usage.CacheReadTokens != nil &&
			(*result.Usage.CacheReadTokens < 0 ||
				*result.Usage.CacheReadTokens > MaxSafeInteger) {
			return fail("INVALID_USAGE")
		}
		if result.Usage.CacheWriteTokens != nil &&
			(*result.Usage.CacheWriteTokens < 0 ||
				*result.Usage.CacheWriteTokens > MaxSafeInteger) {
			return fail("INVALID_USAGE")
		}
	}
	if result.Cost != nil {
		if err := validateCostObservation(*result.Cost); err != nil {
			return err
		}
	}
	if expected.InvocationID != "" && result.InvocationID != expected.InvocationID {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.AdapterID != "" && result.AdapterID != expected.AdapterID {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.AdapterVersion != "" && result.AdapterVersion != expected.AdapterVersion {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.BindModel {
		if result.ObservedModel != expected.Model {
			return fail("RESULT_BINDING_MISMATCH")
		}
	}
	return nil
}
func EncodeResult(result Result) ([]byte, error) {
	return encodeValidated(ValidateResult(result, ResultBinding{}), result)
}
func DecodeResult(body []byte, expected ResultBinding) (Result, error) {
	if len(body) < 2 || body[len(body)-1] != '\n' {
		return Result{}, fail("INVALID_RESULT")
	}
	var result Result
	root, err := decodeTyped(
		body,
		8*1024*1024,
		[]string{
			"schema_version", "invocation_id", "adapter_id", "adapter_version",
			"observed_model", "duration_ms", "transport_status",
		},
		[]string{"usage", "cost"},
		&result,
	)
	if err != nil {
		return Result{}, err
	}
	if usage, present := root["usage"]; present {
		if _, err := closedObject(
			usage,
			[]string{"input_tokens", "output_tokens"},
			[]string{"cache_read_tokens", "cache_write_tokens"},
		); err != nil {
			return Result{}, err
		}
	}
	if cost, present := root["cost"]; present {
		if _, err := closedObject(
			cost,
			[]string{"micro_units", "currency", "source"},
			nil,
		); err != nil {
			return Result{}, err
		}
	}
	if err := ValidateResult(result, expected); err != nil {
		return Result{}, err
	}
	canonical, err := EncodeResult(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(canonical, body) {
		return Result{}, fail("NONCANONICAL_JSON")
	}
	return result, nil
}
func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func encodeValidated(validation error, value any) ([]byte, error) {
	if validation != nil {
		return nil, validation
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return append(body, '\n'), nil
}
func validateIdentity(value string) error {
	if !identityPattern.MatchString(value) {
		return fail("INVALID_IDENTITY")
	}
	return nil
}
func validateText(value string, maximum int, allowEmpty bool) error {
	if (!allowEmpty && value == "") || len([]byte(value)) > maximum || !utf8.ValidString(value) {
		return fail("INVALID_FIELD")
	}
	if containsControlCharacter(value) {
		return fail("INVALID_FIELD")
	}
	return nil
}
func validateWorkspace(workspace Workspace) error {
	if workspace.Path == "" || len([]byte(workspace.Path)) > 4096 ||
		!utf8.ValidString(workspace.Path) || containsControlCharacter(workspace.Path) ||
		!filepath.IsAbs(workspace.Path) || filepath.Clean(workspace.Path) != workspace.Path ||
		strings.ContainsRune(workspace.Path, '\x00') {
		return fail("INVALID_WORKSPACE")
	}
	if workspace.Access != ReadOnly && workspace.Access != ReadWrite {
		return fail("INVALID_ACCESS")
	}
	return nil
}
func validateRepositoryPath(value string) error {
	if value == "" || len([]byte(value)) > 1000 || !utf8.ValidString(value) ||
		containsControlCharacter(value) || strings.Contains(value, `\`) ||
		path.IsAbs(value) || path.Clean(value) != value {
		return fail("INVALID_PATH")
	}
	segments := strings.Split(value, "/")
	switch segments[0] {
	case ".git", ".baton", ".sworn":
		return fail("INVALID_PATH")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return fail("INVALID_PATH")
		}
	}
	return nil
}
func containsControlCharacter(value string) bool {
	for _, r := range value {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func decodeStrict(body []byte, maximum int) (any, error) {
	if len(body) == 0 {
		return nil, fail("MISSING_JSON")
	}
	if len(body) > maximum {
		return nil, fail("RESOURCE_LIMIT")
	}
	if !utf8.Valid(body) {
		return nil, fail("INVALID_UTF8")
	}
	if !validJSONUnicodeEscapes(body) {
		return nil, fail("INVALID_UNICODE")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fail("TRAILING_JSON")
	}
	return value, nil
}
func decodeTyped(
	body []byte,
	maximum int,
	required, optional []string,
	target any,
) (map[string]any, error) {
	value, err := decodeStrict(body, maximum)
	if err != nil {
		return nil, err
	}
	root, err := closedObject(value, required, optional)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return nil, fail("UNKNOWN_FIELD")
		}
		return nil, fail("INVALID_FIELD")
	}
	return root, nil
}
func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				nameToken, err := decoder.Token()
				if err != nil {
					return nil, fail("INVALID_JSON")
				}
				name, ok := nameToken.(string)
				if !ok {
					return nil, fail("INVALID_JSON")
				}
				if _, duplicate := object[name]; duplicate {
					return nil, fail("DUPLICATE_NAME")
				}
				child, err := decodeValue(decoder)
				if err != nil {
					return nil, err
				}
				object[name] = child
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return nil, fail("INVALID_JSON")
			}
			return object, nil
		case '[':
			var array []any
			for decoder.More() {
				child, err := decodeValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, child)
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return nil, fail("INVALID_JSON")
			}
			return array, nil
		default:
			return nil, fail("INVALID_JSON")
		}
	case string:
		return value, nil
	case json.Number, bool, nil:
		return value, nil
	default:
		return nil, fail("INVALID_JSON")
	}
}
func validJSONUnicodeEscapes(body []byte) bool {
	inString := false
	for index := 0; index < len(body); index++ {
		switch body[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(body) {
				continue
			}
			index++
			if body[index] != 'u' {
				continue
			}
			if index+4 >= len(body) {
				return false
			}
			value, ok := parseHexCodeUnit(body[index+1 : index+5])
			if !ok {
				return true // The JSON decoder reports the syntax failure.
			}
			index += 4
			switch {
			case value >= 0xd800 && value <= 0xdbff:
				if index+6 >= len(body) || body[index+1] != '\\' || body[index+2] != 'u' {
					return false
				}
				low, ok := parseHexCodeUnit(body[index+3 : index+7])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				index += 6
			case value >= 0xdc00 && value <= 0xdfff:
				return false
			}
		}
	}
	return true
}
func parseHexCodeUnit(body []byte) (uint16, bool) {
	if len(body) != 4 {
		return 0, false
	}
	var value uint16
	for _, character := range body {
		value <<= 4
		switch {
		case character >= '0' && character <= '9':
			value |= uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value |= uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value |= uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
func closedObject(value any, required, optional []string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fail("INVALID_FIELD")
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = struct{}{}
		if _, present := object[key]; !present {
			return nil, fail("MISSING_FIELD")
		}
	}
	for _, key := range optional {
		allowed[key] = struct{}{}
	}
	for key := range object {
		if _, present := allowed[key]; !present {
			return nil, fail("UNKNOWN_FIELD")
		}
	}
	return object, nil
}
func canonicalJSON(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return body, nil
}
