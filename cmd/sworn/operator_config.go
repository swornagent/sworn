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

type operatorConfig struct {
	SchemaVersion string                  `json:"schema_version"`
	Local         operatorLocalConfig     `json:"local"`
	Public        *operatorPublicConfig   `json:"public"`
	Webhooks      []operatorWebhookConfig `json:"webhooks"`
	OTel          *observe.Config         `json:"otel"`
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
		return operatorSettings{}, errors.New("operator config unavailable")
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
		return nil, errors.New("operator config unavailable")
	}
	parent := filepath.Dir(path)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || filepath.Clean(resolvedParent) != parent {
		return nil, errors.New("operator config unavailable")
	}
	before, err := os.Lstat(path)
	if err != nil || !validOperatorFileInfo(before) {
		return nil, errors.New("operator config unavailable")
	}
	if beforeOpen != nil {
		beforeOpen()
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("operator config unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !validOperatorFileInfo(opened) ||
		!os.SameFile(before, opened) {
		return nil, errors.New("operator config unavailable")
	}
	body, err := io.ReadAll(
		io.LimitReader(file, maxOperatorConfigBytes+1),
	)
	if err != nil || len(body) < 2 ||
		len(body) > maxOperatorConfigBytes ||
		int64(len(body)) != opened.Size() {
		return nil, errors.New("operator config unavailable")
	}
	afterOpen, err := file.Stat()
	if err != nil || !os.SameFile(opened, afterOpen) ||
		afterOpen.Size() != opened.Size() ||
		!afterOpen.ModTime().Equal(opened.ModTime()) {
		return nil, errors.New("operator config unavailable")
	}
	afterPath, err := os.Lstat(path)
	if err != nil || !validOperatorFileInfo(afterPath) ||
		!os.SameFile(opened, afterPath) {
		return nil, errors.New("operator config unavailable")
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
		rejectAmbiguousOperatorJSON(body) != nil {
		return operatorSettings{}, errors.New("operator config unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var config operatorConfig
	if err := decoder.Decode(&config); err != nil {
		return operatorSettings{}, errors.New("operator config unavailable")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return operatorSettings{}, errors.New("operator config unavailable")
	}
	if config.SchemaVersion != operatorConfigSchemaVersion ||
		len(config.Webhooks) > 32 {
		return operatorSettings{}, errors.New("operator config unavailable")
	}
	local, err := literalListen(config.Local.Listen, true)
	if err != nil {
		return operatorSettings{}, errors.New("operator config unavailable")
	}
	result := operatorSettings{localListen: local}
	if config.Public != nil {
		public, err := parsePublicSettings(*config.Public)
		if err != nil {
			return operatorSettings{}, errors.New("operator config unavailable")
		}
		result.public = &public
	}
	seenDestinations := make(map[string]struct{}, len(config.Webhooks))
	for _, webhook := range config.Webhooks {
		if len(webhook.ID) < 1 || len(webhook.ID) > 120 ||
			len(webhook.URL) < 1 || len(webhook.URL) > 1024 ||
			len(webhook.Secret) < 32 || len(webhook.Secret) > 512 {
			return operatorSettings{}, errors.New("operator config unavailable")
		}
		if _, duplicate := seenDestinations[webhook.ID]; duplicate {
			return operatorSettings{}, errors.New("operator config unavailable")
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
			return operatorSettings{}, errors.New("operator config unavailable")
		}
		otelConfig, err := observe.ParseConfig(body)
		if err != nil {
			return operatorSettings{}, errors.New("operator config unavailable")
		}
		result.otel = &otelConfig
	}
	return result, nil
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
			keys[key] = struct{}{}
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
