package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// NotifyEvent is the payload sent to the webhook and SwornAgent API.
// Fields match the spec's JSON payload exactly.
type NotifyEvent struct {
	Release           string `json:"release"`
	Track             string `json:"track"`
	SliceID           string `json:"slice_id"`
	State             string `json:"state"`
	ViolationsSummary string `json:"violations_summary"`
	WorktreePath      string `json:"worktree_path"`
	Timestamp         string `json:"timestamp"`
}

// Notifier wraps credentials + webhook URL and sends notifications on
// FAIL/BLOCKED verdict transitions. It no-ops when neither a webhook URL
// nor an active account is configured.
type Notifier struct {
	WebhookURL string
	creds      *Credentials
	client     *http.Client
}

// NewNotifier creates a Notifier from a webhook URL and credentials.
// If webhookURL is empty and creds is nil, Notify() is a no-op.
func NewNotifier(webhookURL string, creds *Credentials) *Notifier {
	return &Notifier{
		WebhookURL: webhookURL,
		creds:      creds,
		client:     http.DefaultClient,
	}
}

// Notify sends a webhook POST (and optionally an email via the SwornAgent API)
// for the given event. It retries the webhook up to 3 times with exponential
// backoff (1s, 2s, 4s). Failure is logged to stderr and does not block the
// caller — Notify always returns nil.
//
// When no webhook URL is configured and no account is active, Notify is a
// no-op (no error, no network call).
func (n *Notifier) Notify(ctx context.Context, event NotifyEvent) {
	if n == nil {
		return
	}

	// Set timestamp if not already set.
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// ── Webhook POST ──────────────────────────────────────────────────────
	if n.WebhookURL != "" {
		n.sendWebhook(ctx, event)
	}

	// ── SwornAgent API email ──────────────────────────────────────────────
	if n.creds != nil && IsLoggedIn(n.creds) {
		n.sendAPI(ctx, event)
	}
}

// sendWebhook POSTs the event to the configured webhook URL with 3 retries.
func (n *Notifier) sendWebhook(ctx context.Context, event NotifyEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn notify: marshal webhook payload: %v\n", err)
		return
	}

	backoff := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff[attempt-1]):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.WebhookURL, bytes.NewReader(payload))
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn notify: build webhook request: %v\n", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := n.client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn notify: webhook POST attempt %d/3: %v\n", attempt+1, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return // success
		}

		fmt.Fprintf(os.Stderr, "sworn notify: webhook returned HTTP %d (attempt %d/3)\n",
			resp.StatusCode, attempt+1)
	}

	fmt.Fprintf(os.Stderr, "sworn notify: webhook delivery failed after 3 attempts\n")
}

// sendAPI POSTs the event to the SwornAgent /api/notify endpoint for email
// delivery. If the endpoint is unreachable, it logs a warning and continues.
func (n *Notifier) sendAPI(ctx context.Context, event NotifyEvent) {
	host := defaultProxyHost
	if override := os.Getenv("SWORN_PROXY_URL"); override != "" {
		host = strings.TrimRight(override, "/")
	}

	apiURL := host + "/api/notify"

	payload, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn notify: marshal api payload: %v\n", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn notify: build api request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+n.creds.Token)

	resp, err := n.client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn notify: SwornAgent /api/notify unreachable: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return // success
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	fmt.Fprintf(os.Stderr, "sworn notify: SwornAgent /api/notify returned HTTP %d: %s\n",
		resp.StatusCode, strings.TrimSpace(string(body)))
}

// ViolationsSummary extracts a one-line violation summary from proof.md
// or falls back to a generic message. Max 200 chars.
func ViolationsSummary(proofPath string, violationCount int) string {
	if proofPath == "" {
		return fmt.Sprintf("%d violation(s) found", violationCount)
	}

	data, err := os.ReadFile(proofPath)
	if err != nil {
		if violationCount > 0 {
			return fmt.Sprintf("%d violation(s) found", violationCount)
		}
		return "verification failed"
	}

	// Look for the first numbered violation line in proof.md.
	// Lines like "1. Missing reachability artefact" or "1) ...".
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Match lines starting with a digit followed by . or )
		if len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' {
			if trimmed[1] == '.' || trimmed[1] == ')' {
				summary := strings.TrimSpace(trimmed)
				if len(summary) > 200 {
					summary = summary[:197] + "..."
				}
				return summary
			}
		}
	}

	if violationCount > 0 {
		return fmt.Sprintf("%d violation(s) found", violationCount)
	}
	return "verification failed"
}