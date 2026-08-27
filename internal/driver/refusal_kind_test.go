package driver

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyKindCoversEveryNamedBucket(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		code      string
		hardLimit bool
		want      RefusalKind
	}{
		{"provider authorization", "PROVIDER_AUTHORIZATION_FAILED", false, KindAuthorization},
		{"credential stale", "CREDENTIAL_STALE", false, KindAuthorization},
		{"credential unavailable", "CREDENTIAL_UNAVAILABLE", false, KindAuthorization},
		{"credential not certified", "CREDENTIAL_NOT_CERTIFIED", false, KindAuthorization},
		{"credential identity changed", "CREDENTIAL_IDENTITY_CHANGED", false, KindAuthorization},
		{"hard exhaustion", "PROVIDER_LIMITED", true, KindHardExhaustion},
		{"soft rate limit", "PROVIDER_LIMITED", false, KindSoftRateLimit},
		{"provider transport failed", "PROVIDER_TRANSPORT_FAILED", false, KindTransport},
		{"provider unavailable", "PROVIDER_UNAVAILABLE", false, KindTransport},
		{"endpoint unavailable", "ENDPOINT_UNAVAILABLE", false, KindTransport},
		{"process start failed", "PROCESS_START_FAILED", false, KindTransport},
		{"isolation unavailable", "ISOLATION_UNAVAILABLE", false, KindTransport},
		{"transport failure", "TRANSPORT_FAILURE", false, KindTransport},
		{"native surface invalid", "NATIVE_SURFACE_INVALID", false, KindSurfaceIntegrity},
		{"economy turn budget", "ECONOMY_TURN_BUDGET_EXCEEDED", false, KindEconomy},
		{"economy output budget", "ECONOMY_OUTPUT_BUDGET_EXCEEDED", false, KindEconomy},
		{"unclassified stays empty", "PROCESS_FAILED", false, ""},
		{"unadmitted code stays empty", "ADAPTER_FAILURE", false, ""},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyKind(test.code, test.hardLimit); got != test.want {
				t.Fatalf("classifyKind(%q, %v) = %q, want %q", test.code, test.hardLimit, got, test.want)
			}
		})
	}
}

func TestNormalizeAdapterErrorSetsKindOnEveryBranch(t *testing.T) {
	t.Parallel()
	t.Run("detail-preserving branch carries kind", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{Code: "PROVIDER_LIMITED", Detail: "provider words", HardLimit: true}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Kind != KindHardExhaustion {
			t.Fatalf("normalized = %#v", out)
		}
	})
	t.Run("plain re-create branch carries kind", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{Code: "PROVIDER_AUTHORIZATION_FAILED"}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Kind != KindAuthorization {
			t.Fatalf("normalized = %#v", out)
		}
	})
	t.Run("ADAPTER_FAILURE fallback carries kind", func(t *testing.T) {
		t.Parallel()
		out := normalizeAdapterError(errors.New("unrelated failure"))
		got, ok := out.(*ContractError)
		if !ok || got.Code != "ADAPTER_FAILURE" || got.Kind != classifyKind("ADAPTER_FAILURE", false) {
			t.Fatalf("normalized = %#v", out)
		}
	})
}

func TestHardLimitedReadsClassifyKind(t *testing.T) {
	t.Parallel()
	hard := &ContractError{Code: "PROVIDER_LIMITED", HardLimit: true}
	soft := &ContractError{Code: "PROVIDER_LIMITED", HardLimit: false}
	if !hardLimited(hard) {
		t.Fatalf("hard classified soft")
	}
	if hardLimited(soft) {
		t.Fatalf("soft classified hard")
	}
	if classifyKind(hard.Code, hard.HardLimit) != KindHardExhaustion {
		t.Fatalf("hardLimited and classifyKind diverge for the hard case")
	}
	if classifyKind(soft.Code, soft.HardLimit) != KindSoftRateLimit {
		t.Fatalf("hardLimited and classifyKind diverge for the soft case")
	}
}

func TestRevalidateNativeSurfaceDetailStructural(t *testing.T) {
	t.Parallel()
	t.Run("valid envelope round-trips", func(t *testing.T) {
		t.Parallel()
		detail := nativeSurfaceDetailBytes("stream.secret_leak_detected", nil)
		got, ok := revalidateNativeSurfaceDetail(detail)
		if !ok || got != detail {
			t.Fatalf("revalidate = %q, %v", got, ok)
		}
	})
	t.Run("valid envelope with head round-trips", func(t *testing.T) {
		t.Parallel()
		detail := nativeSurfaceDetailBytes("event.malformed_json", []byte("offending head"))
		got, ok := revalidateNativeSurfaceDetail(detail)
		if !ok || got != detail {
			t.Fatalf("revalidate = %q, %v", got, ok)
		}
	})
	t.Run("unknown check drops the detail", func(t *testing.T) {
		t.Parallel()
		detail := nativeSurfaceDetailBytes("not.a.real.check", nil)
		if _, ok := revalidateNativeSurfaceDetail(detail); ok {
			t.Fatalf("unknown check accepted")
		}
	})
	t.Run("malformed json drops the detail", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateNativeSurfaceDetail("not json"); ok {
			t.Fatalf("malformed json accepted")
		}
	})
	t.Run("unknown field drops the detail", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateNativeSurfaceDetail(
			`{"check":"stream.secret_leak_detected","extra":"x"}`,
		); ok {
			t.Fatalf("unknown field accepted")
		}
	})
	t.Run("oversize head drops the detail", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateNativeSurfaceDetail(
			`{"check":"stream.secret_leak_detected","head":"` +
				string(make([]byte, maxNativeSurfaceHeadBytes*2)) + `"}`,
		); ok {
			t.Fatalf("oversize head accepted")
		}
	})
	t.Run("empty detail drops", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateNativeSurfaceDetail(""); ok {
			t.Fatalf("empty detail accepted")
		}
	})
}

func TestNormalizeAdapterErrorPreservesNativeSurfaceEnvelope(t *testing.T) {
	t.Parallel()
	t.Run("valid envelope survives the funnel with kind", func(t *testing.T) {
		t.Parallel()
		in := failNativeSurfaceHead("event.malformed_json", []byte("bad-line"))
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "NATIVE_SURFACE_INVALID" || got.Kind != KindSurfaceIntegrity {
			t.Fatalf("normalized = %#v", out)
		}
		wantDetail := nativeSurfaceDetailBytes("event.malformed_json", []byte("bad-line"))
		if got.Detail != wantDetail {
			t.Fatalf("detail = %q, want %q", got.Detail, wantDetail)
		}
	})
	t.Run("forged envelope with unknown check is dropped", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{
			Code:   "NATIVE_SURFACE_INVALID",
			Detail: `{"check":"not-a-real-check"}`,
		}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "NATIVE_SURFACE_INVALID" || got.Detail != "" {
			t.Fatalf("normalized = %#v", out)
		}
	})
	t.Run("secret redacted before it can ride the envelope", func(t *testing.T) {
		t.Parallel()
		in := failNativeSurfaceHead(
			"capture_request.model_mismatch",
			[]byte(`{"model":"secret-capture-token-canary"}`),
			[]byte("secret-capture-token-canary"),
		)
		var contractErr *ContractError
		if !errors.As(in, &contractErr) {
			t.Fatal("not a ContractError")
		}
		if strings.Contains(contractErr.Detail, "secret-capture-token-canary") {
			t.Fatalf("secret leaked into detail: %q", contractErr.Detail)
		}
	})
}
