package gitx

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type trackBaseFixture struct {
	repository  *Repository
	workspaces  *Workspaces
	product     *ProductExclusionAdmission
	release     string
	plan        OID
	releaseHead OID
	targetHead  OID
	consumer    TrackKey
}

func newTrackBaseFixture(t *testing.T, name string) trackBaseFixture {
	t.Helper()
	repository, target := newRepository(t, SHA1)
	record, product := inertAdmissions(t, repository, nil)
	release := "track-base-" + name
	planPath := recordRoot + "/" + release + "/plan.md"
	prepared, err := repository.PrepareRecordTransition(
		RecordTransitionRequest{Identity: testIdentity,
			ExpectedHead: target,
			Changes: []BlobChange{{
				Path:  planPath,
				Bytes: []byte("fixture plan\n"),
			}},
			Message:          "install fixture plan",
			RecordAdmission:  record,
			ProductAdmission: product,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind:    CreateRef,
		Ref:     "refs/heads/release-wt/" + release,
		NewHead: &prepared.Commit,
	}}); err != nil {
		t.Fatal(err)
	}
	var plan OID
	entries, err := repository.ListTree(prepared.Commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Path == planPath {
			plan = entry.OID
		}
	}
	if plan.IsZero() {
		t.Fatal("fixture plan blob is absent")
	}
	workspaces, err := NewWorkspaces(repository, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := workspaces.Close(); err != nil {
			t.Error(err)
		}
	})
	return trackBaseFixture{
		repository: repository, workspaces: workspaces,
		product: product, release: release, plan: plan,
		releaseHead: prepared.Commit, targetHead: target,
		consumer: TrackKey{Release: release, Track: "consumer"},
	}
}

func TestTrackBaseValidatesLegacyPlanAtRelease(t *testing.T) {
	t.Parallel()
	// A release recorded before the relocation carries its plan only under the
	// historical .baton/releases root. Track-base preparation must still find
	// the exact plan through the legacy fallback (A4).
	repository, target := newRepository(t, SHA1)
	record, product := inertAdmissions(t, repository, nil)
	_ = record
	release := "track-base-legacy"
	legacyPlanPath := LegacyRecordsRoot + "/" + release + "/plan.md"
	timestamp, err := repository.CommitTimestamp(target)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.prepareRecord(RecordRequest{
		Parent: target,
		Changes: []BlobChange{{
			Path: legacyPlanPath, Bytes: []byte("legacy fixture plan\n"),
		}},
		Message:   "install legacy fixture plan\n",
		Identity:  testIdentity,
		Timestamp: timestamp + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var plan OID
	entries, err := repository.ListTree(prepared.Commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Path == legacyPlanPath {
			plan = entry.OID
		}
	}
	if plan.IsZero() {
		t.Fatal("legacy plan blob is absent")
	}
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind:    CreateRef,
		Ref:     "refs/heads/release-wt/" + release,
		NewHead: &prepared.Commit,
	}}); err != nil {
		t.Fatal(err)
	}
	workspaces, err := NewWorkspaces(repository, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()
	consumer := TrackKey{Release: release, Track: "consumer"}
	request := PrepareTrackBaseRequest{
		Identity: testIdentity, Release: release, Plan: plan,
		ReleaseHead: prepared.Commit, TargetRef: "refs/heads/main",
		TargetHead: target, ApprovedTarget: target,
		Consumer: consumer, AuthoritySeed: prepared.Commit,
		ProductAdmission: product,
	}
	result, err := workspaces.ExpectedTrackBase(request)
	if err != nil {
		t.Fatalf("track base over legacy plan: %v", err)
	}
	if result.Base.IsZero() {
		t.Fatal("track base over legacy plan produced no base")
	}
}

func TestTrackBaseConfiguredRootWinsOverLegacyPlanAtRelease(t *testing.T) {
	t.Parallel()
	// A release recorded under both roots must bind the plan at the
	// configured records root (A4): the configured OID succeeds, and the
	// legacy OID is rejected even though the legacy path holds a different
	// plan blob for the same release.
	repository, target := newRepository(t, SHA1)
	record, product := inertAdmissions(t, repository, nil)
	_ = record
	release := "track-base-dual-root"
	configuredPlanPath := repository.recordRoot + "/" + release + "/plan.md"
	legacyPlanPath := LegacyRecordsRoot + "/" + release + "/plan.md"
	timestamp, err := repository.CommitTimestamp(target)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.prepareRecord(RecordRequest{
		Parent: target,
		Changes: []BlobChange{
			{Path: configuredPlanPath, Bytes: []byte("configured fixture plan\n")},
			{Path: legacyPlanPath, Bytes: []byte("legacy fixture plan\n")},
		},
		Message:   "install dual-root fixture plan\n",
		Identity:  testIdentity,
		Timestamp: timestamp + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var configuredPlan, legacyPlan OID
	entries, err := repository.ListTree(prepared.Commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Path {
		case configuredPlanPath:
			configuredPlan = entry.OID
		case legacyPlanPath:
			legacyPlan = entry.OID
		}
	}
	if configuredPlan.IsZero() || legacyPlan.IsZero() {
		t.Fatal("dual-root plan blobs are absent")
	}
	if configuredPlan == legacyPlan {
		t.Fatal("dual-root fixture plans must differ")
	}
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind:    CreateRef,
		Ref:     "refs/heads/release-wt/" + release,
		NewHead: &prepared.Commit,
	}}); err != nil {
		t.Fatal(err)
	}
	workspaces, err := NewWorkspaces(repository, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()
	consumer := TrackKey{Release: release, Track: "consumer"}
	baseRequest := PrepareTrackBaseRequest{
		Identity: testIdentity, Release: release,
		ReleaseHead: prepared.Commit, TargetRef: "refs/heads/main",
		TargetHead: target, ApprovedTarget: target,
		Consumer: consumer, AuthoritySeed: prepared.Commit,
		ProductAdmission: product,
	}
	configured := baseRequest
	configured.Plan = configuredPlan
	result, err := workspaces.ExpectedTrackBase(configured)
	if err != nil {
		t.Fatalf("configured plan at dual-root release: %v", err)
	}
	if result.Base.IsZero() {
		t.Fatal("configured plan at dual-root release produced no base")
	}
	legacy := baseRequest
	legacy.Plan = legacyPlan
	_, err = workspaces.ExpectedTrackBase(legacy)
	requireGitxErrorCode(t, err, "PLAN_MOVED")
}

func (f trackBaseFixture) authority(
	t *testing.T,
	slice string,
	producer TrackKey,
	parent OID,
	path string,
	body string,
) (TrackBaseInput, OID) {
	t.Helper()
	candidate := prepareProduct(
		t,
		f.repository,
		parent,
		[]BlobChange{{Path: path, Bytes: []byte(body)}},
		"candidate "+slice,
	)
	candidateReceipt := nextRecord(
		t,
		f.repository,
		candidate.Commit,
		"candidate-"+slice,
	)
	pass := nextRecord(
		t,
		f.repository,
		candidateReceipt,
		"pass-"+slice,
	)
	product, err := f.repository.ProductTreeIdentity(
		candidate.Commit,
		f.product,
	)
	if err != nil {
		t.Fatal(err)
	}
	return TrackBaseInput{
		Slice: slice, Producer: producer,
		PassReceipt: pass, CandidateReceipt: candidateReceipt,
		Candidate: candidate.Commit, ProductTree: product.ProductTree,
	}, pass
}

func (f trackBaseFixture) request(
	inputs []TrackBaseInput,
	before *OID,
) PrepareTrackBaseRequest {
	return PrepareTrackBaseRequest{Identity: testIdentity,
		Release: f.release, Plan: f.plan,
		ReleaseHead:      f.releaseHead,
		TargetRef:        "refs/heads/main",
		TargetHead:       f.targetHead,
		ApprovedTarget:   f.targetHead,
		Consumer:         f.consumer,
		AuthoritySeed:    f.releaseHead,
		ConsumerBefore:   before,
		Inputs:           inputs,
		ProductAdmission: f.product,
	}
}

func setTrackBaseSources(
	t *testing.T,
	fixture trackBaseFixture,
	inputs []TrackBaseInput,
	heads map[string]OID,
) []TrackBaseInput {
	t.Helper()
	operations := make([]RefOperation, 0, len(heads))
	for track, head := range heads {
		key := TrackKey{Release: fixture.release, Track: track}
		operations = append(operations, RefOperation{
			Kind: CreateRef, Ref: trackHeadRef(key), NewHead: &head,
		})
	}
	if len(operations) > 0 {
		if err := fixture.repository.AtomicUpdateRefs(operations); err != nil {
			t.Fatal(err)
		}
	}
	result := cloneTrackBaseInputs(inputs)
	for index := range result {
		result[index].SourceHead = heads[result[index].Producer.Track]
	}
	return result
}

func requireDirectHead(
	t *testing.T,
	repository *Repository,
	ref string,
	want OID,
) {
	t.Helper()
	captured := captureRefs(t, repository, ref)
	if len(captured) != 1 || captured[0].State != RefDirect ||
		captured[0].Head != want {
		t.Fatalf("%s = %#v, want %s", ref, captured, want)
	}
}

func TestPrepareTrackBaseZeroOneMultipleAndSerialInputs(t *testing.T) {
	for _, test := range []struct {
		name       string
		wantCreate bool
		inputs     func(*testing.T, trackBaseFixture) []TrackBaseInput
	}{
		{
			name:       "zero",
			wantCreate: true,
			inputs: func(_ *testing.T, _ trackBaseFixture) []TrackBaseInput {
				return []TrackBaseInput{}
			},
		},
		{
			name:       "one",
			wantCreate: true,
			inputs: func(t *testing.T, f trackBaseFixture) []TrackBaseInput {
				producer := TrackKey{Release: f.release, Track: "producer-1"}
				input, pass := f.authority(
					t, "S1", producer, f.releaseHead,
					"one.txt", "one\n",
				)
				return setTrackBaseSources(
					t, f, []TrackBaseInput{input},
					map[string]OID{producer.Track: pass},
				)
			},
		},
		{
			name:       "multiple",
			wantCreate: true,
			inputs: func(t *testing.T, f trackBaseFixture) []TrackBaseInput {
				firstKey := TrackKey{Release: f.release, Track: "producer-1"}
				secondKey := TrackKey{Release: f.release, Track: "producer-2"}
				first, firstPass := f.authority(
					t, "S1", firstKey, f.releaseHead,
					"one.txt", "one\n",
				)
				second, secondPass := f.authority(
					t, "S2", secondKey, f.releaseHead,
					"two.txt", "two\n",
				)
				return setTrackBaseSources(
					t, f, []TrackBaseInput{first, second},
					map[string]OID{
						firstKey.Track:  firstPass,
						secondKey.Track: secondPass,
					},
				)
			},
		},
		{
			name:       "serial-same-ref",
			wantCreate: true,
			inputs: func(t *testing.T, f trackBaseFixture) []TrackBaseInput {
				producer := TrackKey{Release: f.release, Track: "producer-1"}
				first, firstPass := f.authority(
					t, "S1", producer, f.releaseHead,
					"one.txt", "one\n",
				)
				second, secondPass := f.authority(
					t, "S2", producer, firstPass,
					"two.txt", "two\n",
				)
				return setTrackBaseSources(
					t, f, []TrackBaseInput{first, second},
					map[string]OID{producer.Track: secondPass},
				)
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := newTrackBaseFixture(t, test.name)
			inputs := test.inputs(t, fixture)
			request := fixture.request(inputs, nil)
			expected, err := fixture.workspaces.ExpectedTrackBase(request)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := fixture.workspaces.ExpectedTrackBase(request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(expected, repeated) ||
				expected.Changed != test.wantCreate ||
				(test.wantCreate &&
					expected.Action != TrackBaseCreate) ||
				(!test.wantCreate &&
					expected.Action != TrackBaseNoop) {
				t.Fatalf("expected base is not deterministic: %#v / %#v", expected, repeated)
			}
			result, err := fixture.workspaces.PrepareTrackBase(request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result, expected) {
				t.Fatalf("prepared = %#v, want %#v", result, expected)
			}
			if test.wantCreate {
				requireDirectHead(
					t,
					fixture.repository,
					trackHeadRef(fixture.consumer),
					result.Base,
				)
			} else {
				captured := captureRefs(
					t,
					fixture.repository,
					trackHeadRef(fixture.consumer),
				)
				if captured[0].State != RefAbsent {
					t.Fatalf("zero-input prepare moved consumer: %#v", captured[0])
				}
			}
			for _, input := range inputs {
				for _, ancestor := range []OID{
					input.Candidate,
					input.CandidateReceipt,
					input.PassReceipt,
				} {
					contained, err := fixture.repository.IsAncestor(
						ancestor,
						result.Base,
					)
					if err != nil || !contained {
						t.Fatalf("%s is absent from %s: %v", ancestor, result.Base, err)
					}
				}
			}
			var retryBefore *OID
			if test.wantCreate {
				retryBefore = &result.Base
			}
			retryRequest := fixture.request(inputs, retryBefore)
			retry, err := fixture.workspaces.PrepareTrackBase(retryRequest)
			if err != nil {
				t.Fatal(err)
			}
			if retry.Changed || retry.Action != TrackBaseNoop ||
				retry.Base != result.Base {
				t.Fatalf("already-prepared retry = %#v", retry)
			}
		})
	}
}

func TestPrepareTrackBaseConflictAndAuthorityDriftMoveNoConsumerRef(t *testing.T) {
	t.Run("conflict", func(t *testing.T) {
		fixture := newTrackBaseFixture(t, "conflict")
		firstKey := TrackKey{Release: fixture.release, Track: "producer-1"}
		secondKey := TrackKey{Release: fixture.release, Track: "producer-2"}
		first, firstPass := fixture.authority(
			t, "S1", firstKey, fixture.releaseHead,
			"shared.txt", "left\n",
		)
		second, secondPass := fixture.authority(
			t, "S2", secondKey, fixture.releaseHead,
			"shared.txt", "right\n",
		)
		inputs := setTrackBaseSources(
			t,
			fixture,
			[]TrackBaseInput{first, second},
			map[string]OID{
				firstKey.Track:  firstPass,
				secondKey.Track: secondPass,
			},
		)
		if _, err := fixture.workspaces.PrepareTrackBase(
			fixture.request(inputs, nil),
		); err == nil {
			t.Fatal("conflicting inputs prepared a base")
		}
		captured := captureRefs(
			t,
			fixture.repository,
			trackHeadRef(fixture.consumer),
		)
		if captured[0].State != RefAbsent {
			t.Fatalf("conflict moved consumer: %#v", captured[0])
		}
	})

	t.Run("producer-drift", func(t *testing.T) {
		fixture := newTrackBaseFixture(t, "producer-drift")
		producer := TrackKey{Release: fixture.release, Track: "producer-1"}
		input, pass := fixture.authority(
			t, "S1", producer, fixture.releaseHead,
			"one.txt", "one\n",
		)
		inputs := setTrackBaseSources(
			t, fixture, []TrackBaseInput{input},
			map[string]OID{producer.Track: pass},
		)
		moved := nextRecord(t, fixture.repository, pass, "producer-moved")
		_, err := fixture.workspaces.PrepareTrackBaseWithClaim(
			fixture.request(inputs, nil),
			func(_ PrepareTrackBaseResult) error {
				return fixture.repository.AtomicUpdateRefs([]RefOperation{{
					Kind: UpdateRef, Ref: trackHeadRef(producer),
					NewHead: &moved, Expected: &pass,
				}})
			},
		)
		if err == nil {
			t.Fatal("producer drift was accepted")
		}
		captured := captureRefs(
			t,
			fixture.repository,
			trackHeadRef(fixture.consumer),
		)
		if captured[0].State != RefAbsent {
			t.Fatalf("producer drift moved consumer: %#v", captured[0])
		}
		requireDirectHead(
			t,
			fixture.repository,
			trackHeadRef(producer),
			moved,
		)
	})

	t.Run("same-ref-incomparable", func(t *testing.T) {
		fixture := newTrackBaseFixture(t, "same-ref-incomparable")
		producer := TrackKey{Release: fixture.release, Track: "producer-1"}
		first, firstPass := fixture.authority(
			t, "S1", producer, fixture.releaseHead,
			"one.txt", "one\n",
		)
		second, secondPass := fixture.authority(
			t, "S2", producer, fixture.releaseHead,
			"two.txt", "two\n",
		)
		source, err := fixture.repository.PrepareComposition(
			CompositionRequest{Identity: testIdentity,
				Expected: firstPass, Candidate: secondPass,
				TargetRef:        trackHeadRef(producer),
				ProductAdmission: fixture.product,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		inputs := []TrackBaseInput{first, second}
		for index := range inputs {
			inputs[index].SourceHead = source.Commit
		}
		if err := fixture.repository.AtomicUpdateRefs([]RefOperation{{
			Kind: CreateRef, Ref: trackHeadRef(producer),
			NewHead: &source.Commit,
		}}); err != nil {
			t.Fatal(err)
		}
		_, err = fixture.workspaces.PrepareTrackBase(
			fixture.request(inputs, nil),
		)
		requireGitxErrorCode(t, err, "INVALID_TRACK_TOPOLOGY")
		captured := captureRefs(
			t,
			fixture.repository,
			trackHeadRef(fixture.consumer),
		)
		if captured[0].State != RefAbsent {
			t.Fatalf("incomparable inputs moved consumer: %#v", captured[0])
		}
	})

	t.Run("changed-product-identity", func(t *testing.T) {
		fixture := newTrackBaseFixture(t, "changed-product-identity")
		producer := TrackKey{Release: fixture.release, Track: "producer-1"}
		input, pass := fixture.authority(
			t, "S1", producer, fixture.releaseHead,
			"one.txt", "one\n",
		)
		inputs := setTrackBaseSources(
			t, fixture, []TrackBaseInput{input},
			map[string]OID{producer.Track: pass},
		)
		inputs[0].ProductTree = "sha256:" +
			"0000000000000000000000000000000000000000000000000000000000000000"
		_, err := fixture.workspaces.PrepareTrackBase(
			fixture.request(inputs, nil),
		)
		requireGitxErrorCode(t, err, "INVALID_PASS_AUTHORITY")
		captured := captureRefs(
			t,
			fixture.repository,
			trackHeadRef(fixture.consumer),
		)
		if captured[0].State != RefAbsent {
			t.Fatalf("changed product moved consumer: %#v", captured[0])
		}
	})
}

func TestPrepareTrackBaseSeparatesAuthoritySeedFromConsumerCAS(t *testing.T) {
	fixture := newTrackBaseFixture(t, "seed-cas")
	consumerRef := trackHeadRef(fixture.consumer)
	if err := fixture.repository.AtomicUpdateRefs([]RefOperation{{
		Kind: CreateRef, Ref: consumerRef, NewHead: &fixture.targetHead,
	}}); err != nil {
		t.Fatal(err)
	}
	before := fixture.targetHead
	request := fixture.request(nil, &before)
	result, err := fixture.workspaces.PrepareTrackBase(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Seed != fixture.releaseHead ||
		result.Base != fixture.releaseHead ||
		result.ConsumerBefore == nil ||
		*result.ConsumerBefore != fixture.targetHead ||
		!result.Changed ||
		result.Action != TrackBaseUpdate {
		t.Fatalf("seed/CAS result = %#v", result)
	}
	requireDirectHead(t, fixture.repository, consumerRef, fixture.releaseHead)
}

func TestPrepareTrackBaseRejectsNonFastForwardConsumerMovement(t *testing.T) {
	for _, test := range []struct {
		name   string
		before func(*testing.T, trackBaseFixture) OID
	}{
		{
			name: "descendant",
			before: func(t *testing.T, fixture trackBaseFixture) OID {
				return prepareProduct(
					t,
					fixture.repository,
					fixture.releaseHead,
					[]BlobChange{{
						Path:  "unrecorded.txt",
						Bytes: []byte("unrecorded work\n"),
					}},
					"unrecorded consumer work",
				).Commit
			},
		},
		{
			name: "unrelated",
			before: func(t *testing.T, fixture trackBaseFixture) OID {
				return prepareProduct(
					t,
					fixture.repository,
					fixture.targetHead,
					[]BlobChange{{
						Path:  "foreign.txt",
						Bytes: []byte("foreign work\n"),
					}},
					"unrelated consumer work",
				).Commit
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := newTrackBaseFixture(t, "rewind-"+test.name)
			consumerRef := trackHeadRef(fixture.consumer)
			before := test.before(t, fixture)
			if err := fixture.repository.AtomicUpdateRefs([]RefOperation{{
				Kind:    CreateRef,
				Ref:     consumerRef,
				NewHead: &before,
			}}); err != nil {
				t.Fatal(err)
			}
			_, err := fixture.workspaces.PrepareTrackBase(
				fixture.request([]TrackBaseInput{}, &before),
			)
			requireGitxErrorCode(t, err, "CHANGED_OWNER_HEAD")
			requireDirectHead(t, fixture.repository, consumerRef, before)
		})
	}
}

func TestPrepareTrackBaseSeparatesLiveAndApprovedTargets(t *testing.T) {
	fixture := newTrackBaseFixture(t, "approved-target")
	approvedTarget := prepareProduct(
		t,
		fixture.repository,
		fixture.targetHead,
		[]BlobChange{{Path: "approved.txt", Bytes: []byte("approved\n")}},
		"approved target",
	)
	liveTarget := prepareProduct(
		t,
		fixture.repository,
		approvedTarget.Commit,
		[]BlobChange{{Path: "live.txt", Bytes: []byte("live\n")}},
		"live target",
	)
	if err := fixture.repository.AtomicUpdateRefs([]RefOperation{{
		Kind: UpdateRef, Ref: "refs/heads/main",
		NewHead: &liveTarget.Commit, Expected: &fixture.targetHead,
	}}); err != nil {
		t.Fatal(err)
	}
	request := fixture.request(nil, nil)
	request.TargetHead = liveTarget.Commit
	request.ApprovedTarget = approvedTarget.Commit
	result, err := fixture.workspaces.PrepareTrackBase(request)
	if err != nil {
		t.Fatal(err)
	}
	parents, err := fixture.repository.Parents(result.Base)
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 2 ||
		parents[0] != fixture.releaseHead ||
		parents[1] != approvedTarget.Commit {
		t.Fatalf("approved target parents = %v", parents)
	}
	requireDirectHead(
		t,
		fixture.repository,
		trackHeadRef(fixture.consumer),
		result.Base,
	)
}

func TestReconcileTrackBaseAllOldAllNewAdvancedAndAmbiguous(t *testing.T) {
	fixture := newTrackBaseFixture(t, "reconcile")
	producer := TrackKey{Release: fixture.release, Track: "producer-1"}
	input, pass := fixture.authority(
		t, "S1", producer, fixture.releaseHead,
		"one.txt", "one\n",
	)
	inputs := setTrackBaseSources(
		t, fixture, []TrackBaseInput{input},
		map[string]OID{producer.Track: pass},
	)
	request := fixture.request(inputs, nil)
	expected, err := fixture.workspaces.ExpectedTrackBase(request)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := fixture.workspaces.ReconcileTrackBase(
		request,
		expected,
	)
	if err != nil || disposition != TrackBaseAllOld {
		t.Fatalf("all-old = %q, %v", disposition, err)
	}
	if _, err := fixture.workspaces.PrepareTrackBase(request); err != nil {
		t.Fatal(err)
	}
	disposition, err = fixture.workspaces.ReconcileTrackBase(
		request,
		expected,
	)
	if err != nil || disposition != TrackBaseAllNew {
		t.Fatalf("all-new = %q, %v", disposition, err)
	}
	advanced := nextRecord(
		t,
		fixture.repository,
		expected.Base,
		"later-consumer-work",
	)
	if err := fixture.repository.AtomicUpdateRefs([]RefOperation{{
		Kind: UpdateRef, Ref: trackHeadRef(fixture.consumer),
		NewHead: &advanced, Expected: &expected.Base,
	}}); err != nil {
		t.Fatal(err)
	}
	disposition, err = fixture.workspaces.ReconcileTrackBase(
		request,
		expected,
	)
	if err != nil || disposition != TrackBaseAdvanced {
		t.Fatalf("advanced = %q, %v", disposition, err)
	}
	movedProducer := nextRecord(
		t,
		fixture.repository,
		pass,
		"later-producer-work",
	)
	if err := fixture.repository.AtomicUpdateRefs([]RefOperation{{
		Kind: UpdateRef, Ref: trackHeadRef(producer),
		NewHead: &movedProducer, Expected: &pass,
	}}); err != nil {
		t.Fatal(err)
	}
	disposition, err = fixture.workspaces.ReconcileTrackBase(
		request,
		expected,
	)
	if err != nil || disposition != TrackBaseAmbiguous {
		t.Fatalf("ambiguous = %q, %v", disposition, err)
	}
	requireDirectHead(
		t,
		fixture.repository,
		trackHeadRef(fixture.consumer),
		advanced,
	)
}

func TestPrepareTrackBaseInputBoundaryMatchesBatonListBound(t *testing.T) {
	fixture := newTrackBaseFixture(t, "boundary")
	inputs := make([]TrackBaseInput, MaxTrackBaseInputs+1)
	for index := range inputs {
		inputs[index].Slice = fmt.Sprintf("S%d", index)
	}
	_, err := fixture.workspaces.ExpectedTrackBase(
		fixture.request(inputs, nil),
	)
	requireGitxErrorCode(t, err, "RESOURCE_LIMIT")

	inputs = inputs[:MaxTrackBaseInputs]
	_, err = fixture.workspaces.ExpectedTrackBase(
		fixture.request(inputs, nil),
	)
	var typed *Error
	if !errors.As(err, &typed) || typed.Code == "RESOURCE_LIMIT" {
		t.Fatalf("boundary introduced a narrower resource limit: %v", err)
	}
}
