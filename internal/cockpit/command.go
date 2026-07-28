package cockpit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

type CommandRuntime interface {
	Start(context.Context, []byte) (runtimepkg.RunStatus, error)
	Control(
		context.Context,
		runtimepkg.ControlCommand,
	) (runtimepkg.RunStatus, error)
}

type NotificationRedeliverer interface {
	RedeliverNotification(
		context.Context,
		string,
		string,
		string,
		time.Time,
	) error
}

type AdmittedManifest struct {
	digest string
	runID  string
	body   []byte
}

func AdmitManifest(body []byte) (AdmittedManifest, error) {
	manifest, err := runtimepkg.ParseManifest(body)
	if err != nil {
		return AdmittedManifest{}, fail("INVALID_MANIFEST")
	}
	sum := sha256.Sum256(body)
	return AdmittedManifest{
		digest: "sha256:" + hex.EncodeToString(sum[:]),
		runID:  manifest.RunID,
		body:   append([]byte(nil), body...),
	}, nil
}

func (m AdmittedManifest) Digest() string { return m.digest }

func (m AdmittedManifest) RunID() string { return m.runID }

type StartCommand struct {
	ManifestDigest string `json:"manifest_digest"`
}

type ControlCommand struct {
	RunID              string              `json:"run_id"`
	CommandID          string              `json:"command_id"`
	Kind               journal.ControlKind `json:"kind"`
	ExpectedGeneration int64               `json:"expected_generation"`
	WorkID             string              `json:"work_id,omitempty"`
	ExpectedEpoch      int64               `json:"expected_epoch,omitempty"`
}

type RedeliveryCommand struct {
	RunID         string `json:"run_id"`
	DestinationID string `json:"destination_id"`
	MessageID     string `json:"message_id"`
}

type CommandFacade struct {
	runtime   CommandRuntime
	outbox    NotificationRedeliverer
	manifests map[string]AdmittedManifest
	now       func() time.Time
}

func NewCommandFacade(
	runtime CommandRuntime,
	outbox NotificationRedeliverer,
	manifests []AdmittedManifest,
) (*CommandFacade, error) {
	if runtime == nil || outbox == nil {
		return nil, fail("INVALID_COMMAND_FACADE")
	}
	admitted := make(map[string]AdmittedManifest, len(manifests))
	for _, manifest := range manifests {
		if manifest.digest == "" || manifest.runID == "" ||
			len(manifest.body) == 0 {
			return nil, fail("INVALID_MANIFEST")
		}
		if _, duplicate := admitted[manifest.digest]; duplicate {
			return nil, fail("DUPLICATE_MANIFEST")
		}
		copy := manifest
		copy.body = append([]byte(nil), manifest.body...)
		admitted[manifest.digest] = copy
	}
	return &CommandFacade{
		runtime:   runtime,
		outbox:    outbox,
		manifests: admitted,
		now:       time.Now,
	}, nil
}

func (f *CommandFacade) Start(
	ctx context.Context,
	command StartCommand,
) (runtimepkg.RunStatus, error) {
	if f == nil || ctx == nil || command.ManifestDigest == "" {
		return runtimepkg.RunStatus{}, fail("INVALID_COMMAND")
	}
	manifest, found := f.manifests[command.ManifestDigest]
	if !found {
		return runtimepkg.RunStatus{}, fail("MANIFEST_NOT_ADMITTED")
	}
	status, err := f.runtime.Start(ctx, append([]byte(nil), manifest.body...))
	if err != nil {
		return runtimepkg.RunStatus{}, fail("COMMAND_REJECTED")
	}
	return status, nil
}

func (f *CommandFacade) Control(
	ctx context.Context,
	command ControlCommand,
) (runtimepkg.RunStatus, error) {
	if f == nil || ctx == nil || command.RunID == "" ||
		command.CommandID == "" || command.ExpectedGeneration < 0 {
		return runtimepkg.RunStatus{}, fail("INVALID_COMMAND")
	}
	switch command.Kind {
	case journal.Pause, journal.Resume, journal.Cancel, journal.Takeover:
		if command.WorkID != "" || command.ExpectedEpoch != 0 {
			return runtimepkg.RunStatus{}, fail("INVALID_COMMAND")
		}
	case journal.Retry:
		if command.WorkID == "" || command.ExpectedEpoch < 1 {
			return runtimepkg.RunStatus{}, fail("INVALID_COMMAND")
		}
	default:
		return runtimepkg.RunStatus{}, fail("INVALID_COMMAND")
	}
	status, err := f.runtime.Control(ctx, runtimepkg.ControlCommand{
		RunID:              command.RunID,
		ID:                 command.CommandID,
		Kind:               command.Kind,
		ExpectedGeneration: command.ExpectedGeneration,
		WorkID:             command.WorkID,
		ExpectedEpoch:      command.ExpectedEpoch,
	})
	if err != nil {
		return runtimepkg.RunStatus{}, fail("COMMAND_REJECTED")
	}
	return status, nil
}

func (f *CommandFacade) Redeliver(
	ctx context.Context,
	command RedeliveryCommand,
) error {
	if f == nil || ctx == nil || command.RunID == "" ||
		command.DestinationID == "" || command.MessageID == "" {
		return fail("INVALID_COMMAND")
	}
	if err := f.outbox.RedeliverNotification(
		ctx,
		command.RunID,
		command.DestinationID,
		command.MessageID,
		f.now().UTC(),
	); err != nil {
		return fail("COMMAND_REJECTED")
	}
	return nil
}
