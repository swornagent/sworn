package runtime

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/journal"
)

// journalOuterTry writes one outer try's git.seal effect at the given epoch
// and try, completed with the given state and error code.
func journalOuterTry(
	t *testing.T,
	ctx context.Context,
	store *journal.Store,
	owner journal.OwnerLease,
	runID, workID string,
	epoch, try int64,
	state journal.EffectState,
	errorCode string,
	now time.Time,
) {
	t.Helper()
	effectID := journal.AttemptEffectID(workID, epoch, try)
	payload := []byte(`{"outer":true}`)
	command := journal.Command{
		RunID: runID, ReplayKey: effectID, Kind: "git.seal",
		Payload: payload, CreatedAt: now,
	}
	effect := journal.Effect{
		RunID: runID, ID: effectID, ReplayKey: effectID, Kind: "git.seal",
		BeforeDigest: workID, ExpectedDigest: sha256Digest(payload), UpdatedAt: now,
	}
	if err := store.EnsureAttempt(ctx, command, effect,
		journal.EffectAttempt{WorkID: workID, Epoch: epoch, Try: try}); err != nil {
		t.Fatalf("ensure outer attempt e%d/t%d: %v", epoch, try, err)
	}
	claim, err := store.ClaimOwned(ctx, owner, effectID, now, time.Minute)
	if err != nil {
		t.Fatalf("claim outer e%d/t%d: %v", epoch, try, err)
	}
	if err := store.CompleteOwned(ctx, owner, journal.Completion{
		RunID: runID, EffectID: effectID, Token: claim.Token, State: state,
		ErrorCode: errorCode, EventKind: "implementation_operational_failure",
		EventBody: []byte("failure"), At: now,
	}); err != nil {
		t.Fatalf("complete outer e%d/t%d: %v", epoch, try, err)
	}
}

// journalInnerDispatch writes one inner driver.dispatch effect at the given
// work/epoch/try, completed Succeeded.
func journalInnerDispatch(
	t *testing.T,
	ctx context.Context,
	store *journal.Store,
	owner journal.OwnerLease,
	runID, dispatchWork string,
	epoch, try int64,
	now time.Time,
) {
	t.Helper()
	effectID := journal.AttemptEffectID(dispatchWork, epoch, try)
	payload := []byte(`{"inner":true}`)
	command := journal.Command{
		RunID: runID, ReplayKey: effectID, Kind: "driver.dispatch",
		Payload: payload, CreatedAt: now,
	}
	effect := journal.Effect{
		RunID: runID, ID: effectID, ReplayKey: effectID, Kind: "driver.dispatch",
		BeforeDigest: dispatchWork, ExpectedDigest: sha256Digest(payload), UpdatedAt: now,
	}
	if err := store.EnsureAttempt(ctx, command, effect,
		journal.EffectAttempt{WorkID: dispatchWork, Epoch: epoch, Try: try}); err != nil {
		t.Fatalf("ensure inner attempt e%d/t%d: %v", epoch, try, err)
	}
	claim, err := store.ClaimOwned(ctx, owner, effectID, now, time.Minute)
	if err != nil {
		t.Fatalf("claim inner e%d/t%d: %v", epoch, try, err)
	}
	if err := store.CompleteOwned(ctx, owner, journal.Completion{
		RunID: runID, EffectID: effectID, Token: claim.Token, State: journal.Succeeded,
		Result: []byte(`{}`), EventKind: "dispatch_succeeded",
		EventBody: []byte("ok"), At: now,
	}); err != nil {
		t.Fatalf("complete inner e%d/t%d: %v", epoch, try, err)
	}
}

func newScopeRefusalRetryFixture(t *testing.T) (
	*Service, context.Context, journal.OwnerLease, string, time.Time,
) {
	t.Helper()
	ctx := context.Background()
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	const runID = "run-scope-escape"
	if err := store.RegisterRun(ctx, journal.Run{
		ID: runID, ManifestDigest: sha256Digest([]byte("manifest")),
		Repository: "/workspace", Release: "rel-scope-escape",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, runID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{journal: store, now: func() time.Time { return now }}
	return service, ctx, owner, runID, now
}

// A1: after a scope refusal follows a succeeded dispatch, the try that
// follows it has escaped and must take a fresh inner identity.
func TestScopeRefusalEscapedAfterSucceededDispatchThenScopeRefusal(t *testing.T) {
	service, ctx, owner, runID, now := newScopeRefusalRetryFixture(t)
	workID := workIdentity("before-fixture-1", "git.seal")
	const epoch = int64(1)

	// try=1 has no prior tries: never escaped.
	escaped, err := service.scopeRefusalEscaped(ctx, runID, workID, epoch, 1)
	if err != nil {
		t.Fatal(err)
	}
	if escaped {
		t.Fatal("try 1 has no prior history and must not be escaped")
	}

	// t1's inner dispatch (epoch-shared identity) succeeds, then the outer
	// seal refuses scope.
	sharedDispatchWork := workIdentity(workID, "driver.dispatch")
	journalInnerDispatch(t, ctx, service.journal, owner,
		runID, sharedDispatchWork, epoch, 1, now)
	journalOuterTry(t, ctx, service.journal, owner, runID, workID,
		epoch, 1, journal.OperationalFailed, "CANDIDATE_SCOPE_FAILED", now)

	escaped, err = service.scopeRefusalEscaped(ctx, runID, workID, epoch, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !escaped {
		t.Fatal("try 2 must escape after t1's succeeded dispatch was scope-refused")
	}

	// A later try (try 3) re-derives the same history and stays escaped.
	escaped, err = service.scopeRefusalEscaped(ctx, runID, workID, epoch, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !escaped {
		t.Fatal("try 3 must stay escaped once a prior try escaped")
	}
}

// A guard for every non-scope-refusal case: a prior try that failed with
// any other code, or whose dispatch never succeeded, must not escape.
func TestScopeRefusalEscapedFalseForNonScopeFailureAndUnsucceededDispatch(t *testing.T) {
	service, ctx, owner, runID, now := newScopeRefusalRetryFixture(t)
	const epoch = int64(1)

	// A different error code must never escape, even with a succeeded
	// dispatch behind it.
	workOther := workIdentity("before-fixture-other", "git.seal")
	sharedDispatchOther := workIdentity(workOther, "driver.dispatch")
	journalInnerDispatch(t, ctx, service.journal, owner,
		runID, sharedDispatchOther, epoch, 1, now)
	journalOuterTry(t, ctx, service.journal, owner, runID, workOther,
		epoch, 1, journal.OperationalFailed, "STALE_DISPATCH", now)
	escaped, err := service.scopeRefusalEscaped(ctx, runID, workOther, epoch, 2)
	if err != nil {
		t.Fatal(err)
	}
	if escaped {
		t.Fatal("a non-scope-refusal failure must never escape")
	}

	// A scope refusal whose inner dispatch never journaled (died before a
	// worker turn) must not escape either.
	workNoDispatch := workIdentity("before-fixture-no-dispatch", "git.seal")
	journalOuterTry(t, ctx, service.journal, owner, runID, workNoDispatch,
		epoch, 1, journal.OperationalFailed, "CANDIDATE_SCOPE_FAILED", now)
	escaped, err = service.scopeRefusalEscaped(ctx, runID, workNoDispatch, epoch, 2)
	if err != nil {
		t.Fatal(err)
	}
	if escaped {
		t.Fatal("a scope refusal with no succeeded dispatch must not escape")
	}
}

// A2: an exhaustion park whose tries died on a scope refusal names the
// refusal's specific code and paths in the identical-failure pattern.
func TestScopeExhaustionDetailRendersCodeAndPathsBounded(t *testing.T) {
	if got := scopeExhaustionDetail(nil); got != "" {
		t.Fatalf("nil result = %q", got)
	}
	binding := productionRefusalBinding{
		Code: "SLICE_OUTSIDE_SCOPE", Paths: []string{"outside.txt"}, TotalPaths: 1,
	}
	detail := scopeExhaustionDetail(mustJSON(binding))
	const want = "SLICE_OUTSIDE_SCOPE: outside.txt"
	if detail != want {
		t.Fatalf("detail = %q, want %q", detail, want)
	}
	if !validParkDetail(detail) {
		t.Fatalf("rendered detail fails validParkDetail: %q", detail)
	}

	// C4: 20 long paths must truncate to fit within validParkDetail's
	// 2048-byte cap rather than emit a detail the park event validation
	// would reject.
	var manyPaths []string
	for i := 0; i < 20; i++ {
		manyPaths = append(manyPaths, strings.Repeat("a", 150)+"/"+strconv.Itoa(i))
	}
	wide := productionRefusalBinding{
		Code: "SLICE_OUTSIDE_SCOPE", Paths: manyPaths, TotalPaths: 20,
	}
	wideDetail := scopeExhaustionDetail(mustJSON(wide))
	if len(wideDetail) == 0 {
		t.Fatal("wide detail truncated to nothing")
	}
	if !validParkDetail(wideDetail) {
		t.Fatalf("wide detail (%d bytes) fails validParkDetail", len(wideDetail))
	}
}
