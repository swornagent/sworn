package gitx

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// RefHead is one exact captured full branch ref. A nil Head means absent.
type RefHead struct {
	Ref  string
	Head *OID
}

// CaptureHeadRefs captures an ordered set of exact, direct commit branch
// heads in one for-each-ref read. Missing refs are represented by nil heads.
func (r *Repository) CaptureHeadRefs(refs []string) ([]RefHead, error) {
	if len(refs) == 0 || len(refs) > MaxHeadRefs {
		return nil, fail("RESOURCE_LIMIT", "capture refs", fmt.Errorf("requires 1-%d refs", MaxHeadRefs))
	}
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if err := ValidateHeadRef(ref); err != nil {
			return nil, err
		}
		if _, duplicate := seen[ref]; duplicate {
			return nil, fail("DUPLICATE_REF", "capture refs", fmt.Errorf("%s is repeated", ref))
		}
		seen[ref] = struct{}{}
	}
	args := []string{"for-each-ref", "--format=%(refname)%00%(objectname)%00%(objecttype)%00%(symref)%00"}
	args = append(args, refs...)
	raw, err := r.run(nil, nil, args...)
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(raw, []byte{0})
	if len(fields) > 0 && strings.TrimSpace(string(fields[len(fields)-1])) == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%4 != 0 {
		return nil, fail("INVALID_GIT_OUTPUT", "capture refs", errors.New("for-each-ref returned a truncated record"))
	}
	observed := make(map[string]OID)
	for index := 0; index < len(fields); index += 4 {
		ref := strings.TrimSpace(string(fields[index]))
		oidText := strings.TrimSpace(string(fields[index+1]))
		objectType := strings.TrimSpace(string(fields[index+2]))
		symbolicTarget := strings.TrimSpace(string(fields[index+3]))
		if _, expected := seen[ref]; !expected {
			return nil, fail("UNEXPECTED_REF", "capture refs", fmt.Errorf("Git returned %s", ref))
		}
		if _, duplicate := observed[ref]; duplicate {
			return nil, fail("INVALID_GIT_OUTPUT", "capture refs", fmt.Errorf("Git returned %s more than once", ref))
		}
		if symbolicTarget != "" {
			return nil, fail("NON_DIRECT_COMMIT_REF", "capture refs", fmt.Errorf("%s is symbolic", ref))
		}
		if objectType != "commit" {
			return nil, fail("NON_DIRECT_COMMIT_REF", "capture refs", fmt.Errorf("%s points to %q, not a commit", ref, objectType))
		}
		oid, err := r.parseOID(oidText)
		if err != nil {
			return nil, err
		}
		observed[ref] = oid
	}
	result := make([]RefHead, 0, len(refs))
	for _, ref := range refs {
		entry := RefHead{Ref: ref}
		if oid, ok := observed[ref]; ok {
			copyOID := oid
			entry.Head = &copyOID
		} else if _, symbolic, err := r.symbolicHeadTarget(ref); err != nil {
			return nil, err
		} else if symbolic {
			// for-each-ref omits a symbolic ref whose referent is absent.
			return nil, fail("NON_DIRECT_COMMIT_REF", "capture refs", fmt.Errorf("%s is symbolic", ref))
		}
		result = append(result, entry)
	}
	return result, nil
}

func (r *Repository) symbolicHeadTarget(ref string) (string, bool, error) {
	command, cleanup, err := r.newLiteralCommand(nil, "/dev/null", "symbolic-ref", "--quiet", "--no-recurse", ref)
	if err != nil {
		return "", false, fail("GIT_EXECUTION_FAILED", "inspect symbolic head ref", err)
	}
	defer cleanup()
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && stdout.Len() == 0 && stderr.Len() == 0 {
			return "", false, nil
		}
		message := strings.TrimSpace(stderr.String())
		if len(message) > 4096 {
			message = message[:4096]
		}
		if message == "" {
			message = err.Error()
		}
		return "", false, fail("GIT_EXECUTION_FAILED", "inspect symbolic head ref", errors.New(message))
	}
	target := strings.TrimSpace(stdout.String())
	if target == "" || strings.ContainsRune(target, 0) {
		return "", false, fail("INVALID_GIT_OUTPUT", "inspect symbolic head ref", errors.New("symbolic-ref returned an invalid target"))
	}
	return target, true, nil
}

// resolveHead resolves one exact direct branch ref. Missing refs return a nil
// head.
func (r *Repository) resolveHead(ref string) (*OID, error) {
	heads, err := r.CaptureHeadRefs([]string{ref})
	if err != nil {
		return nil, err
	}
	return heads[0].Head, nil
}

// RefOperationKind is one atomic exact-ref operation.
type RefOperationKind string

const (
	CreateRef RefOperationKind = "create"
	UpdateRef RefOperationKind = "update"
	VerifyRef RefOperationKind = "verify"
)

// RefOperation describes one compare-and-set operation.
type RefOperation struct {
	Kind     RefOperationKind
	Ref      string
	NewHead  *OID
	Expected *OID
}

func (r *Repository) requireCommitObjects(values []OID) error {
	unique := make([]OID, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	var input strings.Builder
	for _, value := range values {
		if _, duplicate := seen[value.String()]; duplicate {
			continue
		}
		seen[value.String()] = struct{}{}
		unique = append(unique, value)
		input.WriteString(value.String())
		input.WriteByte('\n')
	}
	if len(unique) == 0 {
		return nil
	}
	raw, err := r.run([]byte(input.String()), nil, "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return err
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte{'\n'}), []byte{'\n'})
	if len(lines) != len(unique) {
		return fail("INVALID_GIT_OUTPUT", "validate commit objects", errors.New("cat-file returned an unexpected number of records"))
	}
	for index, line := range lines {
		fields := strings.Fields(string(line))
		if len(fields) != 2 || fields[0] != unique[index].String() {
			return fail("INVALID_GIT_OUTPUT", "validate commit objects", fmt.Errorf("unexpected cat-file record %q", line))
		}
		if fields[1] != "commit" {
			return fail("NON_COMMIT_OBJECT", "validate commit objects", fmt.Errorf("%s is %s, not a commit", fields[0], fields[1]))
		}
	}
	return nil
}

func (r *Repository) inspectPreparedRefOperations(operations []RefOperation) error {
	refs := make([]string, 0, len(operations))
	for _, operation := range operations {
		refs = append(refs, operation.Ref)
	}
	heads, err := r.CaptureHeadRefs(refs)
	if err != nil {
		return err
	}
	for index, operation := range operations {
		if operation.Expected == nil {
			if heads[index].Head != nil {
				return fail("REF_TRANSACTION_CHANGED", "inspect prepared refs", fmt.Errorf("%s appeared during transaction preparation", operation.Ref))
			}
			continue
		}
		if heads[index].Head == nil || *heads[index].Head != *operation.Expected {
			return fail("REF_TRANSACTION_CHANGED", "inspect prepared refs", fmt.Errorf("%s does not equal its expected commit", operation.Ref))
		}
	}
	return nil
}

func refTransactionCause(stderr *bytes.Buffer, cause error) error {
	message := strings.TrimSpace(stderr.String())
	if len(message) > 4096 {
		message = message[:4096]
	}
	if message == "" && cause != nil {
		message = cause.Error()
	}
	if message == "" {
		message = "Git ref transaction failed without a diagnostic"
	}
	return errors.New(message)
}

func (r *Repository) runPreparedRefTransaction(input []byte, operations []RefOperation) error {
	command, cleanup, err := r.newLiteralCommand(nil, "/dev/null", "update-ref", "--stdin", "-z", "--no-deref")
	if err != nil {
		return fail("REF_TRANSACTION_FAILED", "atomic update refs", err)
	}
	defer cleanup()
	stdin, err := command.StdinPipe()
	if err != nil {
		return fail("REF_TRANSACTION_FAILED", "atomic update refs", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fail("REF_TRANSACTION_FAILED", "atomic update refs", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return fail("REF_TRANSACTION_FAILED", "atomic update refs", err)
	}
	waited := false
	finish := func() error {
		if waited {
			return nil
		}
		waited = true
		_ = stdin.Close()
		return command.Wait()
	}
	defer func() {
		_ = finish()
	}()
	failProcess := func(cause error) error {
		waitErr := finish()
		if cause == nil {
			cause = waitErr
		}
		return fail("REF_TRANSACTION_FAILED", "atomic update refs", refTransactionCause(&stderr, cause))
	}
	if _, err := io.Copy(stdin, bytes.NewReader(input)); err != nil {
		return failProcess(err)
	}
	reader := bufio.NewReader(stdout)
	readResponse := func(expected string) error {
		response, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if response != expected {
			return fmt.Errorf("unexpected update-ref response %q", response)
		}
		return nil
	}
	if err := readResponse("start: ok\n"); err != nil {
		return failProcess(err)
	}
	if err := readResponse("prepare: ok\n"); err != nil {
		return failProcess(err)
	}

	// prepare has locked every named ref with --no-deref. Nothing can change a
	// direct ref into a symbolic ref between this inspection and commit.
	if r.afterRefPrepare != nil {
		r.afterRefPrepare()
	}
	if inspectErr := r.inspectPreparedRefOperations(operations); inspectErr != nil {
		if _, err := io.WriteString(stdin, "abort\x00"); err != nil {
			return failProcess(errors.Join(inspectErr, err))
		}
		if err := readResponse("abort: ok\n"); err != nil {
			return failProcess(errors.Join(inspectErr, err))
		}
		if err := finish(); err != nil {
			return fail("REF_TRANSACTION_FAILED", "atomic update refs", errors.Join(inspectErr, refTransactionCause(&stderr, err)))
		}
		return inspectErr
	}
	if _, err := io.WriteString(stdin, "commit\x00"); err != nil {
		return failProcess(err)
	}
	if err := readResponse("commit: ok\n"); err != nil {
		return failProcess(err)
	}
	if err := finish(); err != nil {
		return fail("REF_TRANSACTION_FAILED", "atomic update refs", refTransactionCause(&stderr, err))
	}
	return nil
}

// AtomicUpdateRefs performs one transactional create/update/verify set.
func (r *Repository) AtomicUpdateRefs(operations []RefOperation) error {
	if len(operations) == 0 || len(operations) > MaxHeadRefs {
		return fail("RESOURCE_LIMIT", "atomic update refs", fmt.Errorf("requires 1-%d operations", MaxHeadRefs))
	}
	seen := make(map[string]struct{}, len(operations))
	commitObjects := make([]OID, 0, len(operations)*2)
	var input bytes.Buffer
	input.WriteString("start")
	input.WriteByte(0)
	for _, operation := range operations {
		if err := ValidateHeadRef(operation.Ref); err != nil {
			return err
		}
		if _, duplicate := seen[operation.Ref]; duplicate {
			return fail("DUPLICATE_REF", "atomic update refs", fmt.Errorf("%s is repeated", operation.Ref))
		}
		seen[operation.Ref] = struct{}{}
		switch operation.Kind {
		case CreateRef:
			if operation.NewHead == nil || operation.Expected != nil {
				return fail("INVALID_REF_TRANSACTION", "atomic update refs", errors.New("create requires new head and absent expected head"))
			}
			if err := r.validateOID(*operation.NewHead); err != nil {
				return err
			}
			commitObjects = append(commitObjects, *operation.NewHead)
			input.WriteString("create " + operation.Ref)
			input.WriteByte(0)
			input.WriteString(operation.NewHead.String())
			input.WriteByte(0)
		case UpdateRef:
			if operation.NewHead == nil || operation.Expected == nil {
				return fail("INVALID_REF_TRANSACTION", "atomic update refs", errors.New("update requires new and expected heads"))
			}
			if err := r.validateOID(*operation.NewHead); err != nil {
				return err
			}
			if err := r.validateOID(*operation.Expected); err != nil {
				return err
			}
			commitObjects = append(commitObjects, *operation.NewHead, *operation.Expected)
			input.WriteString("update " + operation.Ref)
			input.WriteByte(0)
			input.WriteString(operation.NewHead.String())
			input.WriteByte(0)
			input.WriteString(operation.Expected.String())
			input.WriteByte(0)
		case VerifyRef:
			input.WriteString("verify " + operation.Ref)
			input.WriteByte(0)
			if operation.Expected != nil {
				if err := r.validateOID(*operation.Expected); err != nil {
					return err
				}
				commitObjects = append(commitObjects, *operation.Expected)
				input.WriteString(operation.Expected.String())
			}
			input.WriteByte(0)
		default:
			return fail("INVALID_REF_TRANSACTION", "atomic update refs", fmt.Errorf("unknown kind %q", operation.Kind))
		}
	}
	input.WriteString("prepare")
	input.WriteByte(0)
	if err := r.requireCommitObjects(commitObjects); err != nil {
		return err
	}
	return r.runPreparedRefTransaction(input.Bytes(), operations)
}
