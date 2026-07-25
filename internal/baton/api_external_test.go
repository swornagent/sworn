package baton_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func methodNames(value any) []string {
	typ := reflect.TypeOf(value)
	result := make([]string, typ.NumMethod())
	for index := range result {
		result[index] = typ.Method(index).Name
	}
	sort.Strings(result)
	return result
}

func TestExternalProjectionAndActionSurfacesAreClosed(t *testing.T) {
	t.Parallel()
	if got, want := methodNames(baton.Reader{}), []string{"Capture"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Reader methods = %v, want %v", got, want)
	}
	if got, want := methodNames(baton.Snapshot{}), []string{
		"CapturedRefs",
		"MaterializationFor",
		"MayAdvanceWork",
		"NextWorkForTrack",
		"ReleaseReadyForAssembly",
		"SelectAssembly",
		"SelectWork",
		"String",
		"TrackReadyForComposition",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Snapshot methods = %v, want %v", got, want)
	}
	if got, want := methodNames((*baton.Actions)(nil)), []string{
		"ComposeTrack",
		"InstallApprovedPlan",
		"IntegrateRelease",
		"MaterializeTrack",
		"PrepareAssembly",
		"ReboundPristinePlan",
		"RecordTransition",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Actions methods = %v, want %v", got, want)
	}
	for name, value := range map[string]any{
		"Plan": baton.Plan{}, "Status": baton.Status{}, "Reader": baton.Reader{},
		"Snapshot": baton.Snapshot{}, "Receipt": baton.Receipt{},
	} {
		typ := reflect.TypeOf(value)
		for index := 0; index < typ.NumField(); index++ {
			if typ.Field(index).IsExported() {
				t.Fatalf("%s exposes mutable field %s", name, typ.Field(index).Name)
			}
		}
	}
}
