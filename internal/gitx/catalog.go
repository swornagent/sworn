package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ListHeadRefsUnder returns the existing local branch refs below one canonical
// namespace prefix. Results are sorted by ref name in byte order and bounded
// by MaxHeadRefs.
func (r *Repository) ListHeadRefsUnder(prefix string) ([]RefHead, error) {
	if r == nil {
		return nil, fail("INVALID_REPOSITORY", "list head refs", nil)
	}
	if !strings.HasSuffix(prefix, "/") ||
		ValidateHeadRef(prefix+"x") != nil {
		return nil, fail(
			"INVALID_REF",
			"list head refs",
			errors.New("prefix must be one canonical refs/heads namespace"),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), CommandTimeout)
	defer cancel()
	outcome := r.runStatus(
		ctx,
		"",
		0,
		MaxRefProtocolBytes,
		"for-each-ref",
		"--sort=refname",
		"--format=%(refname)%09%(objectname)%09%(objecttype)%09%(symref)",
		"--",
		prefix,
	)
	if outcome.overflow {
		return nil, fail(
			"RESOURCE_LIMIT",
			"list head refs",
			errors.New("ref catalog exceeded its byte bound"),
		)
	}
	if !trustworthyStatus(outcome) || outcome.exitCode != 0 ||
		len(outcome.stderr) != 0 {
		return nil, fail(
			"GIT_EXECUTION_FAILED",
			"list head refs",
			errors.New("ref catalog command failed"),
		)
	}
	return parseHeadRefCatalog(outcome.stdout, r.format, prefix)
}

func parseHeadRefCatalog(
	raw []byte,
	format ObjectFormat,
	prefix string,
) ([]RefHead, error) {
	if len(raw) == 0 {
		return []RefHead{}, nil
	}
	if !utf8.Valid(raw) || !bytes.HasSuffix(raw, []byte{'\n'}) {
		return nil, invalidRefCatalog("catalog is not newline-terminated UTF-8")
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte{'\n'}), []byte{'\n'})
	if len(lines) > MaxHeadRefs {
		return nil, fail(
			"RESOURCE_LIMIT",
			"list head refs",
			fmt.Errorf("ref catalog exceeds %d entries", MaxHeadRefs),
		)
	}

	result := make([]RefHead, 0, len(lines))
	previous := ""
	for _, line := range lines {
		fields := bytes.Split(line, []byte{'\t'})
		if len(fields) != 4 {
			return nil, invalidRefCatalog("catalog row has another field count")
		}
		ref := string(fields[0])
		if ValidateHeadRef(ref) != nil || !strings.HasPrefix(ref, prefix) ||
			len(ref) == len(prefix) {
			return nil, invalidRefCatalog("catalog contains an invalid ref")
		}
		if previous != "" && ref <= previous {
			return nil, invalidRefCatalog("catalog refs are not strictly ordered")
		}
		previous = ref

		objectID := string(fields[1])
		objectType := string(fields[2])
		target := string(fields[3])
		if target != "" {
			if !validSymbolicTarget(target) {
				return nil, invalidRefCatalog("catalog contains an invalid symbolic ref")
			}
			result = append(result, RefHead{
				Ref: ref, State: RefSymbolic, Target: target,
			})
			continue
		}
		oid, err := ParseOID(format, objectID)
		if err != nil {
			return nil, invalidRefCatalog("catalog contains an invalid object ID")
		}
		state := RefNonCommit
		if objectType == "commit" {
			state = RefDirect
		} else if objectType == "" {
			state = RefBroken
		}
		result = append(result, RefHead{Ref: ref, State: state, Head: oid})
	}
	return result, nil
}

func invalidRefCatalog(message string) error {
	return fail("INVALID_GIT_OUTPUT", "list head refs", errors.New(message))
}
