package adapter

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

const (
	codexVerifierPermissionProfile = "sworn_verifier"
	codexVerifierSchemaInput       = "assessment-schema"
	codexVerifierPlanInput         = "plan"
	codexVerifierSubmissionInput   = "submission"
	codexVerifierDispatchInput     = "dispatch"
	codexVerifierReviewInputPrefix = "review-"
	codexVerifierMaximumInputs     = 256
)

// CodexVerifierOptions contains the only deployment-selected facts owned by
// the native Codex adapter. The engine completes the process-neutral profile
// with repository, executor, workspace-root, and materialization facts before
// it authorizes an invocation.
type CodexVerifierOptions struct {
	BinaryPath string
	Model      string
	Timeout    time.Duration
}

// CodexVerifier is the immutable native CLI adapter. It realizes the
// protocol-owned Codex argv and delegates the protocol-owned completion
// grammar, but has no Store, scheduler, artifact, or verdict authority.
type CodexVerifier struct {
	profile protocol.VerifierProfile
}

var _ effects.VerifierAdapter = CodexVerifier{}

// NewCodexVerifier admits only the pinned native Codex executable and an
// explicit model. It neither discovers a binary nor supplies a model default.
func NewCodexVerifier(ctx context.Context, options CodexVerifierOptions) (CodexVerifier, error) {
	if err := validateCodexVerifierOptions(options); err != nil {
		return CodexVerifier{}, err
	}
	if err := validatePinnedCodexBinary(ctx, options.BinaryPath); err != nil {
		return CodexVerifier{}, err
	}
	return configureCodexVerifier(options)
}

func configureCodexVerifier(options CodexVerifierOptions) (CodexVerifier, error) {
	if err := validateCodexVerifierOptions(options); err != nil {
		return CodexVerifier{}, err
	}
	outputSchemaDigest, err := protocol.VerifierAssessmentOutputSchemaDigest()
	if err != nil {
		return CodexVerifier{}, fmt.Errorf("measure verifier assessment output schema: %w", err)
	}
	toolSchemaDigest, err := codexBuilderToolSchemaDigest()
	if err != nil {
		return CodexVerifier{}, fmt.Errorf("load pinned Codex verifier tool schema: %w", err)
	}
	argv := protocol.CanonicalCodexVerifierArgv(options.Model)
	return CodexVerifier{profile: protocol.VerifierProfile{
		SchemaVersion:             protocol.VerifierProfileSchemaVersion,
		Agent:                     pinnedCodexVersion,
		BinaryPath:                options.BinaryPath,
		BinaryVersion:             pinnedCodexVersion,
		BinaryDigest:              pinnedCodexDigest,
		BinarySize:                pinnedCodexSize,
		ExecutableInput:           codexExecutableInput,
		Provider:                  codexProvider,
		Authentication:            codexAuthentication,
		CredentialHome:            codexHome,
		PermissionProfile:         codexVerifierPermissionProfile,
		Model:                     options.Model,
		ToolSchemaDigest:          toolSchemaDigest,
		Argv:                      argv,
		EnvironmentNames:          []string{},
		PromptDigest:              protocol.RawDigest([]byte(protocol.NativeCodexVerifierPrompt)),
		OutputSchemaDigest:        outputSchemaDigest,
		TimeoutNanoseconds:        options.Timeout.Nanoseconds(),
		Network:                   string(executor.NetworkHost),
		WorkspaceAccess:           string(executor.WorkspaceReadOnly),
		NestedSandbox:             true,
		CredentialAccess:          true,
		ModelToolNetwork:          false,
		ModelToolCredentialAccess: false,
	}}, nil
}

func validateCodexVerifierOptions(options CodexVerifierOptions) error {
	if options.BinaryPath == "" {
		return errors.New("Codex verifier binary path is required")
	}
	if !protocol.ValidID(options.Model) {
		return errors.New("Codex verifier model must be an explicit safe identifier")
	}
	if options.Timeout <= 0 {
		return errors.New("Codex verifier timeout must be positive")
	}
	return nil
}

// Profile returns a defensive copy of the adapter-owned profile fields. The
// process-neutral verifier worker closes its remaining deployment fields and
// obtains the final protocol digest before Store dispatch authorization.
func (verifier CodexVerifier) Profile() protocol.VerifierProfile {
	profile := verifier.profile
	profile.Argv = slices.Clone(profile.Argv)
	profile.EnvironmentNames = slices.Clone(profile.EnvironmentNames)
	return profile
}

// Invocation converts one engine-derived attempt and fresh candidate
// materialization into the sole accepted credentialed read-only process. The
// worker supplies exact engine records and output schema; this adapter adds the
// pinned executable and fixes input order for stable receipts.
func (verifier CodexVerifier) Invocation(
	identity engine.VerifierAttemptIdentity,
	workspace repo.CandidateWorkspace,
	engineInputs []executor.Input,
) (executor.Invocation, error) {
	if _, err := engine.EncodeVerifierAttemptIdentity(identity); err != nil {
		return executor.Invocation{}, fmt.Errorf("validate Codex verifier attempt: %w", err)
	}
	profile := verifier.Profile()
	if identity.Agent != profile.Agent {
		return executor.Invocation{}, errors.New("Codex verifier attempt agent does not match the pinned profile")
	}
	if workspace.Path() == "" || workspace.Manifest() == "" || workspace.RepositoryID() == "" {
		return executor.Invocation{}, errors.New("Codex verifier requires a fresh exact candidate workspace")
	}
	inputs, err := verifierInputs(profile, engineInputs)
	if err != nil {
		return executor.Invocation{}, err
	}
	return executor.Invocation{
		SchemaVersion:    executor.InvocationSchemaVersion,
		ID:               identity.InvocationID,
		Role:             "verifier",
		NestedSandbox:    true,
		CredentialAccess: true,
		Workspace:        workspace.Path(),
		WorkspaceDigest:  workspace.Manifest(),
		WorkspaceAccess:  executor.WorkspaceReadOnly,
		ExecutableInput:  codexExecutableInput,
		Inputs:           inputs,
		Argv:             slices.Clone(profile.Argv),
		Environment:      nil,
		Network:          executor.NetworkHost,
		Timeout:          time.Duration(profile.TimeoutNanoseconds),
	}, nil
}

func verifierInputs(profile protocol.VerifierProfile, engineInputs []executor.Input) ([]executor.Input, error) {
	want := []string{
		codexVerifierSchemaInput,
		codexVerifierPlanInput,
		codexVerifierSubmissionInput,
		codexVerifierDispatchInput,
	}
	if len(engineInputs) < len(want) || len(engineInputs) >= codexVerifierMaximumInputs {
		return nil, errors.New("Codex verifier requires schema, plan, submission, dispatch, and bounded review inputs")
	}
	byName := make(map[string]executor.Input, len(engineInputs))
	for _, input := range engineInputs {
		if _, exists := byName[input.Name]; exists {
			return nil, fmt.Errorf("Codex verifier input %q is duplicated", input.Name)
		}
		if !protocol.ValidID(input.Name) ||
			(!slices.Contains(want, input.Name) && !validCodexVerifierReviewInput(input.Name)) ||
			!filepath.IsAbs(input.Path) || filepath.Clean(input.Path) != input.Path ||
			!protocol.ValidDigest(input.Digest) {
			return nil, fmt.Errorf("Codex verifier input %q is not exact", input.Name)
		}
		byName[input.Name] = input
	}
	for _, name := range want {
		input, exists := byName[name]
		if !exists {
			return nil, fmt.Errorf("Codex verifier input %q is absent", name)
		}
		if name == codexVerifierSchemaInput && input.Digest != profile.OutputSchemaDigest {
			return nil, errors.New("Codex verifier output schema does not match the engine-owned schema")
		}
	}
	inputs := make([]executor.Input, 0, len(engineInputs)+1)
	inputs = append(inputs, executor.Input{
		Name: codexExecutableInput, Path: profile.BinaryPath, Digest: profile.BinaryDigest,
	})
	inputs = append(inputs, engineInputs...)
	sort.Slice(inputs, func(left, right int) bool { return inputs[left].Name < inputs[right].Name })
	return inputs, nil
}

func validCodexVerifierReviewInput(name string) bool {
	return strings.HasPrefix(name, codexVerifierReviewInputPrefix) &&
		len(name) > len(codexVerifierReviewInputPrefix)
}

// ParseCompletion accepts only one ordinary bounded Codex turn and returns the
// exact model-owned assessment text plus its fresh thread identity. JSONL is
// the authoritative output channel: Codex's best-effort -o file is not used.
func (verifier CodexVerifier) ParseCompletion(
	completion executor.RawCompletion,
) (effects.VerifierAdapterCompletion, error) {
	if completion.ExitCode != 0 || completion.Cancelled || completion.TimedOut || completion.OutputTruncated {
		return effects.VerifierAdapterCompletion{}, errors.New("Codex verifier did not reach an ordinary bounded completion")
	}
	if !engine.ValidID(completion.InvocationID) || !protocol.ValidDigest(completion.WorkspaceDigest) ||
		completion.WorkspaceAccess != executor.WorkspaceReadOnly || !completion.CredentialAccess ||
		completion.ExecutableInput != codexExecutableInput || completion.RuntimeDigest != "" ||
		completion.Export != nil || completion.StartedAt.IsZero() || completion.CompletedAt.IsZero() ||
		completion.CompletedAt.Before(completion.StartedAt) {
		return effects.VerifierAdapterCompletion{}, errors.New("Codex verifier completion does not match the read-only credentialed invocation")
	}
	if err := verifier.validateCompletionInputs(completion.Inputs); err != nil {
		return effects.VerifierAdapterCompletion{}, err
	}
	turn, err := protocol.ParseNativeCodexVerifierJSONL(completion.Stdout)
	if err != nil {
		return effects.VerifierAdapterCompletion{}, err
	}
	if _, err := protocol.ParseVerifierAssessment(turn.Assessment); err != nil {
		return effects.VerifierAdapterCompletion{}, fmt.Errorf("parse Codex verifier assessment: %w", err)
	}
	return effects.VerifierAdapterCompletion{Assessment: turn.Assessment, ThreadID: turn.ThreadID}, nil
}

func (verifier CodexVerifier) validateCompletionInputs(inputs []executor.BoundInput) error {
	profile := verifier.Profile()
	if len(inputs) < 5 || len(inputs) > codexVerifierMaximumInputs {
		return errors.New("Codex verifier completion does not bind the bounded review inputs")
	}
	byName := make(map[string]executor.BoundInput, len(inputs))
	for index, input := range inputs {
		if _, exists := byName[input.Name]; exists || !protocol.ValidID(input.Name) ||
			!protocol.ValidDigest(input.Digest) || input.Size == 0 ||
			(input.Name != codexExecutableInput &&
				input.Name != codexVerifierSchemaInput &&
				input.Name != codexVerifierPlanInput &&
				input.Name != codexVerifierSubmissionInput &&
				input.Name != codexVerifierDispatchInput &&
				!validCodexVerifierReviewInput(input.Name)) {
			return errors.New("Codex verifier completion contains an invalid bound input")
		}
		if index > 0 && inputs[index-1].Name >= input.Name {
			return errors.New("Codex verifier completion inputs are not sorted by name")
		}
		byName[input.Name] = input
	}
	for _, name := range []string{
		codexExecutableInput,
		codexVerifierSchemaInput,
		codexVerifierPlanInput,
		codexVerifierSubmissionInput,
		codexVerifierDispatchInput,
	} {
		if _, exists := byName[name]; !exists {
			return fmt.Errorf("Codex verifier completion omits bound input %q", name)
		}
	}
	if binary := byName[codexExecutableInput]; binary.Digest != profile.BinaryDigest ||
		binary.Size != uint64(profile.BinarySize) {
		return errors.New("Codex verifier completion does not bind the pinned binary")
	}
	if byName[codexVerifierSchemaInput].Digest != profile.OutputSchemaDigest {
		return errors.New("Codex verifier completion does not bind the engine-owned output schema")
	}
	return nil
}
