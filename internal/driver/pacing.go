package driver

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
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
// seconds header. Zero means the provider named none.
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
// exhausted the limited error propagates exactly as before.
func pacedRoundTrip(
	ctx context.Context,
	do func() ([]byte, error),
	budget *time.Duration,
	notify func(error),
	sleep func(context.Context, time.Duration) error,
) ([]byte, error) {
	response, err := do()
	for attempt := 0; attempt < MaxProviderPacedRetries; attempt++ {
		if err == nil || !IsCode(err, "PROVIDER_LIMITED") {
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
