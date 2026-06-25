package board

import (
	"reflect"
	"testing"
)

func TestParseTracks(t *testing.T) {
	const body = `
title: Release board
tracks:
  - id: T1-concurrency-core
    slices: [S01-process-ownership, S02a-run-refactor, S02b-concurrent-scheduler]
    depends_on: null
    worktree_path: /tmp/wt/T1
    worktree_branch: track/x/T1
    state: in_progress
  - id: T2-monitoring
    slices: [S04a-tui-foundation]
    depends_on: T1-concurrency-core
    worktree_path: ''
    worktree_branch: track/x/T2
    state: planned
  - id: T5-providers
    slices: [S10-provider-foundation, S11-anthropic-driver]
    depends_on: [T1-concurrency-core, T3-commercial]
    worktree_path: ''
    worktree_branch: track/x/T5
    state: planned
`

	tracks := ParseTracks(body)
	if len(tracks) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(tracks))
	}

	// T1 — null depends_on
	if tracks[0].ID != "T1-concurrency-core" {
		t.Errorf("track[0].ID = %q, want T1-concurrency-core", tracks[0].ID)
	}
	if len(tracks[0].DependsOn) != 0 {
		t.Errorf("track[0].DependsOn = %v, want empty (null)", tracks[0].DependsOn)
	}
	if len(tracks[0].Slices) != 3 {
		t.Errorf("track[0].Slices = %v, want 3 slices", tracks[0].Slices)
	}
	if tracks[0].State != "in_progress" {
		t.Errorf("track[0].State = %q, want in_progress", tracks[0].State)
	}

	// T2 — single string depends_on
	if tracks[1].ID != "T2-monitoring" {
		t.Errorf("track[1].ID = %q, want T2-monitoring", tracks[1].ID)
	}
	if len(tracks[1].DependsOn) != 1 || tracks[1].DependsOn[0] != "T1-concurrency-core" {
		t.Errorf("track[1].DependsOn = %v, want [T1-concurrency-core]", tracks[1].DependsOn)
	}

	// T5 — list depends_on
	if tracks[2].ID != "T5-providers" {
		t.Errorf("track[2].ID = %q, want T5-providers", tracks[2].ID)
	}
	if len(tracks[2].DependsOn) != 2 {
		t.Errorf("track[2].DependsOn = %v, want 2 deps", tracks[2].DependsOn)
	}
	if tracks[2].DependsOn[0] != "T1-concurrency-core" || tracks[2].DependsOn[1] != "T3-commercial" {
		t.Errorf("track[2].DependsOn = %v, want [T1-concurrency-core T3-commercial]", tracks[2].DependsOn)
	}
}

func TestParseTracks_BlockStyleSlices(t *testing.T) {
	const body = `
tracks:
  - id: T1
    slices:
      - S01
      - S02
    depends_on: null
    worktree_branch: b
  - id: T2
    slices: []
    depends_on: T1
    worktree_branch: c
`

	tracks := ParseTracks(body)
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if len(tracks[0].Slices) != 2 {
		t.Errorf("T1 Slices = %v, want [S01 S02]", tracks[0].Slices)
	}
	if len(tracks[1].Slices) != 0 {
		t.Errorf("T2 Slices = %v, want []", tracks[1].Slices)
	}
}

func TestParseTracks_NoTracks(t *testing.T) {
	tracks := ParseTracks("title: Only fields\nrelease_index: 1")
	if len(tracks) != 0 {
		t.Errorf("expected 0 tracks, got %d", len(tracks))
	}
}

func TestParseTracks_BlockDependsOn(t *testing.T) {
	const body = `
tracks:
  - id: T3
    slices: [S03]
    depends_on:
      - T1
      - T2
    worktree_branch: b
`

	tracks := ParseTracks(body)
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}

	want := []string{"T1", "T2"}
	if !reflect.DeepEqual(tracks[0].DependsOn, want) {
		t.Errorf("DependsOn = %v, want %v", tracks[0].DependsOn, want)
	}
}

func TestParseTrackID(t *testing.T) {
	id, ok := ParseTrackID("  - id: T1-engine")
	if !ok || id != "T1-engine" {
		t.Errorf("ParseTrackID = (%q, %v), want (T1-engine, true)", id, ok)
	}

	_, ok = ParseTrackID("title: Board")
	if ok {
		t.Errorf("ParseTrackID on non-track line returned ok=true")
	}
}