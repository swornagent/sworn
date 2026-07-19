package protocol

import (
	"encoding/json"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

func TestEncodeSubmissionMatchesAdmittedExample(t *testing.T) {
	t.Parallel()

	snapshot, err := SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/standard-submission.json")
	if err != nil {
		t.Fatal(err)
	}
	var submission Submission
	if err := json.Unmarshal(contents, &submission); err != nil {
		t.Fatal(err)
	}
	record, err := EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	const want = "sha256:51532765e47ad1d3414a7753e025ac17646cbbae70cd8ec63f5f3487b125a2f6"
	if record.Kind != SubmissionSchemaVersion || record.Digest != want {
		t.Fatalf("record = %#v, want kind %q digest %q", record, SubmissionSchemaVersion, want)
	}
}

func TestRecordTimeMatchesBatonDateTimeProfile(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"2026-07-19T00:00:00Z",
		"2026-07-19t00:00:00z",
		"2024-02-29T23:59:59.12345678901234567890+23:59",
		"0001-01-01T00:00:00-23:59",
	} {
		if _, err := parseRecordTime(value, "test"); err != nil {
			t.Errorf("valid date-time %q: %v", value, err)
		}
	}
	for _, value := range []string{
		"2026-07-19T00:00:00,1Z",
		"0000-01-01T00:00:00Z",
		"2026-07-19T24:00:00Z",
		"2026-07-19T00:60:00Z",
		"2026-07-19T00:00:60Z",
		"2026-07-19T00:00:00+24:00",
		"2026-07-19T00:00:00+23:60",
		"2026-02-29T00:00:00Z",
	} {
		if _, err := parseRecordTime(value, "test"); err == nil {
			t.Errorf("invalid date-time %q was accepted", value)
		}
	}
	earlier, _ := parseRecordTime("2026-07-19T00:00:00.12345678900000000001Z", "earlier")
	later, _ := parseRecordTime("2026-07-19T00:00:00.12345678900000000002Z", "later")
	if !earlier.Before(later) || !later.After(earlier) {
		t.Fatal("sub-nanosecond timestamp order was lost")
	}
	equivalent, _ := parseRecordTime("2026-07-19t01:00:00.123456789000000000010+01:00", "equivalent")
	if earlier.Before(equivalent) || earlier.After(equivalent) {
		t.Fatal("equivalent offset date-times did not compare equally")
	}
}

func TestEncodeSubmissionAcceptsLowercaseDateTimeAndRejectsWideExitCode(t *testing.T) {
	t.Parallel()
	submission := exampleSubmission(t)
	submission.CreatedAt = strings.ReplaceAll(strings.ReplaceAll(submission.CreatedAt, "T", "t"), "Z", "z")
	submission.Builder.StartedAt = strings.ReplaceAll(strings.ReplaceAll(submission.Builder.StartedAt, "T", "t"), "Z", "z")
	submission.Builder.CompletedAt = strings.ReplaceAll(strings.ReplaceAll(submission.Builder.CompletedAt, "T", "t"), "Z", "z")
	submission.Checks[0].StartedAt = strings.ReplaceAll(strings.ReplaceAll(submission.Checks[0].StartedAt, "T", "t"), "Z", "z")
	submission.Checks[0].CompletedAt = strings.ReplaceAll(strings.ReplaceAll(submission.Checks[0].CompletedAt, "T", "t"), "Z", "z")
	submission.Evidence[0].CapturedAt = strings.ReplaceAll(strings.ReplaceAll(submission.Evidence[0].CapturedAt, "T", "t"), "Z", "z")
	if _, err := EncodeSubmission(submission); err != nil {
		t.Fatalf("lowercase RFC 3339 date-time rejected: %v", err)
	}
	if strconv.IntSize == 64 {
		wide := int(int64(2_147_483_648))
		submission.Checks[0].ExitCode = &wide
		if _, err := EncodeSubmission(submission); err == nil || !strings.Contains(err.Error(), "int32") {
			t.Fatalf("wide exit code error = %v", err)
		}
	}
}

func TestDerivedSubmissionIdentityIsBoundedAndUnambiguous(t *testing.T) {
	t.Parallel()
	left, err := deriveSubmissionID("a.b", "c", 1)
	if err != nil {
		t.Fatal(err)
	}
	right, err := deriveSubmissionID("a", "b.c", 1)
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("distinct work attempts produced the same submission id")
	}
	maximum, err := deriveSubmissionID(strings.Repeat("a", 128), strings.Repeat("b", 128), 9_007_199_254_740_991)
	if err != nil || !ValidID(maximum) || len(maximum) > 128 {
		t.Fatalf("maximum derived id = %q, %v", maximum, err)
	}
}

func TestEncodeSubmissionRejectsFalseProducerBindings(t *testing.T) {
	t.Parallel()

	valid := exampleSubmission(t)
	mutations := map[string]func(*Submission){
		"builder stamped": func(value *Submission) {
			value.Checks[0].RunID = value.Builder.RunID
			value.Evidence[0].ProducerRunID = value.Builder.RunID
		},
		"wrong tree": func(value *Submission) {
			value.Evidence[0].CandidateTree = strings.Repeat("d", 40)
		},
		"early evidence": func(value *Submission) {
			value.Evidence[0].CapturedAt = "2026-07-19T00:04:34Z"
		},
		"mocked assembled": func(value *Submission) {
			value.Evidence[0].UsesMocks = true
		},
		"unknown pack": func(value *Submission) {
			value.Evidence[0].PackIDs = []string{"security@1"}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			value := exampleSubmission(t)
			mutate(&value)
			if _, err := EncodeSubmission(value); err == nil {
				t.Fatal("invalid submission was accepted")
			}
		})
	}
	_ = valid
}

func exampleSubmission(t *testing.T) Submission {
	t.Helper()
	snapshot, err := SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/standard-submission.json")
	if err != nil {
		t.Fatal(err)
	}
	var submission Submission
	if err := json.Unmarshal(contents, &submission); err != nil {
		t.Fatal(err)
	}
	return submission
}
