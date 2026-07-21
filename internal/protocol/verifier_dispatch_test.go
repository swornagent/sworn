package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestVerifierDispatchMatchesBatonSnapshotAndDerivesBindings(t *testing.T) {
	t.Parallel()

	contents := testSnapshotBytes(t, "examples/artifacts/dispatch/verifier-run-1.json")
	dispatch, err := ParseVerifierDispatch(contents)
	if err != nil {
		t.Fatal(err)
	}
	if dispatch.DispatchID != "verifier-run-1" || dispatch.Role != "verifier" ||
		!dispatch.FreshContext || dispatch.BuilderTranscriptIncluded ||
		dispatch.TargetRefWritable || dispatch.RemotesPresent || dispatch.WriteCredentialsPresent {
		t.Fatalf("dispatch = %#v", dispatch)
	}
	const (
		wantRawDigest       = "sha256:25d5e84ec61e8c72c25b257e62d1397cd313cebd97d2038fa783f9926b22bf22"
		wantCanonicalDigest = "sha256:eb84857c04903c647ba2f29fb038409eca798abd85617ce325aad30a593eedb2"
	)
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		t.Fatal(err)
	}
	if RawDigest(contents) != wantRawDigest || CanonicalDigest(canonical) != wantCanonicalDigest ||
		wantRawDigest == wantCanonicalDigest {
		t.Fatal("dispatch raw-artifact and canonical-record digests were conflated")
	}

	submission, err := ParseSubmission(testSnapshotBytes(t, "examples/standard-submission.json"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := BuildVerifierDispatch(VerifierDispatchInput{
		Submission: submission,
		DispatchID: dispatch.DispatchID,
		Workspace:  dispatch.Workspace,
		CreatedAt:  dispatch.CreatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != ControlReceiptSchemaVersion || record.Digest != wantCanonicalDigest ||
		!bytes.Equal(record.CanonicalJSON, canonical) {
		t.Fatalf("record = %#v, want exact canonical snapshot", record)
	}

	tooEarly := VerifierDispatchInput{
		Submission: submission,
		DispatchID: "verifier-run-2",
		Workspace:  "fresh-read-only-materialization",
		CreatedAt:  "2026-07-19T00:04:59Z",
	}
	if _, err := BuildVerifierDispatch(tooEarly); err == nil {
		t.Fatal("dispatch before submission was accepted")
	}
	reusedBuilder := tooEarly
	reusedBuilder.DispatchID = submission.View().Builder.RunID
	reusedBuilder.CreatedAt = dispatch.CreatedAt
	if _, err := BuildVerifierDispatch(reusedBuilder); err == nil {
		t.Fatal("dispatch reusing the builder run was accepted")
	}
	if _, err := BuildVerifierDispatch(VerifierDispatchInput{}); err == nil {
		t.Fatal("dispatch without an exact submission was accepted")
	}
}

func TestParseVerifierDispatchRejectsMissingAndMutatedIsolationFacts(t *testing.T) {
	t.Parallel()

	contents := testSnapshotBytes(t, "examples/artifacts/dispatch/verifier-run-1.json")
	for _, field := range []string{
		"fresh_context", "builder_transcript_included", "target_ref_writable",
		"remotes_present", "write_credentials_present",
	} {
		field := field
		t.Run("missing "+field, func(t *testing.T) {
			t.Parallel()
			root := testJSONObject(t, contents)
			delete(root, field)
			if _, err := ParseVerifierDispatch(testJSONBytes(t, root)); err == nil {
				t.Fatal("missing required isolation fact was accepted")
			}
		})
	}
	mutations := map[string]func(map[string]any){
		"fresh context false": func(root map[string]any) {
			root["fresh_context"] = false
		},
		"builder transcript included": func(root map[string]any) {
			root["builder_transcript_included"] = true
		},
		"target writable": func(root map[string]any) {
			root["target_ref_writable"] = true
		},
		"remotes present": func(root map[string]any) {
			root["remotes_present"] = true
		},
		"write credentials present": func(root map[string]any) {
			root["write_credentials_present"] = true
		},
		"wrong role": func(root map[string]any) {
			root["role"] = "builder"
		},
		"wrong kind": func(root map[string]any) {
			root["kind"] = "authority_approval"
		},
		"wrong schema": func(root map[string]any) {
			root["schema_version"] = "control-receipt-v2"
		},
		"unknown root": func(root map[string]any) {
			root["profile"] = "trusted"
		},
		"missing candidate commit": func(root map[string]any) {
			delete(testNestedObject(t, root, "candidate"), "commit")
		},
		"unknown candidate field": func(root map[string]any) {
			testNestedObject(t, root, "candidate")["ref"] = "refs/heads/main"
		},
		"uppercase candidate oid": func(root map[string]any) {
			testNestedObject(t, root, "candidate")["tree"] = strings.Repeat("C", 40)
		},
		"blank workspace": func(root map[string]any) {
			root["workspace"] = " \t"
		},
		"leap second": func(root map[string]any) {
			root["created_at"] = "2026-07-19T00:05:60Z"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testJSONObject(t, contents)
			mutate(root)
			if _, err := ParseVerifierDispatch(testJSONBytes(t, root)); err == nil {
				t.Fatal("invalid dispatch was accepted")
			}
		})
	}
}

func TestParseVerifierDispatchRejectsHostileJSONAndCeiling(t *testing.T) {
	t.Parallel()

	contents := testSnapshotBytes(t, "examples/artifacts/dispatch/verifier-run-1.json")
	duplicate := append([]byte(`{"schema_version":"control-receipt-v1",`), contents[1:]...)
	for name, value := range map[string][]byte{
		"duplicate key":  duplicate,
		"trailing value": append(append([]byte(nil), contents...), []byte(` []`)...),
		"lone surrogate": []byte(`{"value":"\ud800"}`),
		"unsafe integer": []byte(`{"value":9007199254740992}`),
		"non-finite":     []byte(`{"value":1e9999}`),
		"over ceiling":   bytes.Repeat([]byte{' '}, MaximumControlReceiptBytes+1),
	} {
		if _, err := ParseVerifierDispatch(value); err == nil {
			t.Errorf("%s input was accepted", name)
		}
	}
}
