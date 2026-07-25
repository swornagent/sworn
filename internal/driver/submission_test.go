package driver

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"net"
	"testing"
)

func permissionFixture(
	t *testing.T,
	role Role,
	responsibility Responsibility,
) (SubmissionPermission, Request) {
	t.Helper()
	model := "selected-model"
	access := ReadWrite
	if role == RoleVerifier {
		access = ReadOnly
	}
	request, err := NewRequest(
		"invocation-submission",
		role,
		&model,
		Workspace{Path: "/workspace/project", Access: access},
		[]Input{{
			Name:   "status",
			Path:   ".sworn-inputs/v1/status.json",
			Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		true,
		Limits{TimeoutMillis: 60_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected := SelectedProvider{
		Provider: ProviderConfig{
			Key:           "fake",
			DriverID:      FakeDriverID,
			DriverVersion: FakeDriverVersion,
			Executable: ExecutableIdentity{
				Digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			},
			Network: NetworkNone,
		},
		Model: model,
	}
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(request, selected, containment, responsibility)
	if err != nil {
		t.Fatal(err)
	}
	return permission, request
}

func artifactFixture(t *testing.T, kind ArtifactKind) Artifact {
	t.Helper()
	artifact, err := NewArtifact(kind, []byte("exact "+string(kind)+" bytes\n"))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func decisionFixture(t *testing.T, outcome DecisionOutcome) *Decision {
	t.Helper()
	decision, err := NewDecision(outcome, []byte("digest-bound review evidence\n"))
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func TestEverySubmissionPermissionRowAcceptsOnlyItsExactShape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		role           Role
		responsibility Responsibility
		artifacts      []ArtifactKind
		decision       DecisionOutcome
	}{
		{"planner", RolePlanner, PlannerProposal, []ArtifactKind{ArtifactPlan}, ""},
		{"implementer design", RoleImplementer, ImplementerDesign, []ArtifactKind{ArtifactDesign, ArtifactWorkStatus}, ""},
		{"implementer proof", RoleImplementer, ImplementerImplementation, []ArtifactKind{ArtifactWorkProof, ArtifactWorkStatus}, ""},
		{"captain", RoleCaptain, CaptainReview, []ArtifactKind{ArtifactWorkStatus}, DecisionProceed},
		{"work verifier", RoleVerifier, WorkVerification, []ArtifactKind{ArtifactWorkStatus}, DecisionPass},
		{"assembly verifier", RoleVerifier, AssemblyVerification, []ArtifactKind{ArtifactAssemblyStatus}, DecisionBlocked},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			permission, request := permissionFixture(t, test.role, test.responsibility)
			submission := Submission{
				SchemaVersion: SubmissionSchemaVersion,
				InvocationID:  request.InvocationID,
			}
			for _, kind := range test.artifacts {
				submission.Artifacts = append(submission.Artifacts, artifactFixture(t, kind))
			}
			if test.decision != "" {
				submission.Decision = decisionFixture(t, test.decision)
			}
			body, err := EncodeSubmission(submission)
			if err != nil {
				t.Fatal(err)
			}
			server, err := NewSubmissionServer(permission)
			if err != nil {
				t.Fatal(err)
			}
			seal, sealBytes, err := server.Submit(body)
			if err != nil || !seal.Accepted || seal.Code != "accepted" ||
				seal.SubmissionDigest != Digest(body) {
				t.Fatalf("seal = %#v, bytes=%q, error=%v", seal, sealBytes, err)
			}
			replaySeal, replayBytes, err := server.Submit(body)
			if err != nil || replaySeal != seal || !bytes.Equal(replayBytes, sealBytes) {
				t.Fatal("exact replay did not return the exact seal")
			}
			conflict := append([]byte(nil), body...)
			conflict[len(conflict)-2] ^= 1
			if _, _, err := server.Submit(conflict); !IsCode(err, "SUBMISSION_CONFLICT") {
				t.Fatalf("conflict error = %v", err)
			}
		})
	}
}

func TestSubmissionRejectsSwappedIdentityOrderDecisionAndBytes(t *testing.T) {
	t.Parallel()
	permission, request := permissionFixture(t, RoleImplementer, ImplementerDesign)
	valid := Submission{
		SchemaVersion: SubmissionSchemaVersion,
		InvocationID:  request.InvocationID,
		Artifacts: []Artifact{
			artifactFixture(t, ArtifactDesign),
			artifactFixture(t, ArtifactWorkStatus),
		},
	}
	tests := map[string]Submission{
		"invocation": func() Submission {
			value := valid
			value.InvocationID = "another-invocation"
			return value
		}(),
		"order": func() Submission {
			value := valid
			value.Artifacts = []Artifact{valid.Artifacts[1], valid.Artifacts[0]}
			return value
		}(),
		"decision": func() Submission {
			value := valid
			value.Decision = decisionFixture(t, DecisionProceed)
			return value
		}(),
		"digest": func() Submission {
			value := valid
			value.Artifacts = append([]Artifact(nil), valid.Artifacts...)
			value.Artifacts[0].Digest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			return value
		}(),
	}
	for name, submission := range tests {
		name := name
		submission := submission
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body, encodeErr := EncodeSubmission(submission)
			if name == "digest" {
				if !IsCode(encodeErr, "INVALID_ARTIFACT") {
					t.Fatalf("encode error = %v", encodeErr)
				}
				return
			}
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			server, err := NewSubmissionServer(permission)
			if err != nil {
				t.Fatal(err)
			}
			seal, _, err := server.Submit(body)
			if !IsCode(err, "SUBMISSION_REJECTED") || seal.Accepted {
				t.Fatalf("seal = %#v, error = %v", seal, err)
			}
			replaySeal, _, replayErr := server.Submit(body)
			if !IsCode(replayErr, "SUBMISSION_REJECTED") || replaySeal != seal {
				t.Fatalf("replay seal = %#v, error = %v", replaySeal, replayErr)
			}
		})
	}
}

func TestSubmissionCodecRejectsNoncanonicalDuplicateAndBadBase64(t *testing.T) {
	t.Parallel()
	submission := Submission{
		SchemaVersion: SubmissionSchemaVersion,
		InvocationID:  "invocation-submission",
		Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
		Decision:      nil,
	}
	body, err := EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSubmission(append([]byte(" "), body...)); !IsCode(err, "NONCANONICAL_JSON") {
		t.Fatalf("whitespace error = %v", err)
	}
	duplicate := bytes.Replace(body,
		[]byte(`"schema_version":"sworn.submission/v1"`),
		[]byte(`"schema_version":"sworn.submission/v1","schema_version":"sworn.submission/v1"`),
		1)
	if _, err := DecodeSubmission(duplicate); !IsCode(err, "DUPLICATE_NAME") {
		t.Fatalf("duplicate error = %v", err)
	}
	bad := submission
	bad.Artifacts = append([]Artifact(nil), submission.Artifacts...)
	bad.Artifacts[0].Bytes = base64.RawStdEncoding.EncodeToString([]byte("exact plan bytes\n"))
	if err := ValidateSubmission(bad); !IsCode(err, "INVALID_ARTIFACT") {
		t.Fatalf("base64 error = %v", err)
	}
}

func TestFrameCodecIsCanonicalBoundedAndComplete(t *testing.T) {
	t.Parallel()
	payload := []byte("{\"a\":1}\n")
	frame, err := EncodeFrame(payload)
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(frame[:4]) != uint32(len(payload)) ||
		!bytes.Equal(frame[4:], payload) {
		t.Fatal("frame does not bind its payload length")
	}
	decoded, err := ReadFrame(bytes.NewReader(frame))
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded = %q, %v", decoded, err)
	}
	for name, frame := range map[string][]byte{
		"zero":    {0, 0, 0, 0},
		"partial": {0, 0, 0, 5, '{'},
	} {
		if _, err := ReadFrame(bytes.NewReader(frame)); err == nil {
			t.Fatalf("%s frame was accepted", name)
		}
	}
	if _, err := EncodeFrame([]byte("{ \"a\":1}\n")); !IsCode(err, "NONCANONICAL_JSON") {
		t.Fatalf("noncanonical error = %v", err)
	}
}

func TestEndpointDescribeSubmitAndCrossInvocationRefusal(t *testing.T) {
	t.Parallel()
	permission, request := permissionFixture(t, RolePlanner, PlannerProposal)
	server, err := NewSubmissionServer(permission)
	if err != nil {
		t.Fatal(err)
	}
	parent, child := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- serveSubmissionEndpoint(parent, server)
	}()
	client, err := NewEndpointClient(child, request.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := client.Describe()
	if err != nil || descriptor.InvocationID != request.InvocationID ||
		descriptor.Responsibility != PlannerProposal {
		t.Fatalf("descriptor = %#v, %v", descriptor, err)
	}
	body, err := EncodeSubmission(Submission{
		SchemaVersion: SubmissionSchemaVersion,
		InvocationID:  request.InvocationID,
		Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
	})
	if err != nil {
		t.Fatal(err)
	}
	seal, _, err := client.Submit(body)
	if err != nil || !seal.Accepted {
		t.Fatalf("seal = %#v, %v", seal, err)
	}
	_ = child.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	parent, child = net.Pipe()
	server, err = NewSubmissionServer(permission)
	if err != nil {
		t.Fatal(err)
	}
	done = make(chan error, 1)
	go func() {
		done <- serveSubmissionEndpoint(parent, server)
	}()
	foreign, err := NewEndpointClient(child, "foreign-invocation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.Describe(); err == nil {
		t.Fatal("foreign invocation described the permission")
	}
	_ = child.Close()
	if err := <-done; !IsCode(err, "SUBMISSION_BINDING_MISMATCH") {
		t.Fatalf("server error = %v", err)
	}
}

func TestEndpointRejectsTrailingPartialFrameAfterAcceptedSubmission(t *testing.T) {
	t.Parallel()
	permission, request := permissionFixture(t, RolePlanner, PlannerProposal)
	server, err := NewSubmissionServer(permission)
	if err != nil {
		t.Fatal(err)
	}
	parent, child := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- serveSubmissionEndpoint(parent, server)
	}()
	client, err := NewEndpointClient(child, request.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := EncodeSubmission(Submission{
		SchemaVersion: SubmissionSchemaVersion,
		InvocationID:  request.InvocationID,
		Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Submit(body); err != nil {
		t.Fatal(err)
	}
	if _, err := child.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	_ = child.Close()
	if err := <-done; !IsCode(err, "PARTIAL_FRAME") {
		t.Fatalf("server error = %v", err)
	}
}
