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
