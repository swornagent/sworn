package runtime

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

// hostEvidenceWorkContext builds a WorkVerification production work context
// that projects recorded host-boundary evidence.
func hostEvidenceWorkContext(
	t *testing.T,
	manifest admittedManifest,
	coordinates dispatchCoordinates,
	evidence *productionHostEvidence,
) productionWorkContext {
	t.Helper()
	planBody := []byte("bounded plan bytes\n")
	receiptBody := []byte("{\"bounded\":\"receipt\"}\n")
	receiptDetail := []byte("bounded receipt detail\n")
	value := productionWorkContext{
		SchemaVersion:      productionWorkContextVersion,
		ManifestDigest:     manifest.digest,
		DriverConfigDigest: manifest.value.DriverConfigDigest,
		RunID:              manifest.value.RunID,
		Repository:         manifest.value.Authority.Project,
		Release:            manifest.value.Release,
		Intent:             manifest.value.Intent,
		InvocationID: dispatchInvocationID(
			manifest.value.RunID,
			coordinates,
		),
		Role:            driver.RoleVerifier,
		Track:           "T1",
		Slice:           coordinates.Slice,
		Responsibility:  coordinates.Responsibility,
		Attempt:         coordinates.BatonAttempt,
		Epoch:           coordinates.Epoch,
		Try:             coordinates.Try,
		Before:          "sha256:" + strings.Repeat("1", 64),
		WorkspaceAccess: driver.ReadOnly,
		Authority: productionAuthorityBinding{
			ReleaseRef: "refs/heads/release-wt/" +
				manifest.value.Release,
			ReleaseHead: strings.Repeat("2", 40),
			TargetRef:   manifest.value.TargetRef,
			TargetHead:  strings.Repeat("3", 40),
			TrackRef: "refs/heads/track/" +
				manifest.value.Release + "/T1",
			TrackHead: strings.Repeat("4", 40),
		},
		Plan: &productionPlanBinding{
			OID:      strings.Repeat("5", 40),
			Digest:   driver.Digest(planBody),
			Revision: 1,
			Input: driver.Input{
				Name: "plan", Path: productionPlanPath,
				Digest: driver.Digest(planBody),
			},
			body: planBody,
		},
		Receipt: &productionReceiptBinding{
			OID: strings.Repeat("6", 40),
			BodyInput: driver.Input{
				Name: "receipt", Path: productionReceiptPath,
				Digest: driver.Digest(receiptBody),
			},
			DetailInput: driver.Input{
				Name:   "receipt-detail",
				Path:   productionReceiptDetailPath,
				Digest: driver.Digest(receiptDetail),
			},
			body: receiptBody, detail: receiptDetail,
		},
		Candidate: &productionCandidateBinding{
			Receipt: strings.Repeat("6", 40),
			Commit:  strings.Repeat("7", 40),
			ProductTree: "sha256:" +
				strings.Repeat("8", 64),
		},
		Evidence: []productionEvidenceBinding{},
	}
	if evidence != nil {
		value.HostEvidence = evidence
	}
	return value
}

func TestHostEvidenceWorkContextValidatesProjectsAndDowngrades(t *testing.T) {
	t.Parallel()

	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	coordinates := dispatchCoordinates{
		Slice:          "S1",
		Responsibility: driver.WorkVerification,
		BatonAttempt:   2,
		Epoch:          3,
		Try:            1,
	}
	hostBody := []byte(`{"schema_version":"sworn.host-evidence/v1","slice":"S1"}` + "\n")
	evidence := &productionHostEvidence{
		SchemaVersion:  productionHostEvidenceVersion,
		Slice:          "S1",
		Candidate:      strings.Repeat("7", 40),
		ContractDigest: "sha256:" + strings.Repeat("a", 64),
		ManifestDigest: "sha256:" + strings.Repeat("b", 64),
		Results: []productionHostCheckResult{{
			Check: "go test ./...", Outcome: "pass",
			ExitCode: 0, OutputDigest: "sha256:" + strings.Repeat("c", 64),
			Output: "ok\n", HostEffect: "attempt/host/1/1",
		}},
		Input: driver.Input{
			Name:   "host-evidence",
			Path:   productionHostEvidencePath,
			Digest: driver.Digest(hostBody),
		},
		body: hostBody,
	}

	value := hostEvidenceWorkContext(t, manifest, coordinates, evidence)
	if err := validateProductionWorkContext(manifest, value); err != nil {
		t.Fatal(err)
	}

	// productionInputContents projects the host evidence bytes.
	contextBody := mustJSON(value)
	contents, err := productionInputContents(value, contextBody)
	if err != nil {
		t.Fatal(err)
	}
	foundHostInput := false
	for _, content := range contents {
		if content.Input.Path == productionHostEvidencePath {
			foundHostInput = true
			if driver.Digest(content.Bytes) != content.Input.Digest {
				t.Fatal("host evidence input digest mismatch")
			}
		}
	}
	if !foundHostInput {
		t.Fatal("host evidence input was not projected")
	}

	// productionRequestForContext includes the host evidence input.
	request, err := productionRequestForContext(manifest, value)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, input := range request.Inputs {
		if input.Path == productionHostEvidencePath {
			found = true
		}
	}
	if !found {
		t.Fatal("host evidence input is absent from the request")
	}

	// v1 downgrade strips the additive field.
	v1, err := productionWorkContextV1(manifest, value)
	if err != nil {
		t.Fatal(err)
	}
	if v1.HostEvidence != nil {
		t.Fatal("v1 downgrade retained host evidence")
	}

	// Mutations fail closed.
	for name, mutate := range map[string]func(*productionHostEvidence){
		"wrong slice": func(value *productionHostEvidence) {
			value.Slice = "S2"
		},
		"empty results": func(value *productionHostEvidence) {
			value.Results = nil
		},
		"bad contract digest": func(value *productionHostEvidence) {
			value.ContractDigest = "sha256:not-a-digest"
		},
		"missing host effect": func(value *productionHostEvidence) {
			value.Results[0].HostEffect = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			broken := *evidence
			mutate(&broken)
			brokenValue := hostEvidenceWorkContext(
				t, manifest, coordinates, &broken)
			if err := validateProductionWorkContext(
				manifest, brokenValue,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("mutated host evidence = %v", err)
			}
		})
	}

	// Host evidence is only valid for WorkVerification with a candidate.
	nonVerifier := coordinates
	nonVerifier.Responsibility = driver.CaptainReview
	nonVerifierValue := hostEvidenceWorkContext(
		t, manifest, nonVerifier, evidence)
	nonVerifierValue.Role = driver.RoleCaptain
	nonVerifierValue.Candidate = nil
	nonVerifierValue.InvocationID = dispatchInvocationID(
		manifest.value.RunID, nonVerifier)
	if err := validateProductionWorkContext(
		manifest, nonVerifierValue,
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("non-verifier host evidence = %v", err)
	}
}
