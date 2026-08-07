package cockpit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	observationAttemptLimit = 256
	observationEventLimit   = 128
	maxProjectionAttempts   = 2
)

type Error struct {
	Code string
}

func (e *Error) Error() string { return "cockpit: " + e.Code }

func IsCode(err error, code string) bool {
	var cockpitError *Error
	return errors.As(err, &cockpitError) && cockpitError.Code == code
}

func fail(code string) error { return &Error{Code: code} }

type JournalReader interface {
	RunBinding(context.Context, string) (journal.Run, error)
	ReadObservation(
		context.Context,
		string,
		int,
		int,
	) (journal.Observation, error)
	EventsAfter(
		context.Context,
		string,
		int64,
		int,
	) (journal.EventWindow, error)
}

type RuntimeReader interface {
	Status(context.Context, string) (runtimepkg.RunStatus, error)
}

type StateReader interface {
	Read(context.Context, journal.Run) (baton.State, error)
}

type Projector struct {
	journal JournalReader
	runtime RuntimeReader
	baton   StateReader
	now     func() time.Time
}

func NewProjector(
	journalReader JournalReader,
	runtimeReader RuntimeReader,
	stateReader StateReader,
) (*Projector, error) {
	if journalReader == nil || runtimeReader == nil || stateReader == nil {
		return nil, fail("INVALID_PROJECTOR")
	}
	return &Projector{
		journal: journalReader,
		runtime: runtimeReader,
		baton:   stateReader,
		now:     time.Now,
	}, nil
}

func (p *Projector) Snapshot(
	ctx context.Context,
	runID string,
) (Snapshot, error) {
	if p == nil || ctx == nil || runID == "" {
		return Snapshot{}, fail("INVALID_REQUEST")
	}
	binding, err := p.journal.RunBinding(ctx, runID)
	if err != nil {
		return Snapshot{}, fail("JOURNAL_UNAVAILABLE")
	}
	for attempt := 0; attempt < maxProjectionAttempts; attempt++ {
		firstState, firstStateErr := p.baton.Read(ctx, binding)
		firstRuntime, err := p.runtime.Status(ctx, runID)
		if err != nil {
			return Snapshot{}, fail("RUNTIME_UNAVAILABLE")
		}
		observation, err := p.journal.ReadObservation(
			ctx,
			runID,
			observationAttemptLimit,
			observationEventLimit,
		)
		if err != nil {
			return Snapshot{}, fail("JOURNAL_UNAVAILABLE")
		}
		secondRuntime, err := p.runtime.Status(ctx, runID)
		if err != nil {
			return Snapshot{}, fail("RUNTIME_UNAVAILABLE")
		}
		secondState, secondStateErr := p.baton.Read(ctx, binding)
		if stableObservation(
			binding,
			observation,
			firstRuntime,
			secondRuntime,
			firstState,
			firstStateErr,
			secondState,
			secondStateErr,
		) {
			stateAvailable := firstStateErr == nil
			return buildSnapshot(
				observation,
				secondRuntime,
				firstState,
				stateAvailable,
				p.now().UTC(),
			)
		}
	}
	return Snapshot{}, fail("SNAPSHOT_UNSTABLE")
}

func (p *Projector) Events(
	ctx context.Context,
	runID string,
	afterOffset int64,
	limit int,
) (EventPage, error) {
	if p == nil || ctx == nil || runID == "" {
		return EventPage{}, fail("INVALID_REQUEST")
	}
	window, err := p.journal.EventsAfter(ctx, runID, afterOffset, limit)
	if err != nil {
		return EventPage{}, fail("JOURNAL_UNAVAILABLE")
	}
	result := EventPage{
		SchemaVersion: SnapshotSchemaVersion,
		RunID:         runID,
		Events:        make([]Evidence, 0, len(window.Events)),
		ThroughOffset: window.Through,
		EventOffset:   window.EventOffset,
		HasMore:       window.HasMore,
	}
	for _, event := range window.Events {
		result.Events = append(result.Events, Evidence{
			Offset:    event.Offset,
			Kind:      event.Kind,
			CreatedAt: event.CreatedAt,
		})
	}
	return result, nil
}

func stableObservation(
	binding journal.Run,
	observation journal.Observation,
	firstRuntime, secondRuntime runtimepkg.RunStatus,
	firstState baton.State,
	firstStateErr error,
	secondState baton.State,
	secondStateErr error,
) bool {
	if !reflect.DeepEqual(firstRuntime, secondRuntime) ||
		!reflect.DeepEqual(binding, observation.Run) ||
		observation.EventOffset != secondRuntime.EventOffset ||
		observation.Control.Generation != secondRuntime.ControlGeneration ||
		observation.Control.Desired != secondRuntime.DesiredState ||
		observation.Run.ID != secondRuntime.RunID ||
		observation.Run.ManifestDigest != secondRuntime.ManifestDigest ||
		observation.Run.TargetRef != secondRuntime.TargetRef {
		return false
	}
	if (firstStateErr == nil) != (secondStateErr == nil) {
		return false
	}
	if firstStateErr != nil {
		return true
	}
	return equalRefVectors(firstState.Refs, secondState.Refs) &&
		firstState.Release == observation.Run.Release &&
		firstState.Refs.Target.Ref == observation.Run.TargetRef &&
		secondRuntime.TargetHead == firstState.Refs.Target.Head &&
		secondRuntime.ReleaseHead == firstState.Refs.Release.Head
}

func equalRefVectors(left, right baton.StateRefs) bool {
	return reflect.DeepEqual(left, right)
}

func buildSnapshot(
	observation journal.Observation,
	status runtimepkg.RunStatus,
	state baton.State,
	stateAvailable bool,
	now time.Time,
) (Snapshot, error) {
	result := Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Run: RunView{
			ID:                status.RunID,
			Release:           observation.Run.Release,
			State:             status.State,
			DesiredState:      status.DesiredState,
			ControlGeneration: status.ControlGeneration,
			ManifestDigest:    status.ManifestDigest,
			PlanDigest:        status.PlanDigest,
			TargetRef:         status.TargetRef,
			TargetHead:        status.TargetHead,
			ReleaseHead:       status.ReleaseHead,
			Outcome:           status.Outcome,
		},
		Runtime: RuntimeView{
			Owner: OwnerView{
				Present: observation.OwnerPresent,
			},
			Effects:  make([]EffectView, 0, len(status.Effects)),
			Attempts: make([]AttemptView, 0, len(observation.Attempts)),
			Attentions: make(
				[]AttentionView,
				0,
				len(observation.Attentions),
			),
			Notifications: make(
				[]NotificationView,
				0,
				len(observation.Notifications),
			),
			NotificationsTruncated: observation.NotificationsTruncated,
			AttentionsTruncated:    observation.AttentionsTruncated,
		},
		Evidence:          make([]Evidence, 0, len(observation.Events)),
		Actions:           []Action{},
		Diagnostics:       []Diagnostic{},
		ThroughOffset:     observation.EventOffset,
		ApprovalOffer:     status.ApprovalOffer,
		CaptainDelegation: status.CaptainDelegation,
	}
	redeliveryActions := make([]Action, 0)
	attentionActions := make([]Action, 0)
	if observation.OwnerPresent {
		result.Runtime.Owner.Active = observation.Owner.ExpiresAt.After(now)
		result.Runtime.Owner.Generation = observation.Owner.Generation
		result.Runtime.Owner.ExpiresAt = observation.Owner.ExpiresAt
	}
	for _, effect := range status.Effects {
		result.Runtime.Effects = append(result.Runtime.Effects, EffectView{
			ID:        effect.ID,
			Kind:      effect.Kind,
			State:     effect.State,
			ErrorCode: effect.ErrorCode,
		})
	}
	for _, attempt := range observation.Attempts {
		view, err := projectAttempt(attempt)
		if err != nil {
			return Snapshot{}, err
		}
		result.Runtime.Attempts = append(result.Runtime.Attempts, view)
	}
	for _, attention := range observation.Attentions {
		result.Runtime.Attentions = append(
			result.Runtime.Attentions,
			AttentionView{
				ID:         attention.Attention.ID,
				LaneID:     attention.Attention.Recovery.LaneID,
				State:      string(attention.State),
				Generation: attention.Generation,
				Question:   attention.Question,
				Answer:     attention.Answer,
			},
		)
		if attention.State == journal.AttentionOpen {
			attentionActions = append(attentionActions, Action{
				Kind:               "answer_attention",
				ExpectedGeneration: attention.Generation,
				AttentionID:        attention.Attention.ID,
			})
		}
	}
	for _, notification := range observation.Notifications {
		view := NotificationView{
			DestinationID:     notification.DestinationID,
			SourceEventOffset: notification.SourceEventOffset,
			Sequence:          notification.Sequence,
			MessageID:         notification.MessageID,
			State:             string(notification.State),
			Attempts:          notification.Attempts,
			AvailableAt:       notification.AvailableAt,
			LastErrorCode: safeNotificationError(
				notification.LastErrorCode,
			),
			CreatedAt: notification.CreatedAt,
			UpdatedAt: notification.UpdatedAt,
		}
		if !notification.ClaimedUntil.IsZero() {
			claimedUntil := notification.ClaimedUntil
			view.ClaimedUntil = &claimedUntil
		}
		if !notification.DeliveredAt.IsZero() {
			deliveredAt := notification.DeliveredAt
			view.DeliveredAt = &deliveredAt
		}
		result.Runtime.Notifications = append(
			result.Runtime.Notifications,
			view,
		)
		if notification.State == journal.NotificationDead {
			redeliveryActions = append(redeliveryActions, Action{
				Kind:          "redeliver",
				DestinationID: notification.DestinationID,
				MessageID:     notification.MessageID,
			})
		}
	}
	if observation.AttentionsTruncated {
		result.Diagnostics = append(
			result.Diagnostics,
			Diagnostic{Code: "ATTENTIONS_TRUNCATED"},
		)
	}
	if observation.NotificationsTruncated {
		result.Diagnostics = append(
			result.Diagnostics,
			Diagnostic{Code: "OUTBOX_TRUNCATED"},
		)
	}
	for _, event := range observation.Events {
		result.Evidence = append(result.Evidence, Evidence{
			Offset:    event.Offset,
			Kind:      event.Kind,
			CreatedAt: event.CreatedAt,
		})
	}
	if !stateAvailable {
		result.Graph.Nodes = []Node{{
			ID:    "release:" + observation.Run.Release,
			Kind:  "release",
			Label: observation.Run.Release,
			State: status.State,
		}}
		result.Graph.Edges = []Edge{}
		result.Handoff = Handoff{
			Nodes:            []string{},
			Responsibilities: []string{},
		}
		result.Diagnostics = append(
			result.Diagnostics,
			Diagnostic{Code: "BATON_UNAVAILABLE"},
		)
		return result, nil
	}
	result.Graph = projectGraph(
		state,
		status.State,
		observation.Attentions,
	)
	result.Handoff = projectHandoff(result.Graph)
	for _, diagnostic := range state.Diagnostics {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Code:  diagnostic.Code,
			Track: diagnostic.Track,
			Work:  diagnostic.Work,
		})
	}
	if !state.Plan.TargetStale && len(state.Diagnostics) == 0 {
		controls := safeActions(status, observation.Control)
		result.Actions = append(controls, attentionActions...)
		result.Actions = append(result.Actions, redeliveryActions...)
	}
	return result, nil
}

func safeNotificationError(value string) string {
	switch value {
	case "",
		"DELIVERY_LEASE_EXPIRED",
		"WEBHOOK_AUTHORITY_CHANGED",
		"WEBHOOK_CANCELLED",
		"WEBHOOK_CONFIG_CHANGED",
		"WEBHOOK_DNS_REJECTED",
		"WEBHOOK_DNS_UNAVAILABLE",
		"WEBHOOK_HTTP_REJECTED",
		"WEBHOOK_HTTP_RETRYABLE",
		"WEBHOOK_PAYLOAD_INVALID",
		"WEBHOOK_REDIRECT",
		"WEBHOOK_REQUEST_INVALID",
		"WEBHOOK_TIMEOUT",
		"WEBHOOK_TRANSPORT":
		return value
	default:
		return "DELIVERY_FAILED"
	}
}

// discoveredEvidence is best-effort: evidence is optional, non-authoritative
// material, so any discovery error (an absent directory, a malformed or
// tampered bundle) degrades to no evidence rather than surfacing an error or
// diagnostic that could suppress this snapshot's controls.
func discoveredEvidence(state baton.State, sliceID string) []BoundEvidenceItem {
	items, err := DiscoverBoundEvidence(".", state, sliceID)
	if err != nil {
		return nil
	}
	return items
}

func projectHandoff(graph Graph) Handoff {
	result := Handoff{
		Nodes:            []string{},
		Responsibilities: []string{},
	}
	seen := make(map[string]struct{})
	for _, node := range graph.Nodes {
		if !node.HasBaton || node.NextResponsibility == "" ||
			node.NextResponsibility == "none" {
			continue
		}
		result.Ready = true
		result.Nodes = append(result.Nodes, node.ID)
		if _, found := seen[node.NextResponsibility]; !found {
			result.Responsibilities = append(
				result.Responsibilities,
				node.NextResponsibility,
			)
			seen[node.NextResponsibility] = struct{}{}
		}
	}
	return result
}

func projectAttempt(fact journal.AttemptFact) (AttemptView, error) {
	var usage driver.UsageReceipt
	if err := strictUsage(fact.Usage, &usage); err != nil {
		return AttemptView{}, fail("CORRUPT_JOURNAL")
	}
	return AttemptView{
		EffectID:       fact.EffectID,
		Number:         fact.Number,
		Responsibility: fact.Responsibility,
		Transport:      fact.Transport,
		InputTokens:    usage.InputTokens,
		OutputTokens:   usage.OutputTokens,
		CostMicroUnits: usage.CostMicroUnits,
		Currency:       usage.Currency,
		CreatedAt:      fact.CreatedAt,
	}, nil
}

func strictUsage(body []byte, value *driver.UsageReceipt) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing usage value")
	}
	canonical, err := driver.EncodeUsageReceipt(*value)
	if err != nil || !bytes.Equal(canonical, body) {
		return errors.New("noncanonical usage")
	}
	return nil
}

func projectGraph(
	state baton.State,
	runState string,
	attentions []journal.AttentionProjection,
) Graph {
	parkedLanes := make(map[string]struct{}, len(attentions))
	for _, attention := range attentions {
		if attention.State == journal.AttentionOpen ||
			attention.State == journal.AttentionAnswered {
			parkedLanes[attention.Attention.Recovery.LaneID] =
				struct{}{}
		}
	}
	result := Graph{
		ManifestVersion: state.Plan.Metadata.SchemaVersion,
		Nodes: []Node{{
			ID:    "release:" + state.Release,
			Kind:  "release",
			Label: state.Release,
			State: runState,
		}},
		Edges: []Edge{},
	}
	if history := state.Plan.History; len(history) > 0 {
		for _, relation := range history[len(history)-1].Plan.TouchpointMatrix() {
			result.Touchpoints = append(result.Touchpoints, Touchpoint{
				Left: relation.Left, Right: relation.Right, Path: relation.Path,
				Ordered: relation.Ordered, Before: relation.Before,
			})
		}
	}
	addEdge := func(kind, from, to string) {
		result.Edges = append(result.Edges, Edge{
			ID:   "edge:" + kind + ":" + from + ":" + to,
			From: from,
			To:   to,
			Kind: kind,
		})
	}
	finalSlices := make(map[string]string, len(state.Tracks))
	for _, track := range state.Tracks {
		trackID := "track:" + track.ID
		runtimeState := ""
		if _, parked := parkedLanes[track.ID]; parked {
			runtimeState = "parked"
		}
		result.Nodes = append(result.Nodes, Node{
			ID:           trackID,
			Kind:         "track",
			Label:        track.ID,
			State:        "present",
			RuntimeState: runtimeState,
		})
		addEdge("contains", "release:"+state.Release, trackID)
		for _, dependency := range track.DependsOn {
			addEdge("depends_on", "track:"+dependency, trackID)
		}
		previous := ""
		for _, slice := range track.Slices {
			sliceID := "slice:" + slice.Location.Slice.ID
			hasBaton := slice.Status == "ready" &&
				slice.NextRole != "none"
			runtimeState := ""
			if _, parked := parkedLanes[track.ID]; parked &&
				hasBaton {
				runtimeState = "parked"
			}
			result.Nodes = append(result.Nodes, Node{
				ID:                 sliceID,
				Kind:               "slice",
				Label:              slice.Location.Slice.ID,
				Track:              track.ID,
				State:              slice.Status,
				RuntimeState:       runtimeState,
				Stage:              slice.Stage,
				Outcome:            slice.Outcome,
				NextResponsibility: slice.NextRole,
				Attempt:            slice.Attempt,
				HasBaton:           hasBaton,
				ContractPath:       slice.Location.Slice.ContractPath,
				ContractDigest:     state.Plan.Metadata.Contracts[slice.Location.Slice.ID],
				BoundEvidence:      discoveredEvidence(state, slice.Location.Slice.ID),
			})
			addEdge("contains", trackID, sliceID)
			if previous != "" {
				addEdge("serial", previous, sliceID)
			}
			previous = sliceID
			for _, dependency := range slice.Location.Slice.DependsOn {
				addEdge("depends_on", "slice:"+dependency, sliceID)
			}
			for _, producer := range slice.Location.Slice.Consumes {
				addEdge("consumes", "slice:"+producer, sliceID)
			}
		}
		if previous != "" {
			finalSlices[track.ID] = previous
		}
	}
	assemblyID := "assembly:" + state.Release
	assemblyHasBaton := state.Assembly.Status == "ready" &&
		state.Assembly.NextRole != "none"
	assemblyRuntimeState := ""
	if _, parked := parkedLanes["release"]; parked &&
		assemblyHasBaton {
		assemblyRuntimeState = "parked"
	}
	result.Nodes = append(result.Nodes, Node{
		ID:                 assemblyID,
		Kind:               "assembly",
		Label:              "Assembly",
		State:              state.Assembly.Status,
		RuntimeState:       assemblyRuntimeState,
		Stage:              state.Assembly.Stage,
		Outcome:            state.Assembly.Outcome,
		NextResponsibility: state.Assembly.NextRole,
		HasBaton:           assemblyHasBaton,
	})
	addEdge("contains", "release:"+state.Release, assemblyID)
	for _, track := range state.Tracks {
		if _, bound := state.Assembly.InputPins[track.ID]; !bound {
			continue
		}
		if producer := finalSlices[track.ID]; producer != "" {
			addEdge("assembly_input", producer, assemblyID)
		}
	}
	return result
}

func safeActions(
	status runtimepkg.RunStatus,
	control journal.ControlProjection,
) []Action {
	generation := status.ControlGeneration
	result := make([]Action, 0, 4)
	switch {
	case status.State == "paused" || status.State == "pausing":
		result = append(result, Action{
			Kind:               string(journal.Resume),
			ExpectedGeneration: generation,
		})
	case status.State != "complete" && status.State != "cancelled":
		result = append(result, Action{
			Kind:               string(journal.Pause),
			ExpectedGeneration: generation,
		})
	}
	if status.State != "complete" && status.State != "cancelled" {
		result = append(result, Action{
			Kind:               string(journal.Cancel),
			ExpectedGeneration: generation,
		})
	}
	if status.State == "takeover_required" {
		result = append(result, Action{
			Kind:               string(journal.Takeover),
			ExpectedGeneration: generation,
		})
	}
	for _, effect := range status.Effects {
		work, epoch, ok := exhaustedAttempt(effect)
		currentEpoch := control.RetryEpochs[work]
		if currentEpoch == 0 {
			currentEpoch = 1
		}
		if ok && epoch == currentEpoch {
			result = append(result, Action{
				Kind:               string(journal.Retry),
				ExpectedGeneration: generation,
				WorkID:             work,
				ExpectedEpoch:      epoch,
			})
		}
	}
	if delegation := status.CaptainDelegation; delegation != nil &&
		delegation.State == "active" {
		binding := CaptainDelegationAction{
			RunID: status.RunID, ManifestDigest: status.ManifestDigest,
			ActorClass:     runtimepkg.CaptainDelegationActorClass,
			ActorAuthority: status.ExternalAuthorizer,
			CurrentEpoch:   delegation.Epoch, CurrentDigest: delegation.Digest,
		}
		revoke := binding
		revoke.Action = "revoke"
		replace := binding
		replace.Action = "replace"
		result = append(result,
			Action{Kind: "captain_delegation_revoke", CaptainDelegation: &revoke},
			Action{Kind: "captain_delegation_replace", CaptainDelegation: &replace},
		)
	}
	return result
}

func exhaustedAttempt(effect runtimepkg.EffectStatus) (string, int64, bool) {
	if effect.State != string(journal.OperationalFailed) ||
		!strings.HasSuffix(effect.ID, "/t3") {
		return "", 0, false
	}
	parts := strings.Split(effect.ID, "/")
	if len(parts) != 4 || parts[0] != "attempt" ||
		len(parts[1]) != 64 || !strings.HasPrefix(parts[2], "e") {
		return "", 0, false
	}
	epoch, err := strconv.ParseInt(strings.TrimPrefix(parts[2], "e"), 10, 64)
	if err != nil || epoch < 1 {
		return "", 0, false
	}
	return "sha256:" + parts[1], epoch, true
}
