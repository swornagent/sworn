package baton

import (
	"errors"
	"strings"
	"testing"
)

func TestRepositoryPathCanonicalizationRulesAndLengthDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		label    string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "rule 1 leading slash",
			value:    "/foo/bar",
			label:    "touchpoints[0]",
			wantCode: "INVALID_PATH",
			wantMsg:  "touchpoints[0] is not a canonical repository path: cannot have leading/trailing slash, backslash, consecutive slashes, or control characters",
		},
		{
			name:     "rule 1 trailing slash",
			value:    "foo/bar/",
			label:    "touchpoints[0]",
			wantCode: "INVALID_PATH",
			wantMsg:  "touchpoints[0] is not a canonical repository path: cannot have leading/trailing slash, backslash, consecutive slashes, or control characters",
		},
		{
			name:     "rule 1 backslash",
			value:    "foo\\bar",
			label:    "touchpoints[0]",
			wantCode: "INVALID_PATH",
			wantMsg:  "touchpoints[0] is not a canonical repository path: cannot have leading/trailing slash, backslash, consecutive slashes, or control characters",
		},
		{
			name:     "rule 1 double slash",
			value:    "foo//bar",
			label:    "touchpoints[0]",
			wantCode: "INVALID_PATH",
			wantMsg:  "touchpoints[0] is not a canonical repository path: cannot have leading/trailing slash, backslash, consecutive slashes, or control characters",
		},
		{
			name:     "rule 1 control char",
			value:    "foo\x01bar",
			label:    "touchpoints[0]",
			wantCode: "INVALID_PATH",
			wantMsg:  "touchpoints[0] is not a canonical repository path: cannot have leading/trailing slash, backslash, consecutive slashes, or control characters",
		},
		{
			name:     "rule 2 dot segment",
			value:    "foo/./bar",
			label:    "touchpoints[0]",
			wantCode: "INVALID_PATH",
			wantMsg:  "touchpoints[0] is not a canonical repository path: segments cannot be empty, '.', or '..'",
		},
		{
			name:     "rule 2 dotdot segment",
			value:    "foo/../bar",
			label:    "touchpoints[0]",
			wantCode: "INVALID_PATH",
			wantMsg:  "touchpoints[0] is not a canonical repository path: segments cannot be empty, '.', or '..'",
		},
		{
			name:     "rule 3 git first segment",
			value:    ".git/config",
			label:    "contract_path",
			wantCode: "INVALID_PATH",
			wantMsg:  "contract_path is not a canonical repository path: first segment cannot be '.git'",
		},
		{
			name:     "over 512 length bound",
			value:    strings.Repeat("a", 513),
			label:    "contract_path",
			wantCode: "INVALID_FIELD",
			wantMsg:  "contract_path must be a string of 1-512 characters (got 513)",
		},
		{
			name:     "valid path passes",
			value:    "contracts/2026-09-03/S1.json",
			label:    "contract_path",
			wantCode: "",
			wantMsg:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repositoryPath(tc.value, tc.label)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("repositoryPath(%v) unexpected error: %v", tc.value, err)
				}
				if got != tc.value.(string) {
					t.Fatalf("repositoryPath(%v) = %q, want %q", tc.value, got, tc.value)
				}
				return
			}
			if err == nil {
				t.Fatalf("repositoryPath(%v) want error %s, got nil", tc.value, tc.wantCode)
			}
			var recErr *RecordError
			if !errors.As(err, &recErr) {
				t.Fatalf("repositoryPath(%v) err is not *RecordError: %T", tc.value, err)
			}
			if recErr.Code != tc.wantCode {
				t.Fatalf("repositoryPath(%v) code = %q, want %q", tc.value, recErr.Code, tc.wantCode)
			}
			if recErr.Msg != tc.wantMsg {
				t.Fatalf("repositoryPath(%v) msg = %q, want %q", tc.value, recErr.Msg, tc.wantMsg)
			}
		})
	}
}
