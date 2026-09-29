package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// hostCheckSchemaVersion identifies the engine-owned check.host command
// payload. It is a product constant, not a runtime record.
const hostCheckSchemaVersion = "sworn.host-check/v1"

const (
	// hostCheckOutputBytes bounds one host check's captured output. The full
	// bounded output stays in the journaled effect payload (under the
	// journal's payload cap); the receipt manifest carries only its digest
	// plus a bounded excerpt, so any number of host checks stays within the
	// evidence cap.
	hostCheckOutputBytes = 256 * 1024
)

// hostCheckCommand is the canonical payload journaled with a check.host
// effect. It carries the exact binding (slice, candidate, contract digest)
// and the exact approved check command, never model-supplied input, so a
// recovered or reused effect can be re-admitted exactly.
type hostCheckCommand struct {
	SchemaVersion  string `json:"schema_version"`
	Slice          string `json:"slice"`
	Candidate      string `json:"candidate"`
	ContractDigest string `json:"contract_digest"`
	Check          string `json:"check"`
	OutputBytes    int64  `json:"output_bytes"`
	TimeoutMillis  int64  `json:"timeout_millis"`
	// RerunOf names the check.host effect whose recorded plain failure this
	// command re-executes once for the same candidate (#296); empty for a
	// first execution.
	RerunOf string `json:"rerun_of,omitempty"`
}

// hostCheckResult is the journaled effect result for one host check. The
// output is the full bounded output (with a truthful truncation marker when
// the command produced more than the cap); the receipt manifest carries only
// its digest and a bounded excerpt.
type hostCheckResult struct {
	Slice          string `json:"slice"`
	Candidate      string `json:"candidate"`
	ContractDigest string `json:"contract_digest"`
	Check          string `json:"check"`
	Outcome        string `json:"outcome"`
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	OutputDigest   string `json:"output_digest"`
	Truncated      bool   `json:"truncated"`
	Diagnostic     string `json:"diagnostic,omitempty"`
	EffectID       string `json:"effect_id"`
	// RerunOf names the earlier check.host effect whose plain failure this
	// result re-executed (#296), so the record says this outcome replaced a
	// recorded one rather than being the candidate's first evidence.
	RerunOf string `json:"rerun_of,omitempty"`
}

type hostCheckRefusal struct {
	Slice          string `json:"slice"`
	Candidate      string `json:"candidate"`
	ContractDigest string `json:"contract_digest"`
	Check          string `json:"check"`
	Reason         string `json:"reason"`
	EffectID       string `json:"effect_id"`
}

func hostCheckWork(sliceID, candidate, contractDigest, check string) string {
	return workIdentity("check.host", sliceID, candidate, contractDigest, check)
}

// assemblyHostCheckWork is the check.host work identity of one declared check
// executed against an assembly candidate (sworn#343). It is hostCheckWork
// with an empty slice: no slice owns an assembly, a slice identity is never
// empty, and the contract digest is the assembly's own union digest
// (assemblyContractDigest), so the identity can collide with no slice's
// check.host work and replays exactly like one.
func assemblyHostCheckWork(candidate, contractDigest, check string) string {
	return hostCheckWork("", candidate, contractDigest, check)
}

// assemblyContractDigest binds the set of approved contracts an assembly's
// declared host checks came from: the digest of the canonical sorted list of
// the distinct slice contract digests. It is the contract_digest every
// assembly check.host effect and the assembly manifest carry, so a check is
// bound to exactly the contracts that declared it.
func assemblyContractDigest(contractDigests []string) string {
	distinct := make([]string, 0, len(contractDigests))
	seen := make(map[string]struct{}, len(contractDigests))
	for _, digest := range contractDigests {
		if _, duplicate := seen[digest]; duplicate {
			continue
		}
		seen[digest] = struct{}{}
		distinct = append(distinct, digest)
	}
	sort.Strings(distinct)
	return protocol.DigestBytes(mustJSON(distinct))
}

func hostCheckRefusalWork(sliceID, candidate, check, reason string) string {
	return workIdentity("check.refused", sliceID, candidate, check, reason)
}

func hostCheckEffectID(work string) string {
	return journal.AttemptEffectID(work, 1, 1)
}

// hostCheckRerunWork is the work identity of the one bounded re-execution a
// check.host work may have (#296). It is derived from the first execution's
// identity, so the candidate, the contract and the check bind it exactly as
// they bind the first execution; it is a separate work because the journal
// admits no further attempt on a work that already succeeded.
func hostCheckRerunWork(work string) string {
	return workIdentity(work, "rerun")
}

func hostCheckRerunEffectID(work string) string {
	return hostCheckEffectID(hostCheckRerunWork(work))
}

// hostCheckBoundWork returns the work identity effectID must be bound to
// (its effect's before digest) when it is one of the two effect identities
// a check.host work may carry: the first execution or its one re-execution.
func hostCheckBoundWork(effectID, work string) (string, bool) {
	switch effectID {
	case hostCheckEffectID(work):
		return work, true
	case hostCheckRerunEffectID(work):
		return hostCheckRerunWork(work), true
	default:
		return "", false
	}
}

func isHostCheckEffectID(effectID, work string) bool {
	_, ok := hostCheckBoundWork(effectID, work)
	return ok
}

// hostCheckDeterministicSignatures are output markers of a failure that a
// re-execution cannot honestly change: a detected data race and a build or
// setup failure are defects in the candidate, not flakes, so a recorded
// failure carrying one of them is never re-run (#296).
var hostCheckDeterministicSignatures = []string{
	"WARNING: DATA RACE",
	"[build failed]",
	"[setup failed]",
}

// hostCheckRerunEligible reports whether a recorded host-check failure may
// be re-executed once when the same candidate is checked again (#296): only
// a plain fail (never a timeout or an overflow, which are bounds the
// contract sets) whose bounded output carries no deterministic signature.
func hostCheckRerunEligible(result hostCheckResult) bool {
	if result.Outcome != protocol.CheckOutcomeFail || result.RerunOf != "" {
		return false
	}
	for _, signature := range hostCheckDeterministicSignatures {
		if strings.Contains(result.Output, signature) {
			return false
		}
	}
	return true
}

// latestJournaledHostCheck returns the check.host effect that currently
// speaks for work: its one re-execution when that has succeeded, otherwise
// its first execution. Readers of host evidence go through this so a
// re-executed check's outcome is the one the seal actually consumed.
func latestJournaledHostCheck(
	ctx context.Context,
	engine *engine,
	work string,
) (journal.Effect, string, error) {
	rerunID := hostCheckRerunEffectID(work)
	rerun, err := engine.journal.Effect(ctx, engine.manifest.value.RunID, rerunID)
	if err == nil && rerun.Kind == "check.host" && rerun.State == journal.Succeeded {
		return rerun, rerunID, nil
	}
	if err != nil && !journal.IsCode(err, "EFFECT_NOT_FOUND") {
		return journal.Effect{}, "", runtimeFail("JOURNAL_READ_FAILED", err)
	}
	effectID := hostCheckEffectID(work)
	effect, err := engine.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
	if err != nil {
		return journal.Effect{}, "", runtimeFail("JOURNAL_READ_FAILED", err)
	}
	return effect, effectID, nil
}

// resolveSliceHostChecks resolves the human-approved contract for sliceID at
// the exact captured release head (for digest-addressed resolution from the
// record root) and target head (for the path-keyed fallback), and returns its
// declared host_checks and contract digest. Any divergence from the admitted
// plan fails closed, so the host runner can only ever execute commands that
// the approved contract declared.
func resolveSliceHostChecks(
	engine *engine,
	plan protocol.Plan,
	sliceID, targetHead, releaseHead string,
) ([]string, string, error) {
	if engine == nil {
		return nil, "", runtimeFail("INVALID_ENGINE", nil)
	}
	contract, err := plan.ResolveSliceContractAtHead(
		engine.git,
		sliceID,
		releaseHead,
		targetHead,
	)
	if err != nil {
		return nil, "", runtimeFail("CONTRACT_RESOLUTION_FAILED", err)
	}
	contractDigest, ok := plan.Contract(sliceID)
	if !ok {
		return nil, "", runtimeFail("CONTRACT_RESOLUTION_FAILED", nil)
	}
	return contract.HostChecks, contractDigest, nil
}

func hostCheckTimeout(engine *engine) time.Duration {
	if engine == nil || engine.manifest.value.Limits.TimeoutMillis < 1 {
		return 60 * time.Minute
	}
	return time.Duration(engine.manifest.value.Limits.TimeoutMillis) * time.Millisecond
}

// hostShell returns the POSIX shell the host runner uses, resolved from
// configuration or discovery (SWORN_SH override, else LookPath("sh")) so a
// minimal or non-Debian host works without patching. A resolution failure is
// propagated: a missing or invalid configured shell refuses loudly instead
// of silently restoring a hardcoded literal.
func hostShell() (string, error) {
	return gitx.ResolveShellExecutable()
}

// hostEnvironmentClassificationSchemaVersion versions the durable
// classification record a host runner journals the instant it recognizes a
// check.host command as a host environment failure (S1-host-check-
// environment-failures). It is a new, additive record kind: it never
// changes check.host's own effect or hostCheckResult's outcome vocabulary.
const hostEnvironmentClassificationSchemaVersion = "sworn.host-check-environment/v1"

// hostEnvironmentClassification is the durable payload a host runner
// journals (via journalHostEnvironmentClassification) the moment it
// classifies a check.host command as unresolvable on its own host: the
// check's first word does not resolve, or the command exited 127. It names
// the check.host effect it concerns (HostEffect) so every reader - the
// self-clearing crossing scan, the A5 fact, the cockpit - derives currency
// from that effect's own current journal state instead of re-classifying.
type hostEnvironmentClassification struct {
	SchemaVersion  string `json:"schema_version"`
	Slice          string `json:"slice"`
	Candidate      string `json:"candidate"`
	ContractDigest string `json:"contract_digest"`
	Check          string `json:"check"`
	MissingCommand string `json:"missing_command"`
	HostEffect     string `json:"host_effect"`
}

// hostEnvironmentClassificationWork is the work identity of one check.host
// work's environment-classification record: derived from the check.host
// work identity itself, so it can never collide with any other check.host
// work's record and is exactly-once per check.host work by construction.
func hostEnvironmentClassificationWork(checkHostWork string) string {
	return workIdentity(checkHostWork, "environment")
}

// isShellAssignmentWord reports whether field is a POSIX-shaped NAME=value
// assignment word (a leading portable variable-name character class, then
// '='), the same predicate hostCheckCommandWord uses to skip leading
// assignments before the check's actual command word.
func isShellAssignmentWord(field string) bool {
	equals := strings.IndexByte(field, '=')
	if equals <= 0 {
		return false
	}
	name := field[:equals]
	for index, character := range name {
		switch {
		case character == '_' ||
			(character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z'):
			continue
		case index > 0 && character >= '0' && character <= '9':
			continue
		default:
			return false
		}
	}
	return true
}

// hostCheckCommandWord returns the first simple-command word of check,
// skipping any leading NAME=value assignments (for example "GOFLAGS=... go
// test ./..." resolves to "go"). It inspects only the approved command
// string by whitespace splitting, exactly as isLongSuiteHostCheck already
// inspects only the command string; it is not a shell parser, and every
// declared check in this release's own contracts is a plain leading-word
// command, optionally preceded by simple, single-token assignments.
// Honestly unclassifiable (empty) rather than guessed for anything this
// whitespace split cannot safely handle: no non-assignment word at all, or
// a leading assignment whose own value involves a subshell or expansion
// ('(', ')', or '$' in the value) - such a value can itself span further
// whitespace-separated tokens, which this split would otherwise
// mis-consume as if they followed the assignment, picking a fragment of
// the assignment's own value as if it were the command. An unclassifiable
// check is never treated as a false environment-failure positive; its
// actual exit code (including a genuine 127) is still what
// executeHostCheck's post-run defense in depth reads. A first non-
// assignment word that opens a POSIX compound command - '(' (a subshell,
// for example "(cd dir && make)") or '{' (a brace group, for example
// "{ a; b; }") - is also honestly unclassifiable rather than a false
// environment-failure positive naming that literal opener as a missing
// command (S6-host-environment-park-projection A5(i)): the shell resolves
// the compound command as a whole, not a leading simple-command word, so
// this returns "" and defers entirely to the real exit code, the same way
// an unclassifiable assignment value already does.
func hostCheckCommandWord(check string) string {
	for _, field := range strings.Fields(check) {
		if isShellAssignmentWord(field) {
			if strings.ContainsAny(field, "()$") {
				return ""
			}
			continue
		}
		if strings.HasPrefix(field, "(") || strings.HasPrefix(field, "{") {
			return ""
		}
		return field
	}
	return ""
}

// shellQuote renders s as one single-quoted POSIX shell word, so a missing-
// command classification can never itself become a command-injection
// surface through an adversarial-looking check string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// hostCommandResolves reports whether word resolves on the host that will
// run it, exactly as the shell that runs check itself resolves its first
// word: a path-shaped word (containing '/') is resolved by os.Stat, exactly
// as the shell would attempt to execute it directly; otherwise shell is
// asked via its own `command -v`, which POSIX specifies to report an
// alias, a keyword, a function, and a builtin as found - so cd, export,
// set, ':', '[', if, and '!' are never misclassified as missing, with no
// maintained builtin list to fall out of date.
func hostCommandResolves(shell, word string) bool {
	if word == "" {
		return true
	}
	if strings.ContainsRune(word, '/') {
		_, err := os.Stat(word)
		return err == nil
	}
	command := exec.Command(shell, "-c", "command -v -- "+shellQuote(word)+" >/dev/null 2>&1")
	return command.Run() == nil
}

// classifyHostCheckExecution reports whether check is a host environment
// failure on shell - its first word does not resolve there - and, when so,
// the exact missing command name. It is pure and side-effect free; callers
// journal the answer themselves. This is the one classification rule A1's
// post-hoc exit-127 check, A4's start-time validation, and the recovery
// sweep's re-classification all share, so a check can never be classified
// three different ways.
func classifyHostCheckExecution(shell, check string) (environmentFailure bool, missingCommand string) {
	word := hostCheckCommandWord(check)
	if word == "" || hostCommandResolves(shell, word) {
		return false, ""
	}
	return true, word
}

// journalHostEnvironmentClassification admits and completes one durable
// check.host.environment record (mirrors journalHostCheckRefusal's claim-
// and-complete shape exactly), the moment the host runner recognizes
// checkHostEffectID as a host environment failure. It never touches the
// check.host effect itself, which stays Claimed: A2/A3 hold structurally,
// not by a store-then-suppress step. Idempotent: MissingCommand is check's
// own deterministic first word, so a given check.host work's record can
// only ever carry one value and a later admission of the identical record
// is a safe no-op.
func (s *Service) journalHostEnvironmentClassification(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	checkHostWork, checkHostEffectID string,
	sliceID, candidate, contractDigest, check, missingCommand string,
) error {
	work := hostEnvironmentClassificationWork(checkHostWork)
	effectID := journal.AttemptEffectID(work, 1, 1)
	record := hostEnvironmentClassification{
		SchemaVersion: hostEnvironmentClassificationSchemaVersion,
		Slice:         sliceID, Candidate: candidate, ContractDigest: contractDigest,
		Check: check, MissingCommand: missingCommand, HostEffect: checkHostEffectID,
	}
	body := mustJSON(record)
	now := s.now().UTC()
	command := journal.Command{
		RunID: engine.manifest.value.RunID, ReplayKey: effectID,
		Kind: "check.host.environment", Payload: body, CreatedAt: now,
	}
	effect := journal.Effect{
		RunID: engine.manifest.value.RunID, ID: effectID, ReplayKey: effectID,
		Kind: "check.host.environment", BeforeDigest: work,
		ExpectedDigest: sha256Digest(body), UpdatedAt: now,
	}
	if err := s.journal.EnsureAttempt(ctx, command, effect, journal.EffectAttempt{
		WorkID: work, Epoch: 1, Try: 1,
	}); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	stored, err := s.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if stored.State == journal.Succeeded {
		return nil
	}
	if stored.State != journal.Pending && stored.State != journal.Claimed {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	claim, err := s.journal.ClaimOwned(ctx, owner, effectID, s.now().UTC(), effectLease)
	if err != nil {
		return runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	return s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: engine.manifest.value.RunID, EffectID: effectID, Token: claim.Token,
		State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "host_check_environment", Body: body}},
		EventKind: "host_check_environment_classified",
		EventBody: MarshalAssociation(EventAssociation{
			EffectID: effectID, WorkID: work, Slice: sliceID,
		}),
		At: s.now().UTC(),
	})
}

// runHostCommand executes one approved check command via the fixed
// sh -c surface in a defined bounded environment rooted at dir. Output
// is captured into a bounded buffer with a truthful truncation marker; the
// process group is killed when the timeout expires; a timeout or overflow is
// recorded as such, never as a pass and never as absent.
func runHostCommand(dir, check string, outputBytes int64, timeout time.Duration) hostCheckResult {
	result := hostCheckResult{
		Check:      check,
		Outcome:    protocol.CheckOutcomeFail,
		ExitCode:   -1,
		Diagnostic: "command did not start",
	}
	shell, err := hostShell()
	if err != nil {
		result.Diagnostic = "host runner requires a POSIX shell: " + err.Error()
		return result
	}
	if _, err := os.Stat(shell); err != nil {
		result.Diagnostic = "host runner requires a POSIX shell: " + err.Error()
		return result
	}
	command := exec.Command(shell, "-c", check)
	command.Dir = dir
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &boundedHostBuffer{limit: int(outputBytes)}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		result.Diagnostic = "host command failed to start: " + err.Error()
		return result
	}
	group := command.Process.Pid
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case waitErr := <-done:
		result.ExitCode = command.ProcessState.ExitCode()
		if waitErr == nil {
			result.Outcome = protocol.CheckOutcomePass
			result.Diagnostic = ""
		} else if result.ExitCode == -1 {
			result.Outcome = protocol.CheckOutcomeFail
			result.Diagnostic = "host command terminated abnormally"
		} else {
			result.Outcome = protocol.CheckOutcomeFail
			result.Diagnostic = fmt.Sprintf("exit code %d", result.ExitCode)
		}
	case <-time.After(timeout):
		_ = syscall.Kill(-group, syscall.SIGKILL)
		<-done
		result.Outcome = protocol.CheckOutcomeTimeout
		result.ExitCode = -1
		result.Diagnostic = fmt.Sprintf("host check exceeded %s", timeout)
	}
	_ = syscall.Kill(-group, syscall.SIGCONT) // reap any stopped children
	result.Output = output.String()
	result.Truncated = output.overflow
	// A run bounded by the timeout is recorded as timeout even when it also
	// produced more output than the cap; otherwise overflow of the bounded
	// buffer is recorded as overflow with the truthful marker.
	if result.Outcome != protocol.CheckOutcomeTimeout && result.Truncated {
		marker := fmt.Sprintf("\n[sworn: output truncated at %d bytes]\n", output.limit)
		result.Output += marker
		result.Outcome = protocol.CheckOutcomeOverflow
		result.Diagnostic = fmt.Sprintf("output exceeded %d bytes", output.limit)
	}
	result.OutputDigest = protocol.DigestBytes([]byte(result.Output))
	return result
}

// boundedHostBuffer retains at most limit bytes of combined stdout/stderr and
// records whether more bytes arrived.
type boundedHostBuffer struct {
	limit    int
	overflow bool
	buffer   bytes.Buffer
}

func (b *boundedHostBuffer) Write(p []byte) (int, error) {
	available := b.limit - b.buffer.Len()
	if available > 0 {
		if len(p) <= available {
			b.buffer.Write(p)
		} else {
			b.buffer.Write(p[:available])
			b.overflow = true
		}
	} else if len(p) > 0 {
		b.overflow = true
	}
	return len(p), nil
}

func (b *boundedHostBuffer) String() string { return b.buffer.String() }

// runOneHostCheck runs (or reuses) exactly one declared host check, journaled
// as a durable exactly-once check.host effect bound to the exact slice,
// candidate and contract digest. An identity that the approved contract did
// not declare is refused: a durable check.refused effect is journaled and no
// command executes.
func (s *Service) runOneHostCheck(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	plan protocol.Plan,
	sliceID, candidate, targetHead, releaseHead, check string,
) (hostCheckResult, error) {
	hostChecks, contractDigest, err := resolveSliceHostChecks(engine, plan, sliceID, targetHead, releaseHead)
	if err != nil {
		return hostCheckResult{}, err
	}
	declared := false
	for _, hostCheck := range hostChecks {
		if hostCheck == check {
			declared = true
			break
		}
	}
	if !declared {
		if err := s.journalHostCheckRefusal(
			ctx, engine, owner, sliceID, candidate, contractDigest, check,
		); err != nil {
			return hostCheckResult{}, err
		}
		return hostCheckResult{}, runtimeFail("HOST_CHECK_NOT_DECLARED", nil)
	}
	return s.executeHostCheck(ctx, engine, owner, sliceID, candidate, contractDigest, check)
}

func (s *Service) journalHostCheckRefusal(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	sliceID, candidate, contractDigest, check string,
) error {
	reason := "check is not declared as a containment-requiring check in the approved contract"
	work := hostCheckRefusalWork(sliceID, candidate, check, reason)
	effectID := hostCheckEffectID(work)
	refusal := hostCheckRefusal{
		Slice: sliceID, Candidate: candidate, ContractDigest: contractDigest,
		Check: check, Reason: reason, EffectID: effectID,
	}
	body := mustJSON(refusal)
	now := s.now().UTC()
	command := journal.Command{
		RunID: engine.manifest.value.RunID, ReplayKey: effectID,
		Kind: "check.refused", Payload: body, CreatedAt: now,
	}
	effect := journal.Effect{
		RunID: engine.manifest.value.RunID, ID: effectID, ReplayKey: effectID,
		Kind: "check.refused", BeforeDigest: work,
		ExpectedDigest: sha256Digest(body), UpdatedAt: now,
	}
	if err := s.journal.EnsureAttempt(ctx, command, effect, journal.EffectAttempt{
		WorkID: work, Epoch: 1, Try: 1,
	}); err != nil {
		if journal.IsCode(err, "CONTROL_STOPPED") || journal.IsCode(err, "OPERATION_CANCELLED") {
			return runtimeFail("RUN_STOPPED", err)
		}
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	stored, err := s.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if stored.State == journal.Succeeded {
		return nil
	}
	if stored.State == journal.OperationalFailed {
		return runtimeFail("HOST_CHECK_REFUSAL_FAILED", nil)
	}
	if stored.State != journal.Pending && stored.State != journal.Claimed {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	claim, err := s.journal.ClaimOwned(
		ctx, owner, effectID, s.now().UTC(), effectLease)
	if err != nil {
		if journal.IsCode(err, "CONTROL_STOPPED") || journal.IsCode(err, "OPERATION_CANCELLED") {
			return runtimeFail("RUN_STOPPED", err)
		}
		return runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	return s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: engine.manifest.value.RunID, EffectID: effectID, Token: claim.Token,
		State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "check_refusal", Body: body}},
		EventKind: "host_check_refused",
		EventBody: MarshalAssociation(EventAssociation{
			EffectID: effectID,
			WorkID:   work,
			Slice:    sliceID,
		}),
		At: s.now().UTC(),
	})
}

// executeHostCheck claims and completes one check.host effect, re-running the
// exact approved command when no succeeded result is already journaled
// (exactly-once). The completion path uses context.WithoutCancel and never
// relies on the effect lease expiring: the run happens between claim and
// completion while the owner watch goroutine renews the owner lease, so a
// long-running host check cannot be stranded by a five-minute effect lease.
//
// One bounded exception to exactly-once (#296): when the recorded result is
// a plain failure with no deterministic signature and the same candidate is
// checked again - an implementer resubmitted an identical product tree
// because it judged the failure a flake - the check is executed once more
// under the work's re-execution identity, journaled as a re-execution that
// names the record it replaces. A second identical resubmission replays the
// re-execution's result; nothing runs a third time.
func (s *Service) executeHostCheck(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	sliceID, candidate, contractDigest, check string,
) (hostCheckResult, error) {
	work := hostCheckWork(sliceID, candidate, contractDigest, check)
	timeout := hostCheckTimeout(engine)
	command := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: sliceID,
		Candidate: candidate, ContractDigest: contractDigest,
		Check: check, OutputBytes: hostCheckOutputBytes,
		TimeoutMillis: int64(timeout / time.Millisecond),
	}
	effectID, boundWork := hostCheckEffectID(work), work
	effect, recorded, err := s.admitHostCheckEffect(ctx, engine, owner, boundWork, effectID, command)
	if err != nil {
		return hostCheckResult{}, err
	}
	if recorded != nil {
		result, err := parseHostCheckResult(sliceID, candidate, contractDigest, check, effectID, recorded)
		if err != nil {
			return hostCheckResult{}, err
		}
		if !hostCheckRerunEligible(result) {
			return result, nil
		}
		command.RerunOf = effectID
		effectID, boundWork = hostCheckRerunEffectID(work), hostCheckRerunWork(work)
		effect, recorded, err = s.admitHostCheckEffect(ctx, engine, owner, boundWork, effectID, command)
		if err != nil {
			return hostCheckResult{}, err
		}
		if recorded != nil {
			return parseHostCheckResult(sliceID, candidate, contractDigest, check, effectID, recorded)
		}
	}
	shell, shellErr := hostShell()
	if shellErr != nil {
		// S6-host-environment-park-projection A5(ii): fail closed with the
		// identical typed code validateHostCheckEnvironment (the run-start
		// gate) already uses for the same resolution failure, instead of
		// silently skipping classification and falling through to a plain
		// execution attempt. This is a synthetic in-memory error - nothing
		// is journaled for it - so it cannot spend this try or become
		// repair input; see implementSlice's and runAction's catch chains.
		return hostCheckResult{}, runtimeFail("HOST_SHELL_UNAVAILABLE", shellErr)
	}
	if environmentFailure, missingCommand := classifyHostCheckExecution(shell, check); environmentFailure {
		if journalErr := s.journalHostEnvironmentClassification(
			ctx, engine, owner, boundWork, effectID,
			sliceID, candidate, contractDigest, check, missingCommand,
		); journalErr != nil {
			return hostCheckResult{}, journalErr
		}
		return hostCheckResult{}, runtimeFail("EFFECT_PARKED", nil)
	}
	oid, err := gitx.ParseOID(engine.repository.ObjectFormat(), candidate)
	if err != nil {
		return hostCheckResult{}, runtimeFail("INVALID_CANDIDATE", err)
	}
	workspace, err := engine.workspaces.OpenSnapshot(oid)
	if err != nil {
		return hostCheckResult{}, runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	result := runHostCommand(workspace.Path(), check, hostCheckOutputBytes, timeout)
	closeErr := workspace.Close()
	if closeErr != nil {
		return hostCheckResult{}, runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	// Defense in depth: the resolved word's own interpreter or script can
	// still be missing (for example a shebang naming an absent
	// interpreter), which only exit 127 itself reveals. This discards the
	// just-produced result and parks exactly like the pre-spawn
	// classification above, regardless of what the pre-spawn check found.
	if result.ExitCode == 127 {
		if journalErr := s.journalHostEnvironmentClassification(
			ctx, engine, owner, boundWork, effectID,
			sliceID, candidate, contractDigest, check, hostCheckCommandWord(check),
		); journalErr != nil {
			return hostCheckResult{}, journalErr
		}
		return hostCheckResult{}, runtimeFail("EFFECT_PARKED", nil)
	}
	result.Slice, result.Candidate, result.ContractDigest = sliceID, candidate, contractDigest
	result.EffectID = effectID
	result.RerunOf = command.RerunOf
	body := mustJSON(result)
	if err := s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: engine.manifest.value.RunID, EffectID: effectID,
		Token: effect.CurrentClaim, State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "host_check_result", Body: body}},
		EventKind: hostCheckCompletedEvent(command.RerunOf),
		EventBody: MarshalAssociation(EventAssociation{
			EffectID: effectID,
			WorkID:   boundWork,
			Slice:    sliceID,
		}),
		At: s.now().UTC(),
	}); err != nil {
		return hostCheckResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return result, nil
}

// hostCheckCompletedEvent names the completion event for a check.host
// effect: a re-execution (#296) is journaled under its own kind so the
// board and the operator can tell a replaced record from first evidence.
func hostCheckCompletedEvent(rerunOf string) string {
	if rerunOf != "" {
		return "host_check_rerun_completed"
	}
	return "host_check_completed"
}

// admitHostCheckEffect ensures the single attempt of one check.host effect
// bound to work and returns it claimed by this owner when it still has to
// run, or its recorded result when it already succeeded. A recorded
// operational failure of the effect itself refuses HOST_CHECK_FAILED, and
// any other state is left to recovery.
func (s *Service) admitHostCheckEffect(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	work, effectID string,
	command hostCheckCommand,
) (journal.Effect, []byte, error) {
	payload := mustJSON(command)
	now := s.now().UTC()
	if err := s.journal.EnsureAttempt(ctx,
		journal.Command{RunID: engine.manifest.value.RunID, ReplayKey: effectID,
			Kind: "check.host", Payload: payload, CreatedAt: now},
		journal.Effect{RunID: engine.manifest.value.RunID, ID: effectID,
			ReplayKey: effectID, Kind: "check.host", BeforeDigest: work,
			ExpectedDigest: sha256Digest(payload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); err != nil {
		if journal.IsCode(err, "CONTROL_STOPPED") || journal.IsCode(err, "OPERATION_CANCELLED") {
			return journal.Effect{}, nil, runtimeFail("RUN_STOPPED", err)
		}
		return journal.Effect{}, nil, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	effect, err := s.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
	if err != nil {
		return journal.Effect{}, nil, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	switch effect.State {
	case journal.Succeeded:
		return effect, effect.Result, nil
	case journal.OperationalFailed:
		return journal.Effect{}, nil, runtimeFail("HOST_CHECK_FAILED", nil)
	case journal.Pending:
		claim, err := s.journal.ClaimOwned(
			ctx, owner, effectID, s.now().UTC(), effectLease)
		if err != nil {
			if journal.IsCode(err, "CONTROL_STOPPED") || journal.IsCode(err, "OPERATION_CANCELLED") {
				return journal.Effect{}, nil, runtimeFail("RUN_STOPPED", err)
			}
			return journal.Effect{}, nil, runtimeFail("EFFECT_CLAIM_FAILED", err)
		}
		effect.State, effect.CurrentClaim = journal.Claimed, claim.Token
	case journal.Claimed:
		// A claimed effect left by a crashed prior attempt is re-run and
		// completed by this owner; see recoverHostCheckClaims.
	default:
		return journal.Effect{}, nil, runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	return effect, nil, nil
}

func parseHostCheckResult(
	sliceID, candidate, contractDigest, check, effectID string,
	body []byte,
) (hostCheckResult, error) {
	var result hostCheckResult
	if json.Unmarshal(body, &result) != nil ||
		!bytesEqualCanonicalJSON(body, result) ||
		result.Slice != sliceID || result.Candidate != candidate ||
		result.ContractDigest != contractDigest || result.Check != check ||
		result.EffectID != effectID || result.OutputDigest == "" {
		return hostCheckResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if protocol.DigestBytes([]byte(result.Output)) != result.OutputDigest {
		return hostCheckResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return result, nil
}

// isLongSuiteHostCheck classifies a declared check by a fixed byte predicate
// (S1-seal-time-gates A1): a check is a long process suite iff its command
// text invokes "go test" - the product suite, the serial end-to-end suite
// and the race suite, on this and every contract this release declares.
// Every other declared check (vet, the gofmt assertion, module tidiness, the
// diff check, the Darwin build) is quick. This inspects only the approved
// command string, never check identity or effect state, so it cannot
// diverge from what resolveSliceHostChecks already resolved.
func isLongSuiteHostCheck(check string) bool {
	return strings.Contains(check, "go test")
}

// phaseOrderedHostChecks partitions hostChecks into quick checks followed by
// long-suite checks, each group keeping its declared relative order. A1
// requires the quick checks to run first so a failing one blocks every long
// suite for that candidate; this reorders only iteration, never check
// identity, so hostCheckWork/EnsureAttempt reuse is unaffected.
func phaseOrderedHostChecks(hostChecks []string) []string {
	ordered := make([]string, 0, len(hostChecks))
	for _, check := range hostChecks {
		if !isLongSuiteHostCheck(check) {
			ordered = append(ordered, check)
		}
	}
	for _, check := range hostChecks {
		if isLongSuiteHostCheck(check) {
			ordered = append(ordered, check)
		}
	}
	return ordered
}

// runHostChecks executes every declared host check for the slice against the
// exact candidate and returns their journaled results, quick checks first
// (A1). A failed, timed-out, or overflowed host check returns an error so
// the caller blocks the seal; it is never a pass and never absent.
func (s *Service) runHostChecks(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	plan protocol.Plan,
	sliceID, candidate, targetHead, releaseHead string,
) ([]hostCheckResult, error) {
	hostChecks, _, err := resolveSliceHostChecks(engine, plan, sliceID, targetHead, releaseHead)
	if err != nil {
		return nil, err
	}
	// A1: the quick declared checks run against the exact sealed candidate
	// before any long process suite. A quick-check failure returns
	// immediately below, before phaseOrderedHostChecks's long-suite tail is
	// ever reached, so no long-suite check.host effect is journaled for a
	// candidate that never clears the quick checks.
	hostChecks = phaseOrderedHostChecks(hostChecks)
	results := make([]hostCheckResult, 0, len(hostChecks))
	for _, check := range hostChecks {
		// A2 (S2-pause-safe-host-checks): observed fresh before admitting
		// each check, never cached, so a pause or cancel that lands between
		// two checks stops here - leaving every already-recorded result and
		// the try intact - instead of admitting one more check.host effect.
		if ctx.Err() != nil {
			return nil, runtimeFail("RUN_STOPPED", ctx.Err())
		}
		projection, err := s.journal.ControlProjection(ctx, owner.RunID)
		if err != nil {
			if journal.IsCode(err, "OPERATION_CANCELLED") {
				return nil, runtimeFail("RUN_STOPPED", err)
			}
			return nil, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		if projection.Desired != "running" {
			return nil, runtimeFail("RUN_STOPPED", nil)
		}
		result, err := s.runOneHostCheck(ctx, engine, owner, plan, sliceID, candidate, targetHead, releaseHead, check)
		if err != nil {
			return nil, err
		}
		if result.Outcome != protocol.CheckOutcomePass {
			return nil, &hostCheckFailure{result: result, err: runtimeFail(
				"HOST_CHECK_FAILED",
				fmt.Errorf("%s recorded %s: %s", check, result.Outcome, result.Diagnostic),
			)}
		}
		results = append(results, result)
	}
	return results, nil
}

// buildHostCheckResultsManifest constructs the engine-built sworn.check-results/v1
// manifest that becomes the receipt Checks bytes for a host-check slice: one
// host_boundary entry per journaled host check plus one role entry referencing
// the role's exact submitted check bytes by digest. The manifest carries only
// bounded output excerpts and digests, so it stays within the evidence cap.
func buildHostCheckResultsManifest(
	release, sliceID string,
	attempt int64,
	candidate, contractDigest string,
	hostResults []hostCheckResult,
	roleDigest string,
) ([]byte, error) {
	entries := hostCheckManifestEntries(hostResults)
	entries = append(entries, protocol.CheckResultEntry{
		Check: "role checks", Provenance: protocol.CheckProvenanceRole,
		Outcome: protocol.CheckOutcomePass, RoleDigest: roleDigest,
	})
	manifest := protocol.CheckResults{
		SchemaVersion: protocol.CheckResultsVersion, Release: release,
		Slice: sliceID, Attempt: attempt, Candidate: candidate,
		ContractDigest: contractDigest, Entries: entries,
	}
	return protocol.EncodeCheckResults(manifest)
}

// buildAssemblyHostCheckResultsManifest constructs the sworn.check-results/v1
// manifest that becomes the assembly candidate receipt's Checks bytes
// (sworn#343): one host_boundary entry per journaled (or reused) host check
// of the assembled tree, bound to the empty slice, the plan revision as the
// attempt, the assembly candidate and the assembly's union contract digest.
// An assembly is prepared by the engine, not by a role, so unlike the slice
// manifest it carries no role entry.
func buildAssemblyHostCheckResultsManifest(
	release string,
	attempt int64,
	candidate, contractDigest string,
	hostResults []hostCheckResult,
) ([]byte, error) {
	manifest := protocol.CheckResults{
		SchemaVersion: protocol.CheckResultsVersion, Release: release,
		Slice: "", Attempt: attempt, Candidate: candidate,
		ContractDigest: contractDigest,
		Entries:        hostCheckManifestEntries(hostResults),
	}
	return protocol.EncodeCheckResults(manifest)
}

// hostCheckManifestEntries renders journaled host results as host_boundary
// manifest entries, carrying only bounded output excerpts and digests.
func hostCheckManifestEntries(hostResults []hostCheckResult) []protocol.CheckResultEntry {
	entries := make([]protocol.CheckResultEntry, 0, len(hostResults)+1)
	for _, result := range hostResults {
		entry := protocol.CheckResultEntry{
			Check: result.Check, Provenance: protocol.CheckProvenanceHost,
			Outcome: result.Outcome, OutputDigest: result.OutputDigest,
			Diagnostic: result.Diagnostic, HostEffect: result.EffectID,
		}
		exitCode := result.ExitCode
		entry.ExitCode = &exitCode
		entry.Output, entry.Truncated = hostOutputExcerpt(
			result.Output, result.Truncated)
		entries = append(entries, entry)
	}
	return entries
}

// validateHostCheckEvidenceProof proves that a sealed record's checks evidence
// is the engine-built host-boundary manifest for this exact cycle, rebuilt
// from the journal rather than trusted from the record. Every host entry must
// cite a succeeded check.host effect whose identity is content-addressed from
// the slice, candidate, contract digest, and check text, and whose journaled
// result parses with the same exactness as at execution; the role entry must
// carry the digest of the dispatch's own checks. The manifest rebuilt from
// those journaled results must equal the record's bytes exactly, so neither
// an outcome, an output, an ordering, nor an extra entry can be asserted by
// the record alone. The manifest's attempt number is the one field with no
// journal witness here; it is carried through the rebuild unchanged.
func validateHostCheckEvidenceProof(
	snapshot journal.Snapshot,
	cycle implementationCycle,
	record sealedRecord,
	roleChecks []byte,
) error {
	manifest, err := protocol.ParseCheckResults(record.Receipt.CheckResults)
	if err != nil ||
		manifest.Release != record.Receipt.Release ||
		manifest.Slice != cycle.Slice ||
		manifest.Candidate != record.Candidate {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects[effect.ID] = effect
	}
	hostResults := make([]hostCheckResult, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if entry.Provenance != protocol.CheckProvenanceHost {
			continue
		}
		work := hostCheckWork(
			cycle.Slice, record.Candidate, manifest.ContractDigest, entry.Check)
		effectID := entry.HostEffect
		boundWork, bound := hostCheckBoundWork(effectID, work)
		effect, found := effects[effectID]
		if !found || !bound ||
			effect.Kind != "check.host" ||
			effect.State != journal.Succeeded ||
			effect.BeforeDigest != boundWork ||
			effect.ResultDigest != sha256Digest(effect.Result) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		result, parseErr := parseHostCheckResult(
			cycle.Slice, record.Candidate, manifest.ContractDigest,
			entry.Check, effectID, effect.Result)
		if parseErr != nil {
			return parseErr
		}
		if result.Outcome != protocol.CheckOutcomePass {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		hostResults = append(hostResults, result)
	}
	if len(hostResults) == 0 {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	expected, buildErr := buildHostCheckResultsManifest(
		record.Receipt.Release, cycle.Slice, manifest.Attempt,
		record.Candidate, manifest.ContractDigest, hostResults,
		protocol.DigestBytes(roleChecks))
	if buildErr != nil || !bytes.Equal(expected, record.Receipt.CheckResults) {
		return runtimeFail("CORRUPT_JOURNAL", buildErr)
	}
	return nil
}

// hostOutputExcerpt embeds at most HostCheckOutputManifestBytes of a host
// check's bounded output into a manifest entry, returning the excerpt and a
// truthful truncated flag. Whenever the embedded bytes are not the full
// bounded output, the entry is marked truncated and the marker is present, so
// a reader can never mistake an excerpt for the full output. A full output
// that fits keeps the invariant that its digest matches the entry's
// output_digest exactly.
func hostOutputExcerpt(output string, outputTruncated bool) (string, bool) {
	if output == "" {
		return "", false
	}
	limit := protocol.HostCheckOutputManifestBytes
	if len(output) <= limit {
		return output, outputTruncated
	}
	excerpt := output[:limit] + fmt.Sprintf("\n[sworn: output truncated at %d bytes]\n", len(output))
	return excerpt, true
}

// hostCheckEnvironmentUnresolvedCheck names one declared check (from the
// deduplicated union of every slice's approved checks and host_checks)
// whose first word does not currently resolve on the host runner.
type hostCheckEnvironmentUnresolvedCheck struct {
	Check   string
	Missing string
}

// validateHostCheckEnvironment resolves, for every track and slice in the
// plan's own order, the deduplicated union of the approved contract's
// checks and host_checks (A4's literal-text scope: a Checks-only entry
// actually runs inside a contained dispatch's own sandboxed environment,
// so this host-side check is a conservative superset for that subset, not
// a narrowing), classifies each with the exact shared rule
// classifyHostCheckExecution uses, and returns every currently-unresolved
// one - a check text declared by more than one slice is reported once.
func validateHostCheckEnvironment(
	engine *engine,
	plan protocol.Plan,
	state protocol.State,
) ([]hostCheckEnvironmentUnresolvedCheck, error) {
	shell, err := hostShell()
	if err != nil {
		return nil, runtimeFail("HOST_SHELL_UNAVAILABLE", err)
	}
	seen := make(map[string]struct{})
	var unresolved []hostCheckEnvironmentUnresolvedCheck
	for _, track := range state.Tracks {
		for _, slice := range track.Slices {
			sliceID := slice.Location.Slice.ID
			contract, err := plan.ResolveSliceContractAtHead(
				engine.git, sliceID, state.Refs.Release.Head, state.Refs.Target.Head)
			if err != nil {
				return nil, runtimeFail("CONTRACT_RESOLUTION_FAILED", err)
			}
			union := make([]string, 0, len(contract.Checks)+len(contract.HostChecks))
			union = append(union, contract.Checks...)
			union = append(union, contract.HostChecks...)
			for _, check := range union {
				if _, duplicate := seen[check]; duplicate {
					continue
				}
				seen[check] = struct{}{}
				if environmentFailure, missing := classifyHostCheckExecution(shell, check); environmentFailure {
					unresolved = append(unresolved, hostCheckEnvironmentUnresolvedCheck{
						Check: check, Missing: missing,
					})
				}
			}
		}
	}
	return unresolved, nil
}

// hostCheckEnvironmentDetail renders A4's unresolved list as one bounded
// park detail, "<check>: command not found: <missing>" per entry,
// truncating trailing entries rather than emitting a detail
// validParkDetail would reject - the same discipline scopeExhaustionDetail
// already uses for its own multi-entry detail.
func hostCheckEnvironmentDetail(unresolved []hostCheckEnvironmentUnresolvedCheck) string {
	lines := make([]string, 0, len(unresolved))
	for _, item := range unresolved {
		lines = append(lines, item.Check+": command not found: "+item.Missing)
	}
	detail := strings.Join(lines, "; ")
	for len(lines) > 0 && !validParkDetail(detail) {
		lines = lines[:len(lines)-1]
		detail = strings.Join(lines, "; ")
	}
	if !validParkDetail(detail) {
		return ""
	}
	return detail
}

// hostCheckEnvironmentRunState is the pure journal projection of the A4
// run-scoped host-environment park: among every ParkEventKind park event
// with Cause==ParkCauseHostEnvironment && Work=="" and every
// HostEnvironmentResolvedEventKind event in snapshot, the one with the
// highest event.Offset - an already-monotonic, journal-assigned ordering
// element, so no synthetic generation counter is needed - names the
// current truth. Absent (neither kind ever journaled) reads as not
// parked. Status reads this exact function; it is never re-derived by
// re-running the classifier.
func hostCheckEnvironmentRunState(snapshot journal.Snapshot) (parked bool, detail string) {
	latestOffset := int64(-1)
	for _, event := range snapshot.Events {
		switch event.Kind {
		case ParkEventKind:
			parsed, err := ParseDegradationParkEvent(event.Body)
			if err != nil || parsed.Cause != ParkCauseHostEnvironment || parsed.Work != "" {
				continue
			}
			if event.Offset > latestOffset {
				latestOffset, parked, detail = event.Offset, true, parsed.FailureDetail
			}
		case HostEnvironmentResolvedEventKind:
			if event.Offset > latestOffset {
				latestOffset, parked, detail = event.Offset, false, ""
			}
		}
	}
	return parked, detail
}

// driveHostCheckEnvironmentGate is A4's write side, run once at the top of
// every driveLoop entry. It re-validates fresh (never trusting a stored
// flag) and writes a new journal record only on a genuine transition from
// the latest already-journaled record (hostCheckEnvironmentRunState),
// using plain, non-deduplicated journal.Store.AppendEvent rather than the
// content-addressed appendParkEventOnce: content-addressing (hash of
// cause+body) would silently absorb a second, later occurrence of the
// identical unresolved set as a duplicate of the first after an
// intervening fix, which is exactly the ordering defect a break-fix-break
// cycle must not hit. The resolved transition is journaled under a
// distinct, non-park event kind (HostEnvironmentResolvedEventKind), never
// as a ParkEventKind event, so it can never cross a park_updated webhook
// or read as a park in cockpit history.
func (s *Service) driveHostCheckEnvironmentGate(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	snapshot journal.Snapshot,
	state protocol.State,
) (bool, error) {
	plan, err := planFromState(state)
	if err != nil {
		return false, err
	}
	unresolved, err := validateHostCheckEnvironment(engine, plan, state)
	if err != nil {
		return false, err
	}
	wasParked, priorDetail := hostCheckEnvironmentRunState(snapshot)
	if len(unresolved) != 0 {
		detail := hostCheckEnvironmentDetail(unresolved)
		if !wasParked || priorDetail != detail {
			body, err := hostEnvironmentParkEventBody(owner.RunID, "", detail)
			if err != nil {
				return false, err
			}
			if err := s.journal.AppendEvent(
				ctx, owner.RunID, ParkEventKind, body, s.now().UTC(),
			); err != nil {
				return false, runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
		}
		return true, nil
	}
	if wasParked {
		body, err := canonicalHostEnvironmentResolvedEvent(owner.RunID)
		if err != nil {
			return false, err
		}
		if err := s.journal.AppendEvent(
			ctx, owner.RunID, HostEnvironmentResolvedEventKind, body, s.now().UTC(),
		); err != nil {
			return false, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
	}
	return false, nil
}

// recoverHostCheckClaims reconciles in-flight check.host and check.refused
// effects after a crash. These are engine-owned, re-runnable effects: a
// claimed host check is re-run against the exact candidate and completed (or
// fails closed if its bound envelope is incomplete), and a claimed refusal is
// completed with its deterministic payload. Without this sweep a claimed
// host-check effect could strand the seal forever.
func (s *Service) recoverHostCheckClaims(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
) (bool, error) {
	if engine == nil || s == nil {
		return false, runtimeFail("INVALID_ENGINE", nil)
	}
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		switch effect.Kind {
		case "check.host", "check.refused":
		default:
			continue
		}
		if effect.State != journal.Pending && effect.State != journal.Claimed {
			continue
		}
		command, ok := commands[effect.ReplayKey]
		if !ok || command.Kind != effect.Kind ||
			command.RunID != effect.RunID ||
			command.ReplayKey != effect.ReplayKey {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if effect.ExpectedDigest != sha256Digest(command.Payload) {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if effect.State == journal.Pending {
			claim, err := s.journal.ClaimOwned(
				ctx, owner, effect.ID, s.now().UTC(), effectLease)
			if err != nil {
				return true, runtimeFail("EFFECT_CLAIM_FAILED", err)
			}
			effect.State = journal.Claimed
			effect.CurrentClaim = claim.Token
		}
		switch effect.Kind {
		case "check.refused":
			var refusal hostCheckRefusal
			if json.Unmarshal(command.Payload, &refusal) != nil ||
				!bytesEqualCanonicalJSON(command.Payload, refusal) ||
				effect.BeforeDigest != hostCheckRefusalWork(
					refusal.Slice, refusal.Candidate,
					refusal.Check, refusal.Reason) {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if err := s.journal.CompleteOwned(
				context.WithoutCancel(ctx), owner, journal.Completion{
					RunID: owner.RunID, EffectID: effect.ID,
					Token: effect.CurrentClaim, State: journal.Succeeded,
					Result: command.Payload,
					Receipts: []journal.Receipt{{
						Kind: "check_refusal", Body: command.Payload,
					}},
					EventKind: "host_check_refused",
					EventBody: MarshalAssociation(EventAssociation{
						EffectID: effect.ID,
						WorkID: hostCheckRefusalWork(
							refusal.Slice, refusal.Candidate,
							refusal.Check, refusal.Reason),
						Slice: refusal.Slice,
					}), At: s.now().UTC(),
				}); err != nil {
				return true, runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
			return true, nil
		case "check.host":
			var commandValue hostCheckCommand
			if json.Unmarshal(command.Payload, &commandValue) != nil ||
				!bytesEqualCanonicalJSON(command.Payload, commandValue) ||
				commandValue.SchemaVersion != hostCheckSchemaVersion {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			boundWork := hostCheckWork(
				commandValue.Slice, commandValue.Candidate,
				commandValue.ContractDigest, commandValue.Check)
			if commandValue.RerunOf != "" {
				if commandValue.RerunOf != hostCheckEffectID(boundWork) {
					return true, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				boundWork = hostCheckRerunWork(boundWork)
			}
			if effect.BeforeDigest != boundWork {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			// Classify before re-running, exactly as executeHostCheck
			// does on a fresh claim, so a crash-recovered claim classifies
			// exactly as a live one. An environment failure is handled,
			// not recovered: journal the durable fact and skip this
			// effect, continuing the scan for any other effect that
			// genuinely needs recovering, so this function never returns
			// true (and drives an unbounded rescan) for a claim that is
			// staying Claimed on purpose.
			shell, shellErr := hostShell()
			if shellErr != nil {
				// S6-host-environment-park-projection A5(ii): fail closed
				// with the identical typed code the fresh-claim path and
				// the run-start gate both use, instead of silently
				// skipping classification and falling through to
				// executeHostCheckFromRecovery. This effect stays exactly
				// Claimed - nothing here completes it - so returning this
				// error cannot spend a try or become repair input.
				return true, runtimeFail("HOST_SHELL_UNAVAILABLE", shellErr)
			}
			if environmentFailure, missingCommand := classifyHostCheckExecution(
				shell, commandValue.Check,
			); environmentFailure {
				if journalErr := s.journalHostEnvironmentClassification(
					ctx, engine, owner, boundWork, effect.ID,
					commandValue.Slice, commandValue.Candidate,
					commandValue.ContractDigest, commandValue.Check, missingCommand,
				); journalErr != nil {
					return true, journalErr
				}
				continue
			}
			result, runErr := s.executeHostCheckFromRecovery(
				ctx, engine, owner, effect, commandValue)
			if runErr != nil {
				if IsCode(runErr, "EFFECT_PARKED") {
					continue
				}
				return true, runErr
			}
			_ = result
			return true, nil
		default:
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	return false, nil
}

// executeHostCheckFromRecovery re-runs one claimed host check from its
// journaled command and completes it, so a crash between claim and completion
// never strands the exact candidate's host evidence.
func (s *Service) executeHostCheckFromRecovery(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	effect journal.Effect,
	command hostCheckCommand,
) (hostCheckResult, error) {
	oid, err := gitx.ParseOID(engine.repository.ObjectFormat(), command.Candidate)
	if err != nil {
		return hostCheckResult{}, runtimeFail("INVALID_CANDIDATE", err)
	}
	workspace, err := engine.workspaces.OpenSnapshot(oid)
	if err != nil {
		return hostCheckResult{}, runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	timeout := time.Duration(command.TimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = hostCheckTimeout(engine)
	}
	result := runHostCommand(workspace.Path(), command.Check, command.OutputBytes, timeout)
	closeErr := workspace.Close()
	if closeErr != nil {
		return hostCheckResult{}, runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	// Defense in depth, mirroring executeHostCheck's own post-127 check:
	// recoverHostCheckClaims already pre-classifies before calling this
	// function, but the resolved word's own interpreter or script can
	// still be missing only exit 127 itself reveals.
	if result.ExitCode == 127 {
		if journalErr := s.journalHostEnvironmentClassification(
			ctx, engine, owner, effect.BeforeDigest, effect.ID,
			command.Slice, command.Candidate, command.ContractDigest,
			command.Check, hostCheckCommandWord(command.Check),
		); journalErr != nil {
			return hostCheckResult{}, journalErr
		}
		return hostCheckResult{}, runtimeFail("EFFECT_PARKED", nil)
	}
	result.Slice, result.Candidate, result.ContractDigest =
		command.Slice, command.Candidate, command.ContractDigest
	result.EffectID = effect.ID
	result.RerunOf = command.RerunOf
	body := mustJSON(result)
	if err := s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: owner.RunID, EffectID: effect.ID,
		Token: effect.CurrentClaim, State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "host_check_result", Body: body}},
		EventKind: hostCheckCompletedEvent(command.RerunOf),
		EventBody: MarshalAssociation(EventAssociation{
			EffectID: effect.ID,
			WorkID:   effect.BeforeDigest,
			Slice:    command.Slice,
		}),
		At: s.now().UTC(),
	}); err != nil {
		return hostCheckResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return result, nil
}
