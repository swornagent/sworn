package runtime

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Sworn's own conformance profile: the declared list of observable behaviors
// Sworn's public product contract promises, and the surfaces each behavior
// must be observable on.
//
// The profile is a declaration, never a certification. It records what has to
// be proven; it says nothing about whether anything was proven. Certification
// is executable elsewhere: a real-binary suite registers, at run time, the
// anchor that actually exercised each (case, surface) pair, and fails when a
// declared pair finished the run without one. Nothing in the delivery
// lifecycle -- plan admission, dispatch, the scheduler, or Merge -- reads any
// of it.

// ConformanceProfileVersion identifies the Sworn conformance profile schema.
const ConformanceProfileVersion = "sworn.conformance-profile/v1"

//go:embed conformance_profile.json
var profileJSON []byte

// Surface is one path a caller can reach Sworn's behavior through.
type ConformanceSurface struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Case is one observable behavior of Sworn's public contract, together with
// every surface the behavior must be observable on.
type ConformanceCase struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Surfaces []string `json:"surfaces"`
}

// Profile is the admitted Sworn conformance profile.
type ConformanceProfile struct {
	SchemaVersion string               `json:"schema_version"`
	Name          string               `json:"profile"`
	Subject       string               `json:"subject"`
	Surfaces      []ConformanceSurface `json:"surfaces"`
	Cases         []ConformanceCase    `json:"cases"`
}

// Digest is the sha256 of the exact embedded profile bytes, so a recorded
// conformance result can name the profile revision it was produced against.
func ConformanceDigest() string {
	sum := sha256.Sum256(profileJSON)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Bytes returns a copy of the exact embedded profile bytes.
func ConformanceBytes() []byte {
	return append([]byte(nil), profileJSON...)
}

// ConformanceObligation is one (case, surface) pair the profile declares. A
// certification gate must account for every one of them.
type ConformanceObligation struct {
	Case    string
	Surface string
}

// Load admits the embedded profile. It fails closed on any malformed,
// duplicated, empty, or dangling declaration, so a profile that silently lost
// a case or names a surface that does not exist cannot be used at all.
func LoadConformanceProfile() (ConformanceProfile, error) {
	var profile ConformanceProfile
	if err := json.Unmarshal(profileJSON, &profile); err != nil {
		return ConformanceProfile{}, fmt.Errorf("conformance profile: %w", err)
	}
	if profile.SchemaVersion != ConformanceProfileVersion {
		return ConformanceProfile{}, fmt.Errorf(
			"conformance profile schema_version = %q, want %q",
			profile.SchemaVersion, ConformanceProfileVersion,
		)
	}
	if profile.Name == "" || profile.Subject != "sworn" {
		return ConformanceProfile{}, errors.New("conformance profile is missing its identity")
	}
	if len(profile.Surfaces) == 0 || len(profile.Cases) == 0 {
		return ConformanceProfile{}, errors.New("conformance profile declares no surfaces or no cases")
	}
	surfaces := make(map[string]bool, len(profile.Surfaces))
	for _, surface := range profile.Surfaces {
		if surface.ID == "" || surface.Text == "" {
			return ConformanceProfile{}, fmt.Errorf("conformance surface %#v is incomplete", surface)
		}
		if surfaces[surface.ID] {
			return ConformanceProfile{}, fmt.Errorf("conformance profile repeats surface %q", surface.ID)
		}
		surfaces[surface.ID] = true
	}
	cases := make(map[string]bool, len(profile.Cases))
	for _, item := range profile.Cases {
		if item.ID == "" || item.Text == "" || len(item.Surfaces) == 0 {
			return ConformanceProfile{}, fmt.Errorf("conformance case %#v is incomplete", item)
		}
		if cases[item.ID] {
			return ConformanceProfile{}, fmt.Errorf("conformance profile repeats case %q", item.ID)
		}
		cases[item.ID] = true
		seen := make(map[string]bool, len(item.Surfaces))
		for _, surface := range item.Surfaces {
			if !surfaces[surface] {
				return ConformanceProfile{}, fmt.Errorf(
					"conformance case %q names undeclared surface %q", item.ID, surface,
				)
			}
			if seen[surface] {
				return ConformanceProfile{}, fmt.Errorf(
					"conformance case %q repeats surface %q", item.ID, surface,
				)
			}
			seen[surface] = true
		}
	}
	return profile, nil
}

// Obligations lists every declared (case, surface) pair in a stable order.
func (p ConformanceProfile) Obligations() []ConformanceObligation {
	var result []ConformanceObligation
	for _, item := range p.Cases {
		for _, surface := range item.Surfaces {
			result = append(result, ConformanceObligation{Case: item.ID, Surface: surface})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Case != result[right].Case {
			return result[left].Case < result[right].Case
		}
		return result[left].Surface < result[right].Surface
	})
	return result
}
