package cockpit

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	maxRequestBytes = 8 * 1024
	defaultSSELimit = 32
)

var (
	httpIdentityPattern = regexp.MustCompile(
		`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`,
	)
	tokenPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{32,512}$`)
)

//go:embed web/*
var embeddedWeb embed.FS

type SnapshotAPI interface {
	Snapshot(context.Context, string) (Snapshot, error)
	Events(context.Context, string, int64, int) (EventPage, error)
}

type CommandAPI interface {
	Start(context.Context, StartCommand) (runtimepkg.RunStatus, error)
	Control(context.Context, ControlCommand) (runtimepkg.RunStatus, error)
	Redeliver(context.Context, RedeliveryCommand) error
}

type HTTPConfig struct {
	Host        string
	Origin      string
	BearerToken []byte
	MaxSSE      int
}

type HTTPHandler struct {
	projector SnapshotAPI
	commands  CommandAPI
	host      string
	origin    string
	token     []byte
	localHost bool
	sse       chan struct{}
}

type headResponseWriter struct {
	http.ResponseWriter
}

func (w headResponseWriter) Write(body []byte) (int, error) {
	return len(body), nil
}

func NewHTTPHandler(
	projector SnapshotAPI,
	commands CommandAPI,
	config HTTPConfig,
) (*HTTPHandler, error) {
	if projector == nil || commands == nil ||
		!validHostOrigin(config.Host, config.Origin) {
		return nil, fail("INVALID_HTTP_CONFIG")
	}
	if len(config.BearerToken) != 0 &&
		!tokenPattern.Match(config.BearerToken) {
		return nil, fail("INVALID_HTTP_CONFIG")
	}
	if config.MaxSSE == 0 {
		config.MaxSSE = defaultSSELimit
	}
	if config.MaxSSE < 1 || config.MaxSSE > 256 {
		return nil, fail("INVALID_HTTP_CONFIG")
	}
	return &HTTPHandler{
		projector: projector,
		commands:  commands,
		host:      config.Host,
		origin:    config.Origin,
		token:     append([]byte(nil), config.BearerToken...),
		localHost: loopbackAuthority(config.Host),
		sse:       make(chan struct{}, config.MaxSSE),
	}, nil
}

func validHostOrigin(host, origin string) bool {
	if host == "" || origin == "" ||
		strings.ContainsAny(host, " \t\r\n/@?#") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host != host ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return true
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r != nil && r.Method == http.MethodHead {
		w = headResponseWriter{ResponseWriter: w}
	}
	h.setSecurityHeaders(w)
	if r == nil || r.URL == nil || r.Host != h.host ||
		r.URL.EscapedPath() != r.URL.Path {
		writeHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != h.origin {
		writeHTTPError(w, http.StatusForbidden, "ORIGIN_FORBIDDEN")
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" &&
		site != "same-origin" && site != "none" {
		writeHTTPError(w, http.StatusForbidden, "ORIGIN_FORBIDDEN")
		return
	}
	local := h.localRequest(r)
	mutating := r.Method != http.MethodGet && r.Method != http.MethodHead
	if mutating && !local {
		writeHTTPError(w, http.StatusForbidden, "REMOTE_MUTATION_FORBIDDEN")
		return
	}
	if !local {
		if r.TLS == nil || len(h.token) == 0 {
			writeHTTPError(w, http.StatusForbidden, "REMOTE_READ_FORBIDDEN")
			return
		}
		if !h.authenticated(r.Header.Get("Authorization")) {
			w.Header().Add("WWW-Authenticate", `Bearer realm="sworn"`)
			w.Header().Add(
				"WWW-Authenticate",
				`Basic realm="sworn", charset="UTF-8"`,
			)
			writeHTTPError(w, http.StatusUnauthorized, "AUTH_REQUIRED")
			return
		}
	}
	h.route(w, r)
}

func (h *HTTPHandler) localRequest(r *http.Request) bool {
	return r != nil && loopbackPeer(r.RemoteAddr) && h.localHost
}

func (h *HTTPHandler) setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'self'; "+
			"font-src 'self'; connect-src 'self'; img-src 'self'; "+
			"object-src 'none'; worker-src 'none'; base-uri 'none'; "+
			"frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

func loopbackPeer(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loopbackAuthority(authority string) bool {
	parsed, err := url.Parse("//" + authority)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *HTTPHandler) authenticated(header string) bool {
	var provided []byte
	if strings.HasPrefix(header, "Bearer ") {
		provided = []byte(strings.TrimPrefix(header, "Bearer "))
	} else {
		request := &http.Request{Header: http.Header{
			"Authorization": []string{header},
		}}
		username, password, ok := request.BasicAuth()
		if !ok || username != "sworn" {
			return false
		}
		provided = []byte(password)
	}
	if len(provided) != len(h.token) {
		return false
	}
	return subtle.ConstantTimeCompare(provided, h.token) == 1
}

func (h *HTTPHandler) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" &&
		(strings.HasSuffix(r.URL.Path, "/") ||
			strings.Contains(r.URL.Path, "//")) {
		writeHTTPError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	switch r.URL.Path {
	case "/":
		h.serveAsset(w, r, "web/index.html", "text/html; charset=utf-8")
		return
	case "/assets/app.css":
		h.serveAsset(w, r, "web/app.css", "text/css; charset=utf-8")
		return
	case "/assets/app.js":
		h.serveAsset(w, r, "web/app.js", "text/javascript; charset=utf-8")
		return
	case "/assets/barlow-condensed-regular.woff2":
		h.serveAsset(w, r, "web/fonts/barlow-condensed-regular.woff2", "font/woff2")
		return
	case "/assets/barlow-condensed-bold.woff2":
		h.serveAsset(w, r, "web/fonts/barlow-condensed-bold.woff2", "font/woff2")
		return
	case "/assets/atkinson-hyperlegible-regular.woff2":
		h.serveAsset(w, r, "web/fonts/atkinson-hyperlegible-regular.woff2", "font/woff2")
		return
	case "/assets/atkinson-hyperlegible-bold.woff2":
		h.serveAsset(w, r, "web/fonts/atkinson-hyperlegible-bold.woff2", "font/woff2")
		return
	case "/assets/licenses/OFL-Barlow.txt":
		h.serveAsset(w, r, "web/licenses/OFL-Barlow.txt", "text/plain; charset=utf-8")
		return
	case "/assets/licenses/OFL-Atkinson-Hyperlegible.txt":
		h.serveAsset(
			w,
			r,
			"web/licenses/OFL-Atkinson-Hyperlegible.txt",
			"text/plain; charset=utf-8",
		)
		return
	case "/api/v1/start":
		h.serveStart(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) == 2 && parts[0] == "runs" &&
		httpIdentityPattern.MatchString(parts[1]) {
		h.serveAsset(w, r, "web/index.html", "text/html; charset=utf-8")
		return
	}
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "v1" ||
		parts[2] != "runs" || !httpIdentityPattern.MatchString(parts[3]) {
		writeHTTPError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	runID := parts[3]
	switch {
	case len(parts) == 5 && parts[4] == "snapshot":
		h.serveSnapshot(w, r, runID)
	case len(parts) == 5 && parts[4] == "events":
		h.serveEvents(w, r, runID)
	case len(parts) == 5 && parts[4] == "commands":
		h.serveControl(w, r, runID)
	case len(parts) == 6 && parts[4] == "notifications" &&
		parts[5] == "redeliver":
		h.serveRedelivery(w, r, runID)
	default:
		writeHTTPError(w, http.StatusNotFound, "NOT_FOUND")
	}
}

func (h *HTTPHandler) serveAsset(
	w http.ResponseWriter,
	r *http.Request,
	path, contentType string,
) {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) ||
		r.URL.RawQuery != "" {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	body, err := embeddedWeb.ReadFile(path)
	if err != nil {
		writeHTTPError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

func (h *HTTPHandler) serveSnapshot(
	w http.ResponseWriter,
	r *http.Request,
	runID string,
) {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) ||
		r.URL.RawQuery != "" {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	snapshot, err := h.projector.Snapshot(r.Context(), runID)
	if err != nil {
		status := http.StatusServiceUnavailable
		if IsCode(err, "SNAPSHOT_UNSTABLE") {
			status = http.StatusConflict
		}
		writeHTTPError(w, status, errorCode(err))
		return
	}
	if !h.localRequest(r) {
		snapshot.Actions = []Action{}
	}
	writeJSON(w, r, http.StatusOK, snapshot)
}

func (h *HTTPHandler) serveEvents(
	w http.ResponseWriter,
	r *http.Request,
	runID string,
) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	after, limit, ok := eventQuery(r)
	if !ok {
		writeHTTPError(w, http.StatusBadRequest, "INVALID_EVENT_WINDOW")
		return
	}
	if r.Method == http.MethodGet &&
		strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		h.serveSSE(w, r, runID, after, limit)
		return
	}
	page, err := h.projector.Events(r.Context(), runID, after, limit)
	if err != nil {
		writeHTTPError(w, http.StatusServiceUnavailable, errorCode(err))
		return
	}
	writeJSON(w, r, http.StatusOK, page)
}

func eventQuery(r *http.Request) (int64, int, bool) {
	values := r.URL.Query()
	for key, items := range values {
		if (key != "after" && key != "limit") || len(items) != 1 {
			return 0, 0, false
		}
	}
	after := int64(0)
	if value := values.Get("after"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, false
		}
		after = parsed
	}
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		parsed, err := strconv.ParseInt(last, 10, 64)
		if err != nil || parsed < 0 ||
			(values.Has("after") && parsed < after) {
			return 0, 0, false
		}
		after = parsed
	}
	limit := 128
	if value := values.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 256 {
			return 0, 0, false
		}
		limit = parsed
	}
	return after, limit, true
}

func (h *HTTPHandler) serveSSE(
	w http.ResponseWriter,
	r *http.Request,
	runID string,
	after int64,
	limit int,
) {
	select {
	case h.sse <- struct{}{}:
		defer func() { <-h.sse }()
	default:
		writeHTTPError(w, http.StatusServiceUnavailable, "SSE_LIMIT")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeHTTPError(w, http.StatusServiceUnavailable, "SSE_UNAVAILABLE")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		page, err := h.projector.Events(r.Context(), runID, after, limit)
		if err != nil {
			_, _ = io.WriteString(w, "event: unavailable\ndata: {\"code\":\"REPLAY_UNAVAILABLE\"}\n\n")
			flusher.Flush()
			return
		}
		for _, event := range page.Events {
			body, _ := json.Marshal(struct {
				SchemaVersion string `json:"schema_version"`
				ThroughOffset int64  `json:"through_offset"`
			}{
				SchemaVersion: SnapshotSchemaVersion,
				ThroughOffset: event.Offset,
			})
			_, _ = fmt.Fprintf(
				w,
				"id: %d\nevent: invalidate\ndata: %s\n\n",
				event.Offset,
				body,
			)
			after = event.Offset
		}
		flusher.Flush()
		if page.HasMore {
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = io.WriteString(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (h *HTTPHandler) serveStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	var command StartCommand
	if !decodeRequest(w, r, &command) {
		return
	}
	status, err := h.commands.Start(r.Context(), command)
	if err != nil {
		writeHTTPError(w, http.StatusConflict, errorCode(err))
		return
	}
	writeJSON(w, r, http.StatusOK, status)
}

func (h *HTTPHandler) serveControl(
	w http.ResponseWriter,
	r *http.Request,
	runID string,
) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	var command ControlCommand
	if !decodeRequest(w, r, &command) {
		return
	}
	if command.RunID != runID {
		writeHTTPError(w, http.StatusBadRequest, "RUN_BINDING_MISMATCH")
		return
	}
	status, err := h.commands.Control(r.Context(), command)
	if err != nil {
		writeHTTPError(w, http.StatusConflict, errorCode(err))
		return
	}
	writeJSON(w, r, http.StatusOK, status)
}

func (h *HTTPHandler) serveRedelivery(
	w http.ResponseWriter,
	r *http.Request,
	runID string,
) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	var command RedeliveryCommand
	if !decodeRequest(w, r, &command) {
		return
	}
	if command.RunID != runID {
		writeHTTPError(w, http.StatusBadRequest, "RUN_BINDING_MISMATCH")
		return
	}
	if err := h.commands.Redeliver(r.Context(), command); err != nil {
		writeHTTPError(w, http.StatusConflict, errorCode(err))
		return
	}
	writeJSON(w, r, http.StatusOK, struct {
		Accepted bool `json:"accepted"`
	}{Accepted: true})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, parameters, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)
	if err != nil || mediaType != "application/json" ||
		(len(parameters) != 0 &&
			(len(parameters) != 1 || parameters["charset"] != "utf-8")) {
		writeHTTPError(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeHTTPError(w, http.StatusBadRequest, "INVALID_JSON")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeHTTPError(w, http.StatusBadRequest, "INVALID_JSON")
		return false
	}
	return true
}

func writeJSON(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	value any,
) {
	body, err := json.Marshal(value)
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "ENCODING_FAILED")
		return
	}
	body = append(body, '\n')
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func writeHTTPError(w http.ResponseWriter, status int, code string) {
	body, _ := json.Marshal(struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}{Error: struct {
		Code string `json:"code"`
	}{Code: code}})
	body = append(body, '\n')
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func errorCode(err error) string {
	var cockpitError *Error
	if errors.As(err, &cockpitError) {
		return cockpitError.Code
	}
	return "UNAVAILABLE"
}
