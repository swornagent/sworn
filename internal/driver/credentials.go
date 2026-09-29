package driver

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"
)

type FileCredentialResolver func(context.Context, string) (string, error)

type nativeCredentialLease interface {
	File() *os.File
	Close() error
	// benignRotation reports whether the most recent Close verified a benign
	// atomic-rename rotation: the live path entry was a new inode on the
	// same device with the full safe shape intact, so the lease closed
	// without failing the dispatch. It is false when Close failed, never
	// ran, or verified the original inode (including in-place refresh).
	benignRotation() bool
}

// nativeCredentialEpochFloorMillis is the floor below which an expiresAt
// millis value is not positively readable as an OAuth expiry. A value at or
// below the floor (zero, negative, a seconds-epoch value, or an ancient
// millis value) passes preflight: the only in-tree vocabulary datum is the
// far-future millis fixture, and a false refusal of a healthy dispatch is a
// worse defect than the status quo.
const nativeCredentialEpochFloorMillis int64 = 1_000_000_000_000

// nativeCredentialExpiryMillis reads the positive OAuth expiry millis value
// out of body, or reports not-ok. The vocabulary is strictly the Claude
// OAuth shape pinned by the certification fixture
// (native_capture_linux.go:758-780): {"claudeAiOauth":{"expiresAt":<int>}}
// with a strictly positive integer millis value strictly above the epoch
// floor. The read is a strict bounded JSON decode of the object path -
// never a substring or regex scan - so token text containing "expiresAt"
// cannot trip a refusal, and no credential byte ever surfaces in an error,
// detail, or log. Anything unparseable, non-integer, exponent-form,
// floor-or-below, missing, or from a family without the vocabulary is
// reported not-ok (fail-open on ignorance, fail-closed on knowledge).
func nativeCredentialExpiryMillis(family ProfileFamily, body []byte) (millis int64, ok bool) {
	if family != ProfileClaude || len(body) == 0 {
		return 0, false
	}
	value, err := decodeStrict(body, 1_048_576)
	if err != nil {
		return 0, false
	}
	root, ok2 := value.(map[string]any)
	if !ok2 {
		return 0, false
	}
	oauth, ok2 := root["claudeAiOauth"].(map[string]any)
	if !ok2 {
		return 0, false
	}
	expiry, present := oauth["expiresAt"]
	if !present {
		return 0, false
	}
	number, ok2 := expiry.(json.Number)
	if !ok2 {
		return 0, false
	}
	parsed, err := number.Int64()
	if err != nil || parsed <= nativeCredentialEpochFloorMillis {
		return 0, false
	}
	return parsed, true
}

// nativeCredentialStale reports whether body positively reads as an expired
// native credential: nowMillis at or below the epoch floor is honestly
// unevaluated (false), otherwise the verdict is nativeCredentialExpiryMillis's
// own reading compared against nowMillis. Unchanged behavior, unchanged
// signature - only the parsing itself moved into nativeCredentialExpiryMillis
// so the admission-time dispatch-lifetime check (A1) can reuse it without
// duplicating the vocabulary.
func nativeCredentialStale(family ProfileFamily, body []byte, nowMillis int64) bool {
	if nowMillis <= nativeCredentialEpochFloorMillis {
		return false
	}
	millis, ok := nativeCredentialExpiryMillis(family, body)
	return ok && millis < nowMillis
}

// nativeCredentialDispatchMarginMillis is the fixed, engine-internal
// lookahead margin A1 adds on top of a dispatch's own declared timeout: a
// credential that will expire before the dispatch's timeout plus this
// margin elapses is refused at admission rather than left to expire mid-turn.
// Not a manifest knob - this release names it as a fixed constant.
const nativeCredentialDispatchMarginMillis int64 = 300_000

// nativeCredentialDispatchVerdict is nativeCredentialStale's dispatch-lifetime
// sibling (A1): stale reports the already-expired case (unchanged from
// nativeCredentialStale), expiresDuringDispatch reports a credential that is
// not yet expired but will expire before deadlineMillis, expiresAtMillis is
// the credential's own reading (valid only when evaluated), and evaluated
// mirrors nativeCredentialExpiryMillis's own ok: unparseable, missing, floor-
// or-below, or non-Claude bodies are honestly unevaluated, never a refusal.
func nativeCredentialDispatchVerdict(
	family ProfileFamily,
	body []byte,
	nowMillis int64,
	deadlineMillis int64,
) (stale bool, expiresDuringDispatch bool, expiresAtMillis int64, evaluated bool) {
	if nowMillis <= nativeCredentialEpochFloorMillis {
		return false, false, 0, false
	}
	millis, ok := nativeCredentialExpiryMillis(family, body)
	if !ok {
		return false, false, 0, false
	}
	switch {
	case millis < nowMillis:
		return true, false, millis, true
	case millis < deadlineMillis:
		return false, true, millis, true
	default:
		return false, false, millis, true
	}
}

// nativeCredentialDispatchDetail renders A1's bounded, duration-only park
// detail from three already-computed int64 millis values: no credential
// byte, no absolute timestamp - only "remaining <duration>, required
// <duration>" built from time.Duration's own compact formatting.
func nativeCredentialDispatchDetail(
	nowMillis int64,
	expiresAtMillis int64,
	deadlineMillis int64,
) string {
	remaining := time.Duration(expiresAtMillis-nowMillis) * time.Millisecond
	required := time.Duration(deadlineMillis-nowMillis) * time.Millisecond
	return "remaining " + remaining.String() + ", required " + required.String()
}

// nativeCredentialPreflight refuses CREDENTIAL_STALE when the credential at
// pathValue positively reads as already expired, and refuses
// CREDENTIAL_EXPIRES_DURING_DISPATCH (A1) when it positively reads as not
// yet expired but due to expire before timeoutMillis (the dispatch's own
// declared timeout - 0 when the caller has none, e.g. automation) plus the
// fixed margin elapses. It is a bounded read-only advisory probe run as the
// last gate before the CLI spawns (nativeRuntime/nativeAutomationRuntime):
// any open, read, or parse failure passes unchanged (fail-open on
// ignorance), and the exclusive lease keeps enforcing the full security
// posture (0600, owner, single link, O_NOFOLLOW) before any byte reaches the
// CLI.
func nativeCredentialPreflight(
	family ProfileFamily,
	pathValue string,
	maximum int64,
	timeoutMillis int64,
) error {
	nowMillis := time.Now().UnixMilli()
	deadlineMillis := nowMillis + timeoutMillis + nativeCredentialDispatchMarginMillis
	stale, expiresDuringDispatch, expiresAtMillis, evaluated :=
		nativeCredentialDispatchLivenessCheck(
			family, pathValue, maximum, nowMillis, deadlineMillis,
		)
	switch {
	case evaluated && stale:
		return fail("CREDENTIAL_STALE")
	case evaluated && expiresDuringDispatch:
		return failWithDetail(
			"CREDENTIAL_EXPIRES_DURING_DISPATCH",
			nativeCredentialDispatchDetail(nowMillis, expiresAtMillis, deadlineMillis),
		)
	}
	return nil
}

// nativeCredentialLivenessCheck is the shared bounded, read-only
// open-then-read-then-nativeCredentialStale logic behind every credential
// liveness caller: the dispatch-time fail-open gate (nativeCredentialPreflight),
// certify (checkCertify's honest reporting), and the admission-time probe
// (ProbeNativeCredentialLiveness). evaluated reports whether the credential
// was opened and read within maximum; when it is false, stale is always
// false and callers must not read it as a liveness claim - it is simply "not
// evaluated". When evaluated is true, stale is nativeCredentialStale's own
// verdict, which only ever positively proves expiry (evaluated-and-not-stale
// is "not positively stale", never a claim of confirmed liveness). The
// bounded body is cleared before return; no credential byte enters an error,
// detail, or log.
func nativeCredentialLivenessCheck(
	family ProfileFamily,
	pathValue string,
	maximum int64,
) (stale bool, evaluated bool) {
	body, ok := nativeCredentialReadBody(pathValue, maximum)
	if !ok {
		return false, false
	}
	stale = nativeCredentialStale(family, body, time.Now().UnixMilli())
	clearBytes(body)
	return stale, true
}

// nativeCredentialReadBody is the shared bounded, read-only open-then-read
// dance every credential liveness caller needs: an exclusive-lease open of
// pathValue, then a read bounded to maximum bytes. ok is false for any open,
// read, or bound failure - fail-open on ignorance, exactly as before this
// was extracted out of nativeCredentialLivenessCheck.
func nativeCredentialReadBody(pathValue string, maximum int64) (body []byte, ok bool) {
	if maximum < 1 || maximum > 1_048_576 {
		return nil, false
	}
	file, err := openCredentialPreflight(pathValue)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	body, readErr := io.ReadAll(io.LimitReader(file, maximum+1))
	if readErr != nil || int64(len(body)) > maximum {
		clearBytes(body)
		return nil, false
	}
	return body, true
}

// nativeCredentialDispatchLivenessCheck is nativeCredentialLivenessCheck's
// dispatch-lifetime sibling (A1): the same bounded, read-only open-then-read
// dance, then nativeCredentialDispatchVerdict instead of nativeCredentialStale.
func nativeCredentialDispatchLivenessCheck(
	family ProfileFamily,
	pathValue string,
	maximum int64,
	nowMillis int64,
	deadlineMillis int64,
) (stale bool, expiresDuringDispatch bool, expiresAtMillis int64, evaluated bool) {
	body, ok := nativeCredentialReadBody(pathValue, maximum)
	if !ok {
		return false, false, 0, false
	}
	stale, expiresDuringDispatch, expiresAtMillis, evaluated =
		nativeCredentialDispatchVerdict(family, body, nowMillis, deadlineMillis)
	clearBytes(body)
	return
}
