package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/observe"
)

const (
	operatorConfigSchemaVersion = "sworn.operator-config/v1"
	maxOperatorConfigBytes      = 64 * 1024
	defaultOperatorListen       = "127.0.0.1:7337"
)

var operatorTokenPattern = regexp.MustCompile(
	`^[A-Za-z0-9._~-]{32,512}$`,
)

// Operator-config refusal codes, enumerated in one place.
//
//   - OPERATOR_CONFIG_UNAVAILABLE: the file cannot be admitted (path, parent,
//     symlink, replacement, read or size failure).
//   - OPERATOR_CONFIG_INSECURE_MODE: a regular, non-symlink file with an
//     in-range size whose mode is not 0600.
//   - OPERATOR_CONFIG_INVALID: the bytes are not an admitted operator config
//     (ambiguous JSON, non-exact fields, schema, listen, public, webhook,
//     otel or share validation).
//
// All reasons are fixed and value-free: no path, content or secret is echoed.
const (
	operatorConfigUnavailableCode  = "OPERATOR_CONFIG_UNAVAILABLE"
	operatorConfigInsecureModeCode = "OPERATOR_CONFIG_INSECURE_MODE"
	operatorConfigInvalidCode      = "OPERATOR_CONFIG_INVALID"
)

type operatorConfigError struct {
	Code   string
	reason string
}

func (e *operatorConfigError) Error() string {
	if e == nil {
		return operatorConfigUnavailableCode
	}
	if e.reason != "" {
		return e.Code + ": " + e.reason
	}
	return e.Code
}

func operatorConfigUnavailableErr() error {
	return &operatorConfigError{Code: operatorConfigUnavailableCode}
}

func operatorConfigInsecureModeErr() error {
	return &operatorConfigError{
		Code:   operatorConfigInsecureModeCode,
		reason: "operator config file mode must be 0600",
	}
}

func operatorConfigInvalidErr() error {
	return &operatorConfigError{Code: operatorConfigInvalidCode}
}

func isInsecureModeFile(info os.FileInfo) bool {
	return info != nil &&
		info.Mode().IsRegular() &&
		info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm() != 0o600 &&
		info.Size() >= 2 &&
		info.Size() <= maxOperatorConfigBytes
}

type operatorConfig struct {
	SchemaVersion string                  `json:"schema_version"`
	Local         operatorLocalConfig     `json:"local"`
	Public        *operatorPublicConfig   `json:"public"`
	Webhooks      []operatorWebhookConfig `json:"webhooks"`
	OTel          *observe.Config         `json:"otel"`
	// Share is the additive opt-in share-channel sibling of the private otel
	// block (revision-5 ruling: additive top-level block, never a nested
	// restructuring). It parses and configures independently of otel.
	Share *observe.ShareConfig `json:"share"`
}

type operatorLocalConfig struct {
	Listen string `json:"listen"`
}

type operatorPublicConfig struct {
	Listen         string `json:"listen"`
	Origin         string `json:"origin"`
	CertificatePEM string `json:"certificate_pem"`
	PrivateKeyPEM  string `json:"private_key_pem"`
	Token          string `json:"token"`
}

type operatorWebhookConfig struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

type operatorSettings struct {
	localListen string
	public      *operatorPublicSettings
	webhooks    []cockpit.WebhookDestination
	otel        *observe.Config
	share       *observe.ShareConfig
}

type operatorPublicSettings struct {
	listen      string
	host        string
	origin      string
	token       []byte
	certificate tls.Certificate
}

func loadOperatorSettings(path string) (operatorSettings, error) {
	if path == "" {
		return operatorSettings{localListen: defaultOperatorListen}, nil
	}
	body, err := readPrivateOperatorFile(path, nil)
	if err != nil {
		return operatorSettings{}, err
	}
	return parseOperatorConfig(body)
}

// beforeOpen exists only so admission tests can deterministically replace the
// file between inspection and open. Production always passes nil.
func readPrivateOperatorFile(
	path string,
	beforeOpen func(),
) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) ||
		filepath.Clean(path) != path ||
		strings.ContainsRune(path, 0) {
		return nil, operatorConfigUnavailableErr()
	}
	parent := filepath.Dir(path)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || filepath.Clean(resolvedParent) != parent {
		return nil, operatorConfigUnavailableErr()
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, operatorConfigUnavailableErr()
	}
	if !validOperatorFileInfo(before) {
		if isInsecureModeFile(before) {
			return nil, operatorConfigInsecureModeErr()
		}
		return nil, operatorConfigUnavailableErr()
	}
	if beforeOpen != nil {
		beforeOpen()
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, operatorConfigUnavailableErr()
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, operatorConfigUnavailableErr()
	}
	if !os.SameFile(before, opened) {
		return nil, operatorConfigUnavailableErr()
	}
	if !validOperatorFileInfo(opened) {
		if isInsecureModeFile(opened) {
			return nil, operatorConfigInsecureModeErr()
		}
		return nil, operatorConfigUnavailableErr()
	}
	body, err := io.ReadAll(
		io.LimitReader(file, maxOperatorConfigBytes+1),
	)
	if err != nil || len(body) < 2 ||
		len(body) > maxOperatorConfigBytes ||
		int64(len(body)) != opened.Size() {
		return nil, operatorConfigUnavailableErr()
	}
	afterOpen, err := file.Stat()
	if err != nil || !os.SameFile(opened, afterOpen) ||
		afterOpen.Size() != opened.Size() ||
		!afterOpen.ModTime().Equal(opened.ModTime()) {
		return nil, operatorConfigUnavailableErr()
	}
	afterPath, err := os.Lstat(path)
	if err != nil {
		return nil, operatorConfigUnavailableErr()
	}
	if !os.SameFile(opened, afterPath) {
		return nil, operatorConfigUnavailableErr()
	}
	if !validOperatorFileInfo(afterPath) {
		if isInsecureModeFile(afterPath) {
			return nil, operatorConfigInsecureModeErr()
		}
		return nil, operatorConfigUnavailableErr()
	}
	return body, nil
}

func validOperatorFileInfo(info os.FileInfo) bool {
	return info != nil &&
		info.Mode().IsRegular() &&
		info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm() == 0o600 &&
		info.Size() >= 2 &&
		info.Size() <= maxOperatorConfigBytes
}

func parseOperatorConfig(body []byte) (operatorSettings, error) {
	if len(body) < 2 || len(body) > maxOperatorConfigBytes ||
		rejectAmbiguousOperatorJSON(body) != nil ||
		validateExactOperatorFields(body) != nil {
		return operatorSettings{}, operatorConfigInvalidErr()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var config operatorConfig
	if err := decoder.Decode(&config); err != nil {
		return operatorSettings{}, operatorConfigInvalidErr()
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return operatorSettings{}, operatorConfigInvalidErr()
	}
	if config.SchemaVersion != operatorConfigSchemaVersion ||
		len(config.Webhooks) > 32 {
		return operatorSettings{}, operatorConfigInvalidErr()
	}
	local, err := literalListen(config.Local.Listen, true)
	if err != nil {
		return operatorSettings{}, operatorConfigInvalidErr()
	}
	result := operatorSettings{localListen: local}
	if config.Public != nil {
		public, err := parsePublicSettings(*config.Public)
		if err != nil {
			return operatorSettings{}, operatorConfigInvalidErr()
		}
		result.public = &public
	}
	seenDestinations := make(map[string]struct{}, len(config.Webhooks))
	for _, webhook := range config.Webhooks {
		if len(webhook.ID) < 1 || len(webhook.ID) > 120 ||
			len(webhook.URL) < 1 || len(webhook.URL) > 1024 ||
			len(webhook.Secret) < 32 || len(webhook.Secret) > 512 {
			return operatorSettings{}, operatorConfigInvalidErr()
		}
		if _, duplicate := seenDestinations[webhook.ID]; duplicate {
			return operatorSettings{}, operatorConfigInvalidErr()
		}
		seenDestinations[webhook.ID] = struct{}{}
		result.webhooks = append(
			result.webhooks,
			cockpit.WebhookDestination{
				ID:     webhook.ID,
				URL:    webhook.URL,
				Secret: []byte(webhook.Secret),
			},
		)
	}
	if config.OTel != nil {
		body, err := json.Marshal(config.OTel)
		if err != nil {
			return operatorSettings{}, operatorConfigInvalidErr()
		}
		otelConfig, err := observe.ParseConfig(body)
		if err != nil {
			return operatorSettings{}, operatorConfigInvalidErr()
		}
		result.otel = &otelConfig
	}
	if config.Share != nil {
		body, err := json.Marshal(config.Share)
		if err != nil {
			return operatorSettings{}, operatorConfigInvalidErr()
		}
		shareConfig, err := observe.ParseShareConfig(body)
		if err != nil {
			return operatorSettings{}, operatorConfigInvalidErr()
		}
		result.share = &shareConfig
	}
	return result, nil
}

func validateExactOperatorFields(body []byte) error {
	root, err := exactJSONObject(body, []string{
		"schema_version", "local", "public", "webhooks", "otel", "share",
	})
	if err != nil {
		return err
	}
	if local, found := root["local"]; found {
		if _, err := exactJSONObject(local, []string{"listen"}); err != nil {
			return err
		}
	}
	if public, found := root["public"]; found && !jsonNull(public) {
		if _, err := exactJSONObject(public, []string{
			"listen",
			"origin",
			"certificate_pem",
			"private_key_pem",
			"token",
		}); err != nil {
			return err
		}
	}
	if webhooks, found := root["webhooks"]; found && !jsonNull(webhooks) {
		var items []json.RawMessage
		if err := json.Unmarshal(webhooks, &items); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := exactJSONObject(
				item,
				[]string{"id", "url", "secret"},
			); err != nil {
				return err
			}
		}
	}
	if otel, found := root["otel"]; found && !jsonNull(otel) {
		if err := validateExactTelemetryBlock(otel, []string{
			"schema_version", "endpoint", "headers",
		}); err != nil {
			return err
		}
	}
	if share, found := root["share"]; found && !jsonNull(share) {
		if err := validateExactTelemetryBlock(share, []string{
			"schema_version", "enabled", "endpoint", "headers",
		}); err != nil {
			return err
		}
	}
	return nil
}

func validateExactTelemetryBlock(
	body []byte,
	allowed []string,
) error {
	fields, err := exactJSONObject(body, allowed)
	if err != nil {
		return err
	}
	if headers, found := fields["headers"]; found &&
		!jsonNull(headers) {
		var values map[string]json.RawMessage
		if err := json.Unmarshal(headers, &values); err != nil {
			return err
		}
	}
	return nil
}

func exactJSONObject(
	body []byte,
	allowed []string,
) (map[string]json.RawMessage, error) {
	var result map[string]json.RawMessage
	if err := json.Unmarshal(body, &result); err != nil || result == nil {
		return nil, errors.New("invalid JSON object")
	}
	fields := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		fields[name] = struct{}{}
	}
	for name := range result {
		if _, found := fields[name]; !found {
			return nil, errors.New("non-exact JSON field")
		}
	}
	return result, nil
}

func jsonNull(body []byte) bool {
	return bytes.Equal(bytes.TrimSpace(body), []byte("null"))
}

func parsePublicSettings(
	config operatorPublicConfig,
) (operatorPublicSettings, error) {
	listen, err := literalListen(config.Listen, false)
	if err != nil || len(config.Origin) < 1 || len(config.Origin) > 2048 ||
		len(config.CertificatePEM) < 1 ||
		len(config.CertificatePEM) > 24*1024 ||
		len(config.PrivateKeyPEM) < 1 ||
		len(config.PrivateKeyPEM) > 24*1024 ||
		!operatorTokenPattern.MatchString(config.Token) {
		return operatorPublicSettings{}, errors.New("invalid public config")
	}
	origin, err := url.Parse(config.Origin)
	if err != nil || origin.Scheme != "https" ||
		origin.User != nil || origin.Host == "" ||
		origin.Path != "" || origin.RawPath != "" ||
		origin.RawQuery != "" || origin.Fragment != "" ||
		origin.String() != config.Origin ||
		loopbackOrAmbiguousHost(origin.Hostname()) {
		return operatorPublicSettings{}, errors.New("invalid public config")
	}
	certificate, err := tls.X509KeyPair(
		[]byte(config.CertificatePEM),
		[]byte(config.PrivateKeyPEM),
	)
	if err != nil {
		return operatorPublicSettings{}, errors.New("invalid public config")
	}
	return operatorPublicSettings{
		listen:      listen,
		host:        origin.Host,
		origin:      origin.String(),
		token:       []byte(config.Token),
		certificate: certificate,
	}, nil
}

func literalListen(value string, wantLoopback bool) (string, error) {
	if len(value) < 1 || len(value) > 256 ||
		strings.ContainsAny(value, " \t\r\n/@?#") {
		return "", errors.New("invalid listen authority")
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || strings.Contains(host, "%") {
		return "", errors.New("invalid listen authority")
	}
	ip := net.ParseIP(host)
	number, err := strconv.Atoi(port)
	if ip == nil || err != nil || number < 1 || number > 65535 ||
		ip.IsLoopback() != wantLoopback {
		return "", errors.New("invalid listen authority")
	}
	canonical := net.JoinHostPort(ip.String(), strconv.Itoa(number))
	if canonical != value {
		return "", errors.New("invalid listen authority")
	}
	return canonical, nil
}

func loopbackOrAmbiguousHost(host string) bool {
	if host == "" || strings.Contains(host, "%") ||
		strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

func rejectAmbiguousOperatorJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	tokens := 0
	if err := scanOperatorJSONValue(decoder, 0, &tokens); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("ambiguous JSON")
	}
	return nil
}

func scanOperatorJSONValue(
	decoder *json.Decoder,
	depth int,
	tokens *int,
) error {
	if depth > 16 || *tokens > 4096 {
		return errors.New("ambiguous JSON")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	*tokens++
	delim, structured := token.(json.Delim)
	if !structured {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		foldedKeys := make([]string, 0)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			*tokens++
			key, ok := keyToken.(string)
			if !ok || len(key) > 256 {
				return errors.New("ambiguous JSON")
			}
			if _, duplicate := keys[key]; duplicate {
				return errors.New("ambiguous JSON")
			}
			for _, prior := range foldedKeys {
				if strings.EqualFold(prior, key) {
					return errors.New("ambiguous JSON")
				}
			}
			keys[key] = struct{}{}
			foldedKeys = append(foldedKeys, key)
			if err := scanOperatorJSONValue(
				decoder,
				depth+1,
				tokens,
			); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("ambiguous JSON")
		}
	case '[':
		items := 0
		for decoder.More() {
			items++
			if items > 256 {
				return errors.New("ambiguous JSON")
			}
			if err := scanOperatorJSONValue(
				decoder,
				depth+1,
				tokens,
			); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("ambiguous JSON")
		}
	default:
		return errors.New("ambiguous JSON")
	}
	return nil
}
