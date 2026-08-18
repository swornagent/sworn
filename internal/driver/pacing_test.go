package driver

import (
	"context"
	"testing"
	"time"
)

// The exact quota-exhaustion body gemini-3.7-flash returned on
// 2026-08-18 (captured live from loop-economics r5): RetryInfo advises the
// wait the engine previously discarded along with the body.
const geminiQuota429Body = `{
  "error": {
    "code": 429,
    "message": "You exceeded your current quota. * Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_paid_tier_2_input_token_count, limit: 3000000, model: gemini-3.7-flash\nPlease retry in 11.769242877s.",
    "status": "RESOURCE_EXHAUSTED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.Help",
        "links": [{"description": "Learn more", "url": "https://ai.google.dev/gemini-api/docs/rate-limits"}]
      },
      {
        "@type": "type.googleapis.com/google.rpc.QuotaFailure",
        "violations": [{"quotaMetric": "generativelanguage.googleapis.com/generate_content_paid_tier_2_input_token_count", "quotaValue": "3000000"}]
      },
      {
        "@type": "type.googleapis.com/google.rpc.RetryInfo",
        "retryDelay": "11.769242877s"
      }
    ]
  }
}`

func TestProviderRetryDelay(t *testing.T) {
	t.Parallel()
	t.Run("parses google.rpc.RetryInfo from the recorded body", func(t *testing.T) {
		delay := providerRetryDelay("", []byte(geminiQuota429Body))
		if delay != 11769242877*time.Nanosecond {
			t.Fatalf("delay = %v", delay)
		}
	})
	t.Run("falls back to Retry-After seconds", func(t *testing.T) {
		if delay := providerRetryDelay("30", []byte(`{}`)); delay != 30*time.Second {
			t.Fatalf("delay = %v", delay)
		}
	})
	t.Run("names zero when the provider names nothing", func(t *testing.T) {
		if delay := providerRetryDelay("", []byte("not json")); delay != 0 {
			t.Fatalf("delay = %v", delay)
		}
	})
}

func TestPacedRetryDelayClamps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		err   error
		delay time.Duration
	}{
		{"advised delay passes through",
			&ContractError{Code: "PROVIDER_LIMITED", RetryAfter: 12 * time.Second},
			12 * time.Second},
		{"sub-second advisory floors at the minimum",
			&ContractError{Code: "PROVIDER_LIMITED", RetryAfter: 200 * time.Millisecond},
			MinProviderRetryDelay},
		{"oversized advisory clamps at the cap",
			&ContractError{Code: "PROVIDER_LIMITED", RetryAfter: time.Hour},
			MaxProviderRetryDelay},
		{"absent advisory uses the default",
			&ContractError{Code: "PROVIDER_LIMITED"},
			DefaultProviderRetryDelay},
	}
	for _, testCase := range cases {
		if delay := pacedRetryDelay(testCase.err); delay != testCase.delay {
			t.Fatalf("%s: delay = %v", testCase.name, delay)
		}
	}
}

func TestPacedRoundTripHoldsTheRequest(t *testing.T) {
	t.Parallel()
	limited := &ContractError{Code: "PROVIDER_LIMITED", RetryAfter: 10 * time.Second}
	var slept []time.Duration
	fakeSleep := func(_ context.Context, delay time.Duration) error {
		slept = append(slept, delay)
		return nil
	}

	t.Run("retries the same request after the advised delay", func(t *testing.T) {
		slept = nil
		budget := MaxProviderPacedWait
		calls := 0
		response, err := pacedRoundTrip(context.Background(), func() ([]byte, error) {
			calls++
			if calls < 3 {
				return nil, limited
			}
			return []byte("ok"), nil
		}, &budget, nil, fakeSleep)
		if err != nil || string(response) != "ok" || calls != 3 {
			t.Fatalf("response=%q err=%v calls=%d", response, err, calls)
		}
		if len(slept) != 2 || slept[0] != 10*time.Second {
			t.Fatalf("slept = %v", slept)
		}
		if budget != MaxProviderPacedWait-20*time.Second {
			t.Fatalf("budget = %v", budget)
		}
	})

	t.Run("propagates the limited error when the budget runs dry", func(t *testing.T) {
		slept = nil
		budget := 15 * time.Second
		calls := 0
		_, err := pacedRoundTrip(context.Background(), func() ([]byte, error) {
			calls++
			return nil, limited
		}, &budget, nil, fakeSleep)
		if !IsCode(err, "PROVIDER_LIMITED") {
			t.Fatalf("err = %v", err)
		}
		if calls != 2 || len(slept) != 1 {
			t.Fatalf("calls=%d slept=%v", calls, slept)
		}
	})

	t.Run("returns non-limited errors immediately", func(t *testing.T) {
		slept = nil
		budget := MaxProviderPacedWait
		calls := 0
		_, err := pacedRoundTrip(context.Background(), func() ([]byte, error) {
			calls++
			return nil, fail("PROVIDER_UNAVAILABLE")
		}, &budget, nil, fakeSleep)
		if !IsCode(err, "PROVIDER_UNAVAILABLE") || calls != 1 || len(slept) != 0 {
			t.Fatalf("err=%v calls=%d slept=%v", err, calls, slept)
		}
	})
}

func TestInputTokenPacerPacesOnlyAtTheWall(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	t.Run("zero cap never waits", func(t *testing.T) {
		pacer := newInputTokenPacer(0)
		pacer.record(1_000_000, base)
		if wait := pacer.waitBefore(pacer.estimate(500_000), base); wait != 0 {
			t.Fatalf("wait = %v", wait)
		}
	})

	t.Run("under the cap adds zero latency", func(t *testing.T) {
		pacer := newInputTokenPacer(3_000_000)
		pacer.record(1_000_000, base)
		if wait := pacer.waitBefore(pacer.estimate(0), base.Add(time.Second)); wait != 0 {
			t.Fatalf("wait = %v", wait)
		}
	})

	t.Run("waits for the oldest entries to drain when the next request would cross", func(t *testing.T) {
		pacer := newInputTokenPacer(3_000_000)
		pacer.record(1_500_000, base)
		pacer.record(1_200_000, base.Add(20*time.Second))
		now := base.Add(30 * time.Second)
		wait := pacer.waitBefore(pacer.estimate(0), now)
		// The first entry (1.5M at base) must age out: base+60s-now = 30s.
		if wait != 30*time.Second {
			t.Fatalf("wait = %v", wait)
		}
	})

	t.Run("expired entries free the window", func(t *testing.T) {
		pacer := newInputTokenPacer(3_000_000)
		pacer.record(2_900_000, base)
		if wait := pacer.waitBefore(pacer.estimate(0), base.Add(61*time.Second)); wait != 0 {
			t.Fatalf("wait = %v", wait)
		}
	})

	t.Run("an estimate at or beyond the whole cap is sent unpaced", func(t *testing.T) {
		pacer := newInputTokenPacer(1_000_000)
		pacer.record(900_000, base)
		if wait := pacer.waitBefore(2_000_000, base.Add(time.Second)); wait != 0 {
			t.Fatalf("wait = %v", wait)
		}
	})

	t.Run("estimate grows from the last reported turn", func(t *testing.T) {
		pacer := newInputTokenPacer(3_000_000)
		if estimate := pacer.estimate(100); estimate != 100+pacingEstimateHeadroom {
			t.Fatalf("estimate = %d", estimate)
		}
		pacer.record(250_000, base)
		if estimate := pacer.estimate(100); estimate != 250_000+pacingEstimateHeadroom {
			t.Fatalf("estimate = %d", estimate)
		}
	})
}
