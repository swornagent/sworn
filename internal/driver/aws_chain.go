package driver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	AWSCLIVersion     = "aws-cli/2.35.9"
	AWSCLIDigest      = "sha256:89fc1fb51e8a89f8ae1d6f8fb973c0587860dd66aeac1f4c0be7106cd7b4c6ec"
	MaxAWSListBytes   = 32_768
	MaxAWSExportBytes = 65_536
)

type AWSSourceKind string

const (
	AWSSourceEnvironment       AWSSourceKind = "environment"
	AWSSourceSharedFiles       AWSSourceKind = "shared_files"
	AWSSourceCredentialProcess AWSSourceKind = "credential_process"
	AWSSourceSSO               AWSSourceKind = "sso"
	AWSSourceAssumeRole        AWSSourceKind = "assume_role"
	AWSSourceWebIdentity       AWSSourceKind = "web_identity"
	AWSSourceContainer         AWSSourceKind = "container"
	AWSSourceInstance          AWSSourceKind = "instance"
)

func (kind AWSSourceKind) valid() bool {
	switch kind {
	case AWSSourceEnvironment, AWSSourceSharedFiles, AWSSourceCredentialProcess,
		AWSSourceSSO, AWSSourceAssumeRole, AWSSourceWebIdentity,
		AWSSourceContainer, AWSSourceInstance:
		return true
	default:
		return false
	}
}

type AWSChainSpec struct {
	CLI                    ExecutableIdentity  `json:"cli"`
	CLIVersion             string              `json:"cli_version"`
	Profile                string              `json:"profile"`
	Region                 string              `json:"region"`
	RegionSource           AWSSourceKind       `json:"region_source"`
	CredentialSource       AWSSourceKind       `json:"credential_source"`
	EnvironmentKeys        []string            `json:"environment_keys"`
	RuntimeFiles           []PinnedRuntimeFile `json:"runtime_files"`
	RequiredRuntimeTargets []string            `json:"required_runtime_targets"`
}

// AWSRuntimeResolver supplies one invocation-private standard-chain
// environment. Entries are mutable KEY=VALUE byte slices and are cleared by
// the adapter after use. The Sworn process environment is never inherited.
type AWSRuntimeResolver func(context.Context, string) ([][]byte, error)

type awsSnapshot struct {
	Profile           string
	Region            string
	RegionSource      AWSSourceKind
	CredentialSource  AWSSourceKind
	sourceFingerprint [32]byte
}

type awsCredentials struct {
	accessKeyID     []byte
	secretAccessKey []byte
	sessionToken    []byte
	expiration      time.Time
}

func (credentials *awsCredentials) Close() {
	if credentials == nil {
		return
	}
	clearBytes(credentials.accessKeyID)
	clearBytes(credentials.secretAccessKey)
	clearBytes(credentials.sessionToken)
	credentials.accessKeyID = nil
	credentials.secretAccessKey = nil
	credentials.sessionToken = nil
	credentials.expiration = time.Time{}
}

type awsCommandRunner func(
	context.Context,
	AWSChainSpec,
	[][]byte,
	...string,
) ([]byte, error)

func validateAWSChainSpec(spec AWSChainSpec) error {
	if spec.CLIVersion != AWSCLIVersion || spec.CLI.Digest != AWSCLIDigest ||
		validateExecutableIdentity(spec.CLI) != nil ||
		validateText(spec.Region, 128, false) != nil ||
		!regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,126}$`).MatchString(spec.Region) ||
		!spec.RegionSource.valid() || !spec.CredentialSource.valid() ||
		(spec.RegionSource != AWSSourceEnvironment &&
			spec.RegionSource != AWSSourceSharedFiles) ||
		(spec.Profile != "" && !providerKeyPattern.MatchString(spec.Profile)) ||
		validatePinnedRuntimeFiles(
			spec.RuntimeFiles,
			spec.RequiredRuntimeTargets,
			"AWS_NOT_CERTIFIED",
		) != nil {
		return fail("AWS_CONFIGURATION_INVALID")
	}
	keys := make(map[string]struct{}, len(spec.EnvironmentKeys))
	for _, key := range spec.EnvironmentKeys {
		if !awsEnvironmentKeyAllowed(key) {
			return fail("AWS_CONFIGURATION_INVALID")
		}
		if _, duplicate := keys[key]; duplicate {
			return fail("AWS_CONFIGURATION_INVALID")
		}
		keys[key] = struct{}{}
	}
	if len(keys) == 0 {
		return fail("AWS_CONFIGURATION_INVALID")
	}
	if _, region := keys["AWS_REGION"]; region {
		if _, defaultRegion := keys["AWS_DEFAULT_REGION"]; defaultRegion {
			// Both are supported only when the runtime values are identical.
		}
	}
	if directAWSEnvironmentSpec(spec) &&
		validateDirectAWSEnvironmentKeys(spec) != nil {
		return fail("AWS_CONFIGURATION_INVALID")
	}
	return nil
}

func resolveAWSChain(
	ctx context.Context,
	spec AWSChainSpec,
	environment [][]byte,
	runner awsCommandRunner,
) (awsSnapshot, *awsCredentials, error) {
	if validateAWSChainSpec(spec) != nil {
		clearEnvironment(environment)
		return awsSnapshot{}, nil, fail("AWS_NOT_CERTIFIED")
	}
	if err := validateAWSEnvironment(spec, environment); err != nil {
		clearEnvironment(environment)
		return awsSnapshot{}, nil, err
	}
	if directAWSEnvironmentSpec(spec) {
		return resolveDirectAWSEnvironment(ctx, spec, environment)
	}
	if runner == nil {
		clearEnvironment(environment)
		return awsSnapshot{}, nil, fail("AWS_NOT_CERTIFIED")
	}
	defer clearEnvironment(environment)
	arguments := []string{"configure", "list"}
	if spec.Profile != "" {
		arguments = append(arguments, "--profile", spec.Profile)
	}
	firstBody, err := runner(ctx, spec, environment, arguments...)
	if err != nil {
		clearBytes(firstBody)
		return awsSnapshot{}, nil, err
	}
	first, err := parseAWSConfigureList(firstBody)
	clearBytes(firstBody)
	if err != nil {
		return awsSnapshot{}, nil, err
	}
	exportArguments := []string{"configure", "export-credentials", "--format", "process"}
	if spec.Profile != "" {
		exportArguments = append(exportArguments, "--profile", spec.Profile)
	}
	exportBody, err := runner(ctx, spec, environment, exportArguments...)
	if err != nil {
		clearBytes(exportBody)
		return awsSnapshot{}, nil, err
	}
	credentials, err := parseAWSExport(exportBody, time.Now().UTC())
	clearBytes(exportBody)
	if err != nil {
		return awsSnapshot{}, nil, err
	}
	secondBody, err := runner(ctx, spec, environment, arguments...)
	if err != nil {
		credentials.Close()
		clearBytes(secondBody)
		return awsSnapshot{}, nil, err
	}
	second, err := parseAWSConfigureList(secondBody)
	clearBytes(secondBody)
	if err != nil {
		credentials.Close()
		return awsSnapshot{}, nil, err
	}
	expected := awsSnapshot{
		Profile: spec.Profile, Region: spec.Region,
		RegionSource:     spec.RegionSource,
		CredentialSource: spec.CredentialSource,
	}
	if first != second ||
		first.Profile != expected.Profile ||
		first.Region != expected.Region ||
		first.RegionSource != expected.RegionSource ||
		first.CredentialSource != expected.CredentialSource {
		credentials.Close()
		return awsSnapshot{}, nil, fail("AWS_NOT_CERTIFIED")
	}
	return first, credentials, nil
}

func directAWSEnvironmentSpec(spec AWSChainSpec) bool {
	return spec.RegionSource == AWSSourceEnvironment &&
		spec.CredentialSource == AWSSourceEnvironment
}

func validateDirectAWSEnvironmentKeys(spec AWSChainSpec) error {
	if spec.Profile != "" {
		return fail("AWS_CONFIGURATION_INVALID")
	}
	keys := make(map[string]struct{}, len(spec.EnvironmentKeys))
	for _, key := range spec.EnvironmentKeys {
		switch key {
		case "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
			"AWS_SESSION_TOKEN", "AWS_REGION", "AWS_DEFAULT_REGION":
		default:
			return fail("AWS_CONFIGURATION_INVALID")
		}
		keys[key] = struct{}{}
	}
	for _, required := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
	} {
		if _, present := keys[required]; !present {
			return fail("AWS_CONFIGURATION_INVALID")
		}
	}
	_, region := keys["AWS_REGION"]
	_, defaultRegion := keys["AWS_DEFAULT_REGION"]
	if !region && !defaultRegion {
		return fail("AWS_CONFIGURATION_INVALID")
	}
	return nil
}

func resolveDirectAWSEnvironment(
	ctx context.Context,
	spec AWSChainSpec,
	environment [][]byte,
) (awsSnapshot, *awsCredentials, error) {
	if ctx == nil || ctx.Err() != nil ||
		validateDirectAWSEnvironmentKeys(spec) != nil {
		clearEnvironment(environment)
		return awsSnapshot{}, nil, fail("AWS_NOT_CERTIFIED")
	}
	defer clearEnvironment(environment)
	values := make(map[string][]byte, len(environment))
	for _, entry := range environment {
		separator := bytes.IndexByte(entry, '=')
		if separator < 1 {
			return awsSnapshot{}, nil, fail("AWS_NOT_CERTIFIED")
		}
		values[string(entry[:separator])] = entry[separator+1:]
	}
	region := values["AWS_REGION"]
	if len(region) == 0 {
		region = values["AWS_DEFAULT_REGION"]
	}
	if len(region) == 0 || string(region) != spec.Region {
		return awsSnapshot{}, nil, fail("AWS_NOT_CERTIFIED")
	}
	access := values["AWS_ACCESS_KEY_ID"]
	secret := values["AWS_SECRET_ACCESS_KEY"]
	token := values["AWS_SESSION_TOKEN"]
	if !validDirectAWSSecret(access, 8, 256) ||
		!validDirectAWSSecret(secret, 8, 4_096) ||
		(len(token) != 0 && !validDirectAWSSecret(token, 1, 65_536)) {
		return awsSnapshot{}, nil, fail("AWS_NOT_CERTIFIED")
	}
	credentials := &awsCredentials{
		accessKeyID:     append([]byte(nil), access...),
		secretAccessKey: append([]byte(nil), secret...),
		sessionToken:    append([]byte(nil), token...),
	}
	keys := normalizedAWSEnvironmentKeys(spec.EnvironmentKeys)
	fingerprintBody := []byte(
		"environment\x00environment\x00" + strings.Join(keys, "\x00"),
	)
	fingerprint := sha256.Sum256(fingerprintBody)
	clearBytes(fingerprintBody)
	return awsSnapshot{
		Profile:           "",
		Region:            spec.Region,
		RegionSource:      AWSSourceEnvironment,
		CredentialSource:  AWSSourceEnvironment,
		sourceFingerprint: fingerprint,
	}, credentials, nil
}

func validDirectAWSSecret(body []byte, minimum, maximum int) bool {
	return len(body) >= minimum && len(body) <= maximum &&
		validOpaqueText(body) &&
		!bytes.ContainsAny(body, "\x00\r\n")
}

func parseAWSConfigureList(body []byte) (awsSnapshot, error) {
	if len(body) == 0 || len(body) > MaxAWSListBytes || !utf8.Valid(body) {
		return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
	}
	lines := bytes.Split(bytes.TrimSuffix(body, []byte("\n")), []byte("\n"))
	if len(lines) != 5 ||
		!equalByteColumns(splitAWSColumns(lines[0]), [][]byte{
			[]byte("NAME"), []byte("VALUE"), []byte("TYPE"), []byte("LOCATION"),
		}) {
		return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
	}
	rows := make(map[string][][]byte, 4)
	for _, line := range lines[1:] {
		columns := splitAWSColumns(line)
		if len(columns) != 4 {
			return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
		}
		name := string(columns[0])
		switch name {
		case "profile", "access_key", "secret_key", "region":
		default:
			return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
		}
		if _, duplicate := rows[name]; duplicate {
			return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
		}
		rows[name] = columns
	}
	if len(rows) != 4 {
		return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
	}
	access, secret := rows["access_key"], rows["secret_key"]
	if !validAWSMaskedValue(access[1]) || !validAWSMaskedValue(secret[1]) ||
		!bytes.Equal(access[2], secret[2]) || !bytes.Equal(access[3], secret[3]) {
		return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
	}
	credentialSource, err := normalizeAWSSource(access[2], access[3], false)
	if err != nil {
		return awsSnapshot{}, err
	}
	region := rows["region"]
	if bytes.Equal(region[1], []byte("<not set>")) ||
		!regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,126}$`).Match(region[1]) {
		return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
	}
	regionSource, err := normalizeAWSSource(region[2], region[3], true)
	if err != nil {
		return awsSnapshot{}, err
	}
	profile := rows["profile"][1]
	profileValue := ""
	if bytes.Equal(profile, []byte("<not set>")) {
		if !bytes.Equal(rows["profile"][2], []byte("None")) ||
			!bytes.Equal(rows["profile"][3], []byte("None")) {
			return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
		}
	} else {
		if !providerKeyPattern.Match(profile) {
			return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
		}
		if !bytes.Equal(rows["profile"][2], []byte("manual")) ||
			!bytes.Equal(rows["profile"][3], []byte("--profile")) {
			return awsSnapshot{}, fail("AWS_NOT_CERTIFIED")
		}
		profileValue = string(profile)
	}
	fingerprintBody := bytes.Join([][]byte{
		rows["profile"][2], rows["profile"][3],
		access[2], access[3], region[2], region[3],
	}, []byte{0})
	fingerprint := sha256.Sum256(fingerprintBody)
	clearBytes(fingerprintBody)
	return awsSnapshot{
		Profile: profileValue, Region: string(region[1]),
		RegionSource: regionSource, CredentialSource: credentialSource,
		sourceFingerprint: fingerprint,
	}, nil
}

func splitAWSColumns(line []byte) [][]byte {
	raw := bytes.SplitN(line, []byte(":"), 4)
	columns := make([][]byte, len(raw))
	for index := range raw {
		columns[index] = bytes.TrimSpace(raw[index])
	}
	return columns
}

func equalByteColumns(left, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !bytes.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}

func validAWSMaskedValue(value []byte) bool {
	if len(value) < 20 {
		return false
	}
	stars := 0
	for stars < len(value) && value[stars] == '*' {
		stars++
	}
	if stars < 16 || len(value)-stars != 4 {
		return false
	}
	for _, character := range value[stars:] {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9')) {
			return false
		}
	}
	return true
}

func normalizeAWSSource(typeValue, location []byte, region bool) (AWSSourceKind, error) {
	if len(location) > 4_096 || !utf8.Valid(location) {
		return "", fail("AWS_NOT_CERTIFIED")
	}
	source := string(typeValue)
	switch source {
	case "env":
		if region {
			if !bytes.Equal(
				location,
				[]byte("['AWS_REGION', 'AWS_DEFAULT_REGION']"),
			) {
				return "", fail("AWS_NOT_CERTIFIED")
			}
		} else if len(location) != 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceEnvironment, nil
	case "config-file", "shared-credentials-file":
		if len(location) == 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceSharedFiles, nil
	case "custom-process":
		if region || len(location) == 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceCredentialProcess, nil
	case "sso":
		if region || len(location) == 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceSSO, nil
	case "assume-role":
		if region || len(location) == 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceAssumeRole, nil
	case "web-identity":
		if region || len(location) == 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceWebIdentity, nil
	case "container-role":
		if region || len(location) == 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceContainer, nil
	case "iam-role":
		if region || len(location) == 0 {
			return "", fail("AWS_NOT_CERTIFIED")
		}
		return AWSSourceInstance, nil
	default:
		return "", fail("AWS_NOT_CERTIFIED")
	}
}

func parseAWSExport(body []byte, now time.Time) (*awsCredentials, error) {
	if len(body) == 0 || len(body) > MaxAWSExportBytes || !utf8.Valid(body) {
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	fields, err := splitFlatJSONObject(body)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, raw := range fields {
			clearBytes(raw)
		}
	}()
	required := []string{"Version", "AccessKeyId", "SecretAccessKey"}
	optional := map[string]struct{}{"SessionToken": {}, "Expiration": {}}
	for _, name := range required {
		if _, present := fields[name]; !present {
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
	}
	for name := range fields {
		found := false
		for _, requiredName := range required {
			found = found || name == requiredName
		}
		if !found {
			_, found = optional[name]
		}
		if !found {
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
	}
	if !bytes.Equal(bytes.TrimSpace(fields["Version"]), []byte("1")) {
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	access, err := decodeJSONStringMutable(fields["AccessKeyId"])
	if err != nil {
		return nil, err
	}
	secret, err := decodeJSONStringMutable(fields["SecretAccessKey"])
	if err != nil {
		clearBytes(access)
		return nil, err
	}
	credentials := &awsCredentials{accessKeyID: access, secretAccessKey: secret}
	ok := false
	defer func() {
		if !ok {
			credentials.Close()
		}
	}()
	if len(access) < 8 || len(access) > 256 || len(secret) < 8 || len(secret) > 4_096 ||
		!validOpaqueText(access) || !validOpaqueText(secret) {
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	if raw, present := fields["SessionToken"]; present {
		credentials.sessionToken, err = decodeJSONStringMutable(raw)
		if err != nil || len(credentials.sessionToken) == 0 ||
			len(credentials.sessionToken) > 16_384 ||
			!validOpaqueText(credentials.sessionToken) {
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
	}
	if raw, present := fields["Expiration"]; present {
		expirationBytes, decodeErr := decodeJSONStringMutable(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		expiration, parseErr := time.Parse(time.RFC3339, string(expirationBytes))
		clearBytes(expirationBytes)
		if parseErr != nil || !expiration.After(now.Add(5*time.Minute)) {
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
		credentials.expiration = expiration
	}
	ok = true
	return credentials, nil
}

func splitFlatJSONObject(body []byte) (map[string][]byte, error) {
	index := skipJSONSpace(body, 0)
	if index >= len(body) || body[index] != '{' {
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	index++
	fields := make(map[string][]byte)
	for {
		index = skipJSONSpace(body, index)
		if index < len(body) && body[index] == '}' {
			index++
			break
		}
		keyStart, keyEnd, next, ok := scanJSONString(body, index)
		if !ok {
			clearRawMap(fields)
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
		keyBytes, err := decodeJSONStringMutable(body[keyStart:keyEnd])
		if err != nil || len(keyBytes) == 0 || len(keyBytes) > 64 {
			clearBytes(keyBytes)
			clearRawMap(fields)
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
		key := string(keyBytes)
		clearBytes(keyBytes)
		if _, duplicate := fields[key]; duplicate {
			clearRawMap(fields)
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
		index = skipJSONSpace(body, next)
		if index >= len(body) || body[index] != ':' {
			clearRawMap(fields)
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
		index = skipJSONSpace(body, index+1)
		valueStart := index
		if index < len(body) && body[index] == '"' {
			_, _, index, ok = scanJSONString(body, index)
			if !ok {
				clearRawMap(fields)
				return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
			}
		} else {
			for index < len(body) && body[index] >= '0' && body[index] <= '9' {
				index++
			}
			if index == valueStart {
				clearRawMap(fields)
				return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
			}
		}
		fields[key] = append([]byte(nil), body[valueStart:index]...)
		index = skipJSONSpace(body, index)
		if index >= len(body) {
			clearRawMap(fields)
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
		if body[index] == ',' {
			index++
			continue
		}
		if body[index] == '}' {
			index++
			break
		}
		clearRawMap(fields)
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	if skipJSONSpace(body, index) != len(body) {
		clearRawMap(fields)
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	return fields, nil
}

func scanJSONString(body []byte, start int) (int, int, int, bool) {
	if start >= len(body) || body[start] != '"' {
		return 0, 0, 0, false
	}
	for index := start + 1; index < len(body); index++ {
		switch body[index] {
		case '\\':
			index++
			if index >= len(body) {
				return 0, 0, 0, false
			}
			if body[index] == 'u' {
				index += 4
				if index >= len(body) {
					return 0, 0, 0, false
				}
			}
		case '"':
			return start, index + 1, index + 1, true
		case '\n', '\r':
			return 0, 0, 0, false
		}
	}
	return 0, 0, 0, false
}

func decodeJSONStringMutable(raw []byte) ([]byte, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	result := make([]byte, 0, len(raw)-2)
	for index := 1; index < len(raw)-1; index++ {
		character := raw[index]
		if character != '\\' {
			if character < 0x20 {
				clearBytes(result)
				return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
			}
			result = append(result, character)
			continue
		}
		index++
		if index >= len(raw)-1 {
			clearBytes(result)
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
		switch raw[index] {
		case '"', '\\', '/':
			result = append(result, raw[index])
		case 'b':
			result = append(result, '\b')
		case 'f':
			result = append(result, '\f')
		case 'n':
			result = append(result, '\n')
		case 'r':
			result = append(result, '\r')
		case 't':
			result = append(result, '\t')
		case 'u':
			if index+4 >= len(raw) {
				clearBytes(result)
				return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
			}
			value, valid := parseHexCodeUnit(raw[index+1 : index+5])
			if !valid {
				clearBytes(result)
				return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
			}
			index += 4
			runeValue := rune(value)
			if utf16.IsSurrogate(runeValue) {
				if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
					clearBytes(result)
					return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
				}
				low, lowValid := parseHexCodeUnit(raw[index+3 : index+7])
				if !lowValid {
					clearBytes(result)
					return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
				}
				runeValue = utf16.DecodeRune(runeValue, rune(low))
				index += 6
			}
			var encoded [utf8.UTFMax]byte
			count := utf8.EncodeRune(encoded[:], runeValue)
			result = append(result, encoded[:count]...)
		default:
			clearBytes(result)
			return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
		}
	}
	if !utf8.Valid(result) {
		clearBytes(result)
		return nil, fail("AWS_CREDENTIAL_EXPORT_INVALID")
	}
	return result, nil
}

func skipJSONSpace(body []byte, index int) int {
	for index < len(body) {
		switch body[index] {
		case ' ', '\t', '\r', '\n':
			index++
		default:
			return index
		}
	}
	return index
}

func clearRawMap(fields map[string][]byte) {
	for _, value := range fields {
		clearBytes(value)
	}
}

func validateAWSEnvironment(spec AWSChainSpec, environment [][]byte) error {
	expected := make(map[string]struct{}, len(spec.EnvironmentKeys))
	for _, key := range spec.EnvironmentKeys {
		expected[key] = struct{}{}
	}
	seen := make(map[string][]byte, len(environment))
	for _, entry := range environment {
		if len(entry) == 0 || len(entry) > 65_536 {
			return fail("AWS_NOT_CERTIFIED")
		}
		separator := bytes.IndexByte(entry, '=')
		if separator < 1 {
			return fail("AWS_NOT_CERTIFIED")
		}
		key := string(entry[:separator])
		if _, ok := expected[key]; !ok {
			return fail("AWS_NOT_CERTIFIED")
		}
		if _, duplicate := seen[key]; duplicate {
			return fail("AWS_NOT_CERTIFIED")
		}
		seen[key] = entry[separator+1:]
	}
	if len(seen) != len(expected) {
		return fail("AWS_NOT_CERTIFIED")
	}
	if region, present := seen["AWS_REGION"]; present {
		if defaultRegion, also := seen["AWS_DEFAULT_REGION"]; also &&
			!bytes.Equal(region, defaultRegion) {
			return fail("AWS_NOT_CERTIFIED")
		}
	}
	if _, present := seen["AWS_PROFILE"]; present {
		return fail("AWS_NOT_CERTIFIED")
	}
	if _, present := seen["AWS_DEFAULT_PROFILE"]; present {
		return fail("AWS_NOT_CERTIFIED")
	}
	return nil
}

func awsEnvironmentKeyAllowed(key string) bool {
	switch key {
	case "HOME", "PATH", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE",
		"AWS_REGION", "AWS_DEFAULT_REGION",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
		"AWS_EC2_METADATA_DISABLED", "AWS_EC2_METADATA_SERVICE_ENDPOINT",
		"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE", "AWS_METADATA_SERVICE_TIMEOUT",
		"AWS_METADATA_SERVICE_NUM_ATTEMPTS", "AWS_SDK_LOAD_CONFIG":
		return true
	default:
		return false
	}
}

func clearEnvironment(environment [][]byte) {
	for _, entry := range environment {
		clearBytes(entry)
	}
}

func normalizedAWSEnvironmentKeys(keys []string) []string {
	copyKeys := append([]string(nil), keys...)
	sort.Strings(copyKeys)
	return copyKeys
}

func awsArguments(profile string, base ...string) []string {
	arguments := append([]string(nil), base...)
	if profile != "" {
		arguments = append(arguments, "--profile", profile)
	}
	return arguments
}

func awsVersionMatches(body []byte) bool {
	return strings.HasPrefix(string(bytes.TrimSpace(body)), AWSCLIVersion+" ")
}
