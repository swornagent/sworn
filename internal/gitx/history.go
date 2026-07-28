package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxHistoryBytes = 32 * 1024 * 1024

// HistoryEntry is one bounded first-parent commit envelope, newest first.
type HistoryEntry struct {
	OID     OID
	Parents []OID
	Tree    OID
	Message []byte
}

func (r *Repository) ReadFirstParentHistory(head OID, maxCount int) ([]HistoryEntry, error) {
	if err := r.validateOID(head); err != nil {
		return nil, err
	}
	if maxCount < 1 || maxCount > MaxHistory {
		return nil, fail("INVALID_HISTORY_LIMIT", "read first-parent history", fmt.Errorf("limit must be 1-%d", MaxHistory))
	}
	raw, err := r.run(
		nil,
		nil,
		"log",
		"--first-parent",
		"--max-count="+strconv.Itoa(maxCount),
		"-z",
		"--format=%H%x00%P%x00%T%x00%B%x00",
		head.String(),
	)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxHistoryBytes {
		return nil, fail("RESOURCE_LIMIT", "read first-parent history", fmt.Errorf("history exceeds %d bytes", MaxHistoryBytes))
	}
	if !utf8.Valid(raw) {
		return nil, fail("MALFORMED_GIT_OUTPUT", "read first-parent history", errors.New("history is not valid UTF-8"))
	}
	if len(raw) == 0 || raw[len(raw)-1] != 0 {
		return nil, fail("MALFORMED_GIT_OUTPUT", "read first-parent history", errors.New("history is not terminated"))
	}
	fields := bytes.Split(raw[:len(raw)-1], []byte{0})
	result := make([]HistoryEntry, 0, maxCount)
	for len(fields) > 0 {
		if len(fields) < 5 {
			return nil, fail("MALFORMED_GIT_OUTPUT", "read first-parent history", errors.New("history envelope is malformed"))
		}
		oidRaw, parentsRaw, treeRaw, message, separator := fields[0], fields[1], fields[2], fields[3], fields[4]
		fields = fields[5:]
		if len(separator) != 0 {
			return nil, fail("MALFORMED_GIT_OUTPUT", "read first-parent history", errors.New("history separator is malformed"))
		}
		oid, err := r.parseOID(string(oidRaw))
		if err != nil {
			return nil, fail("MALFORMED_GIT_OUTPUT", "read first-parent history", err)
		}
		tree, err := r.parseOID(string(treeRaw))
		if err != nil {
			return nil, fail("MALFORMED_GIT_OUTPUT", "read first-parent history", err)
		}
		var parents []OID
		if len(parentsRaw) > 0 {
			for _, value := range strings.Split(string(parentsRaw), " ") {
				parent, err := r.parseOID(value)
				if err != nil {
					return nil, fail("MALFORMED_GIT_OUTPUT", "read first-parent history", err)
				}
				parents = append(parents, parent)
			}
		}
		result = append(result, HistoryEntry{
			OID: oid, Parents: parents, Tree: tree,
			Message: append([]byte(nil), message...),
		})
	}
	return result, nil
}

// FirstParentPathChange returns the newest first-parent commit at or below
// head that changed one exact repository path. The bounded output proves
// whether a release record path existed in earlier history without parsing
// inherited commit messages as current Baton authority.
func (r *Repository) FirstParentPathChange(head OID, path string) (OID, bool, error) {
	if err := r.validateOID(head); err != nil {
		return OID{}, false, err
	}
	if err := ValidatePath(path, false); err != nil {
		return OID{}, false, err
	}
	raw, err := r.run(
		nil,
		nil,
		"rev-list",
		"--first-parent",
		"--full-history",
		"--max-count=1",
		head.String(),
		"--",
		path,
	)
	if err != nil {
		return OID{}, false, err
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return OID{}, false, nil
	}
	oid, err := r.parseOID(value)
	if err != nil {
		return OID{}, false, fail(
			"MALFORMED_GIT_OUTPUT",
			"read first-parent path history",
			err,
		)
	}
	return oid, true, nil
}
