package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/observe"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	operatorPollInterval      = time.Second
	operatorShutdownTimeout   = 5 * time.Second
	operatorReadHeaderTimeout = 5 * time.Second
	operatorReadTimeout       = 15 * time.Second
	operatorIdleTimeout       = 60 * time.Second
	operatorMaxHeaderBytes    = 16 * 1024
)

var operatorIdentityPattern = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`,
)

type serveOptions struct {
	runID          string
	journalPath    string
	manifestPath   string
	driverConfig   string
	operatorConfig string
}

type operatorListeners struct {
	local  net.Listener
	public net.Listener
}

type telemetryStatusSource interface {
	Status() observe.Status
}

type telemetryHealthAdapter struct {
	telemetry telemetryStatusSource
}

func (a telemetryHealthAdapter) TelemetryHealth() cockpit.TelemetryHealth {
	status := a.telemetry.Status()
	return cockpit.TelemetryHealth{
		SchemaVersion:   cockpit.TelemetryHealthSchemaVersion,
		Enabled:         status.Enabled,
		QueueCapacity:   status.QueueCapacity,
		QueueDepth:      status.QueueDepth,
		Accepted:        status.Accepted,
		Processed:       status.Processed,
		Dropped:         status.Dropped,
		TraceExports:    status.TraceExports,
		MetricExports:   status.MetricExports,
		Failures:        status.Failures,
		LastSuccessAt:   status.LastSuccessAt,
		LastFailureAt:   status.LastFailureAt,
		LastFailureCode: status.LastFailureCode,
	}
}

func runServe(args []string, stdout, stderr io.Writer) int {
	options, ok := parseServeOptions(args)
	if !ok {
		_, _ = io.WriteString(
			stderr,
			"usage: sworn serve --run ID --journal ABS "+
				"[--manifest ABS] [--config ABS] [--operator-config ABS]\n",
		)
		return 2
	}
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	if err := serveOperator(ctx, options, stdout); err != nil {
		_, _ = io.WriteString(stderr, "sworn serve: unavailable\n")
		return 1
	}
	return 0
}

func parseServeOptions(args []string) (serveOptions, bool) {
	allowed := map[string]bool{
		"--run":             true,
		"--journal":         true,
		"--manifest":        true,
		"--config":          true,
		"--operator-config": true,
	}
	values := make(map[string]string, len(allowed))
	for index := 0; index < len(args); index++ {
		name := args[index]
		if !allowed[name] || index+1 >= len(args) ||
			values[name] != "" {
			return serveOptions{}, false
		}
		index++
		value := args[index]
		if value == "" || strings.HasPrefix(value, "--") {
			return serveOptions{}, false
		}
		values[name] = value
	}
	if values["--run"] == "" || values["--journal"] == "" ||
		!operatorIdentityPattern.MatchString(values["--run"]) ||
		!absoluteCleanOperatorPath(values["--journal"]) ||
		(values["--manifest"] != "" &&
			!absoluteCleanOperatorPath(values["--manifest"])) ||
		(values["--config"] != "" &&
			!absoluteCleanOperatorPath(values["--config"])) ||
		(values["--operator-config"] != "" &&
			!absoluteCleanOperatorPath(values["--operator-config"])) {
		return serveOptions{}, false
	}
	return serveOptions{
		runID:          values["--run"],
		journalPath:    values["--journal"],
		manifestPath:   values["--manifest"],
		driverConfig:   values["--config"],
		operatorConfig: values["--operator-config"],
	}, true
}

func absoluteCleanOperatorPath(value string) bool {
	return value != "" &&
		filepath.IsAbs(value) &&
		filepath.Clean(value) == value &&
		!strings.ContainsRune(value, 0)
}

func serveOperator(
	parent context.Context,
	options serveOptions,
	stdout io.Writer,
) error {
	if parent == nil || stdout == nil {
		return errors.New("operator unavailable")
	}
	settings, err := loadOperatorSettings(options.operatorConfig)
	if err != nil {
		return err
	}
	manifest, err := admitOperatorManifest(options)
	if err != nil {
		return err
	}
	expectedDigest := ""
	allowAbsent := manifest != nil
	if manifest != nil {
		expectedDigest = manifest.Digest()
	}
	if manifest == nil {
		statusReader, err := runtimepkg.OpenStatusService(
			parent,
			options.journalPath,
		)
		if err != nil {
			return errors.New("operator unavailable")
		}
		status, statusErr := statusReader.Status(
			parent,
			options.runID,
		)
		closeErr := statusReader.Close()
		if statusErr != nil || closeErr != nil {
			return errors.New("operator unavailable")
		}
		expectedDigest = status.ManifestDigest
	}

	runtimeService, driverFactory, err := openRuntimeService(
		parent,
		options.journalPath,
		options.driverConfig,
	)
	if err != nil {
		return errors.New("operator unavailable")
	}
	defer runtimeService.Close()
	defer driverFactory.Close()
	operatorStore, err := journal.Open(parent, options.journalPath)
	if err != nil {
		return errors.New("operator unavailable")
	}
	defer operatorStore.Close()
	authority := &operatorRunAuthority{
		journal:        operatorStore,
		runID:          options.runID,
		manifestDigest: expectedDigest,
		allowAbsent:    allowAbsent,
	}
	matched, err := authority.matches(parent)
	if err != nil || (!matched && !allowAbsent) {
		return errors.New("operator unavailable")
	}
	manifests := make([]cockpit.AdmittedManifest, 0, 1)
	if manifest != nil {
		manifests = append(manifests, *manifest)
	}

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return errors.New("operator unavailable")
	}
	stateReader, err := cockpit.NewGitStateReader(gitExecutable)
	if err != nil {
		return errors.New("operator unavailable")
	}
	baseProjector, err := cockpit.NewProjector(
		operatorStore,
		runtimeService,
		stateReader,
	)
	if err != nil {
		return errors.New("operator unavailable")
	}
	projector := &operatorProjector{
		authority: authority,
		delegate:  baseProjector,
	}
	baseCommands, err := cockpit.NewCommandFacade(
		runtimeService,
		operatorStore,
		manifests,
	)
	if err != nil {
		return errors.New("operator unavailable")
	}
	commands := &operatorCommands{
		authority: authority,
		delegate:  baseCommands,
	}
	evaluator, err := observe.NewEvaluator(
		operatorStore,
		projector,
		swornVersion,
	)
	if err != nil {
		return errors.New("operator unavailable")
	}
	var webhookService *cockpit.WebhookService
	if len(settings.webhooks) != 0 {
		webhookService, err = cockpit.NewWebhookService(
			operatorStore,
			settings.webhooks,
		)
		if err != nil {
			return errors.New("operator unavailable")
		}
	}

	listeners, err := bindOperatorListeners(settings)
	if err != nil {
		return errors.New("operator unavailable")
	}
	defer listeners.close()

	telemetry := observe.Noop()
	if settings.otel != nil {
		telemetry, err = observe.NewOTLP(
			parent,
			*settings.otel,
			swornVersion,
		)
		if err != nil {
			return errors.New("operator unavailable")
		}
	}
	telemetryOpen := true
	defer func() {
		if telemetryOpen {
			shutdownTelemetry(
				telemetry.Shutdown,
				operatorShutdownTimeout,
			)
		}
	}()

	localHandler, err := cockpit.NewHTTPHandler(
		projector,
		commands,
		cockpit.HTTPConfig{
			RunID:     options.runID,
			Host:      settings.localListen,
			Origin:    "http://" + settings.localListen,
			Telemetry: telemetryHealthAdapter{telemetry: telemetry},
		},
	)
	if err != nil {
		return errors.New("operator unavailable")
	}
	localServer := newOperatorHTTPServer(localHandler)
	var publicServer *http.Server
	if settings.public != nil {
		publicHandler, err := cockpit.NewHTTPHandler(
			projector,
			commands,
			cockpit.HTTPConfig{
				RunID:       options.runID,
				Host:        settings.public.host,
				Origin:      settings.public.origin,
				BearerToken: settings.public.token,
			},
		)
		if err != nil {
			return errors.New("operator unavailable")
		}
		publicServer = newOperatorHTTPServer(publicHandler)
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	localServer.BaseContext = func(net.Listener) context.Context { return ctx }
	if publicServer != nil {
		publicServer.BaseContext = func(net.Listener) context.Context {
			return ctx
		}
	}

	var workers sync.WaitGroup
	serverErrors := make(chan error, 2)
	workers.Add(1)
	go func() {
		defer workers.Done()
		if err := localServer.Serve(listeners.local); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			select {
			case serverErrors <- err:
			default:
			}
		}
	}()
	if publicServer != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := publicServer.Serve(listeners.public); err != nil &&
				!errors.Is(err, http.ErrServerClosed) {
				select {
				case serverErrors <- err:
				default:
				}
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		runEvaluationLoop(
			ctx,
			authority,
			evaluator,
			telemetry,
			options.runID,
		)
	}()
	if webhookService != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if !waitForRunAuthority(
				ctx,
				authority,
				operatorPollInterval,
			) {
				return
			}
			_ = webhookService.Run(
				ctx,
				options.runID,
				operatorPollInterval,
			)
		}()
	}
	var serveErr error
	if _, err := io.WriteString(stdout, "sworn serve: ready\n"); err != nil {
		serveErr = errors.New("operator unavailable")
		cancel()
	}

	if serveErr == nil {
		select {
		case <-ctx.Done():
		case serveErr = <-serverErrors:
		}
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		operatorShutdownTimeout,
	)
	defer shutdownCancel()
	var shutdowns sync.WaitGroup
	shutdownErrors := make(chan error, 2)
	shutdownServer := func(server *http.Server) {
		defer shutdowns.Done()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			select {
			case shutdownErrors <- err:
			default:
			}
		}
	}
	shutdowns.Add(1)
	go shutdownServer(localServer)
	if publicServer != nil {
		shutdowns.Add(1)
		go shutdownServer(publicServer)
	}
	shutdowns.Wait()
	close(shutdownErrors)
	var shutdownErr error
	for err := range shutdownErrors {
		if shutdownErr == nil {
			shutdownErr = err
		}
	}
	workers.Wait()
	shutdownTelemetry(telemetry.Shutdown, operatorShutdownTimeout)
	telemetryOpen = false
	if serveErr != nil || shutdownErr != nil {
		return errors.New("operator unavailable")
	}
	return nil
}

func admitOperatorManifest(
	options serveOptions,
) (*cockpit.AdmittedManifest, error) {
	if options.manifestPath == "" {
		return nil, nil
	}
	body, err := readManifest(options.manifestPath)
	if err != nil {
		return nil, errors.New("operator unavailable")
	}
	manifest, err := cockpit.AdmitManifest(body)
	if err != nil || manifest.RunID() != options.runID {
		return nil, errors.New("operator unavailable")
	}
	return &manifest, nil
}

func shutdownTelemetry(
	shutdown func(context.Context) error,
	timeout time.Duration,
) {
	// Telemetry is a lossy projection. Its shutdown outcome must never become
	// delivery or process authority.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = shutdown(ctx)
}

func runEvaluationLoop(
	ctx context.Context,
	authority *operatorRunAuthority,
	evaluator *observe.Evaluator,
	telemetry *observe.Telemetry,
	runID string,
) {
	if !waitForRunAuthority(ctx, authority, operatorPollInterval) {
		return
	}
	ticker := time.NewTicker(operatorPollInterval)
	defer ticker.Stop()
	for {
		record, advanced, _ := evaluator.Advance(ctx, runID)
		if advanced {
			telemetry.TryEnqueue(record)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func newOperatorHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: operatorReadHeaderTimeout,
		ReadTimeout:       operatorReadTimeout,
		IdleTimeout:       operatorIdleTimeout,
		MaxHeaderBytes:    operatorMaxHeaderBytes,
	}
}

func bindOperatorListeners(
	settings operatorSettings,
) (operatorListeners, error) {
	local, err := net.Listen("tcp", settings.localListen)
	if err != nil {
		return operatorListeners{}, err
	}
	result := operatorListeners{local: local}
	if settings.public == nil {
		return result, nil
	}
	public, err := net.Listen("tcp", settings.public.listen)
	if err != nil {
		_ = local.Close()
		return operatorListeners{}, err
	}
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{settings.public.certificate},
	}
	result.public = tls.NewListener(public, tlsConfig)
	return result, nil
}

func (l *operatorListeners) close() {
	if l == nil {
		return
	}
	if l.public != nil {
		_ = l.public.Close()
	}
	if l.local != nil {
		_ = l.local.Close()
	}
}
