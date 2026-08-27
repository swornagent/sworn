package driver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The monthly spend-cap body sworn#227's postscript describes: a
// google.rpc.Status RESOURCE_EXHAUSTED envelope with no RetryInfo detail and
// no Retry-After header. This is a reconstructed shape-family fixture, not a
// live capture. The documented google.rpc.Status envelope is the same shape
// geminiQuota429Body records live (pacing_test.go), so a labelled shape
// fixture stands in for the bytes the engine discarded in the field; an
// unlabelled one would misstate its own provenance.
const googleSpendCap429Body = `{
  "error": {
    "code": 429,
    "message": "Monthly spending limit reached for this billing account. Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_paid_tier_2_input_token_count. Contact billing to raise the cap.",
    "status": "RESOURCE_EXHAUSTED"
  }
}`

const googleSpendCapDetail = "Monthly spending limit reached for this billing account. Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_paid_tier_2_input_token_count. Contact billing to raise the cap."

// A reconstructed google.rpc.Status 5xx body, labelled as a shape-family
// fixture (the documented google.rpc.Status envelope, not a live capture)
// per the same provenance standard that heads geminiQuota429Body.
const googleUnavailable503Body = `{
  "error": {
    "code": 503,
    "message": "The service is temporarily overloaded.",
    "status": "UNAVAILABLE"
  }
}`

// The OpenAI error envelope shares the {"error":{"message":...}} shape.
const openAIBadRequest400Body = `{"error":{"message":"Invalid tool definition: strict mode unsupported.","type":"invalid_request_error"}}`

// A 429 envelope whose sibling fields must never enter Detail.
const googleSiblingEcho429Body = `{
  "error": {
    "code": 429,
    "message": "Monthly spending limit reached.",
    "status": "RESOURCE_EXHAUSTED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.Help",
        "links": [{"description": "secret-token-canary", "url": "https://secret.example/token=abc"}]
      }
    ]
  }
}`

const geminiQuotaNormalizedDetail = "You exceeded your current quota. * Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_paid_tier_2_input_token_count, limit: 3000000, model: gemini-3.7-flash Please retry in 11.769242877s."

func TestProviderErrorDetailExtractsOnlyTheEnvelopeMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string
	}{
		{"recorded gemini quota body normalizes single-line", geminiQuota429Body, geminiQuotaNormalizedDetail},
		{"spend-cap shape carries the cap message", googleSpendCap429Body, googleSpendCapDetail},
		{"openai envelope carries the message", openAIBadRequest400Body, "Invalid tool definition: strict mode unsupported."},
		{"missing message yields empty", `{"error":{"code":429}}`, ""},
		{"non-string message yields empty", `{"error":{"message":42}}`, ""},
		{"unparseable body yields empty", `<html>upstream error</html>`, ""},
		{"empty message yields empty", `{"error":{"message":""}}`, ""},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail := providerErrorDetail([]byte(test.body))
			if detail != test.want {
				t.Fatalf("detail = %q, want %q", detail, test.want)
			}
			if detail != "" &&
				validateText(detail, maxProviderErrorDetailBytes, false) != nil {
				t.Fatalf("extracted detail fails boundary re-validation: %q", detail)
			}
		})
	}
}

func TestProviderErrorDetailNormalizesToValidateTextShape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string
	}{
		{"control characters become single spaces",
			`{"error":{"message":"bad\u001b\u007f\u009ftext\nline"}}`,
			"bad text line"},
		{"whitespace runs collapse and trim",
			`{"error":{"message":"  line1 \n\n line2   line3\tend  "}}`,
			"line1 line2 line3 end"},
		{"newline joins the recorded quota sentence",
			geminiQuota429Body,
			geminiQuotaNormalizedDetail},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail := providerErrorDetail([]byte(test.body))
			if detail != test.want {
				t.Fatalf("detail = %q, want %q", detail, test.want)
			}
			if validateText(detail, maxProviderErrorDetailBytes, false) != nil {
				t.Fatalf("normalized detail fails boundary re-validation: %q", detail)
			}
		})
	}

	t.Run("invalid UTF-8 resolves deterministically to U+FFFD", func(t *testing.T) {
		body := []byte(`{"error":{"message":"bad` + "\xff" + `text"}}`)
		detail := providerErrorDetail(body)
		want := "bad" + string(utf8.RuneError) + "text"
		if detail != want {
			t.Fatalf("detail = %q, want %q", detail, want)
		}
		if validateText(detail, maxProviderErrorDetailBytes, false) != nil {
			t.Fatalf("detail fails boundary re-validation: %q", detail)
		}
	})

	t.Run("over-long message truncates at a rune boundary", func(t *testing.T) {
		long := strings.Repeat("a", 510) + "é" + strings.Repeat("b", 100)
		body := `{"error":{"message":"` + long + `"}}`
		detail := providerErrorDetail([]byte(body))
		want := strings.Repeat("a", 510) + "é"
		if detail != want {
			t.Fatalf("detail length = %d, want %q", len(detail), want)
		}
		if len(detail) != maxProviderErrorDetailBytes {
			t.Fatalf("detail length = %d, want %d", len(detail), maxProviderErrorDetailBytes)
		}
		if !utf8.ValidString(detail) {
			t.Fatalf("truncated detail is not valid UTF-8: %q", detail)
		}
		if validateText(detail, maxProviderErrorDetailBytes, false) != nil {
			t.Fatalf("truncated detail fails boundary re-validation: %q", detail)
		}
	})
}

func TestProviderErrorDetailSiblingFieldsNeverEnterDetail(t *testing.T) {
	t.Parallel()
	detail := providerErrorDetail([]byte(googleSiblingEcho429Body))
	if detail != "Monthly spending limit reached." {
		t.Fatalf("detail = %q", detail)
	}
	for _, canary := range []string{"secret-token-canary", "secret.example", "token=abc"} {
		if strings.Contains(detail, canary) {
			t.Fatalf("sibling field leaked into detail: %q in %q", canary, detail)
		}
	}
}

func TestProviderNamesRetryWindow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		header string
		body   string
		want   bool
	}{
		{"RetryInfo in body names a window", "", geminiQuota429Body, true},
		{"Retry-After seconds names a window", "30", `{}`, true},
		{"fractional Retry-After still names a window", "1.5", `{}`, true},
		{"HTTP-date Retry-After still names a window", "Wed, 21 Oct 2026 07:28:00 GMT", `{}`, true},
		{"zero Retry-After still names a window", "0", `{}`, true},
		{"blank Retry-After names no window", "  ", `{}`, false},
		{"empty retryDelay names no window", "",
			`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":""}]}}`, false},
		{"unparseable retryDelay still names a window", "",
			`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"soon"}]}}`, true},
		{"zero retryDelay still names a window", "",
			`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"0s"}]}}`, true},
		{"no window anywhere", "", `{"error":{"code":429,"message":"cap"}}`, false},
		{"unparseable body with no header names no window", "", `not json`, false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := providerNamesRetryWindow(test.header, []byte(test.body)); got != test.want {
				t.Fatalf("window named = %v, want %v", got, test.want)
			}
		})
	}
}

func TestHardLimitExhaustedPhraseTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		message string
		want    bool
	}{
		{"Monthly spending limit reached", true},
		{"SPEND CAP EXCEEDED", true},
		{"billing account disabled", true},
		{"spend limit of 100 USD hit", true},
		{"spending cap", true},
		{"quota exceeded for metric", false},
		{"", false},
		{"retry later", false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.message, func(t *testing.T) {
			t.Parallel()
			if got := hardLimitExhausted(test.message); got != test.want {
				t.Fatalf("matched = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProviderLimitHardClassifies(t *testing.T) {
	t.Parallel()
	retryInfoCapBody := `{"error":{"code":429,"message":"Monthly spending limit reached.","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"0s"}]}}`
	cases := []struct {
		name   string
		header string
		body   string
		want   bool
	}{
		{"windowed phrase-free quota body stays paced", "", geminiQuota429Body, false},
		{"windowless spend-cap shape is hard", "", googleSpendCap429Body, true},
		{"windowless bodyless 429 is hard", "", `{}`, true},
		{"windowless unparseable body is hard", "", `not json`, true},
		{"a cap phrase overrides an HTTP-date Retry-After window", "Wed, 21 Oct 2026 07:28:00 GMT", googleSpendCap429Body, true},
		{"a cap phrase overrides a fractional Retry-After window", "1.5", googleSpendCap429Body, true},
		{"a cap phrase overrides a zero Retry-After window", "0", googleSpendCap429Body, true},
		{"a cap phrase overrides a RetryInfo window", "", retryInfoCapBody, true},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := providerLimitHard(test.header, []byte(test.body)); got != test.want {
				t.Fatalf("hard = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProviderHTTPStatusErrorCarriesBoundedDetail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		code   string
	}{
		{"authorization", 401, "PROVIDER_AUTHORIZATION_FAILED"},
		{"limited", 429, "PROVIDER_LIMITED"},
		{"request rejected", 400, "PROVIDER_REQUEST_REJECTED"},
		{"unavailable", 503, "PROVIDER_UNAVAILABLE"},
		{"other", 302, "PROVIDER_ERROR"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := providerHTTPStatusError(test.status, "provider words")
			var contractErr *ContractError
			if !errors.As(err, &contractErr) ||
				contractErr.Code != test.code ||
				contractErr.Detail != "provider words" ||
				contractErr.HardLimit {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestPacedRoundTripHardLimitFailsImmediately(t *testing.T) {
	t.Parallel()
	t.Run("hard no-window quota shape: one call, zero sleeps, zero budget drain, no notify", func(t *testing.T) {
		hard := &ContractError{Code: "PROVIDER_LIMITED", Detail: "Monthly spending limit reached.", HardLimit: true}
		budget := MaxProviderPacedWait
		calls := 0
		var slept []time.Duration
		var notified []error
		_, err := pacedRoundTrip(
			context.Background(),
			func() ([]byte, error) {
				calls++
				return nil, hard
			},
			&budget,
			func(err error) { notified = append(notified, err) },
			func(_ context.Context, delay time.Duration) error {
				slept = append(slept, delay)
				return nil
			},
		)
		if calls != 1 || len(slept) != 0 || len(notified) != 0 {
			t.Fatalf("calls=%d slept=%v notified=%d", calls, slept, len(notified))
		}
		if budget != MaxProviderPacedWait {
			t.Fatalf("budget = %v, want %v", budget, MaxProviderPacedWait)
		}
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Code != "PROVIDER_LIMITED" ||
			contractErr.Detail != "Monthly spending limit reached." ||
			!contractErr.HardLimit {
			t.Fatalf("err = %#v", err)
		}
	})

	t.Run("hard cap-shaped error behaves identically", func(t *testing.T) {
		hard := &ContractError{Code: "PROVIDER_LIMITED", Detail: "Spending cap reached.", HardLimit: true}
		budget := MaxProviderPacedWait
		calls := 0
		var slept []time.Duration
		var notified []error
		_, err := pacedRoundTrip(
			context.Background(),
			func() ([]byte, error) {
				calls++
				return nil, hard
			},
			&budget,
			func(err error) { notified = append(notified, err) },
			func(_ context.Context, delay time.Duration) error {
				slept = append(slept, delay)
				return nil
			},
		)
		if calls != 1 || len(slept) != 0 || len(notified) != 0 {
			t.Fatalf("calls=%d slept=%v notified=%d", calls, slept, len(notified))
		}
		if budget != MaxProviderPacedWait {
			t.Fatalf("budget = %v, want %v", budget, MaxProviderPacedWait)
		}
		if !IsCode(err, "PROVIDER_LIMITED") || !hardLimited(err) {
			t.Fatalf("err = %#v", err)
		}
	})

	t.Run("windowed error with detail keeps the paced loop and carries its words", func(t *testing.T) {
		soft := &ContractError{Code: "PROVIDER_LIMITED", Detail: "quota words", RetryAfter: 10 * time.Second}
		budget := 15 * time.Second
		calls := 0
		var slept []time.Duration
		var notified []error
		_, err := pacedRoundTrip(
			context.Background(),
			func() ([]byte, error) {
				calls++
				return nil, soft
			},
			&budget,
			func(err error) { notified = append(notified, err) },
			func(_ context.Context, delay time.Duration) error {
				slept = append(slept, delay)
				return nil
			},
		)
		if calls != 2 || len(slept) != 1 || slept[0] != 10*time.Second {
			t.Fatalf("calls=%d slept=%v", calls, slept)
		}
		if len(notified) != 1 {
			t.Fatalf("notified = %d", len(notified))
		}
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Code != "PROVIDER_LIMITED" ||
			contractErr.Detail != "quota words" ||
			contractErr.HardLimit {
			t.Fatalf("err = %#v", err)
		}
	})
}

func TestNormalizeAdapterErrorPreservesBoundedProviderDetail(t *testing.T) {
	t.Parallel()
	providerCodes := []string{
		"PROVIDER_LIMITED",
		"PROVIDER_AUTHORIZATION_FAILED",
		"PROVIDER_REQUEST_REJECTED",
		"PROVIDER_UNAVAILABLE",
		"PROVIDER_ERROR",
	}
	for _, code := range providerCodes {
		code := code
		t.Run("preserves detail and hard flag for "+code, func(t *testing.T) {
			t.Parallel()
			in := &ContractError{Code: code, Detail: "provider words", HardLimit: true}
			out := normalizeAdapterError(in)
			got, ok := out.(*ContractError)
			if !ok || got.Code != code || got.Detail != "provider words" || !got.HardLimit {
				t.Fatalf("normalized = %#v", out)
			}
		})
	}

	t.Run("wrapped provider error keeps words and drops the wrapper", func(t *testing.T) {
		t.Parallel()
		in := fmt.Errorf("wrapper-secret-canary: %w", &ContractError{
			Code: "PROVIDER_LIMITED", Detail: "provider words", HardLimit: true,
		})
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROVIDER_LIMITED" || got.Detail != "provider words" || !got.HardLimit {
			t.Fatalf("normalized = %#v", out)
		}
		if strings.Contains(got.Error(), "wrapper-secret-canary") {
			t.Fatalf("wrapper leaked: %v", got.Error())
		}
	})

	t.Run("non-provider codes drop detail and hard flag", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{Code: "PROCESS_FAILED", Detail: "secret-canary", HardLimit: true}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROCESS_FAILED" || got.Detail != "" || got.HardLimit {
			t.Fatalf("normalized = %#v", out)
		}
	})

	t.Run("non-conforming text drops detail and hard flag", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{Code: "PROVIDER_LIMITED", Detail: "bad\x1btext", HardLimit: true}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROVIDER_LIMITED" || got.Detail != "" || got.HardLimit {
			t.Fatalf("normalized = %#v", out)
		}
	})

	t.Run("over-long text drops detail and hard flag", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{
			Code:      "PROVIDER_LIMITED",
			Detail:    strings.Repeat("a", maxProviderErrorDetailBytes+1),
			HardLimit: true,
		}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROVIDER_LIMITED" || got.Detail != "" || got.HardLimit {
			t.Fatalf("normalized = %#v", out)
		}
	})

	t.Run("empty detail drops the hard flag", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{Code: "PROVIDER_LIMITED", HardLimit: true}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROVIDER_LIMITED" || got.Detail != "" || got.HardLimit {
			t.Fatalf("normalized = %#v", out)
		}
	})
}

func TestStreamRendererDriverErrorCarriesProviderDetail(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	renderer := &streamRenderer{out: &buffer}
	renderer.driverError("transport", &ContractError{
		Code:   "PROVIDER_LIMITED",
		Detail: "Monthly spending limit reached.",
	})
	want := "── driver transport error: driver contract: PROVIDER_LIMITED: Monthly spending limit reached. ──\n"
	if got := buffer.String(); got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newProviderLimitTransport(t *testing.T, roundTripper http.RoundTripper) *httpTransport {
	t.Helper()
	config := HTTPProfileConfig{
		Key:              "provider-limit-test",
		ID:               "sworn.provider-limit-test",
		Version:          "1.0.0",
		Endpoint:         "https://provider.test/v1/chat",
		CredentialHeader: "Authorization",
		CredentialPrefix: "Bearer ",
		CredentialRefs:   []string{"cred"},
		ResponseBytes:    MaxProviderResponseBytes,
	}
	resolver := func(context.Context, string) ([]byte, error) {
		return []byte("secret"), nil
	}
	transport, err := newHTTPTransport(config, AuthModeBearer, resolver, nil, roundTripper)
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func providerLimitRequest() providerRequest {
	return providerRequest{
		Method:      http.MethodPost,
		URL:         "https://provider.test/v1/chat",
		ContentType: "application/json",
		Body:        []byte(`{"prompt":["hello"]}`),
	}
}

func statusResponse(status int, header http.Header, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestHTTPTransportCarriesProviderLimitEvidence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		status         int
		header         http.Header
		body           string
		wantCode       string
		wantDetail     string
		wantRetryAfter time.Duration
		wantHard       bool
	}{
		{"recorded gemini quota body stays paced with detail",
			429, nil, geminiQuota429Body,
			"PROVIDER_LIMITED", geminiQuotaNormalizedDetail, 11769242877 * time.Nanosecond, false},
		{"spend-cap shape hard-fails with detail",
			429, nil, googleSpendCap429Body,
			"PROVIDER_LIMITED", googleSpendCapDetail, 0, true},
		{"HTTP-date Retry-After yields to a cap phrase",
			429, http.Header{"Retry-After": []string{"Wed, 21 Oct 2026 07:28:00 GMT"}}, googleSpendCap429Body,
			"PROVIDER_LIMITED", googleSpendCapDetail, 0, true},
		{"Retry-After seconds parses the advisory",
			429, http.Header{"Retry-After": []string{"30"}}, `{"error":{"code":429,"message":"quota exhausted"}}`,
			"PROVIDER_LIMITED", "quota exhausted", 30 * time.Second, false},
		{"503 google.rpc.Status carries its message",
			503, nil, googleUnavailable503Body,
			"PROVIDER_UNAVAILABLE", "The service is temporarily overloaded.", 0, false},
		{"400 openai envelope carries its message",
			400, nil, openAIBadRequest400Body,
			"PROVIDER_REQUEST_REJECTED", "Invalid tool definition: strict mode unsupported.", 0, false},
		{"non-JSON error body yields empty detail",
			500, nil, `<html>upstream error</html>`,
			"PROVIDER_UNAVAILABLE", "", 0, false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transport := newProviderLimitTransport(t, roundTripperFunc(func(
				request *http.Request,
			) (*http.Response, error) {
				return statusResponse(test.status, test.header, test.body), nil
			}))
			ref := "cred"
			_, err := transport.roundTrip(context.Background(), &ref, providerLimitRequest())
			var contractErr *ContractError
			if !errors.As(err, &contractErr) ||
				contractErr.Code != test.wantCode ||
				contractErr.Detail != test.wantDetail ||
				contractErr.RetryAfter != test.wantRetryAfter ||
				contractErr.HardLimit != test.wantHard {
				t.Fatalf("err = %#v", err)
			}
		})
	}

	t.Run("HTTP-date Retry-After paces at the default delay", func(t *testing.T) {
		t.Parallel()
		// A phrase-free, RetryInfo-free body: the header still names a
		// window (so this stays soft, not hard), and it parses to no usable
		// seconds delay (so pacedRetryDelay falls back to the default).
		transport := newProviderLimitTransport(t, roundTripperFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			return statusResponse(429, http.Header{
				"Retry-After": []string{"Wed, 21 Oct 2026 07:28:00 GMT"},
			}, `{"error":{"code":429,"message":"quota exceeded"}}`), nil
		}))
		ref := "cred"
		_, err := transport.roundTrip(context.Background(), &ref, providerLimitRequest())
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.RetryAfter != 0 ||
			contractErr.HardLimit {
			t.Fatalf("err = %#v", err)
		}
		if delay := pacedRetryDelay(err); delay != DefaultProviderRetryDelay {
			t.Fatalf("paced delay = %v, want %v", delay, DefaultProviderRetryDelay)
		}
	})

	t.Run("over-long message truncates at a rune boundary", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("a", 510) + "é" + strings.Repeat("b", 100)
		transport := newProviderLimitTransport(t, roundTripperFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			return statusResponse(429, nil, `{"error":{"message":"`+long+`"}}`), nil
		}))
		ref := "cred"
		_, err := transport.roundTrip(context.Background(), &ref, providerLimitRequest())
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Detail != strings.Repeat("a", 510)+"é" ||
			!contractErr.HardLimit {
			t.Fatalf("err = %#v", err)
		}
	})

	t.Run("sibling envelope fields never enter detail", func(t *testing.T) {
		t.Parallel()
		transport := newProviderLimitTransport(t, roundTripperFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			return statusResponse(429, nil, googleSiblingEcho429Body), nil
		}))
		ref := "cred"
		_, err := transport.roundTrip(context.Background(), &ref, providerLimitRequest())
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Detail != "Monthly spending limit reached." {
			t.Fatalf("err = %#v", err)
		}
		for _, canary := range []string{"secret-token-canary", "secret.example", "token=abc"} {
			if strings.Contains(contractErr.Detail, canary) {
				t.Fatalf("sibling field leaked into detail: %q in %q", canary, contractErr.Detail)
			}
		}
	})
}
