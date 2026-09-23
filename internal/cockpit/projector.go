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

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
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
	Read(context.Context, journal.Run) (protocol.State, error)
}

type Projector struct {
	journal  JournalReader
	runtime  RuntimeReader
	protocol StateReader
	now      func() time.Time
	// activityRing is the optional live ring the serve host wires in.
	// Nil means journal-only (the TUI, the project-wide host, and any
	// run driven by another process); non-nil means the serve host
	// drives the run in-process and the activity route merges the ring
	// ahead of the journal on one cursor.
	activityRing *ActivityRing
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
		journal:  journalReader,
		runtime:  runtimeReader,
		protocol: stateReader,
		now:      time.Now,
	}, nil
}

// SetActivityRing wires the live ring for serve-driven runs. A nil ring
// disables liveness; Activity then serves the same content from the
// journal alone with Live=false.
func (p *Projector) SetActivityRing(ring *ActivityRing) {
	if p == nil {
		return
	}
	p.activityRing = ring
}

type windowReader interface {
	ReadWindow(
		ctx context.Context,
		runID string,
		afterOffset int64,
		limit int,
	) (journal.Window, error)
}

func projectEvidence(fact journal.EventFact) Evidence {
	evidence := Evidence{
		Offset:    fact.Offset,
		Kind:      fact.Kind,
		CreatedAt: fact.CreatedAt,
	}
	if len(fact.SafeBody) == 0 {
		return evidence
	}
	var assoc runtimepkg.EventAssociation
	if err := json.Unmarshal(fact.SafeBody, &assoc); err == nil {
		if assoc.EffectID != "" && assoc.WorkID != "" {
			evidence.EffectID = assoc.EffectID
			evidence.WorkID = assoc.WorkID
			evidence.Track = assoc.Track
			evidence.Slice = assoc.Slice
		}
	}
	return evidence
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
		firstState, firstStateErr := p.protocol.Read(ctx, binding)
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
		secondState, secondStateErr := p.protocol.Read(ctx, binding)
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
			if wr, ok := p.journal.(windowReader); ok && len(observation.Events) > 0 {
				after := observation.Events[0].Offset - 1
				if after < 0 {
					after = 0
				}
				if rw, err := wr.ReadWindow(ctx, runID, after, observationEventLimit); err == nil {
					rawBodies := make(map[int64][]byte, len(rw.Snapshot.Events))
					for _, ev := range rw.Snapshot.Events {
						rawBodies[ev.Offset] = ev.Body
					}
					for i := range observation.Events {
						if raw, found := rawBodies[observation.Events[i].Offset]; found {
							observation.Events[i].SafeBody = raw
						}
					}
				}
			}
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
	track ...string,
) (EventPage, error) {
	if p == nil || ctx == nil || runID == "" {
		return EventPage{}, fail("INVALID_REQUEST")
	}
	var filterTrack string
	if len(track) > 0 {
		filterTrack = track[0]
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
	var rawBodies map[int64][]byte
	if wr, ok := p.journal.(windowReader); ok && len(window.Events) > 0 {
		if rw, err := wr.ReadWindow(ctx, runID, afterOffset, limit); err == nil {
			rawBodies = make(map[int64][]byte, len(rw.Snapshot.Events))
			for _, ev := range rw.Snapshot.Events {
				rawBodies[ev.Offset] = ev.Body
			}
		}
	}
	for _, event := range window.Events {
		fact := event
		if raw, found := rawBodies[event.Offset]; found {
			fact.SafeBody = raw
		}
		evidence := projectEvidence(fact)
		if filterTrack == "" || evidence.Track == "" || evidence.Track == filterTrack {
			result.Events = append(result.Events, evidence)
		}
	}
	return result, nil
}

func stableObservation(
	binding journal.Run,
	observation journal.Observation,
	firstRuntime, secondRuntime runtimepkg.RunStatus,
	firstState protocol.State,
	firstStateErr error,
	secondState protocol.State,
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

func equalRefVectors(left, right protocol.StateRefs) bool {
	return reflect.DeepEqual(left, right)
}

func buildSnapshot(
	observation journal.Observation,
	status runtimepkg.RunStatus,
	state protocol.State,
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
			Park:              status.Park,
			Recovery:          status.Recovery,
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
		Evidence:       make([]Evidence, 0, len(observation.Events)),
		Actions:        []Action{},
		Diagnostics:    []Diagnostic{},
		ThroughOffset:  observation.EventOffset,
		ApprovalOffer:  status.ApprovalOffer,
		LeadDelegation: status.LeadDelegation,
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
			ID:                 effect.ID,
			Kind:               effect.Kind,
			State:              effect.State,
			ErrorCode:          effect.ErrorCode,
			Derived:            effect.Derived,
			FailureTurnContext: effect.FailureTurnContext,
			HostCheckFailure:   effect.HostCheckFailure,
			CheckOutcome:       effect.CheckOutcome,
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
		var humanTurn *HumanAttentionView
		if human := attention.Attention.HumanTurn; human != nil {
			humanTurn = &HumanAttentionView{
				SchemaVersion:         human.SchemaVersion,
				Kind:                  human.Kind,
				RunID:                 human.RunID,
				Track:                 human.Track,
				Slice:                 human.Slice,
				Role:                  human.Role,
				Responsibility:        human.Responsibility,
				InvocationID:          human.InvocationID,
				ProtocolAttempt:       human.ProtocolAttempt,
				PlanAuthorityDigest:   human.PlanAuthorityDigest,
				TargetAuthorityDigest: human.TargetAuthorityDigest,
				WorkIdentity:          human.WorkIdentity,
				CycleID:               human.CycleID,
				TurnID:                human.TurnID,
				Ordinal:               human.Ordinal,
				OpenGeneration:        human.OpenGeneration,
			}
		}
		result.Runtime.Attentions = append(
			result.Runtime.Attentions,
			AttentionView{
				ID:         attention.Attention.ID,
				LaneID:     attention.Attention.Recovery.LaneID,
				State:      string(attention.State),
				Generation: attention.Generation,
				Question:   attention.Question,
				Answer:     attention.Answer,
				HumanTurn:  humanTurn,
			},
		)
		if attention.State == journal.AttentionOpen {
			attentionActions = append(attentionActions, Action{
				Kind:               "answer_attention",
				RunID:              status.RunID,
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
		result.Evidence = append(result.Evidence, projectEvidence(event))
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
			Diagnostic{Code: "PROTOCOL_UNAVAILABLE"},
		)
		return result, nil
	}
	result.Graph = projectGraph(
		state,
		status.State,
		observation.Attentions,
	)
	result.Checkpoint = status.Checkpoint
	result.Checkpoints = status.Checkpoints
	for i := range result.Graph.Nodes {
		node := &result.Graph.Nodes[i]
		for _, cp := range status.Checkpoints {
			if cp.AffectedSlice == node.Label || cp.AffectedSlice == node.ID || "slice:"+cp.AffectedSlice == node.ID {
				cpCopy := cp
				node.Checkpoint = &cpCopy
				break
			}
		}
	}
	// S3: PinnedWork->Node via lane->actionable-node, the existing
	// Track==lane && HasProtocol precedent (projector.go marks slice nodes
	// with Track==lane && HasProtocol, and maps the release lane to the
	// assembly node). At most one slice node per lane carries it; the
	// release lane maps to the assembly node. Same pointer, no re-decode,
	// so board/TUI/MCP detail stay in parity with Effects/PinnedWork.
	for _, pinned := range status.PinnedWork {
		if pinned.FailureTurnContext == nil && pinned.HostCheckFailure == nil {
			continue
		}
		if pinned.Lane == "release" {
			for i := range result.Graph.Nodes {
				if result.Graph.Nodes[i].Kind == "assembly" {
					if pinned.FailureTurnContext != nil {
						result.Graph.Nodes[i].FailureTurnContext = pinned.FailureTurnContext
					}
					if pinned.HostCheckFailure != nil {
						result.Graph.Nodes[i].HostCheckFailure = pinned.HostCheckFailure
					}
				}
			}
			continue
		}
		for i := range result.Graph.Nodes {
			node := &result.Graph.Nodes[i]
			if node.Kind == "slice" && node.Track == pinned.Lane && node.HasProtocol {
				if pinned.FailureTurnContext != nil {
					node.FailureTurnContext = pinned.FailureTurnContext
				}
				if pinned.HostCheckFailure != nil {
					node.HostCheckFailure = pinned.HostCheckFailure
				}
			}
		}
	}
	// S5: RunStatus.ProviderStall->Node via the identical lane->actionable-
	// node rule PinnedWork uses above, so a still-waiting work (never
	// itself pinned or parked) reads the same way in the board/TUI as a
	// pinned one.
	for _, stall := range status.ProviderStall {
		stallCopy := stall
		if stall.Lane == "release" {
			for i := range result.Graph.Nodes {
				if result.Graph.Nodes[i].Kind == "assembly" {
					result.Graph.Nodes[i].ProviderStall = &stallCopy
				}
			}
			continue
		}
		for i := range result.Graph.Nodes {
			node := &result.Graph.Nodes[i]
			if node.Kind == "slice" && node.Track == stall.Lane && node.HasProtocol {
				node.ProviderStall = &stallCopy
			}
		}
	}
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
func discoveredEvidence(state protocol.State, sliceID string) []BoundEvidenceItem {
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
		if !node.HasProtocol || node.NextResponsibility == "" ||
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
	state protocol.State,
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
			hasProtocol := slice.Status == "ready" &&
				slice.NextRole != "none"
			runtimeState := ""
			if _, parked := parkedLanes[track.ID]; parked &&
				hasProtocol {
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
				HasProtocol:        hasProtocol,
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
	assemblyHasProtocol := state.Assembly.Status == "ready" &&
		state.Assembly.NextRole != "none"
	assemblyRuntimeState := ""
	if _, parked := parkedLanes["release"]; parked &&
		assemblyHasProtocol {
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
		HasProtocol:        assemblyHasProtocol,
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
	// The reconciled-uncertain shape carries the one verb the control gate
	// admits for it, derived by the runtime from the same predicate
	// ApplyControl evaluates. The board offers exactly that action: a
	// needs-you row must never name a verb the action list does not carry.
	if status.State == "uncertain" && status.Recovery != nil {
		switch status.Recovery.Action {
		case string(journal.Retry):
			result = append(result, Action{
				Kind:               string(journal.Retry),
				ExpectedGeneration: generation,
				WorkID:             status.Recovery.WorkID,
				ExpectedEpoch:      status.Recovery.Epoch,
			})
		case string(journal.Takeover):
			result = append(result, Action{
				Kind:               string(journal.Takeover),
				ExpectedGeneration: generation,
			})
		case string(journal.Resume):
			result = append(result, Action{
				Kind:               string(journal.Resume),
				ExpectedGeneration: generation,
			})
		}
	}
	for _, effect := range status.Effects {
		// An economy-caused exhaustion never offers a bare retry (S4-
		// resumable-budget-stops): the control gate refuses it with
		// ECONOMY_GRANT_REQUIRED, so the board must not name a verb
		// ApplyControl will reject. The grant action below covers it
		// instead, from status.PinnedWork.
		if isEconomyErrorCode(effect.ErrorCode) {
			continue
		}
		// A context-window exhaustion parks on any try, not only the
		// third, and always names its owner via status.PinnedWork, never
		// this per-effect t3 scan (S6-context-window-clamp A3): skipping
		// it here, exactly as isEconomyErrorCode's codes are skipped
		// above, keeps this loop from ever offering a second, differently
		// (dispatch-identity-)named Retry action for the same crossing a
		// nested dispatch's PinnedWork branch below already names by its
		// owner.
		if effect.ErrorCode == "ECONOMY_CONTEXT_EXHAUSTED" {
			continue
		}
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
	for _, pinned := range status.PinnedWork {
		if pinned.Cause != runtimepkg.ParkCauseEconomyContext {
			continue
		}
		// economy_context_window is never Grant-eligible (economyGrantUnit
		// returns "" for it): there is no manifest Limits value a Grant
		// could raise to fix a fixed driver-config context window. A bare
		// Retry naming pinned.WorkID (the owner) is the only verb, admitted
		// at any try by the runtime's EconomyContextRetry stamp
		// (S6-context-window-clamp A3), so this dedicated branch - not the
		// t3-only loop above, which skips this code entirely - is this
		// cause's sole source of a board action.
		epoch := control.RetryEpochs[pinned.WorkID]
		if epoch == 0 {
			epoch = 1
		}
		result = append(result, Action{
			Kind:               string(journal.Retry),
			ExpectedGeneration: generation,
			WorkID:             pinned.WorkID,
			ExpectedEpoch:      epoch,
		})
	}
	for _, pinned := range status.PinnedWork {
		unit := economyGrantUnit(pinned.Cause)
		if unit == "" {
			continue
		}
		// The epoch a Grant is checked and advances against is the crossing's
		// owner (pinned.WorkID, the board-visible lane work), never the
		// crossing's own stable dispatch-work identity
		// (pinned.DispatchWorkID): that owner's epoch is what the runtime
		// admission gate reads back too (Service.admitEconomyControl's
		// RetryWorkID), and for a direct dispatch the two identities already
		// coincide (S4-resumable-budget-stops V2).
		epoch := control.RetryEpochs[pinned.WorkID]
		if epoch == 0 {
			epoch = 1
		}
		result = append(result, Action{
			Kind:               string(journal.Grant),
			ExpectedGeneration: generation,
			WorkID:             pinned.DispatchWorkID,
			ExpectedEpoch:      epoch,
			Unit:               unit,
		})
	}
	if delegation := status.LeadDelegation; delegation != nil &&
		delegation.State == "active" {
		binding := LeadDelegationAction{
			RunID: status.RunID, ManifestDigest: status.ManifestDigest,
			ActorClass:     runtimepkg.LeadDelegationActorClass,
			ActorAuthority: status.ExternalAuthorizer,
			CurrentEpoch:   delegation.Epoch, CurrentDigest: delegation.Digest,
		}
		revoke := binding
		revoke.Action = "revoke"
		replace := binding
		replace.Action = "replace"
		result = append(result,
			Action{Kind: "lead_delegation_revoke", LeadDelegation: &revoke},
			Action{Kind: "lead_delegation_replace", LeadDelegation: &replace},
		)
	}
	return result
}

func isEconomyErrorCode(code string) bool {
	return code == "ECONOMY_TURN_BUDGET_EXCEEDED" ||
		code == "ECONOMY_OUTPUT_BUDGET_EXCEEDED"
}

// economyGrantUnit maps a PinnedWork cause to the Grant unit it admits, or
// "" for a cause a Grant does not apply to (identical_failure, exhaustion).
// The cause and unit vocabularies share their exact string values by
// construction (runtime.ParkCauseEconomy*).
func economyGrantUnit(cause string) string {
	switch cause {
	case runtimepkg.ParkCauseEconomyTurns,
		runtimepkg.ParkCauseEconomyOutputTokens,
		runtimepkg.ParkCauseEconomyOutputBytes:
		return cause
	default:
		return ""
	}
}

func exhaustedAttempt(effect runtimepkg.EffectStatus) (string, int64, bool) {
	if effect.Derived || effect.State != string(journal.OperationalFailed) ||
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
