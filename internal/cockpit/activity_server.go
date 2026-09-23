package cockpit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// serveActivity exposes the activity projection on a new route under the
// run's API, as a JSON page and, when the client accepts
// text/event-stream, as server-sent events whose frames carry the turn
// content. It reuses the events route's resume (Last-Event-ID or after),
// keepalive cadence (1s), concurrent-stream gate and SSE_LIMIT refusal,
// and sits behind the same canonicalisation, origin, loopback and bearer
// rules as every other run route (the shared ServeHTTP gate). The
// existing events route keeps its contract byte for byte.
func (h *HTTPHandler) serveActivity(
	w http.ResponseWriter,
	r *http.Request,
	runID string,
) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeHTTPError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	after, limit, filter, ok := activityQuery(r)
	if !ok {
		writeHTTPError(w, http.StatusBadRequest, "INVALID_EVENT_WINDOW")
		return
	}
	if r.Method == http.MethodGet &&
		strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		h.serveActivitySSE(w, r, runID, after, limit, filter)
		return
	}
	activity, ok := h.projector.(ActivityAPI)
	if !ok {
		writeHTTPError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	page, err := activity.Activity(r.Context(), runID, after, limit, filter)
	if err != nil {
		writeHTTPError(w, http.StatusServiceUnavailable, errorCode(err))
		return
	}
	writeJSON(w, r, http.StatusOK, page)
}

func activityQuery(r *http.Request) (int64, int, ActivityFilter, bool) {
	var filter ActivityFilter
	values := r.URL.Query()
	for key, items := range values {
		if (key != "after" && key != "limit" && key != "track" && key != "slice" && key != "effect_id" && key != "work_id") || len(items) != 1 {
			return 0, 0, ActivityFilter{}, false
		}
	}
	after := int64(0)
	if value := values.Get("after"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, ActivityFilter{}, false
		}
		after = parsed
	}
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		parsed, err := strconv.ParseInt(last, 10, 64)
		if err != nil || parsed < 0 ||
			(values.Has("after") && parsed < after) {
			return 0, 0, ActivityFilter{}, false
		}
		after = parsed
	}
	limit := 128
	if value := values.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 256 {
			return 0, 0, ActivityFilter{}, false
		}
		limit = parsed
	}
	filter.Track = values.Get("track")
	filter.Slice = values.Get("slice")
	filter.EffectID = values.Get("effect_id")
	filter.WorkID = values.Get("work_id")
	return after, limit, filter, true
}

func (h *HTTPHandler) serveActivitySSE(
	w http.ResponseWriter,
	r *http.Request,
	runID string,
	after int64,
	limit int,
	filter ActivityFilter,
) {
	activity, ok := h.projector.(ActivityAPI)
	if !ok {
		writeHTTPError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
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
		page, err := activity.Activity(r.Context(), runID, after, limit, filter)
		if err != nil {
			_, _ = io.WriteString(w, "event: unavailable\ndata: {\"code\":\"REPLAY_UNAVAILABLE\"}\n\n")
			flusher.Flush()
			return
		}
		for _, turn := range page.Turns {
			body, _ := json.Marshal(struct {
				SchemaVersion string       `json:"schema_version"`
				Turn          ActivityTurn `json:"turn"`
			}{
				SchemaVersion: ActivitySchemaVersion,
				Turn:          turn,
			})
			_, _ = fmt.Fprintf(
				w,
				"id: %d\nevent: activity\ndata: %s\n\n",
				turn.Offset,
				body,
			)
			after = turn.Offset
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
