package gitx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	MaxRefProtocolBytes = 512 * 1024
	RefTransactionLimit = 10 * time.Second
)

type RefState string

const (
	RefDirect     RefState = "direct-commit"
	RefAbsent     RefState = "absent"
	RefSymbolic   RefState = "symbolic"
	RefBroken     RefState = "broken"
	RefNonCommit  RefState = "non-commit"
	RefUnreadable RefState = "unreadable"
)

type RefHead struct {
	Ref    string
	State  RefState
	Head   OID
	Target string
}

func trustworthyStatus(value commandOutcome) bool {
	return value.started && value.reaped && value.groupQuiet && !value.timedOut && !value.overflow && value.exitCode >= 0 && value.signal == "" && value.cleanupErr == nil
}
func unreadableRefs(refs []string) []RefHead {
	result := make([]RefHead, len(refs))
	for index, ref := range refs {
		result[index] = RefHead{Ref: ref, State: RefUnreadable}
	}
	return result
}
func exactRefs(refs []string) ([]string, error) {
	if len(refs) == 0 || len(refs) > MaxHeadRefs {
		return nil, fail("RESOURCE_LIMIT", "capture refs", fmt.Errorf("requires 1-%d refs", MaxHeadRefs))
	}
	result := append([]string(nil), refs...)
	sort.Strings(result)
	for index, ref := range result {
		if err := ValidateHeadRef(ref); err != nil {
			return nil, err
		}
		if index > 0 && ref == result[index-1] {
			return nil, fail("DUPLICATE_REF", "capture refs", fmt.Errorf("%s is repeated", ref))
		}
	}
	return result, nil
}
func validSymbolicTarget(value string) bool {
	return value != "" && utf8.ValidString(value) && strings.HasPrefix(value, "refs/") &&
		!strings.ContainsFunc(value, func(character rune) bool { return character < 0x20 || character == 0x7f })
}
func (r *Repository) probeOmittedRef(ctx context.Context, home string, group int, ref string) RefHead {
	unreadable := RefHead{Ref: ref, State: RefUnreadable}
	symbolic := r.runStatus(ctx, home, group, MaxRefProtocolBytes,
		"symbolic-ref", "--quiet", "--no-recurse", ref)
	if !trustworthyStatus(symbolic) {
		return unreadable
	}
	if symbolic.exitCode == 0 {
		target := strings.TrimSuffix(string(symbolic.stdout), "\n")
		if len(symbolic.stderr) != 0 || !bytes.HasSuffix(symbolic.stdout, []byte{'\n'}) ||
			bytes.Count(symbolic.stdout, []byte{'\n'}) != 1 || !validSymbolicTarget(target) {
			return unreadable
		}
		return RefHead{Ref: ref, State: RefSymbolic, Target: target}
	}
	if symbolic.exitCode != 1 || len(symbolic.stdout) != 0 || len(symbolic.stderr) != 0 {
		return unreadable
	}
	exists := r.runStatus(ctx, home, group, MaxRefProtocolBytes,
		"show-ref", "--verify", "--quiet", ref)
	if !trustworthyStatus(exists) || len(exists.stdout) != 0 {
		return unreadable
	}
	if exists.exitCode == 1 && len(exists.stderr) == 0 {
		return RefHead{Ref: ref, State: RefAbsent}
	}
	if exists.exitCode == 0 || exists.exitCode == 128 {
		return RefHead{Ref: ref, State: RefBroken}
	}
	return unreadable
}
func (r *Repository) captureHeadRefs(
	ctx context.Context,
	home string,
	group int,
	refs []string,
) ([]RefHead, error) {
	exact, err := exactRefs(refs)
	if err != nil {
		return nil, err
	}
	args := []string{"for-each-ref", "--format=%(refname)%09%(objectname)%09%(objecttype)%09%(symref)"}
	args = append(args, exact...)
	batch := r.runStatus(ctx, home, group, MaxRefProtocolBytes, args...)
	if !trustworthyStatus(batch) || batch.exitCode != 0 || len(batch.stdout) > 0 &&
		!bytes.HasSuffix(batch.stdout, []byte{'\n'}) || !utf8.Valid(batch.stdout) || !utf8.Valid(batch.stderr) {
		return unreadableRefs(exact), nil
	}
	requested := make(map[string]bool, len(exact))
	for _, ref := range exact {
		requested[ref] = true
	}
	observed := make(map[string]RefHead, len(exact))
	if len(batch.stderr) > 0 {
		if !bytes.HasSuffix(batch.stderr, []byte{'\n'}) {
			return unreadableRefs(exact), nil
		}
		for _, warning := range strings.Split(strings.TrimSuffix(string(batch.stderr), "\n"), "\n") {
			ref := strings.TrimPrefix(warning, "warning: ignoring broken ref ")
			if ref == warning || !requested[ref] || observed[ref].Ref != "" {
				return unreadableRefs(exact), nil
			}
			observed[ref] = RefHead{Ref: ref, State: RefBroken}
		}
	}
	rendered := strings.TrimSuffix(string(batch.stdout), "\n")
	if rendered != "" {
		for _, line := range strings.Split(rendered, "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) != 4 {
				return unreadableRefs(exact), nil
			}
			ref, oidText, objectType, target := fields[0], fields[1], fields[2], fields[3]
			if !requested[ref] || observed[ref].Ref != "" {
				return unreadableRefs(exact), nil
			}
			if target != "" {
				if validSymbolicTarget(target) {
					observed[ref] = RefHead{Ref: ref, State: RefSymbolic, Target: target}
				} else {
					observed[ref] = RefHead{Ref: ref, State: RefUnreadable}
				}
				continue
			}
			oid, oidErr := ParseOID(r.format, oidText)
			switch {
			case oidErr != nil:
				observed[ref] = RefHead{Ref: ref, State: RefBroken}
			case objectType == "commit":
				observed[ref] = RefHead{Ref: ref, State: RefDirect, Head: oid}
			case objectType == "":
				observed[ref] = RefHead{Ref: ref, State: RefBroken}
			default:
				observed[ref] = RefHead{Ref: ref, State: RefNonCommit}
			}
		}
	}
	result := make([]RefHead, len(exact))
	for index, ref := range exact {
		value, ok := observed[ref]
		if !ok {
			value = r.probeOmittedRef(ctx, home, group, ref)
		}
		result[index] = value
	}
	return result, nil
}

// CaptureHeadRefs returns one sorted typed observation of exact branch refs.
func (r *Repository) CaptureHeadRefs(refs []string) ([]RefHead, error) {
	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	return r.captureHeadRefs(ctx, "", 0, refs)
}

type RefOperationKind string

const (
	CreateRef RefOperationKind = "create"
	UpdateRef RefOperationKind = "update"
	VerifyRef RefOperationKind = "verify"
)

type RefOperation struct {
	Kind              RefOperationKind
	Ref               string
	NewHead, Expected *OID
}
type preparedRefTransaction struct {
	pre, desired []RefHead
	objects      []OID
	commands     []string
	meaningful   bool
}

func vectorMatches(observed, expected []RefHead) bool {
	if len(observed) != len(expected) {
		return false
	}
	for index := range observed {
		left, right := observed[index], expected[index]
		if left.Ref != right.Ref || left.State != right.State || left.Target != right.Target || left.Head != right.Head {
			return false
		}
	}
	return true
}
func (r *Repository) prepareRefTransaction(snapshot []RefHead, operations []RefOperation) (preparedRefTransaction, error) {
	if len(operations) == 0 || len(operations) > MaxHeadRefs {
		return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", fmt.Errorf("requires 1-%d operations", MaxHeadRefs))
	}
	captured := snapshot
	for index, value := range captured {
		if err := ValidateHeadRef(value.Ref); err != nil {
			return preparedRefTransaction{}, err
		}
		if index > 0 && value.Ref <= captured[index-1].Ref {
			return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", errors.New("captured vector is not raw-byte sorted and unique"))
		}
		if !value.Head.IsZero() && r.validateOID(value.Head) != nil {
			return preparedRefTransaction{}, fail("OBJECT_FORMAT_MISMATCH", "prepare refs", errors.New("captured head uses another format"))
		}
	}
	sorted := append([]RefOperation(nil), operations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Ref < sorted[j].Ref })
	if len(sorted) != len(captured) {
		return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", errors.New("operation vector differs from captured vector"))
	}
	result := preparedRefTransaction{}
	for index, operation := range sorted {
		if err := ValidateHeadRef(operation.Ref); err != nil {
			return preparedRefTransaction{}, err
		}
		if index > 0 && operation.Ref == sorted[index-1].Ref {
			return preparedRefTransaction{}, fail("DUPLICATE_REF", "prepare refs", fmt.Errorf("%s is repeated", operation.Ref))
		}
		if captured[index].Ref != operation.Ref {
			return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", errors.New("operation vector differs from captured vector"))
		}
		pre := RefHead{Ref: operation.Ref}
		desired := RefHead{Ref: operation.Ref}
		switch operation.Kind {
		case CreateRef:
			if operation.NewHead == nil || operation.Expected != nil || r.validateOID(*operation.NewHead) != nil {
				return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", errors.New("create requires one format-bound new commit"))
			}
			pre.State, desired.State = RefAbsent, RefDirect
			head := *operation.NewHead
			desired.Head = head
			result.objects = append(result.objects, head)
			result.commands = append(result.commands, "create "+operation.Ref+" "+head.String())
			result.meaningful = true
		case UpdateRef:
			if operation.NewHead == nil || operation.Expected == nil ||
				r.validateOID(*operation.NewHead) != nil || r.validateOID(*operation.Expected) != nil {
				return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", errors.New("update requires two format-bound commits"))
			}
			pre.State, desired.State = RefDirect, RefDirect
			oldHead, newHead := *operation.Expected, *operation.NewHead
			pre.Head, desired.Head = oldHead, newHead
			result.objects = append(result.objects, oldHead, newHead)
			result.commands = append(result.commands, "update "+operation.Ref+" "+newHead.String()+" "+oldHead.String())
			result.meaningful = result.meaningful || oldHead != newHead
		case VerifyRef:
			if operation.NewHead != nil {
				return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", errors.New("verify cannot have a new head"))
			}
			if operation.Expected == nil {
				pre.State, desired.State = RefAbsent, RefAbsent
				result.commands = append(result.commands, "verify "+operation.Ref+" "+strings.Repeat("0", r.format.oidLength()))
			} else {
				if r.validateOID(*operation.Expected) != nil {
					return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", errors.New("verify expected head uses another format"))
				}
				pre.State, desired.State = RefDirect, RefDirect
				head := *operation.Expected
				pre.Head, desired.Head = head, head
				result.objects = append(result.objects, head)
				result.commands = append(result.commands, "verify "+operation.Ref+" "+head.String())
			}
		default:
			return preparedRefTransaction{}, fail("INVALID_REF_TRANSACTION", "prepare refs", fmt.Errorf("unknown kind %q", operation.Kind))
		}
		result.pre, result.desired = append(result.pre, pre), append(result.desired, desired)
	}
	if vectorMatches(captured, result.desired) {
		return result, nil
	}
	if !vectorMatches(captured, result.pre) {
		return preparedRefTransaction{}, fail("REF_TRANSACTION_RECOVERY_REQUIRED", "prepare refs", errors.New("captured vector is neither complete pre-state nor desired state"))
	}
	return result, nil
}
func (r *Repository) requireTransactionCommits(ctx context.Context, home string, prepared preparedRefTransaction) error {
	unique := make(map[string]OID)
	for _, object := range prepared.objects {
		unique[object.String()] = object
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	input := []byte(strings.Join(keys, "\n") + "\n")
	outcome := r.runOutcome(ctx, home, 0, input, nil, "/dev/null", MaxRefProtocolBytes,
		"cat-file", "--batch-check=%(objectname) %(objecttype)")
	if !outcome.successful() || outcome.cleanupErr != nil || len(outcome.stderr) != 0 {
		return fail("REF_TRANSACTION_TRANSPORT", "validate ref objects", outcome.waitErr)
	}
	lines := strings.Split(strings.TrimSuffix(string(outcome.stdout), "\n"), "\n")
	if len(lines) != len(keys) {
		return fail("INVALID_GIT_OUTPUT", "validate ref objects", errors.New("cat-file returned another record count"))
	}
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != keys[index] || fields[1] != "commit" {
			return fail("NON_COMMIT_OBJECT", "validate ref objects", fmt.Errorf("invalid commit record %q", line))
		}
	}
	return nil
}

type protocolEvent struct {
	line []byte
	err  error
}

func protocolEvents(reader io.Reader) <-chan protocolEvent {
	events := make(chan protocolEvent, 128)
	go func() {
		defer close(events)
		buffered := bufio.NewReaderSize(reader, 32*1024)
		total := 0
		for {
			line, err := buffered.ReadBytes('\n')
			total += len(line)
			if total > MaxRefProtocolBytes {
				events <- protocolEvent{err: errors.New("protocol output exceeded its byte bound")}
				return
			}
			if len(line) > 0 {
				events <- protocolEvent{line: line}
			}
			if err != nil {
				events <- protocolEvent{err: err}
				return
			}
		}
	}()
	return events
}
func awaitProtocol(ctx context.Context, events <-chan protocolEvent, expected string) error {
	select {
	case event, ok := <-events:
		if !ok || errors.Is(event.err, io.EOF) {
			return errors.New("protocol ended before acknowledgement")
		}
		if event.err != nil {
			return event.err
		}
		if !utf8.Valid(event.line) || string(event.line) != expected+"\n" {
			return fmt.Errorf("unexpected protocol acknowledgement %q", event.line)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func noQueuedProtocol(events <-chan protocolEvent) error {
	select {
	case event := <-events:
		if event.err != nil {
			return event.err
		}
		return errors.New("protocol emitted queued or trailing output")
	default:
		return nil
	}
}
func writeProtocol(ctx context.Context, writer io.Writer, line string) error {
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, line+"\n")
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func proceed(current error, action func() error) error {
	if current != nil {
		return current
	}
	return action()
}

type refFault struct {
	afterPrepare, afterProcess func()
	failStage, uncertain       string
	cleanup, force             bool
	observe                    func(refTrace)
}
type refTrace struct {
	pid, group                                         int
	started, waited, reaped, groupQuiet, locksReleased bool
	attempts                                           int
	committed                                          bool
}

func (r *Repository) inspectPrepared(ctx context.Context, home string, group int, expected []RefHead) error {
	refs := make([]string, len(expected))
	for index, value := range expected {
		refs[index] = value.Ref
	}
	observed, err := r.captureHeadRefs(ctx, home, group, refs)
	if err != nil {
		return err
	}
	if !vectorMatches(observed, expected) {
		return fail("REF_TRANSACTION_CHANGED", "inspect prepared refs", errors.New("locked refs differ from captured pre-state"))
	}
	return nil
}
func (r *Repository) locksReleased(refs []RefHead) bool {
	for _, value := range refs {
		name := filepath.Join(r.commonDir, filepath.FromSlash(value.Ref)) + ".lock"
		if _, err := os.Lstat(name); err == nil || !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	if _, err := os.Lstat(filepath.Join(r.commonDir, "packed-refs.lock")); err == nil || !errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}
func transactionError(code, message string, cause error) error {
	return fail(code, message, errors.Join(cause, errors.New(message)))
}
func (r *Repository) applyRefTransaction(
	ctx context.Context,
	home string,
	prepared preparedRefTransaction,
) (primary error, quiet bool, trace refTrace) {
	command := r.command(home, 0, nil, "/dev/null", "update-ref", "--no-deref", "--stdin")
	var err error
	stdin, err := command.StdinPipe()
	if err != nil {
		return err, false, trace
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err, false, trace
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err, false, trace
	}
	if err := command.Start(); err != nil {
		return err, false, trace
	}
	trace.started, trace.pid, trace.group, trace.attempts = true, command.Process.Pid, command.Process.Pid, 1
	events := protocolEvents(stdout)
	stderrBuffer := &boundedBuffer{limit: MaxRefProtocolBytes}
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(stderrBuffer, stderr)
		close(stderrDone)
	}()
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	stage := func(names ...string) error {
		for _, name := range names {
			if r.refFault != nil && r.refFault.failStage == name {
				return errors.New("injected " + name + " fault")
			}
		}
		return nil
	}
	err = proceed(stage("start-write"), func() error { return writeProtocol(ctx, stdin, "start") })
	err = proceed(err, func() error { return stage("start-ack") })
	err = proceed(err, func() error { return awaitProtocol(ctx, events, "start: ok") })
	err = proceed(err, func() error { return stage("start-extra") })
	err = proceed(err, func() error { return noQueuedProtocol(events) })
	for _, line := range prepared.commands {
		err = proceed(err, func() error { return stage("input-write") })
		err = proceed(err, func() error { return writeProtocol(ctx, stdin, line) })
	}
	err = proceed(err, func() error { return stage("prepare-write") })
	err = proceed(err, func() error { return writeProtocol(ctx, stdin, "prepare") })
	err = proceed(err, func() error { return stage("prepare-ack") })
	err = proceed(err, func() error { return awaitProtocol(ctx, events, "prepare: ok") })
	err = proceed(err, func() error { return stage("prepare-extra") })
	err = proceed(err, func() error { return noQueuedProtocol(events) })
	if err == nil && r.refFault != nil && r.refFault.afterPrepare != nil {
		r.refFault.afterPrepare()
	}
	err = proceed(err, func() error {
		return stage("inspection", "timeout", "sigkill", "early-exit", "missing-ack",
			"malformed-ack", "extra-ack", "stdout-overflow", "stderr-overflow")
	})
	err = proceed(err, func() error {
		return r.inspectPrepared(ctx, home, trace.group, prepared.pre)
	})
	err = proceed(err, func() error { return stage("commit-write") })
	err = proceed(err, func() error { return writeProtocol(ctx, stdin, "commit") })
	err = proceed(err, func() error { return stage("commit-ack") })
	err = proceed(err, func() error { return awaitProtocol(ctx, events, "commit: ok") })
	if err == nil {
		trace.committed = true
		err = stage("post-commit", "nonzero-exit", "signal", "post-timeout", "post-extra",
			"post-overflow", "parser-failure", "ack-loss")
	}
	primary = err
	if primary != nil && !trace.committed {
		_ = writeProtocol(ctx, stdin, "abort")
	}
	_ = stdin.Close()
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-ctx.Done():
		_ = syscall.Kill(-trace.group, syscall.SIGKILL)
		waitErr = <-waited
		primary = errors.Join(primary, ctx.Err())
	}
	trace.waited, trace.reaped = true, true
	primary = errors.Join(primary, waitErr)
	<-stderrDone
	if stderrBuffer.overflow || stderrBuffer.Len() != 0 {
		primary = errors.Join(primary, errors.New("transaction stderr was non-empty or overflowed"))
	}
	abortSeen := false
	for event := range events {
		switch {
		case event.err != nil:
			primary = errors.Join(primary, event.err)
		case errors.Is(event.err, io.EOF):
		case !trace.committed && !abortSeen && string(event.line) == "abort: ok\n":
			abortSeen = true
		default:
			primary = errors.Join(primary, errors.New("protocol emitted trailing output"))
		}
	}
	trace.groupQuiet = processGroupQuiet(ctx, trace.group)
	if !trace.groupQuiet {
		_ = syscall.Kill(-trace.group, syscall.SIGKILL)
		trace.groupQuiet = processGroupQuiet(ctx, trace.group)
	}
	trace.locksReleased = r.locksReleased(prepared.pre)
	if r.refFault != nil {
		switch r.refFault.uncertain {
		case "kill", "wait", "reap", "group":
			trace.groupQuiet = false
		case "lock":
			trace.locksReleased = false
		}
	}
	quiet = trace.started && trace.waited && trace.reaped && trace.groupQuiet && trace.locksReleased
	return primary, quiet, trace
}

// ApplyRefTransaction reconciles one CAS from its exact captured ref vector.
func (r *Repository) ApplyRefTransaction(snapshot []RefHead, operations []RefOperation) error {
	ctx, cancel := context.WithTimeout(context.Background(), RefTransactionLimit)
	defer cancel()
	prepared, err := r.prepareRefTransaction(snapshot, operations)
	if err != nil {
		return err
	}
	if vectorMatches(snapshot, prepared.desired) && (r.refFault == nil || !r.refFault.force) {
		return nil
	}
	tempRoot, err := ResolveTempRoot()
	if err != nil {
		return transactionError("REF_TRANSACTION_RECOVERY_REQUIRED", "resolve temp root", err)
	}
	home, err := os.MkdirTemp(tempRoot, "sworn-ref-transaction-*")
	if err != nil {
		return transactionError("REF_TRANSACTION_RECOVERY_REQUIRED", "create private transaction directory", err)
	}
	defer func() {
		cleanupErr := os.RemoveAll(home)
		if r.refFault != nil && r.refFault.cleanup {
			cleanupErr = errors.New("injected inert cleanup failure")
		}
		_ = cleanupErr // Inert private-directory cleanup is subordinate.
	}()
	if err := os.Mkdir(filepath.Join(home, "hooks"), 0o700); err != nil {
		return transactionError("REF_TRANSACTION_RECOVERY_REQUIRED", "create private hooks directory", err)
	}
	if err := r.requireTransactionCommits(ctx, home, prepared); err != nil {
		return err
	}
	primary, quiet, trace := r.applyRefTransaction(ctx, home, prepared)
	if r.refFault != nil && r.refFault.observe != nil {
		r.refFault.observe(trace)
	}
	if !quiet {
		return transactionError("REF_TRANSACTION_RECOVERY_REQUIRED", "exact ref transaction quiescence is uncertain", primary)
	}
	if r.refFault != nil && r.refFault.afterProcess != nil {
		r.refFault.afterProcess()
	}
	refs := make([]string, len(prepared.pre))
	for index, value := range prepared.pre {
		refs[index] = value.Ref
	}
	observed, captureErr := r.captureHeadRefs(ctx, "", 0, refs)
	if captureErr != nil {
		return transactionError("REF_TRANSACTION_RECOVERY_REQUIRED", "exact ref reconciliation failed", errors.Join(primary, captureErr))
	}
	if vectorMatches(observed, prepared.desired) {
		return nil
	}
	if prepared.meaningful && vectorMatches(observed, prepared.pre) {
		return transactionError("REF_TRANSACTION_NOT_APPLIED", "snapshot-scoped ref transaction was not applied; a fresh Snapshot is required and ABA history is not disproved", primary)
	}
	return transactionError("REF_TRANSACTION_RECOVERY_REQUIRED", "exact ref transaction outcome is ambiguous", errors.Join(primary, captureErr))
}

// AtomicUpdateRefs is the low-level mechanical convenience, not Baton authority.
func (r *Repository) AtomicUpdateRefs(operations []RefOperation) error {
	refs := make([]string, len(operations))
	for index, operation := range operations {
		refs[index] = operation.Ref
	}
	snapshot, err := r.CaptureHeadRefs(refs)
	if err != nil {
		return err
	}
	return r.ApplyRefTransaction(snapshot, operations)
}
