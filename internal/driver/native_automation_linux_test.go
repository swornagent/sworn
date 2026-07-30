//go:build linux

package driver

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNativeAutomationCertificateIsDisjointAndFailClosed(t *testing.T) {
	base, _, _ := memoryInvocationFixture(t)
	ref := "native-automation-credential"
	base.Selected.Profile.Network = NetworkRequired
	base.Selected.Profile.CredentialRef = &ref
	pair, err := nativeAutomationCertificationInvocations(base.Selected)
	if err != nil {
		t.Fatal(err)
	}
	config := NativeAdapterConfig{
		Family: ProfileCodex,
		CLI: ExecutableIdentity{
			Digest: Digest([]byte("native-automation-cli")),
		},
		CLIVersion: "test",
	}
	stage := func(
		invocation AutomationInvocation,
		expected nativeInvocationStage,
	) nativeSurfaceStageCertificate {
		t.Helper()
		actual, definitions, err := nativeAutomationSurface(invocation)
		if err != nil || actual != expected || len(definitions) != 1 {
			t.Fatalf(
				"surface = %d %#v, error = %v",
				actual,
				definitions,
				err,
			)
		}
		initialize, _ := canonicalJSON(map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name": "native-automation", "version": "1.0.0",
			},
		})
		empty, _ := canonicalJSON(map[string]any{})
		return nativeSurfaceStageCertificate{
			InvocationStage:       expected,
			ToolDigest:            nativeToolDefinitionsDigest(definitions),
			CaptureEvidenceDigest: Digest([]byte("capture")),
			ArgumentDigest: nativeAutomationCLIArgumentDigest(
				config.Family,
				invocation.Selected.Model,
				definitions,
			),
			AuthorityDigest: nativeAutomationAuthorityDigest(
				config.Family,
				invocation.Selected.Model,
				expected,
				definitions,
			),
			Protocol:           "2025-06-18",
			ClientName:         "native-automation",
			ClientVersion:      "1.0.0",
			InitializeDigest:   Digest(initialize),
			NotificationDigest: Digest(empty),
			ListDigest:         Digest(empty),
		}
	}
	certificate := nativeAutomationSurfaceCertificate{
		Family:              config.Family,
		ProfileDigest:       nativeProfileDigest(base.Selected.Profile),
		Model:               base.Selected.Model,
		AdapterConfigDigest: base.Selected.Adapter.ConfigurationDigest,
		ExecutableDigest:    config.CLI.Digest,
		CLIVersion:          config.CLIVersion,
		Recovery: stage(
			pair.Recovery,
			nativeInvocationStageRecovery,
		),
		Advisory: stage(
			pair.Advisory,
			nativeInvocationStageAdvisory,
		),
	}
	for _, invocation := range []AutomationInvocation{
		pair.Recovery,
		pair.Advisory,
	} {
		if err := validateNativeAutomationSurfaceCertificate(
			certificate,
			invocation,
			config,
		); err != nil {
			t.Fatalf("certificate rejected: %v", err)
		}
	}
	if certificate.Recovery.ToolDigest ==
		certificate.Advisory.ToolDigest {
		t.Fatal("recovery and advisory reused one tool certificate")
	}

	mutated := certificate
	mutated.Recovery = certificate.Advisory
	if err := validateNativeAutomationSurfaceCertificate(
		mutated,
		pair.Recovery,
		config,
	); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("cross-operation certificate error = %v", err)
	}
	mutated = certificate
	mutated.Recovery.ToolDigest = nativeToolSurfaceDigest(ReadOnly)
	if err := validateNativeAutomationSurfaceCertificate(
		mutated,
		pair.Recovery,
		config,
	); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("Baton tool certificate error = %v", err)
	}
	mutated = certificate
	mutated.Recovery.AuthorityDigest = ""
	if err := validateNativeAutomationSurfaceCertificate(
		mutated,
		pair.Recovery,
		config,
	); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
		t.Fatalf("authority certificate error = %v", err)
	}
}

func TestNativeAutomationSessionExposesOnlyItsOneTerminal(t *testing.T) {
	base, _, _ := memoryInvocationFixture(t)
	selection := ModelSelection{
		Profile: base.Selected.Profile.Key,
		Model:   base.Selected.Model,
	}
	invocation := AutomationInvocation{
		Selected: base.Selected,
		Recovery: pointerTo(recoveryInvocationFixture(selection)),
	}
	session, err := newNativeAutomationSession(time.Now(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	definitions := session.brokerToolDefinitions()
	if len(definitions) != 1 ||
		definitions[0].Name != "sworn_recovery_decide" {
		t.Fatalf("definitions = %#v", definitions)
	}
	answer := invocation.Recovery.Facts[3].Value
	arguments, err := json.Marshal(map[string]any{
		"decision": RecoveryDecision{
			SchemaVersion: RecoveryDecisionSchemaVersion,
			InvocationID:  invocation.Recovery.InvocationID,
			Action:        RecoveryResumeWorker,
			Answer:        &answer,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := session.execute(context.Background(), providerToolCall{
		ID:        "native-terminal-1",
		Name:      definitions[0].Name,
		Arguments: arguments,
	})
	terminated, terminalErr := session.terminated()
	if result.Failed || string(result.Content) != "accepted" ||
		!terminated || terminalErr != nil {
		t.Fatalf(
			"result = %#v, terminated=%t, error=%v",
			result,
			terminated,
			terminalErr,
		)
	}
	observation, err := session.complete(UsageReceipt{})
	if err != nil || observation.Recovery == nil ||
		observation.Recovery.Action != RecoveryResumeWorker ||
		observation.Advisory != nil {
		t.Fatalf("observation = %#v, error = %v", observation, err)
	}
}
