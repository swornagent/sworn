package driver

import (
	"strconv"
	"testing"
	"time"
)

func TestNativeCredentialStaleRefusesOnlyPositivelyExpired(t *testing.T) {
	now := time.Now().UnixMilli()
	expired := now - 60_000
	fixtureFuture := int64(8_000_000_000_000_000)
	tokenContainingExpiryWord := `{"claudeAiOauth":{"accessToken":"expiresAt-not-a-field","refreshToken":"x"}}`

	cases := []struct {
		name    string
		family  ProfileFamily
		body    string
		now     int64
		expired bool
	}{
		{
			name:    "fixture far-future value is not expired",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"accessToken":"a","refreshToken":"r","expiresAt":` + strconv.FormatInt(fixtureFuture, 10) + `,"scopes":["user:inference"],"subscriptionType":"max"}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "expired millis value is positively stale",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"accessToken":"a","expiresAt":` + strconv.FormatInt(expired, 10) + `}}`,
			now:     now,
			expired: true,
		},
		{
			name:    "expiry-less credential passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"accessToken":"a","refreshToken":"r","scopes":["user:inference"],"subscriptionType":"max"}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "missing oauth object passes",
			family:  ProfileClaude,
			body:    `{"token":"first"}`,
			now:     now,
			expired: false,
		},
		{
			name:    "token text mentioning expiresAt cannot trip a refusal",
			family:  ProfileClaude,
			body:    tokenContainingExpiryWord,
			now:     now,
			expired: false,
		},
		{
			name:    "zero boundary passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":0}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "negative boundary passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":-1}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "epoch floor passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(nativeCredentialEpochFloorMillis, 10) + `}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "seconds-epoch value is not positively readable",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":1700000000}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "fractional number passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":123.5}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "exponent form passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":1e12}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "string expiry passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":"` + strconv.FormatInt(expired, 10) + `"}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "malformed json passes",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":`,
			now:     now,
			expired: false,
		},
		{
			name:    "trailing json passes",
			family:  ProfileClaude,
			body:    `{} {}`,
			now:     now,
			expired: false,
		},
		{
			name:    "empty body passes",
			family:  ProfileClaude,
			body:    ``,
			now:     now,
			expired: false,
		},
		{
			name:    "non-object root passes",
			family:  ProfileClaude,
			body:    `[1,2]`,
			now:     now,
			expired: false,
		},
		{
			name:    "codex family has no expiry vocabulary",
			family:  ProfileCodex,
			body:    `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(expired, 10) + `}}`,
			now:     now,
			expired: false,
		},
		{
			name:    "now at the floor never refuses",
			family:  ProfileClaude,
			body:    `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(expired, 10) + `}}`,
			now:     nativeCredentialEpochFloorMillis,
			expired: false,
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := nativeCredentialStale(
				test.family,
				[]byte(test.body),
				test.now,
			); got != test.expired {
				t.Fatalf(
					"nativeCredentialStale(%s, %q, %d) = %v, want %v",
					test.family,
					test.body,
					test.now,
					got,
					test.expired,
				)
			}
		})
	}
}

// TestNativeCredentialDispatchVerdict pins A1's timeout+margin lookahead:
// already-expired keeps staying stale, a credential expiring inside the
// deadline refuses as expiring-during-dispatch, one expiring after the
// deadline (or unparseable, or from a family without the vocabulary, or at
// the floor) is not evaluated as either and does not refuse.
func TestNativeCredentialDispatchVerdict(t *testing.T) {
	const now int64 = 2_000_000_000_000
	const deadline int64 = now + 300_000 // now + 5 minute margin, no timeout

	cases := []struct {
		name                      string
		family                    ProfileFamily
		body                      string
		now                       int64
		deadline                  int64
		wantStale                 bool
		wantExpiresDuringDispatch bool
		wantEvaluated             bool
	}{
		{
			name:          "already expired stays stale",
			family:        ProfileClaude,
			body:          `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(now-60_000, 10) + `}}`,
			now:           now,
			deadline:      deadline,
			wantStale:     true,
			wantEvaluated: true,
		},
		{
			name:                      "expires inside the deadline refuses",
			family:                    ProfileClaude,
			body:                      `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(now+120_000, 10) + `}}`,
			now:                       now,
			deadline:                  deadline,
			wantExpiresDuringDispatch: true,
			wantEvaluated:             true,
		},
		{
			name:          "expires after the deadline passes",
			family:        ProfileClaude,
			body:          `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(deadline+1, 10) + `}}`,
			now:           now,
			deadline:      deadline,
			wantEvaluated: true,
		},
		{
			name:     "unparseable body is unevaluated",
			family:   ProfileClaude,
			body:     `{"claudeAiOauth":{"expiresAt":`,
			now:      now,
			deadline: deadline,
		},
		{
			name:     "codex has no expiry vocabulary",
			family:   ProfileCodex,
			body:     `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(now+120_000, 10) + `}}`,
			now:      now,
			deadline: deadline,
		},
		{
			name:     "now at the floor is unevaluated",
			family:   ProfileClaude,
			body:     `{"claudeAiOauth":{"expiresAt":` + strconv.FormatInt(now+120_000, 10) + `}}`,
			now:      nativeCredentialEpochFloorMillis,
			deadline: nativeCredentialEpochFloorMillis + 300_000,
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			stale, expiresDuringDispatch, _, evaluated := nativeCredentialDispatchVerdict(
				test.family, []byte(test.body), test.now, test.deadline,
			)
			if stale != test.wantStale ||
				expiresDuringDispatch != test.wantExpiresDuringDispatch ||
				evaluated != test.wantEvaluated {
				t.Fatalf(
					"nativeCredentialDispatchVerdict(%s, %q, %d, %d) = (%v, %v, _, %v), want (%v, %v, _, %v)",
					test.family, test.body, test.now, test.deadline,
					stale, expiresDuringDispatch, evaluated,
					test.wantStale, test.wantExpiresDuringDispatch, test.wantEvaluated,
				)
			}
		})
	}
}

// TestNativeCredentialDispatchDetailIsDurationOnly pins A2's bounded,
// duration-only detail: no credential byte, no absolute timestamp, only the
// remaining and required durations rendered from the three int64 millis
// this package already computed.
func TestNativeCredentialDispatchDetailIsDurationOnly(t *testing.T) {
	const now int64 = 1_000_000
	detail := nativeCredentialDispatchDetail(now, now+90_000, now+300_000)
	if detail != "remaining 1m30s, required 5m0s" {
		t.Fatalf("nativeCredentialDispatchDetail = %q", detail)
	}
}
