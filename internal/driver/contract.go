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
	DriverContractVersion = "baton.driver/v1"
	RequestSchemaVersion  = "baton.driver-request/v1"
	ResultSchemaVersion   = "baton.driver-result/v1"
	OperationVersion      = "baton.operation/v1"

	MaxRequestBytes           = 1_048_576
	MaxInstructionBytes       = 262_144
	MaxResultTextBytes        = 1_048_576
	MaxInputs                 = 256
	MaxTimeoutMillis          = 86_400_000
	MaxSafeInteger      int64 = 9_007_199_254_740_991
)

var (
	identityPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)
	driverIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	versionPattern        = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)
	digestPattern         = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ContractError is deliberately value-free: errors crossing the invocation
// boundary carry a stable code, never request, model, stderr, or secret bytes.
type ContractError struct {
	Code string
}

func (e *ContractError) Error() string {
	return "driver contract: " + e.Code
}

func fail(code string) error {
	return &ContractError{Code: code}
}

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
	RoleMerge       Role = "merge"
)

func (r Role) valid() bool {
	switch r {
	case RolePlanner, RoleImplementer, RoleCaptain, RoleVerifier, RoleMerge:
		return true
	default:
		return false
	}
}

var operationForRole = map[Role]string{
	RolePlanner:     "baton-plan",
	RoleImplementer: "baton-implement",
	RoleCaptain:     "baton-design-review",
	RoleVerifier:    "baton-verify",
	RoleMerge:       "baton-merge",
}

type Operation struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Digest       string `json:"digest"`
	Instructions string `json:"instructions"`
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
	Model         *string   `json:"model"`
	Workspace     Workspace `json:"workspace"`
	Inputs        []Input   `json:"inputs"`
	FreshContext  bool      `json:"fresh_context"`
	Limits        Limits    `json:"limits"`
}

type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
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
	switch s {
	case Completed, TransportError, TimedOut, Cancelled, RunnerError:
		return true
	default:
		return false
	}
}

type Result struct {
	SchemaVersion   string          `json:"schema_version"`
	InvocationID    string          `json:"invocation_id"`
	DriverID        string          `json:"driver_id"`
	DriverVersion   string          `json:"driver_version"`
	ObservedModel   *string         `json:"observed_model"`
	DurationMillis  int64           `json:"duration_ms"`
	Text            string          `json:"text"`
	TransportStatus TransportStatus `json:"transport_status"`
	Usage           *Usage          `json:"usage,omitempty"`
}

type ResultBinding struct {
	InvocationID  string
	DriverID      string
	DriverVersion string
	Model         *string
	BindModel     bool
}

type DriverInfo struct {
	ContractVersion string `json:"contract_version"`
	DriverID        string `json:"driver_id"`
	DriverVersion   string `json:"driver_version"`
}

type DriverInfoBinding struct {
	DriverID      string
	DriverVersion string
}

func ValidateDriverInfo(info DriverInfo, expected DriverInfoBinding) error {
	if info.ContractVersion != DriverContractVersion {
		return fail("INVALID_VERSION")
	}
	if !driverIdentityPattern.MatchString(info.DriverID) ||
		!versionPattern.MatchString(info.DriverVersion) {
		return fail("INVALID_DRIVER")
	}
	if expected.DriverID != "" && info.DriverID != expected.DriverID {
		return fail("DRIVER_BINDING_MISMATCH")
	}
	if expected.DriverVersion != "" && info.DriverVersion != expected.DriverVersion {
		return fail("DRIVER_BINDING_MISMATCH")
	}
	return nil
}

func EncodeDriverInfo(info DriverInfo) ([]byte, error) {
	if err := ValidateDriverInfo(info, DriverInfoBinding{}); err != nil {
		return nil, err
	}
	body, err := json.Marshal(info)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return append(body, '\n'), nil
}

func DecodeDriverInfo(body []byte, expected DriverInfoBinding) (DriverInfo, error) {
	value, err := decodeStrict(body, 4_096)
	if err != nil {
		return DriverInfo{}, err
	}
	root, err := closedObject(value,
		[]string{"contract_version", "driver_id", "driver_version"}, nil)
	if err != nil {
		return DriverInfo{}, err
	}
	var info DriverInfo
	if info.ContractVersion, err = requiredString(root, "contract_version"); err != nil {
		return DriverInfo{}, err
	}
	if info.DriverID, err = requiredString(root, "driver_id"); err != nil {
		return DriverInfo{}, err
	}
	if info.DriverVersion, err = requiredString(root, "driver_version"); err != nil {
		return DriverInfo{}, err
	}
	if err := ValidateDriverInfo(info, expected); err != nil {
		return DriverInfo{}, err
	}
	return info, nil
}

func CanonicalOperation(role Role) (Operation, error) {
	id, ok := operationForRole[role]
	if !ok {
		return Operation{}, fail("INVALID_ROLE")
	}
	pkg, err := baton.Load()
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

func NewRequest(
	invocationID string,
	role Role,
	model *string,
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
		Model:         cloneString(model),
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
	if request.Model != nil {
		if err := validateText(*request.Model, 500, false); err != nil {
			return fail("INVALID_MODEL")
		}
	}
	if err := validateWorkspace(request.Workspace); err != nil {
		return err
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
	if request.Limits.OutputBytes < 1 || request.Limits.OutputBytes > MaxResultTextBytes {
		return fail("INVALID_LIMIT")
	}
	body, err := json.Marshal(request)
	if err != nil || len(body)+1 > MaxRequestBytes {
		return fail("RESOURCE_LIMIT")
	}
	return nil
}

func EncodeRequest(request Request) ([]byte, error) {
	if err := ValidateRequest(request); err != nil {
		return nil, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return append(body, '\n'), nil
}

func DecodeRequest(body []byte) (Request, error) {
	value, err := decodeStrict(body, MaxRequestBytes)
	if err != nil {
		return Request{}, err
	}
	root, err := closedObject(value, []string{
		"schema_version", "invocation_id", "role", "operation", "model",
		"workspace", "inputs", "fresh_context", "limits",
	}, nil)
	if err != nil {
		return Request{}, err
	}
	var request Request
	if request.SchemaVersion, err = requiredString(root, "schema_version"); err != nil {
		return Request{}, err
	}
	if request.InvocationID, err = requiredString(root, "invocation_id"); err != nil {
		return Request{}, err
	}
	role, err := requiredString(root, "role")
	if err != nil {
		return Request{}, err
	}
	request.Role = Role(role)
	operation, err := closedObject(root["operation"], []string{"id", "version", "digest", "instructions"}, nil)
	if err != nil {
		return Request{}, err
	}
	if request.Operation.ID, err = requiredString(operation, "id"); err != nil {
		return Request{}, err
	}
	if request.Operation.Version, err = requiredString(operation, "version"); err != nil {
		return Request{}, err
	}
	if request.Operation.Digest, err = requiredString(operation, "digest"); err != nil {
		return Request{}, err
	}
	if request.Operation.Instructions, err = requiredString(operation, "instructions"); err != nil {
		return Request{}, err
	}
	if request.Model, err = nullableString(root["model"]); err != nil {
		return Request{}, err
	}
	workspace, err := closedObject(root["workspace"], []string{"path", "access"}, nil)
	if err != nil {
		return Request{}, err
	}
	if request.Workspace.Path, err = requiredString(workspace, "path"); err != nil {
		return Request{}, err
	}
	access, err := requiredString(workspace, "access")
	if err != nil {
		return Request{}, err
	}
	request.Workspace.Access = WorkspaceAccess(access)
	inputValues, ok := root["inputs"].([]any)
	if !ok {
		return Request{}, fail("INVALID_FIELD")
	}
	request.Inputs = make([]Input, 0, len(inputValues))
	for _, value := range inputValues {
		inputObject, err := closedObject(value, []string{"name", "path", "digest"}, nil)
		if err != nil {
			return Request{}, err
		}
		var input Input
		if input.Name, err = requiredString(inputObject, "name"); err != nil {
			return Request{}, err
		}
		if input.Path, err = requiredString(inputObject, "path"); err != nil {
			return Request{}, err
		}
		if input.Digest, err = requiredString(inputObject, "digest"); err != nil {
			return Request{}, err
		}
		request.Inputs = append(request.Inputs, input)
	}
	if request.FreshContext, ok = root["fresh_context"].(bool); !ok {
		return Request{}, fail("INVALID_FIELD")
	}
	limits, err := closedObject(root["limits"], []string{"timeout_ms", "output_bytes"}, nil)
	if err != nil {
		return Request{}, err
	}
	if request.Limits.TimeoutMillis, err = requiredInteger(limits, "timeout_ms"); err != nil {
		return Request{}, err
	}
	if request.Limits.OutputBytes, err = requiredInteger(limits, "output_bytes"); err != nil {
		return Request{}, err
	}
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
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
	if !driverIdentityPattern.MatchString(result.DriverID) ||
		!versionPattern.MatchString(result.DriverVersion) {
		return fail("INVALID_DRIVER")
	}
	if result.ObservedModel != nil {
		if err := validateText(*result.ObservedModel, 500, false); err != nil {
			return fail("INVALID_MODEL")
		}
	}
	if result.DurationMillis < 0 || result.DurationMillis > MaxSafeInteger {
		return fail("INVALID_FIELD")
	}
	if !utf8.ValidString(result.Text) || len([]byte(result.Text)) > MaxResultTextBytes {
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
	}
	if expected.InvocationID != "" && result.InvocationID != expected.InvocationID {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.DriverID != "" && result.DriverID != expected.DriverID {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.DriverVersion != "" && result.DriverVersion != expected.DriverVersion {
		return fail("RESULT_BINDING_MISMATCH")
	}
	if expected.BindModel || expected.Model != nil {
		if (result.ObservedModel == nil) != (expected.Model == nil) {
			return fail("RESULT_BINDING_MISMATCH")
		}
		if result.ObservedModel != nil && *result.ObservedModel != *expected.Model {
			return fail("RESULT_BINDING_MISMATCH")
		}
	}
	return nil
}

func EncodeResult(result Result) ([]byte, error) {
	if err := ValidateResult(result, ResultBinding{}); err != nil {
		return nil, err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return append(body, '\n'), nil
}

func DecodeResult(body []byte, expected ResultBinding) (Result, error) {
	value, err := decodeStrict(body, 8*1024*1024)
	if err != nil {
		return Result{}, err
	}
	root, err := closedObject(value, []string{
		"schema_version", "invocation_id", "driver_id", "driver_version",
		"observed_model", "duration_ms", "text", "transport_status",
	}, []string{"usage"})
	if err != nil {
		return Result{}, err
	}
	var result Result
	if result.SchemaVersion, err = requiredString(root, "schema_version"); err != nil {
		return Result{}, err
	}
	if result.InvocationID, err = requiredString(root, "invocation_id"); err != nil {
		return Result{}, err
	}
	if result.DriverID, err = requiredString(root, "driver_id"); err != nil {
		return Result{}, err
	}
	if result.DriverVersion, err = requiredString(root, "driver_version"); err != nil {
		return Result{}, err
	}
	if result.ObservedModel, err = nullableString(root["observed_model"]); err != nil {
		return Result{}, err
	}
	if result.DurationMillis, err = requiredInteger(root, "duration_ms"); err != nil {
		return Result{}, err
	}
	if result.Text, err = requiredStringAllowEmpty(root, "text"); err != nil {
		return Result{}, err
	}
	status, err := requiredString(root, "transport_status")
	if err != nil {
		return Result{}, err
	}
	result.TransportStatus = TransportStatus(status)
	if usageValue, present := root["usage"]; present {
		usageObject, err := closedObject(usageValue, []string{"input_tokens", "output_tokens"}, nil)
		if err != nil {
			return Result{}, err
		}
		usage := &Usage{}
		if usage.InputTokens, err = requiredInteger(usageObject, "input_tokens"); err != nil {
			return Result{}, err
		}
		if usage.OutputTokens, err = requiredInteger(usageObject, "output_tokens"); err != nil {
			return Result{}, err
		}
		result.Usage = usage
	}
	if err := ValidateResult(result, expected); err != nil {
		return Result{}, err
	}
	return result, nil
}

func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
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
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || segment == ".git" {
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

func requiredString(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	if !ok || value == "" {
		return "", fail("INVALID_FIELD")
	}
	return value, nil
}

func requiredStringAllowEmpty(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	if !ok {
		return "", fail("INVALID_FIELD")
	}
	return value, nil
}

func nullableString(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	stringValue, ok := value.(string)
	if !ok || stringValue == "" {
		return nil, fail("INVALID_FIELD")
	}
	return &stringValue, nil
}

func requiredInteger(object map[string]any, key string) (int64, error) {
	number, ok := object[key].(json.Number)
	if !ok || strings.ContainsAny(string(number), ".eE") {
		return 0, fail("INVALID_FIELD")
	}
	value, err := number.Int64()
	if err != nil || value < -MaxSafeInteger || value > MaxSafeInteger {
		return 0, fail("INVALID_FIELD")
	}
	return value, nil
}

func canonicalJSON(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return body, nil
}
