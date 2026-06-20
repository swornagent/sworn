// Package scheduler builds execution plans from release-board track info.
// It topologically sorts tracks into concurrent phases based on their
// depends_on edges, enabling parallel execution of independent tracks.
//
// Stdlib only — zero runtime dependencies.
package scheduler

import (
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/board"
)

// Phase is a set of tracks that may be executed concurrently.
// All tracks in a phase have all their dependencies satisfied by tracks
// in earlier phases (lower index = earlier).
type Phase struct {
	Tracks []board.TrackInfo
}

// ExecutionPlan is the ordered list of phases for a release. Phase[0] runs
// first (no dependencies), Phase[1] waits for Phase[0], etc.
type ExecutionPlan struct {
	Phases []Phase
	// TrackLookup maps track ID → info for efficient indexing
	TrackLookup map[string]board.TrackInfo
}

// BuildPlan topologically sorts tracks into an ExecutionPlan.
// tracks should come from board.ParseTracks().
//
// Returns an error if a dependency cycle is detected or a track depends on
// a non-existent track.
func BuildPlan(tracks []board.TrackInfo) (*ExecutionPlan, error) {
	// Build lookup and inbound-edge counts.
	trackMap := make(map[string]board.TrackInfo, len(tracks))
	inDegree := make(map[string]int, len(tracks))

	for _, t := range tracks {
		trackMap[t.ID] = t
		inDegree[t.ID] = 0
	}

	// Validate dependencies and compute in-degrees.
	for _, t := range tracks {
		for _, dep := range t.DependsOn {
			if _, ok := trackMap[dep]; !ok {
				return nil, fmt.Errorf("scheduler: track %q depends on non-existent track %q", t.ID, dep)
			}
			inDegree[t.ID]++
		}
	}

	// Kahn's algorithm for topological sort into phases.
	plan := &ExecutionPlan{
		TrackLookup: trackMap,
	}

	// Track which tracks have been added.
	added := make(map[string]bool, len(tracks))

	// Worklist of tracks whose dependencies are all satisfied.
	worklist := make([]string, 0)
	for _, t := range tracks {
		if inDegree[t.ID] == 0 {
			worklist = append(worklist, t.ID)
		}
	}

	// Process phases: each phase = all tracks in the current worklist.
	// Their removal may unblock new tracks for the next phase.
	for len(worklist) > 0 {
		phase := Phase{}

		// Copy current worklist into the phase.
		for _, id := range worklist {
			phase.Tracks = append(phase.Tracks, trackMap[id])
			added[id] = true
		}

		plan.Phases = append(plan.Phases, phase)

		// Build the next worklist: tracks whose deps are all resolved.
		// Collect candidates by walking all tracks not yet added.
		nextWorklist := make([]string, 0)
		for _, t := range tracks {
			if added[t.ID] {
				continue
			}
			// Check if all deps are now satisfied.
			allDepsAdded := true
			for _, dep := range t.DependsOn {
				if !added[dep] {
					allDepsAdded = false
					break
				}
			}
			if allDepsAdded {
				nextWorklist = append(nextWorklist, t.ID)
			}
		}
		worklist = nextWorklist
	}

	// Check for cycle: if not all tracks were placed, there's a cycle.
	if len(added) != len(tracks) {
		var missing []string
		for _, t := range tracks {
			if !added[t.ID] {
				missing = append(missing, t.ID)
			}
		}
		return nil, fmt.Errorf("scheduler: dependency cycle detected among tracks: %s", strings.Join(missing, ", "))
	}

	return plan, nil
}

// PhaseOf returns the phase index (0-based) containing the given track ID,
// or -1 if the track is not in the plan.
func (p *ExecutionPlan) PhaseOf(trackID string) int {
	for i, phase := range p.Phases {
		for _, t := range phase.Tracks {
			if t.ID == trackID {
				return i
			}
		}
	}
	return -1
}

// Summary returns a human-readable summary of the execution plan.
func (p *ExecutionPlan) Summary() string {
	var b strings.Builder
	b.WriteString("Execution plan:\n")
	for i, phase := range p.Phases {
		var ids []string
		for _, t := range phase.Tracks {
			ids = append(ids, t.ID)
		}
		b.WriteString(fmt.Sprintf("  Phase %d: %s\n", i+1, strings.Join(ids, ", ")))
	}
	return b.String()
}