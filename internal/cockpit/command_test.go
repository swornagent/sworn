package cockpit

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

type fakeCommandRuntime struct {
	startCalls   int
	controlCalls int
	answerCalls  int
	startBody    []byte
	control      runtimepkg.ControlCommand
	answer       runtimepkg.AnswerAttentionCommand
	status       runtimepkg.RunStatus
	err          error
}

func (f *fakeCommandRuntime) Start(
	_ context.Context,
	body []byte,
) (runtimepkg.RunStatus, error) {
	f.startCalls++
	f.startBody = append([]byte(nil), body...)
	return f.status, f.err
}

func (f *fakeCommandRuntime) Control(
	_ context.Context,
	command runtimepkg.ControlCommand,
) (runtimepkg.RunStatus, error) {
	f.controlCalls++
	f.control = command
	return f.status, f.err
}

func (f *fakeCommandRuntime) AnswerAttention(
	_ context.Context,
	command runtimepkg.AnswerAttentionCommand,
) (runtimepkg.RunStatus, error) {
	f.answerCalls++
	f.answer = command
	return f.status, f.err
}

type fakeRedeliverer struct {
	calls       int
	runID       string
	destination string
	message     string
	at          time.Time
	err         error
}

func (f *fakeRedeliverer) RedeliverNotification(
	_ context.Context,
	runID, destination, message string,
	at time.Time,
) error {
	f.calls++
	f.runID = runID
	f.destination = destination
	f.message = message
	f.at = at
	return f.err
}

func TestCommandFacadeDelegatesEachClosedCommandExactlyOnce(t *testing.T) {
	t.Parallel()

	body := []byte("server-admitted manifest bytes\n")
	manifest := AdmittedManifest{
		digest: "sha256:" + strings.Repeat("a", 64),
		runID:  "run-1",
		body:   body,
	}
	runtime := &fakeCommandRuntime{
		status: runtimepkg.RunStatus{RunID: "run-1"},
	}
	outbox := &fakeRedeliverer{}
	facade, err := NewCommandFacade(
		runtime,
		outbox,
		[]AdmittedManifest{manifest},
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	facade.now = func() time.Time { return now }
	body[0] = 'X'

	if _, err := facade.Start(context.Background(), StartCommand{
		ManifestDigest: manifest.Digest(),
	}); err != nil {
		t.Fatal(err)
	}
	if runtime.startCalls != 1 ||
		!bytes.Equal(runtime.startBody, []byte("server-admitted manifest bytes\n")) {
		t.Fatalf("start delegation = calls=%d body=%q", runtime.startCalls, runtime.startBody)
	}
	if _, err := facade.Control(context.Background(), ControlCommand{
		RunID: "run-1", CommandID: "retry-1", Kind: journal.Retry,
		ExpectedGeneration: 3,
		WorkID:             "sha256:" + strings.Repeat("b", 64),
		ExpectedEpoch:      2,
	}); err != nil {
		t.Fatal(err)
	}
	if runtime.controlCalls != 1 ||
		runtime.control.ID != "retry-1" ||
		runtime.control.Kind != journal.Retry ||
		runtime.control.ExpectedGeneration != 3 ||
		runtime.control.ExpectedEpoch != 2 {
		t.Fatalf("control delegation = calls=%d command=%#v", runtime.controlCalls, runtime.control)
	}
	if _, err := facade.AnswerAttention(
		context.Background(),
		AnswerAttentionCommand{
			RunID:              "run-1",
			AttentionID:        "sha256:" + strings.Repeat("c", 64),
			ExpectedGeneration: 1,
			Answer:             "Use the existing release authority.",
		},
	); err != nil {
		t.Fatal(err)
	}
	if runtime.answerCalls != 1 ||
		runtime.answer.ExpectedGeneration != 1 {
		t.Fatalf(
			"answer delegation = calls=%d command=%#v",
			runtime.answerCalls,
			runtime.answer,
		)
	}
	if err := facade.Redeliver(context.Background(), RedeliveryCommand{
		RunID: "run-1", DestinationID: "primary", MessageID: "message-1",
	}); err != nil {
		t.Fatal(err)
	}
	if outbox.calls != 1 || outbox.runID != "run-1" ||
		outbox.destination != "primary" || outbox.message != "message-1" ||
		outbox.at != now {
		t.Fatalf("redelivery delegation = %#v", outbox)
	}
}

func TestCommandFacadeRejectsOpenOrUnpinnedInputBeforeDelegation(t *testing.T) {
	t.Parallel()

	runtime := &fakeCommandRuntime{}
	outbox := &fakeRedeliverer{}
	facade, err := NewCommandFacade(
		runtime,
		outbox,
		[]AdmittedManifest{{
			digest: "sha256:" + strings.Repeat("a", 64),
			runID:  "run-1",
			body:   []byte("manifest\n"),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Start(context.Background(), StartCommand{
		ManifestDigest: "sha256:" + strings.Repeat("f", 64),
	}); !IsCode(err, "MANIFEST_NOT_ADMITTED") {
		t.Fatalf("unadmitted start = %v", err)
	}
	for name, command := range map[string]ControlCommand{
		"unknown": {
			RunID: "run-1", CommandID: "command-1",
			Kind: "merge", ExpectedGeneration: 1,
		},
		"unpinned retry": {
			RunID: "run-1", CommandID: "command-2",
			Kind: journal.Retry, ExpectedGeneration: 1,
		},
		"open pause": {
			RunID: "run-1", CommandID: "command-3",
			Kind: journal.Pause, ExpectedGeneration: 1,
			WorkID: "sha256:" + strings.Repeat("b", 64),
		},
	} {
		if _, err := facade.Control(
			context.Background(),
			command,
		); !IsCode(err, "INVALID_COMMAND") {
			t.Errorf("%s = %v", name, err)
		}
	}
	if _, err := facade.AnswerAttention(
		context.Background(),
		AnswerAttentionCommand{
			RunID:              "run-1",
			AttentionID:        "attention",
			ExpectedGeneration: 2,
		},
	); !IsCode(err, "INVALID_COMMAND") {
		t.Fatalf("invalid answer = %v", err)
	}
	if runtime.startCalls != 0 || runtime.controlCalls != 0 ||
		runtime.answerCalls != 0 ||
		outbox.calls != 0 {
		t.Fatalf(
			"rejected input delegated: start=%d control=%d outbox=%d",
			runtime.startCalls,
			runtime.controlCalls,
			outbox.calls,
		)
	}
}
