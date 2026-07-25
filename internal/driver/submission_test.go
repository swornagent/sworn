package driver

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

type shortWriter struct{}

func (shortWriter) Write(body []byte) (int, error) { return len(body) - 1, nil }

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
			Path:   "status.json",
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
			server, err := newSubmissionServer(permission)
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
			conflictSeal, conflictBytes, err := server.Submit(conflict)
			if !IsCode(err, "SUBMISSION_CONFLICT") ||
				conflictSeal != seal || !bytes.Equal(conflictBytes, sealBytes) {
				t.Fatalf("conflict seal = %#v, bytes=%q, error=%v", conflictSeal, conflictBytes, err)
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
			server, err := newSubmissionServer(permission)
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

func TestSubmissionPermissionRejectsEqualOperationFromOldPackageLineage(t *testing.T) {
	t.Parallel()
	permission, _ := permissionFixture(t, RolePlanner, PlannerProposal)
	permission.descriptor.Package.Version = "1.0.0-rc.2"
	if _, err := permission.Describe(); !IsCode(err, "INVALID_PERMISSION") {
		t.Fatalf("old package permission error = %v", err)
	}
	permission, _ = permissionFixture(t, RolePlanner, PlannerProposal)
	permission.descriptor.Package.Commit = "0136a96c4355e60c815b5cab043b54e860d00062"
	if _, err := newSubmissionServer(permission); !IsCode(err, "INVALID_PERMISSION") {
		t.Fatalf("old lineage server error = %v", err)
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
	if err := WriteFrame(shortWriter{}, payload); !IsCode(err, "FRAME_WRITE_FAILED") {
		t.Fatalf("short write error = %v", err)
	}
}

func endpointFixture(t *testing.T) (*terminalArbiter, Request, []byte) {
	t.Helper()
	permission, request := permissionFixture(t, RolePlanner, PlannerProposal)
	server, err := newSubmissionServer(permission)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := permission.Describe()
	if err != nil {
		t.Fatal(err)
	}
	arbiter := newTerminalArbiter(Invocation{
		Request: request,
		Selected: SelectedProvider{
			Provider: ProviderConfig{
				DriverID:      descriptor.DriverID,
				DriverVersion: descriptor.DriverVersion,
			},
			Model: descriptor.Model,
		},
	}, server)
	result, err := RunFake(request, FakeCompleted, false)
	if err != nil {
		t.Fatal(err)
	}
	resultBody, err := EncodeResult(result)
	if err != nil {
		t.Fatal(err)
	}
	return arbiter, request, resultBody
}

func acknowledgedEndpoint(t *testing.T) *terminalArbiter {
	t.Helper()
	arbiter, request, resultBody := endpointFixture(t)
	parent, child := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- serveSubmissionEndpoint(parent, arbiter) }()
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
	submitted := make(chan error, 1)
	go func() {
		_, _, submitErr := client.Submit(body)
		submitted <- submitErr
	}()
	_, _ = arbiter.Write(resultBody)
	if err := <-submitted; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = child.Close()
	return arbiter
}

func waitForSubmitAttempt(t *testing.T, arbiter *terminalArbiter) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		arbiter.mu.Lock()
		attempted := arbiter.attempted
		arbiter.mu.Unlock()
		if attempted {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("submit attempt did not linearize")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEndpointBlocksSubmitUntilExactlyOneCompletedResult(t *testing.T) {
	t.Parallel()
	for _, resultFirst := range []bool{false, true} {
		resultFirst := resultFirst
		t.Run(map[bool]string{false: "submit-first", true: "result-first"}[resultFirst], func(t *testing.T) {
			arbiter, request, resultBody := endpointFixture(t)
			parent, child := net.Pipe()
			done := make(chan error, 1)
			go func() { done <- serveSubmissionEndpoint(parent, arbiter) }()
			client, err := NewEndpointClient(child, request.InvocationID)
			if err != nil {
				t.Fatal(err)
			}
			descriptor, err := client.Describe()
			if err != nil || descriptor.Package.Version != "1.0.0-rc.3" {
				t.Fatalf("descriptor = %#v, error = %v", descriptor, err)
			}
			body, err := EncodeSubmission(Submission{
				SchemaVersion: SubmissionSchemaVersion,
				InvocationID:  request.InvocationID,
				Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
			})
			if err != nil {
				t.Fatal(err)
			}
			if resultFirst {
				_, _ = arbiter.Write(resultBody)
			}
			submitted := make(chan error, 1)
			go func() {
				seal, _, submitErr := client.Submit(body)
				if submitErr == nil && !seal.Accepted {
					submitErr = fail("SUBMISSION_REJECTED")
				}
				submitted <- submitErr
			}()
			if !resultFirst {
				waitForSubmitAttempt(t, arbiter)
				select {
				case err := <-submitted:
					t.Fatalf("submit acknowledged before result: %v", err)
				default:
				}
				_, _ = arbiter.Write(resultBody)
			}
			if err := <-submitted; err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if arbiter.terminalError() != nil {
				t.Fatal(arbiter.terminalError())
			}
			_ = child.Close()
		})
	}
}

func TestEndpointFirstAttemptRejectsAndForeignInvocationFailsClosed(t *testing.T) {
	t.Parallel()
	arbiter, request, resultBody := endpointFixture(t)
	parent, child := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- serveSubmissionEndpoint(parent, arbiter) }()
	client, _ := NewEndpointClient(child, request.InvocationID)
	submitted := make(chan error, 1)
	go func() {
		_, _, err := client.Submit([]byte("{}\n"))
		submitted <- err
	}()
	waitForSubmitAttempt(t, arbiter)
	select {
	case err := <-submitted:
		t.Fatalf("rejection acknowledged before result: %v", err)
	default:
	}
	_, _ = arbiter.Write(resultBody)
	if err := <-submitted; !IsCode(err, "SUBMISSION_REJECTED") {
		t.Fatalf("rejection error = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = child.Close()

	arbiter, _, _ = endpointFixture(t)
	parent, child = net.Pipe()
	done = make(chan error, 1)
	go func() { done <- serveSubmissionEndpoint(parent, arbiter) }()
	foreign, _ := NewEndpointClient(child, "foreign-invocation")
	if _, err := foreign.Describe(); err == nil {
		t.Fatal("foreign invocation described the permission")
	}
	_ = child.Close()
	if err := <-done; !IsCode(err, "SUBMISSION_BINDING_MISMATCH") {
		t.Fatalf("server error = %v", err)
	}
}

func TestEndpointRejectsPartialFrameAndPostResultStdout(t *testing.T) {
	t.Parallel()
	arbiter, _, _ := endpointFixture(t)
	parent, child := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- serveSubmissionEndpoint(parent, arbiter) }()
	go func() {
		_, _ = child.Write([]byte{0})
		_ = child.Close()
	}()
	if err := <-done; !IsCode(err, "PARTIAL_FRAME") {
		t.Fatalf("server error = %v", err)
	}

	arbiter = acknowledgedEndpoint(t)
	_, _ = arbiter.Write([]byte("{}\n"))
	if err := arbiter.terminalError(); !IsCode(err, "PROTOCOL_FAILURE") {
		t.Fatalf("post-result error = %v", err)
	}
}

func TestTerminalArbiterCancellationAndPublicationShareOneGate(t *testing.T) {
	t.Parallel()
	t.Run("cancellation first", func(t *testing.T) {
		arbiter := acknowledgedEndpoint(t)
		arbiter.processDone(nil, false)
		arbiter.cancel("invocation_cancelled", fail("INVOCATION_CANCELLED"), fatalCancellation)
		arbiter.publish(nil)
		observation, err := arbiter.observation()
		if !IsCode(err, "INVOCATION_CANCELLED") || observation.Handoff != nil {
			t.Fatalf("observation=%#v error=%v", observation, err)
		}
	})
	t.Run("publication first", func(t *testing.T) {
		arbiter := acknowledgedEndpoint(t)
		arbiter.processDone(nil, false)
		arbiter.publish(nil)
		arbiter.cancel("invocation_cancelled", fail("INVOCATION_CANCELLED"), fatalCancellation)
		observation, err := arbiter.observation()
		if err != nil || observation.Handoff == nil {
			t.Fatalf("observation=%#v error=%v", observation, err)
		}
	})
	for _, stage := range []string{
		"before-submit",
		"after-submit-before-result",
		"after-result-before-submit",
	} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			arbiter, request, resultBody := endpointFixture(t)
			body, err := EncodeSubmission(Submission{
				SchemaVersion: SubmissionSchemaVersion,
				InvocationID:  request.InvocationID,
				Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
			})
			if err != nil {
				t.Fatal(err)
			}
			submitted := make(chan error, 1)
			startSubmit := func() {
				go func() {
					_, _, submitErr := arbiter.submit(body)
					submitted <- submitErr
				}()
			}
			switch stage {
			case "before-submit":
				arbiter.cancel("invocation_cancelled", fail("INVOCATION_CANCELLED"), fatalCancellation)
				startSubmit()
			case "after-submit-before-result":
				startSubmit()
				waitForSubmitAttempt(t, arbiter)
				arbiter.cancel("invocation_cancelled", fail("INVOCATION_CANCELLED"), fatalCancellation)
			case "after-result-before-submit":
				_, _ = arbiter.Write(resultBody)
				arbiter.cancel("invocation_cancelled", fail("INVOCATION_CANCELLED"), fatalCancellation)
				startSubmit()
			}
			if err := <-submitted; !IsCode(err, "INVOCATION_CANCELLED") {
				t.Fatalf("submit error = %v", err)
			}
		})
	}
}

func TestTerminalArbiterOverflowRacingSubmitCannotPublish(t *testing.T) {
	t.Parallel()
	arbiter, request, _ := endpointFixture(t)
	arbiter.outputMaximum = 1
	body, err := EncodeSubmission(Submission{
		SchemaVersion: SubmissionSchemaVersion,
		InvocationID:  request.InvocationID,
		Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
	})
	if err != nil {
		t.Fatal(err)
	}
	submitted := make(chan error, 1)
	go func() {
		_, _, submitErr := arbiter.submit(body)
		submitted <- submitErr
	}()
	waitForSubmitAttempt(t, arbiter)
	_, _ = arbiter.Write([]byte("{}\n"))
	if err := <-submitted; !IsCode(err, "OUTPUT_OVERFLOW") {
		t.Fatalf("submit error = %v", err)
	}
	arbiter.publish(nil)
	if observation, err := arbiter.observation(); err == nil || observation.Handoff != nil {
		t.Fatalf("observation=%#v error=%v", observation, err)
	}
}

func TestTerminalArbiterRefusesEveryMissingInvalidOrExtraBoundResult(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*terminalArbiter, []byte){
		"missing": func(arbiter *terminalArbiter, _ []byte) {
			arbiter.processDone(nil, false)
		},
		"partial": func(arbiter *terminalArbiter, valid []byte) {
			_, _ = arbiter.Write(valid[:len(valid)-1])
			arbiter.processDone(nil, false)
		},
		"malformed": func(arbiter *terminalArbiter, _ []byte) {
			_, _ = arbiter.Write([]byte("{\n"))
		},
		"duplicate": func(arbiter *terminalArbiter, valid []byte) {
			_, _ = arbiter.Write(append(append([]byte(nil), valid...), valid...))
		},
		"mismatched": func(arbiter *terminalArbiter, valid []byte) {
			result, _ := DecodeResult(valid, ResultBinding{})
			result.InvocationID = "different-invocation"
			body, _ := EncodeResult(result)
			_, _ = arbiter.Write(body)
		},
		"non-completed": func(arbiter *terminalArbiter, valid []byte) {
			result, _ := DecodeResult(valid, ResultBinding{})
			result.TransportStatus = TransportError
			body, _ := EncodeResult(result)
			_, _ = arbiter.Write(body)
		},
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			arbiter, request, resultBody := endpointFixture(t)
			submissionBody, err := EncodeSubmission(Submission{
				SchemaVersion: SubmissionSchemaVersion,
				InvocationID:  request.InvocationID,
				Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
			})
			if err != nil {
				t.Fatal(err)
			}
			submitted := make(chan error, 1)
			go func() {
				_, _, submitErr := arbiter.submit(submissionBody)
				submitted <- submitErr
			}()
			mutate(arbiter, resultBody)
			if err := <-submitted; err == nil {
				t.Fatal("invalid result released a submission")
			}
			arbiter.publish(nil)
			if observation, err := arbiter.observation(); err == nil ||
				observation.Handoff != nil {
				t.Fatalf("observation=%#v error=%v", observation, err)
			}
		})
	}
}
