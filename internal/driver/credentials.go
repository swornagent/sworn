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

// nativeCredentialStale reports whether body positively reads as an expired
// native credential. The vocabulary is strictly the Claude OAuth shape
// pinned by the certification fixture
// (native_capture_linux.go:758-780): {"claudeAiOauth":{"expiresAt":<int>}}
// with a strictly positive integer millis value strictly between the epoch
// floor and now. The read is a strict bounded JSON decode of the object
// path - never a substring or regex scan - so token text containing
// "expiresAt" cannot trip a refusal, and no credential byte ever surfaces in
// an error, detail, or log. Anything unparseable, non-integer, exponent-form,
// floor-or-below, missing, or from a family without the vocabulary is not
// positively expired and passes (fail-open on ignorance, fail-closed on
// knowledge).
func nativeCredentialStale(family ProfileFamily, body []byte, nowMillis int64) bool {
	if family != ProfileClaude || len(body) == 0 ||
		nowMillis <= nativeCredentialEpochFloorMillis {
		return false
	}
	value, err := decodeStrict(body, 1_048_576)
	if err != nil {
		return false
	}
	root, ok := value.(map[string]any)
	if !ok {
		return false
	}
	oauth, ok := root["claudeAiOauth"].(map[string]any)
	if !ok {
		return false
	}
	expiry, present := oauth["expiresAt"]
	if !present {
		return false
	}
	number, ok := expiry.(json.Number)
	if !ok {
		return false
	}
	millis, err := number.Int64()
	if err != nil || millis <= nativeCredentialEpochFloorMillis {
		return false
	}
	return millis < nowMillis
}

// nativeCredentialPreflight refuses CREDENTIAL_STALE only when the credential
// at pathValue positively reads as expired under nativeCredentialStale. It is
// a bounded read-only advisory probe run at dispatch preparation: any open,
// read, or parse failure passes unchanged (fail-open on ignorance), and the
// exclusive lease keeps enforcing the full security posture (0600, owner,
// single link, O_NOFOLLOW) before any byte reaches the CLI. The bounded body
// is cleared before return; no credential byte enters an error, detail, or
// log.
func nativeCredentialPreflight(family ProfileFamily, pathValue string, maximum int64) error {
	if maximum < 1 || maximum > 1_048_576 {
		return nil
	}
	file, err := openCredentialPreflight(pathValue)
	if err != nil {
		return nil
	}
	defer file.Close()
	body, readErr := io.ReadAll(io.LimitReader(file, maximum+1))
	if readErr != nil || int64(len(body)) > maximum {
		clearBytes(body)
		return nil
	}
	stale := nativeCredentialStale(family, body, time.Now().UnixMilli())
	clearBytes(body)
	if stale {
		return fail("CREDENTIAL_STALE")
	}
	return nil
}
