package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
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
		"--config", "/private/drivers.json",
		"--run", "run-1",
	})
	if !ok || valid.runID != "run-1" ||
		valid.journalPath != "/private/run.sqlite" ||
		valid.manifestPath != "/private/manifest.json" ||
		valid.driverConfig != "/private/drivers.json" ||
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
					"[--manifest ABS] [--config ABS] [--operator-config ABS]\n" {
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
		`{"schema_version":"sworn.operator-config/v1",` +
			`"LOCAL":{"listen":"127.0.0.1:7444"}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"},` +
			`"LOCAL":{"listen":"127.0.0.1:7445"}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"LISTEN":"127.0.0.1:7444"}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"},` +
			`"public":{"TOKEN":"` + strings.Repeat("t", 32) + `"}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"},` +
			`"webhooks":[{"SECRET":"` + strings.Repeat("s", 32) + `"}]}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"},` +
			`"otel":{"ENDPOINT":"https://otel.example"}}`,
		`{"schema_version":"sworn.operator-config/v1",` +
			`"local":{"listen":"127.0.0.1:7444"},` +
			`"otel":{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://otel.example",` +
			`"headers":{"X-Token":"one","x-token":"two"}}}`,
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

func TestOperatorManifestAdmissionDoesNotCreateRun(t *testing.T) {
	t.Parallel()

	body := operatorManifestBody(t, "run-1", "one")
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := admitOperatorManifest(serveOptions{
		runID: "run-1", manifestPath: path,
	})
	if err != nil || manifest == nil ||
		manifest.RunID() != "run-1" {
		t.Fatalf("manifest = %#v, %v", manifest, err)
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
	authority := &operatorRunAuthority{
		journal:        store,
		runID:          "run-1",
		manifestDigest: manifest.Digest(),
		allowAbsent:    true,
	}
	matched, err := authority.matches(context.Background())
	if err != nil || matched {
		t.Fatalf("absent authority = %t, %v", matched, err)
	}
	if _, err := store.RunBinding(
		context.Background(),
		"run-1",
	); !journal.IsCode(err, "RUN_NOT_FOUND") {
		t.Fatalf("admission created a run: %v", err)
	}
}

type fakeOperatorProjector struct {
	snapshotCalls int
	eventCalls    int
}

func (f *fakeOperatorProjector) Snapshot(
	context.Context,
	string,
) (cockpit.Snapshot, error) {
	f.snapshotCalls++
	return cockpit.Snapshot{SchemaVersion: cockpit.SnapshotSchemaVersion}, nil
}

func (f *fakeOperatorProjector) Events(
	context.Context,
	string,
	int64,
	int,
) (cockpit.EventPage, error) {
	f.eventCalls++
	return cockpit.EventPage{SchemaVersion: cockpit.SnapshotSchemaVersion}, nil
}

type fakeOperatorCommands struct {
	startCalls     int
	controlCalls   int
	redeliverCalls int
	onStart        func() error
}

func (f *fakeOperatorCommands) Start(
	context.Context,
	cockpit.StartCommand,
) (runtimepkg.RunStatus, error) {
	f.startCalls++
	if f.onStart != nil {
		if err := f.onStart(); err != nil {
			return runtimepkg.RunStatus{}, err
		}
	}
	return runtimepkg.RunStatus{RunID: "run-1"}, nil
}

func (f *fakeOperatorCommands) Control(
	context.Context,
	cockpit.ControlCommand,
) (runtimepkg.RunStatus, error) {
	f.controlCalls++
	return runtimepkg.RunStatus{RunID: "run-1"}, nil
}

func (f *fakeOperatorCommands) Redeliver(
	context.Context,
	cockpit.RedeliveryCommand,
) error {
	f.redeliverCalls++
	return nil
}

func TestOperatorAuthorityGuardsEveryConsumerAndActivatesOnStart(
	t *testing.T,
) {
	t.Parallel()

	ctx := context.Background()
	body := operatorManifestBody(t, "run-1", "expected")
	command, err := cockpit.AdmitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := runtimepkg.ParseManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "run.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authority := &operatorRunAuthority{
		journal:        store,
		runID:          "run-1",
		manifestDigest: command.Digest(),
		allowAbsent:    true,
	}
	baseProjector := &fakeOperatorProjector{}
	projector := &operatorProjector{
		authority: authority,
		delegate:  baseProjector,
	}
	baseCommands := &fakeOperatorCommands{onStart: func() error {
		return store.RegisterRun(ctx, journal.Run{
			ID:             parsed.RunID,
			ManifestDigest: command.Digest(),
			Repository:     parsed.Repository,
			Release:        parsed.Release,
			TargetRef:      parsed.TargetRef,
			CreatedAt:      time.Now().UTC(),
		})
	}}
	commands := &operatorCommands{
		authority: authority,
		delegate:  baseCommands,
	}

	if _, err := projector.Snapshot(ctx, "run-1"); err == nil {
		t.Fatal("snapshot consumed an absent run")
	}
	if _, err := projector.Events(ctx, "run-1", 0, 1); err == nil {
		t.Fatal("events consumed an absent run")
	}
	if _, err := commands.Control(ctx, cockpit.ControlCommand{
		RunID: "run-1",
	}); err == nil {
		t.Fatal("control consumed an absent run")
	}
	if err := commands.Redeliver(ctx, cockpit.RedeliveryCommand{
		RunID: "run-1",
	}); err == nil {
		t.Fatal("redelivery consumed an absent run")
	}
	if _, err := commands.Start(ctx, cockpit.StartCommand{
		ManifestDigest: "sha256:" + strings.Repeat("f", 64),
	}); err == nil || baseCommands.startCalls != 0 {
		t.Fatalf(
			"wrong start err=%v calls=%d",
			err,
			baseCommands.startCalls,
		)
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, time.Second)
	defer cancelWait()
	activated := make(chan bool, 1)
	go func() {
		activated <- waitForRunAuthority(
			waitCtx,
			authority,
			5*time.Millisecond,
		)
	}()
	beforeStart := time.Now().UTC()
	status, err := commands.Start(ctx, cockpit.StartCommand{
		ManifestDigest: command.Digest(),
	})
	if err != nil || status.RunID != "run-1" ||
		baseCommands.startCalls != 1 {
		t.Fatalf("matching start = %#v, %v", status, err)
	}
	select {
	case matched := <-activated:
		if !matched {
			t.Fatal("matching Start did not activate background authority")
		}
	case <-time.After(time.Second):
		t.Fatal("background authority did not activate")
	}
	binding, err := store.RunBinding(ctx, "run-1")
	if err != nil || binding.CreatedAt.Before(beforeStart) {
		t.Fatalf("start binding = %#v, %v", binding, err)
	}
	if _, err := projector.Snapshot(ctx, "run-1"); err != nil {
		t.Fatalf("matching snapshot: %v", err)
	}
	if _, err := projector.Events(ctx, "run-1", 0, 1); err != nil {
		t.Fatalf("matching events: %v", err)
	}
	if _, err := commands.Control(ctx, cockpit.ControlCommand{
		RunID: "run-1",
	}); err != nil {
		t.Fatalf("matching control: %v", err)
	}
	if err := commands.Redeliver(ctx, cockpit.RedeliveryCommand{
		RunID: "run-1",
	}); err != nil {
		t.Fatalf("matching redelivery: %v", err)
	}
	if baseProjector.snapshotCalls != 1 ||
		baseProjector.eventCalls != 1 ||
		baseCommands.controlCalls != 1 ||
		baseCommands.redeliverCalls != 1 {
		t.Fatalf(
			"delegate calls projector=%#v commands=%#v",
			baseProjector,
			baseCommands,
		)
	}

	conflictStore, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "conflict.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conflictStore.Close()
	otherBody := operatorManifestBody(t, "run-1", "conflict")
	other, err := cockpit.AdmitManifest(otherBody)
	if err != nil {
		t.Fatal(err)
	}
	if err := conflictStore.RegisterRun(ctx, journal.Run{
		ID:             parsed.RunID,
		ManifestDigest: other.Digest(),
		Repository:     parsed.Repository,
		Release:        parsed.Release,
		TargetRef:      parsed.TargetRef,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	conflictAuthority := &operatorRunAuthority{
		journal:        conflictStore,
		runID:          "run-1",
		manifestDigest: command.Digest(),
		allowAbsent:    true,
	}
	conflictProjectorDelegate := &fakeOperatorProjector{}
	conflictProjector := &operatorProjector{
		authority: conflictAuthority,
		delegate:  conflictProjectorDelegate,
	}
	conflictCommandDelegate := &fakeOperatorCommands{}
	conflictCommands := &operatorCommands{
		authority: conflictAuthority,
		delegate:  conflictCommandDelegate,
	}
	if waitForRunAuthority(ctx, conflictAuthority, time.Millisecond) {
		t.Fatal("conflicting run activated evaluator or webhook work")
	}
	if _, err := conflictProjector.Snapshot(ctx, "run-1"); err == nil {
		t.Fatal("conflicting snapshot was projected")
	}
	if _, err := conflictProjector.Events(ctx, "run-1", 0, 1); err == nil {
		t.Fatal("conflicting events were projected")
	}
	if _, err := conflictCommands.Start(ctx, cockpit.StartCommand{
		ManifestDigest: command.Digest(),
	}); err == nil {
		t.Fatal("conflicting run accepted Start")
	}
	if _, err := conflictCommands.Control(ctx, cockpit.ControlCommand{
		RunID: "run-1",
	}); err == nil {
		t.Fatal("conflicting run accepted control")
	}
	if err := conflictCommands.Redeliver(ctx, cockpit.RedeliveryCommand{
		RunID: "run-1",
	}); err == nil {
		t.Fatal("conflicting run accepted redelivery")
	}
	if conflictProjectorDelegate.snapshotCalls != 0 ||
		conflictProjectorDelegate.eventCalls != 0 ||
		conflictCommandDelegate.startCalls != 0 ||
		conflictCommandDelegate.controlCalls != 0 ||
		conflictCommandDelegate.redeliverCalls != 0 {
		t.Fatal("a conflicting consumer reached its delegate")
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

func TestTelemetryShutdownFailureAndTimeoutAreNonControlling(t *testing.T) {
	t.Parallel()

	called := false
	shutdownTelemetry(func(context.Context) error {
		called = true
		return fmt.Errorf("exporter failed")
	}, time.Second)
	if !called {
		t.Fatal("telemetry shutdown was not attempted")
	}

	started := time.Now()
	shutdownTelemetry(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, 20*time.Millisecond)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("telemetry shutdown was not bounded: %s", elapsed)
	}
}

func TestServeRejectsIncompleteExistingRunBeforeReadiness(t *testing.T) {
	ctx := context.Background()
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("b", 64)
	if err := store.RegisterRun(ctx, journal.Run{
		ID:             "run-incomplete",
		ManifestDigest: digest,
		Repository:     "/tmp/repository",
		Release:        "release-1",
		TargetRef:      "refs/heads/main",
		CreatedAt:      time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

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
	var stdout bytes.Buffer
	serveCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := serveOperator(serveCtx, serveOptions{
		runID:          "run-incomplete",
		journalPath:    journalPath,
		operatorConfig: configPath,
	}, &stdout); err == nil {
		t.Fatal("incomplete existing run unexpectedly became available")
	}
	if stdout.Len() != 0 {
		t.Fatalf("incomplete run emitted readiness output %q", stdout.String())
	}

	reader, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, snapshotErr := reader.Snapshot(ctx, "run-incomplete")
	closeErr := reader.Close()
	if snapshotErr != nil || closeErr != nil {
		t.Fatalf("read incomplete run: snapshot=%v close=%v", snapshotErr, closeErr)
	}
	if snapshot.Run.ManifestDigest != digest ||
		len(snapshot.Commands) != 0 ||
		len(snapshot.Effects) != 0 ||
		len(snapshot.Events) != 0 {
		t.Fatalf("incomplete run was consumed or changed: %#v", snapshot)
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

func TestServeManifestCreatesRunOnlyAtStart(t *testing.T) {
	body := operatorManifestBody(t, "run-start", "start-time authority")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := cockpit.AdmitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
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
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout synchronizedBuffer
	result := make(chan error, 1)
	go func() {
		result <- serveOperator(ctx, serveOptions{
			runID:          "run-start",
			journalPath:    journalPath,
			manifestPath:   manifestPath,
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
	reader, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if _, err := reader.RunBinding(
		context.Background(),
		"run-start",
	); !journal.IsCode(err, "RUN_NOT_FOUND") {
		_ = reader.Close()
		cancel()
		t.Fatalf("serve created a run before Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	beforeStart := time.Now().UTC()
	requestBody, err := json.Marshal(cockpit.StartCommand{
		ManifestDigest: manifest.Digest(),
	})
	if err != nil {
		_ = reader.Close()
		cancel()
		t.Fatal(err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		"http://"+address+"/api/v1/start",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		_ = reader.Close()
		cancel()
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   5 * time.Second,
	}
	response, err := client.Do(request)
	if err != nil {
		_ = reader.Close()
		cancel()
		t.Fatal(err)
	}
	_ = response.Body.Close()
	binding, err := reader.RunBinding(
		context.Background(),
		"run-start",
	)
	if closeErr := reader.Close(); closeErr != nil {
		cancel()
		t.Fatal(closeErr)
	}
	if err != nil || binding.ManifestDigest != manifest.Digest() ||
		binding.CreatedAt.Before(beforeStart) {
		cancel()
		t.Fatalf(
			"Start binding=%#v err=%v response=%d",
			binding,
			err,
			response.StatusCode,
		)
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after Start")
	}
}

func TestServeLaterListenerFailureLeavesNoPhantomRun(t *testing.T) {
	body := operatorManifestBody(t, "run-later-fail", "no phantom")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	configBody, err := json.Marshal(operatorConfig{
		SchemaVersion: operatorConfigSchemaVersion,
		Local: operatorLocalConfig{
			Listen: occupied.Addr().String(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "operator.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	var stdout bytes.Buffer
	if err := serveOperator(
		context.Background(),
		serveOptions{
			runID:          "run-later-fail",
			journalPath:    journalPath,
			manifestPath:   manifestPath,
			operatorConfig: configPath,
		},
		&stdout,
	); err == nil {
		t.Fatal("occupied listener unexpectedly started")
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed startup output = %q", stdout.String())
	}
	reader, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.RunBinding(
		context.Background(),
		"run-later-fail",
	); !journal.IsCode(err, "RUN_NOT_FOUND") {
		t.Fatalf("failed startup left a phantom run: %v", err)
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
		Driver: &runtimepkg.FakeDriverConfig{
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
		server.ReadTimeout != operatorReadTimeout ||
		server.IdleTimeout != operatorIdleTimeout ||
		server.MaxHeaderBytes != operatorMaxHeaderBytes ||
		server.WriteTimeout != 0 {
		t.Fatalf("HTTP server policy = %#v", server)
	}
}

func TestOperatorHTTPServerTimesOutSlowMutationBody(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "request timeout", http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := newOperatorHTTPServer(handler)
	server.ReadTimeout = 50 * time.Millisecond
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() {
		served <- server.Serve(listener)
	}()
	connection, err := net.DialTimeout(
		"tcp",
		listener.Addr().String(),
		time.Second,
	)
	if err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := fmt.Fprintf(
		connection,
		"POST / HTTP/1.1\r\nHost: %s\r\n"+
			"Content-Type: application/json\r\n"+
			"Content-Length: 32\r\n\r\n{",
		listener.Addr().String(),
	); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(
		time.Now().Add(time.Second),
	); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	response, err := http.ReadResponse(
		bufio.NewReader(connection),
		&http.Request{Method: http.MethodPost},
	)
	if err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusRequestTimeout {
		_ = server.Close()
		t.Fatalf("slow mutation status = %d", response.StatusCode)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-served:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serve result: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
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
