package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
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

func hostCheckRefusalWork(sliceID, candidate, check, reason string) string {
	return workIdentity("check.refused", sliceID, candidate, check, reason)
}

func hostCheckEffectID(work string) string {
	return journal.AttemptEffectID(work, 1, 1)
}

// resolveSliceHostChecks resolves the human-approved contract for sliceID at
// the exact captured target head and returns its declared host_checks and
// contract digest. Any divergence from the admitted plan fails closed, so the
// host runner can only ever execute commands that the approved contract
// declared.
func resolveSliceHostChecks(
	engine *engine,
	plan baton.Plan,
	sliceID, targetHead string,
) ([]string, string, error) {
	if engine == nil {
		return nil, "", runtimeFail("INVALID_ENGINE", nil)
	}
	contract, err := plan.ResolveSliceContractAt(
		engine.git,
		sliceID,
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
// configuration or discovery (SWORN_SH override, else /bin/sh, else
// LookPath("sh")) so a minimal or non-Debian host works without patching.
func hostShell() string {
	path, err := gitx.ResolveShellExecutable()
	if err != nil {
		return "/bin/sh"
	}
	return path
}

// runHostCommand executes one approved check command via the fixed
// sh -c surface in a defined bounded environment rooted at dir. Output
// is captured into a bounded buffer with a truthful truncation marker; the
// process group is killed when the timeout expires; a timeout or overflow is
// recorded as such, never as a pass and never as absent.
func runHostCommand(dir, check string, outputBytes int64, timeout time.Duration) hostCheckResult {
	result := hostCheckResult{
		Check:      check,
		Outcome:    baton.CheckOutcomeFail,
		ExitCode:   -1,
		Diagnostic: "command did not start",
	}
	shell := hostShell()
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
			result.Outcome = baton.CheckOutcomePass
			result.Diagnostic = ""
		} else if result.ExitCode == -1 {
			result.Outcome = baton.CheckOutcomeFail
			result.Diagnostic = "host command terminated abnormally"
		} else {
			result.Outcome = baton.CheckOutcomeFail
			result.Diagnostic = fmt.Sprintf("exit code %d", result.ExitCode)
		}
	case <-time.After(timeout):
		_ = syscall.Kill(-group, syscall.SIGKILL)
		<-done
		result.Outcome = baton.CheckOutcomeTimeout
		result.ExitCode = -1
		result.Diagnostic = fmt.Sprintf("host check exceeded %s", timeout)
	}
	_ = syscall.Kill(-group, syscall.SIGCONT) // reap any stopped children
	result.Output = output.String()
	result.Truncated = output.overflow
	// A run bounded by the timeout is recorded as timeout even when it also
	// produced more output than the cap; otherwise overflow of the bounded
	// buffer is recorded as overflow with the truthful marker.
	if result.Outcome != baton.CheckOutcomeTimeout && result.Truncated {
		marker := fmt.Sprintf("\n[sworn: output truncated at %d bytes]\n", output.limit)
		result.Output += marker
		result.Outcome = baton.CheckOutcomeOverflow
		result.Diagnostic = fmt.Sprintf("output exceeded %d bytes", output.limit)
	}
	result.OutputDigest = baton.DigestBytes([]byte(result.Output))
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
	plan baton.Plan,
	sliceID, candidate, targetHead, check string,
) (hostCheckResult, error) {
	hostChecks, contractDigest, err := resolveSliceHostChecks(engine, plan, sliceID, targetHead)
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
		return runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	return s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: engine.manifest.value.RunID, EffectID: effectID, Token: claim.Token,
		State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "check_refusal", Body: body}},
		EventKind: "host_check_refused", EventBody: []byte(sliceID + "\x00" + check),
		At: s.now().UTC(),
	})
}

// executeHostCheck claims and completes one check.host effect, re-running the
// exact approved command when no succeeded result is already journaled
// (exactly-once). The completion path uses context.WithoutCancel and never
// relies on the effect lease expiring: the run happens between claim and
// completion while the owner watch goroutine renews the owner lease, so a
// long-running host check cannot be stranded by a five-minute effect lease.
func (s *Service) executeHostCheck(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	sliceID, candidate, contractDigest, check string,
) (hostCheckResult, error) {
	work := hostCheckWork(sliceID, candidate, contractDigest, check)
	effectID := hostCheckEffectID(work)
	timeout := hostCheckTimeout(engine)
	command := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: sliceID,
		Candidate: candidate, ContractDigest: contractDigest,
		Check: check, OutputBytes: hostCheckOutputBytes,
		TimeoutMillis: int64(timeout / time.Millisecond),
	}
	payload := mustJSON(command)
	now := s.now().UTC()
	if err := s.journal.EnsureAttempt(ctx,
		journal.Command{RunID: engine.manifest.value.RunID, ReplayKey: effectID,
			Kind: "check.host", Payload: payload, CreatedAt: now},
		journal.Effect{RunID: engine.manifest.value.RunID, ID: effectID,
			ReplayKey: effectID, Kind: "check.host", BeforeDigest: work,
			ExpectedDigest: sha256Digest(payload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); err != nil {
		return hostCheckResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	effect, err := s.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
	if err != nil {
		return hostCheckResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	switch effect.State {
	case journal.Succeeded:
		return parseHostCheckResult(sliceID, candidate, contractDigest, check, effectID, effect.Result)
	case journal.OperationalFailed:
		return hostCheckResult{}, runtimeFail("HOST_CHECK_FAILED", nil)
	case journal.Pending:
		claim, err := s.journal.ClaimOwned(
			ctx, owner, effectID, s.now().UTC(), effectLease)
		if err != nil {
			return hostCheckResult{}, runtimeFail("EFFECT_CLAIM_FAILED", err)
		}
		effect.State, effect.CurrentClaim = journal.Claimed, claim.Token
	case journal.Claimed:
		// A claimed effect left by a crashed prior attempt is re-run and
		// completed by this owner; see recoverHostCheckClaims.
	default:
		return hostCheckResult{}, runtimeFail("RECOVERY_UNCERTAIN", nil)
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
	result.Slice, result.Candidate, result.ContractDigest = sliceID, candidate, contractDigest
	result.EffectID = effectID
	body := mustJSON(result)
	if err := s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: engine.manifest.value.RunID, EffectID: effectID,
		Token: effect.CurrentClaim, State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "host_check_result", Body: body}},
		EventKind: "host_check_completed", EventBody: []byte(sliceID + "\x00" + check + "\x00" + result.Outcome),
		At: s.now().UTC(),
	}); err != nil {
		return hostCheckResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return result, nil
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
	if baton.DigestBytes([]byte(result.Output)) != result.OutputDigest {
		return hostCheckResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return result, nil
}

// runHostChecks executes every declared host check for the slice against the
// exact candidate and returns their journaled results in declaration order. A
// failed, timed-out, or overflowed host check returns an error so the caller
// blocks the seal; it is never a pass and never absent.
func (s *Service) runHostChecks(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	plan baton.Plan,
	sliceID, candidate, targetHead string,
) ([]hostCheckResult, error) {
	hostChecks, _, err := resolveSliceHostChecks(engine, plan, sliceID, targetHead)
	if err != nil {
		return nil, err
	}
	results := make([]hostCheckResult, 0, len(hostChecks))
	for _, check := range hostChecks {
		result, err := s.runOneHostCheck(ctx, engine, owner, plan, sliceID, candidate, targetHead, check)
		if err != nil {
			return nil, err
		}
		if result.Outcome != baton.CheckOutcomePass {
			return nil, runtimeFail(
				"HOST_CHECK_FAILED",
				fmt.Errorf("%s recorded %s: %s", check, result.Outcome, result.Diagnostic),
			)
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
	entries := make([]baton.CheckResultEntry, 0, len(hostResults)+1)
	for _, result := range hostResults {
		entry := baton.CheckResultEntry{
			Check: result.Check, Provenance: baton.CheckProvenanceHost,
			Outcome: result.Outcome, OutputDigest: result.OutputDigest,
			Diagnostic: result.Diagnostic, HostEffect: result.EffectID,
		}
		exitCode := result.ExitCode
		entry.ExitCode = &exitCode
		entry.Output, entry.Truncated = hostOutputExcerpt(
			result.Output, result.Truncated)
		entries = append(entries, entry)
	}
	entries = append(entries, baton.CheckResultEntry{
		Check: "role checks", Provenance: baton.CheckProvenanceRole,
		Outcome: baton.CheckOutcomePass, RoleDigest: roleDigest,
	})
	manifest := baton.CheckResults{
		SchemaVersion: baton.CheckResultsVersion, Release: release,
		Slice: sliceID, Attempt: attempt, Candidate: candidate,
		ContractDigest: contractDigest, Entries: entries,
	}
	return baton.EncodeCheckResults(manifest)
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
	limit := baton.HostCheckOutputManifestBytes
	if len(output) <= limit {
		return output, outputTruncated
	}
	excerpt := output[:limit] + fmt.Sprintf("\n[sworn: output truncated at %d bytes]\n", len(output))
	return excerpt, true
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
					EventBody: []byte(effect.ID), At: s.now().UTC(),
				}); err != nil {
				return true, runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
			return true, nil
		case "check.host":
			var commandValue hostCheckCommand
			if json.Unmarshal(command.Payload, &commandValue) != nil ||
				!bytesEqualCanonicalJSON(command.Payload, commandValue) ||
				commandValue.SchemaVersion != hostCheckSchemaVersion ||
				effect.BeforeDigest != hostCheckWork(
					commandValue.Slice, commandValue.Candidate,
					commandValue.ContractDigest, commandValue.Check) {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			result, runErr := s.executeHostCheckFromRecovery(
				ctx, engine, owner, effect, commandValue)
			if runErr != nil {
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
	result.Slice, result.Candidate, result.ContractDigest =
		command.Slice, command.Candidate, command.ContractDigest
	result.EffectID = effect.ID
	body := mustJSON(result)
	if err := s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: owner.RunID, EffectID: effect.ID,
		Token: effect.CurrentClaim, State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "host_check_result", Body: body}},
		EventKind: "host_check_completed",
		EventBody: []byte(command.Slice + "\x00" + command.Check + "\x00" + result.Outcome),
		At:        s.now().UTC(),
	}); err != nil {
		return hostCheckResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return result, nil
}
