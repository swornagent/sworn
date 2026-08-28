package driver

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Provider pacing bounds. Pacing is strictly reactive-or-at-the-wall: a
// conversation is never delayed unless the configured input-token budget
// would actually be exceeded or the provider has already answered 429.
const (
	// MaxProviderRetryDelay clamps any provider-advised or default wait.
	MaxProviderRetryDelay = 2 * time.Minute
	// MinProviderRetryDelay floors a parsed sub-second advisory so a burst
	// of instant retries can never re-exhaust the window it waits for.
	MinProviderRetryDelay = time.Second
	// DefaultProviderRetryDelay is used when a 429 names no delay.
	DefaultProviderRetryDelay = 15 * time.Second
	// MaxProviderPacedRetries bounds in-place retries of one request.
	MaxProviderPacedRetries = 20
	// MaxProviderPacedWait bounds the cumulative paced wait of one
	// conversation; past it a limited provider fails as it does today.
	MaxProviderPacedWait = 10 * time.Minute
	// pacingWindow is the provider quota window the ledger models.
	pacingWindow = time.Minute
	// pacingEstimateHeadroom pads the next-request token estimate so the
	// ledger errs toward waiting slightly early rather than tripping 429.
	pacingEstimateHeadroom = 4096
)

// providerRetryDelay extracts the provider-advised retry delay from a 429
// response: google.rpc.RetryInfo in the body first, then a Retry-After
// seconds header. Zero means no usable seconds delay could be read; whether
// the provider named a window at all (whatever its value parses to) is
// providerNamesRetryWindow's question, and classification keys on that
// signal, never on this parse.
func providerRetryDelay(retryAfterHeader string, body []byte) time.Duration {
	var envelope struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		for _, detail := range envelope.Error.Details {
			if !strings.HasSuffix(detail.Type, "google.rpc.RetryInfo") ||
				detail.RetryDelay == "" {
				continue
			}
			if delay, err := time.ParseDuration(detail.RetryDelay); err == nil && delay > 0 {
				return delay
			}
		}
	}
	if seconds, err := strconv.ParseInt(
		strings.TrimSpace(retryAfterHeader), 10, 64,
	); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}

// maxProviderErrorDetailBytes bounds the provider status-envelope message
// that may ride a ContractError as Detail (S5-provider-limit-evidence). The
// dispatcher boundary re-validates against the same bound.
const maxProviderErrorDetailBytes = 512

// providerErrorDetail extracts the provider's own words from a non-2xx
// status envelope: the {"error":{"message":...}} shape the google.rpc.Status
// and OpenAI error envelopes share. Only error.message is ever read, so
// request bytes, headers, credentials, and every sibling envelope field
// structurally cannot enter the result. The message is normalized to
// single-line, control-free, whitespace-collapsed valid UTF-8 and bounded
// at maxProviderErrorDetailBytes on a UTF-8 rune boundary, so the
// dispatcher boundary's validateText re-validation is a no-op for anything
// extracted here. A missing, non-string, or empty message and an
// unparseable body all yield "".
func providerErrorDetail(body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil ||
		envelope.Error.Message == "" {
		return ""
	}
	return normalizeProviderErrorDetail(envelope.Error.Message)
}

// normalizeProviderErrorDetail renders one provider message into the only
// shape validateText accepts at the dispatcher boundary: every rune the
// secret-guard posture rejects (C0 0x00-0x1f and C1 0x7f-0x9f) becomes a
// space, whitespace runs collapse to one space, the result is trimmed, and
// invalid UTF-8 is resolved deterministically by Go's rune decoding (an
// invalid byte decodes as U+FFFD). The final bytes are cut at the last
// UTF-8 rune boundary within maxProviderErrorDetailBytes. A message that
// normalizes to nothing yields "".
func normalizeProviderErrorDetail(message string) string {
	var builder strings.Builder
	builder.Grow(len(message))
	spaceRun := false
	for _, r := range message {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) || r == ' ' {
			if spaceRun {
				continue
			}
			spaceRun = true
			builder.WriteByte(' ')
			continue
		}
		spaceRun = false
		builder.WriteRune(r)
	}
	normalized := strings.TrimSpace(builder.String())
	if normalized == "" {
		return ""
	}
	if len(normalized) <= maxProviderErrorDetailBytes {
		return normalized
	}
	bounded := normalized[:maxProviderErrorDetailBytes]
	for len(bounded) > 0 && !utf8.ValidString(bounded) {
		bounded = bounded[:len(bounded)-1]
	}
	return bounded
}

// providerNamesRetryWindow reports whether a 429 names a retry window by
// signal, not by parse value: a google.rpc.RetryInfo detail carrying a
// non-empty retryDelay, or a non-empty Retry-After header. The paced path
// wins whenever a window signal is present, whatever its value parses to —
// an HTTP-date Retry-After, a fractional or zero delay, or a RetryInfo
// whose retryDelay fails ParseDuration all stay paced — so classification
// never guesses from a parser result.
func providerNamesRetryWindow(retryAfterHeader string, body []byte) bool {
	if strings.TrimSpace(retryAfterHeader) != "" {
		return true
	}
	var envelope struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	for _, detail := range envelope.Error.Details {
		if strings.HasSuffix(detail.Type, "google.rpc.RetryInfo") &&
			detail.RetryDelay != "" {
			return true
		}
	}
	return false
}

// hardLimitPhrases is the closed hard-cap phrase table: the monthly or
// daily spend-cap exhaustion shapes the S5 ruling names. Matching is
// case-insensitive and over the normalized provider message, so an
// unrecognized shape never fires.
var hardLimitPhrases = []string{
	"spending limit",
	"spend limit",
	"spending cap",
	"spend cap",
	"billing account",
}

// hardLimitExhausted matches the closed hard-cap phrase table over a
// normalized provider message. Per the S4-refusal-taxonomy A2 ruling
// (superseding the prior Captain C2 inert-table reading), a matched phrase
// classifies hard even under a provider-named retry window: the exhaustion
// vocabulary is live, not classification-inert.
func hardLimitExhausted(message string) bool {
	lower := strings.ToLower(message)
	for _, phrase := range hardLimitPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// providerLimitHard classifies a 429 as a hard wall that must fail the
// dispatch immediately instead of burning the paced-retry budget. A
// windowless 429 is always hard, whatever its body says. A windowed 429
// stays paced unless its body matches the closed hard-cap phrase table
// (hardLimitExhausted), in which case the matched phrase overrides the
// named window: an account that is provably out of money must never be
// paced into a dead wall just because the provider also named a retry
// delay.
func providerLimitHard(retryAfterHeader string, body []byte) bool {
	if !providerNamesRetryWindow(retryAfterHeader, body) {
		return true
	}
	return hardLimitExhausted(providerErrorDetail(body))
}

// detailPreservingCode reports whether code belongs to the family whose
// bounded Detail may ride past the dispatcher boundary
// (S5-provider-limit-evidence, widened by S4-refusal-taxonomy A3/A4): the
// five provider status codes carry the status envelope's plain
// error.message; PROVIDER_TRANSPORT_FAILED carries a bounded native-stderr
// tail in the same plain shape; NATIVE_SURFACE_INVALID carries a distinct
// structured {"check":...,"head":...} envelope, re-validated by its own
// structural check rather than validateText. Every other code keeps
// dropping adapter text exactly as before.
func detailPreservingCode(code string) bool {
	switch code {
	case "PROVIDER_LIMITED",
		"PROVIDER_AUTHORIZATION_FAILED",
		"PROVIDER_REQUEST_REJECTED",
		"PROVIDER_UNAVAILABLE",
		"PROVIDER_ERROR",
		"PROVIDER_TRANSPORT_FAILED",
		"NATIVE_SURFACE_INVALID":
		return true
	default:
		return false
	}
}

// plainDetailCode reports whether code's Detail is the plain, single-line,
// control-free provider/native-stderr text validateText already governs
// (as opposed to NATIVE_SURFACE_INVALID's structured envelope).
func plainDetailCode(code string) bool {
	return detailPreservingCode(code) && code != "NATIVE_SURFACE_INVALID"
}

// hardLimited reports whether an error classifies as hard provider
// exhaustion: the single source of truth is classifyKind, so pacing and the
// funnel's Kind field can never diverge. Such an error fails the dispatch
// after the one transport attempt that produced it — zero sleeps, zero
// budget drain, and no notify — instead of default-pacing into a wall.
func hardLimited(err error) bool {
	var contractErr *ContractError
	return errors.As(err, &contractErr) &&
		classifyKind(contractErr.Code, contractErr.HardLimit) == KindHardExhaustion
}

// pacedRetryDelay renders the wait for one 429: the provider's advisory
// when present, the default otherwise, clamped to [floor, cap].
func pacedRetryDelay(err error) time.Duration {
	delay := DefaultProviderRetryDelay
	var contractErr *ContractError
	if errors.As(err, &contractErr) && contractErr.RetryAfter > 0 {
		delay = contractErr.RetryAfter
	}
	if delay < MinProviderRetryDelay {
		delay = MinProviderRetryDelay
	}
	if delay > MaxProviderRetryDelay {
		delay = MaxProviderRetryDelay
	}
	return delay
}

// pacedRoundTrip issues one provider request, retrying it in place while the
// provider answers PROVIDER_LIMITED. The conversation is held: the same
// request bytes are re-sent after the provider-advised delay, so a paced
// turn never restarts the dispatch or re-sends the whole context. budget is
// shared across the conversation; when it or the per-request retry bound is
// exhausted the limited error propagates exactly as before. A hard wall
// (PROVIDER_LIMITED with HardLimit) returns after the single attempt that
// produced it: no sleep, no budget drain, and no notify.
func pacedRoundTrip(
	ctx context.Context,
	do func() ([]byte, error),
	budget *time.Duration,
	notify func(error),
	sleep func(context.Context, time.Duration) error,
) ([]byte, error) {
	response, err := do()
	for attempt := 0; attempt < MaxProviderPacedRetries; attempt++ {
		if err == nil || !IsCode(err, "PROVIDER_LIMITED") || hardLimited(err) {
			return response, err
		}
		delay := pacedRetryDelay(err)
		if *budget < delay {
			return response, err
		}
		*budget -= delay
		if notify != nil {
			notify(err)
		}
		if sleepErr := sleep(ctx, delay); sleepErr != nil {
			return nil, sleepErr
		}
		response, err = do()
	}
	return response, err
}

func contextSleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// inputTokenPacer models the provider's per-model input-tokens-per-minute
// quota from reported usage. It delays a request only when the sliding
// window plus the next request's estimate would cross the configured cap —
// otherwise it adds zero latency. A zero cap disables it entirely.
type inputTokenPacer struct {
	cap     int64
	entries []pacerEntry
	last    int64
}

type pacerEntry struct {
	at     time.Time
	tokens int64
}

func newInputTokenPacer(cap int64) *inputTokenPacer {
	return &inputTokenPacer{cap: cap}
}

// record notes one accepted turn's reported input tokens. Cached tokens are
// included by the caller: the provider counts them against the quota even
// when they discount cost.
func (pacer *inputTokenPacer) record(tokens int64, now time.Time) {
	if pacer == nil || pacer.cap <= 0 || tokens <= 0 {
		return
	}
	pacer.last = tokens
	pacer.entries = append(pacer.entries, pacerEntry{at: now, tokens: tokens})
	pacer.prune(now)
}

// estimate is the pacer's guess at the next request's input tokens: the
// last reported turn plus headroom, or the caller's fallback before any
// turn has reported.
func (pacer *inputTokenPacer) estimate(fallback int64) int64 {
	if pacer == nil || pacer.cap <= 0 {
		return 0
	}
	if pacer.last > 0 {
		return pacer.last + pacingEstimateHeadroom
	}
	return fallback + pacingEstimateHeadroom
}

// waitBefore returns how long to hold the next request so the window stays
// under the cap: zero when it already fits, the drain time of the oldest
// entries otherwise. An estimate larger than the whole cap can never fit;
// the request is sent unpaced and the reactive path owns the outcome.
func (pacer *inputTokenPacer) waitBefore(estimate int64, now time.Time) time.Duration {
	if pacer == nil || pacer.cap <= 0 || estimate >= pacer.cap {
		return 0
	}
	pacer.prune(now)
	sum := int64(0)
	for _, entry := range pacer.entries {
		sum += entry.tokens
	}
	if sum+estimate <= pacer.cap {
		return 0
	}
	for _, entry := range pacer.entries {
		sum -= entry.tokens
		if sum+estimate <= pacer.cap {
			wait := entry.at.Add(pacingWindow).Sub(now)
			if wait < 0 {
				return 0
			}
			if wait > pacingWindow {
				wait = pacingWindow
			}
			return wait
		}
	}
	return 0
}

func (pacer *inputTokenPacer) prune(now time.Time) {
	cutoff := now.Add(-pacingWindow)
	kept := pacer.entries[:0]
	for _, entry := range pacer.entries {
		if entry.at.After(cutoff) {
			kept = append(kept, entry)
		}
	}
	pacer.entries = kept
}
