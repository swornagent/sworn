package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/observe"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

func TestServeArgumentsAreClosedAndContentFree(t *testing.T) {
	t.Parallel()

	valid, ok := parseServeOptions([]string{
		"--operator-config", "/private/config.json",
		"--journal", "/private/run.sqlite",
		"--manifest", "/private/manifest.json",
		"--run", "run-1",
	})
	if !ok || valid.runID != "run-1" ||
		valid.journalPath != "/private/run.sqlite" ||
		valid.manifestPath != "/private/manifest.json" ||
		valid.operatorConfig != "/private/config.json" {
		t.Fatalf("valid options = %#v, %t", valid, ok)
	}

	for _, args := range [][]string{
		{"serve", "--run", "run-1"},
		{
			"serve", "--run", "run-1", "--journal", "/private/run.sqlite",
			"--journal", "/private/other.sqlite",
		},
		{
			"serve", "--run", "run-1", "--journal", "--manifest",
			"/private/manifest.json",
		},
		{
			"serve", "--run", "bad/run", "--journal", "/private/run.sqlite",
		},
		{
			"serve", "--run", "run-1", "--journal", "relative.sqlite",
		},
		{
			"serve", "--run", "run-1", "--journal", "/private/run.sqlite",
			"--token", "TOP-SECRET",
		},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 ||
			stderr.String() !=
				"usage: sworn serve --run ID --journal ABS "+
					"[--manifest ABS] [--operator-config ABS]\n" {
			t.Fatalf(
				"run(%v) stdout=%q stderr=%q",
				args,
				stdout.String(),
				stderr.String(),
			)
		}
		for _, secret := range []string{
			"TOP-SECRET",
			"/private/run.sqlite",
			"/private/manifest.json",
		} {
			if strings.Contains(stderr.String(), secret) {
				t.Fatalf("serve exposed rejected input %q", secret)
			}
		}
	}

	missing := filepath.Join(t.TempDir(), "TOP-SECRET.sqlite")
	var stdout, stderr bytes.Buffer
	if code := run(
		[]string{
			"serve", "--run", "run-1", "--journal", missing,
		},
		&stdout,
		&stderr,
	); code != 1 {
		t.Fatalf("missing existing run = %d, want 1", code)
	}
	if stdout.Len() != 0 ||
		stderr.String() != "sworn serve: unavailable\n" ||
		strings.Contains(stderr.String(), "TOP-SECRET") {
		t.Fatalf(
			"missing existing run stdout=%q stderr=%q",
			stdout.String(),
			stderr.String(),
		)
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatalf("serve created a journal without a manifest: %v", err)
	}
}

func TestOperatorDefaultsAreLocalAndOptIn(t *testing.T) {
	t.Parallel()

	settings, err := loadOperatorSettings("")
	if err != nil {
		t.Fatal(err)
	}
	if settings.localListen != defaultOperatorListen ||
		settings.public != nil ||
		len(settings.webhooks) != 0 ||
		settings.otel != nil {
		t.Fatalf("default settings = %#v", settings)
	}
}

func TestOperatorConfigFileAdmissionRejectsUnsafePathsAndReplacement(
	t *testing.T,
) {
	body := []byte(
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"}}`,
	)
	write := func(t *testing.T, root, name string, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, body, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("private regular file", func(t *testing.T) {
		path := write(t, t.TempDir(), "operator.json", 0o600)
		got, err := readPrivateOperatorFile(path, nil)
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("read = %q, %v", got, err)
		}
	})
	t.Run("permissions", func(t *testing.T) {
		path := write(t, t.TempDir(), "operator.json", 0o644)
		if _, err := readPrivateOperatorFile(path, nil); err == nil {
			t.Fatal("insecure config admitted")
		}
	})
	t.Run("file symlink", func(t *testing.T) {
		root := t.TempDir()
		target := write(t, root, "target.json", 0o600)
		link := filepath.Join(root, "operator.json")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := readPrivateOperatorFile(link, nil); err == nil {
			t.Fatal("symlink config admitted")
		}
	})
	t.Run("parent symlink", func(t *testing.T) {
		root := t.TempDir()
		real := filepath.Join(root, "real")
		if err := os.Mkdir(real, 0o700); err != nil {
			t.Fatal(err)
		}
		write(t, real, "operator.json", 0o600)
		link := filepath.Join(root, "linked")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		if _, err := readPrivateOperatorFile(
			filepath.Join(link, "operator.json"),
			nil,
		); err == nil {
			t.Fatal("symlink parent admitted")
		}
	})
	t.Run("replacement", func(t *testing.T) {
		root := t.TempDir()
		path := write(t, root, "operator.json", 0o600)
		replacement := write(t, root, "replacement.json", 0o600)
		if _, err := readPrivateOperatorFile(path, func() {
			if err := os.Rename(replacement, path); err != nil {
				t.Fatal(err)
			}
		}); err == nil {
			t.Fatal("replaced config admitted")
		}
	})
	t.Run("oversize", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "operator.json")
		if err := os.WriteFile(
			path,
			bytes.Repeat([]byte{'x'}, maxOperatorConfigBytes+1),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := readPrivateOperatorFile(path, nil); err == nil {
			t.Fatal("oversize config admitted")
		}
	})
	t.Run("relative", func(t *testing.T) {
		if _, err := readPrivateOperatorFile("operator.json", nil); err == nil {
			t.Fatal("relative config admitted")
		}
	})
}

func TestOperatorConfigRejectsAmbiguousAndOpenShapes(t *testing.T) {
	t.Parallel()

	valid := `{"schema_version":"sworn.operator-config/v1",` +
		`"local":{"listen":"127.0.0.1:7444"}}`
	tests := []string{
		valid + ` {}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444",` +
			`"listen":"127.0.0.1:7445"}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444","extra":true}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"},"extra":true}`,
	}
	for _, body := range tests {
		if _, err := parseOperatorConfig([]byte(body)); err == nil {
			t.Fatalf("open config admitted: %s", body)
		}
	}

	settings, err := parseOperatorConfig([]byte(valid))
	if err != nil || settings.localListen != "127.0.0.1:7444" {
		t.Fatalf("valid config = %#v, %v", settings, err)
	}
}

func TestOperatorConfigValidatesPublicWebhookAndOTelBoundaries(t *testing.T) {
	t.Parallel()

	certificate, key := operatorTestCertificate(t)
	config := operatorConfig{
		SchemaVersion: operatorConfigSchemaVersion,
		Local: operatorLocalConfig{
			Listen: "127.0.0.1:7444",
		},
		Public: &operatorPublicConfig{
			Listen:         "0.0.0.0:7445",
			Origin:         "https://sworn.example:7445",
			CertificatePEM: certificate,
			PrivateKeyPEM:  key,
			Token:          strings.Repeat("t", 32),
		},
		Webhooks: []operatorWebhookConfig{{
			ID:     "delivery",
			URL:    "https://hooks.example/updates",
			Secret: strings.Repeat("s", 32),
		}},
		OTel: &observe.Config{
			SchemaVersion: observe.OTelConfigSchemaVersion,
			Endpoint:      "https://otel.example/collect",
			Headers:       map[string]string{"Authorization": "private"},
		},
	}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := parseOperatorConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	if settings.public == nil ||
		settings.public.listen != "0.0.0.0:7445" ||
		settings.public.host != "sworn.example:7445" ||
		len(settings.webhooks) != 1 ||
		settings.otel == nil {
		t.Fatalf("settings = %#v", settings)
	}

	for _, mutate := range []func(*operatorConfig){
		func(value *operatorConfig) {
			value.Public.Listen = "127.0.0.1:7445"
		},
		func(value *operatorConfig) {
			value.Public.Origin = "https://127.0.0.1:7445"
		},
		func(value *operatorConfig) {
			value.Public.Token = "short"
		},
		func(value *operatorConfig) {
			value.Webhooks = append(
				value.Webhooks,
				value.Webhooks[0],
			)
		},
		func(value *operatorConfig) {
			value.OTel.Endpoint = "http://otel.example/collect"
		},
	} {
		copy := config
		publicCopy := *config.Public
		copy.Public = &publicCopy
		copy.Webhooks = append(
			[]operatorWebhookConfig(nil),
			config.Webhooks...,
		)
		otelCopy := *config.OTel
		otelCopy.Headers = map[string]string{"Authorization": "private"}
		copy.OTel = &otelCopy
		mutate(&copy)
		body, err := json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseOperatorConfig(body); err == nil {
			t.Fatalf("invalid config admitted: %s", body)
		} else if strings.Contains(
			err.Error(),
			"private",
		) || strings.Contains(err.Error(), config.Public.Token) {
			t.Fatalf("config error exposed a secret: %v", err)
		}
	}
}

func TestOperatorManifestAndExistingRunBinding(t *testing.T) {
	t.Parallel()

	body := operatorManifestBody(t, "run-1", "one")
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	manifests, err := admitOperatorManifest(serveOptions{
		runID: "run-1", manifestPath: path,
	})
	if err != nil || len(manifests) != 1 ||
		manifests[0].RunID() != "run-1" {
		t.Fatalf("manifest = %#v, %v", manifests, err)
	}
	if _, err := admitOperatorManifest(serveOptions{
		runID: "run-2", manifestPath: path,
	}); err == nil {
		t.Fatal("manifest run mismatch admitted")
	}

	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterRun(context.Background(), journal.Run{
		ID:             "run-1",
		ManifestDigest: manifests[0].Digest(),
		Repository:     "/tmp/repository",
		Release:        "release-1",
		TargetRef:      "refs/heads/main",
		CreatedAt:      time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := checkOperatorBinding(
		context.Background(),
		store,
		"run-1",
		manifests,
	); err != nil {
		t.Fatalf("matching binding: %v", err)
	}
	if err := checkOperatorBinding(
		context.Background(),
		store,
		"run-1",
		nil,
	); err != nil {
		t.Fatalf("existing binding: %v", err)
	}
	if err := checkOperatorBinding(
		context.Background(),
		store,
		"missing",
		nil,
	); err == nil {
		t.Fatal("missing run admitted without manifest")
	}
	other, err := cockpit.AdmitManifest(
		operatorManifestBody(t, "run-1", "two"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkOperatorBinding(
		context.Background(),
		store,
		"run-1",
		[]cockpit.AdmittedManifest{other},
	); err == nil {
		t.Fatal("mismatched manifest digest admitted")
	}
}

type fakeTelemetryStatus struct {
	status observe.Status
}

func (f fakeTelemetryStatus) Status() observe.Status { return f.status }

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type failedReadyWriter struct{}

func (failedReadyWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("closed")
}

func TestTelemetryHealthAdapterMapsOnlySafeStatus(t *testing.T) {
	t.Parallel()

	failedAt := time.Unix(1_700_000_000, 0).UTC()
	source := observe.Status{
		SchemaVersion:   observe.TelemetryStatusSchemaVersion,
		Enabled:         true,
		QueueCapacity:   256,
		QueueDepth:      7,
		Accepted:        20,
		Processed:       12,
		Dropped:         8,
		TraceExports:    6,
		MetricExports:   4,
		Failures:        3,
		LastFailureAt:   &failedAt,
		LastFailureCode: "trace_export",
	}
	got := (telemetryHealthAdapter{
		telemetry: fakeTelemetryStatus{status: source},
	}).TelemetryHealth()
	if got.SchemaVersion != cockpit.TelemetryHealthSchemaVersion ||
		!got.Enabled ||
		got.QueueDepth != 7 ||
		got.Dropped != 8 ||
		got.Failures != 3 ||
		got.LastFailureCode != "trace_export" ||
		got.LastFailureAt == nil ||
		!got.LastFailureAt.Equal(failedAt) {
		t.Fatalf("health = %#v", got)
	}
	disabled := (telemetryHealthAdapter{
		telemetry: observe.Noop(),
	}).TelemetryHealth()
	if disabled.Enabled || disabled.SchemaVersion !=
		cockpit.TelemetryHealthSchemaVersion {
		t.Fatalf("disabled health = %#v", disabled)
	}
}

func TestServeWiresExistingRunAndStopsOnContext(t *testing.T) {
	journalPath := boardJournalFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	configBody, err := json.Marshal(operatorConfig{
		SchemaVersion: operatorConfigSchemaVersion,
		Local:         operatorLocalConfig{Listen: address},
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "operator.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout synchronizedBuffer
	result := make(chan error, 1)
	go func() {
		result <- serveOperator(ctx, serveOptions{
			runID:          "run-1",
			journalPath:    journalPath,
			operatorConfig: configPath,
		}, &stdout)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(stdout.String(), "sworn serve: ready\n") &&
		time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(stdout.String(), "sworn serve: ready\n") {
		cancel()
		t.Fatalf("serve did not become ready; output %q", stdout.String())
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   2 * time.Second,
	}
	response, err := client.Get(
		"http://" + address + "/api/v1/operator/telemetry",
	)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var health cockpit.TelemetryHealth
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		_ = response.Body.Close()
		cancel()
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK ||
		health.Enabled ||
		health.SchemaVersion != cockpit.TelemetryHealthSchemaVersion {
		cancel()
		t.Fatalf("health status=%d body=%#v", response.StatusCode, health)
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after cancellation")
	}
}

func TestServeReadinessFailureUsesBoundedShutdown(t *testing.T) {
	journalPath := boardJournalFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	configBody, err := json.Marshal(operatorConfig{
		SchemaVersion: operatorConfigSchemaVersion,
		Local:         operatorLocalConfig{Listen: address},
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "operator.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- serveOperator(
			context.Background(),
			serveOptions{
				runID:          "run-1",
				journalPath:    journalPath,
				operatorConfig: configPath,
			},
			failedReadyWriter{},
		)
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("readiness output failure returned success")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("readiness output failure did not stop the operator")
	}
}

func operatorManifestBody(t *testing.T, runID, intent string) []byte {
	t.Helper()
	profile := driver.RoleSelection{
		Profile: "fixture",
		Model:   "fixture-model",
	}
	manifest := runtimepkg.Manifest{
		SchemaVersion:     runtimepkg.ManifestVersion,
		RunID:             runID,
		Repository:        "/tmp/repository",
		Release:           "release-1",
		TargetRef:         "refs/heads/main",
		Intent:            intent,
		MaxParallelTracks: 1,
		Approval: runtimepkg.ApprovalPolicy{
			Repository:          "acme/repo",
			Issue:               1,
			AllowedAuthorIDs:    []int64{1},
			AllowedAssociations: []string{"OWNER"},
		},
		Driver: runtimepkg.FakeDriverConfig{
			Executable: "/bin/true",
			Digest:     "sha256:" + strings.Repeat("a", 64),
			AdapterKey: "fixture",
			Profile:    "fixture",
		},
		Roles: driver.RoleSelections{
			Planner:     profile,
			Implementer: profile,
			Captain:     profile,
			Verifier:    profile,
		},
		Limits: driver.Limits{
			TimeoutMillis: 1,
			OutputBytes:   1,
		},
		Scripts: []runtimepkg.ScriptedAttempt{{
			Responsibility: driver.PlannerProposal,
			BatonAttempt:   1,
			Epoch:          1,
			Try:            1,
			Behavior:       "none",
		}},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := runtimepkg.ParseManifest(body); err != nil {
		t.Fatalf("manifest fixture: %v", err)
	}
	return body
}

func operatorTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sworn.example"},
		DNSNames:     []string{"sworn.example"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certificateDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&key.PublicKey,
		key,
	)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certificateDER,
		})),
		string(pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: keyDER,
		}))
}

func TestLiteralListenRejectsNonCanonicalAndWrongAuthority(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		value    string
		loopback bool
		ok       bool
	}{
		{"127.0.0.1:7337", true, true},
		{"[::1]:7337", true, true},
		{"0.0.0.0:7338", false, true},
		{"127.0.0.1:7338", false, false},
		{"0.0.0.0:7338", true, false},
		{"localhost:7337", true, false},
		{"127.000.000.001:7337", true, false},
		{"127.0.0.1:0", true, false},
		{"127.0.0.1:" + strconv.Itoa(1<<16), true, false},
	} {
		got, err := literalListen(test.value, test.loopback)
		if (err == nil) != test.ok {
			t.Errorf(
				"literalListen(%q, %t) = %q, %v",
				test.value,
				test.loopback,
				got,
				err,
			)
		}
	}
}

func TestOperatorHTTPServerKeepsSSEAndHeadersBounded(t *testing.T) {
	t.Parallel()

	server := newOperatorHTTPServer(http.NotFoundHandler())
	if server.ReadHeaderTimeout != operatorReadHeaderTimeout ||
		server.IdleTimeout != operatorIdleTimeout ||
		server.MaxHeaderBytes != operatorMaxHeaderBytes ||
		server.WriteTimeout != 0 {
		t.Fatalf("HTTP server policy = %#v", server)
	}
}

func TestOperatorErrorsNeverContainConfigSecrets(t *testing.T) {
	t.Parallel()

	const secret = "PRIVATE-OPERATOR-CREDENTIAL"
	body := []byte(fmt.Sprintf(
		`{"schema_version":"sworn.operator-config/v1",`+
			`"local":{"listen":"127.0.0.1:7444"},`+
			`"otel":{"schema_version":"sworn.otel-config/v1",`+
			`"endpoint":"http://example.invalid","headers":`+
			`{"Authorization":%q}}}`,
		secret,
	))
	_, err := parseOperatorConfig(body)
	if err == nil {
		t.Fatal("unsafe OTLP endpoint admitted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed secret: %v", err)
	}
}
