package baton_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func exportedMethodNames(value any) []string {
	typ := reflect.TypeOf(value)
	result := make([]string, typ.NumMethod())
	for index := range result {
		result[index] = typ.Method(index).Name
	}
	sort.Strings(result)
	return result
}

func TestExternalActionSurfaceIsExactlyTheFourBatonFacades(t *testing.T) {
	t.Parallel()
	got := exportedMethodNames((*baton.Actions)(nil))
	want := []string{
		"AppendReceipt",
		"MergePassedCandidate",
		"PrepareAssembly",
		"RecordPlanRevision",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Actions methods = %v, want %v", got, want)
	}
	if got := exportedMethodNames(baton.GitRepository{}); len(got) != 0 {
		t.Fatalf("GitRepository leaks raw methods: %v", got)
	}
}

func TestPublicInputsAndResultsRemainStronglyTyped(t *testing.T) {
	t.Parallel()
	for name, value := range map[string]any{
		"RecordPlanRevisionInput":   baton.RecordPlanRevisionInput{},
		"AppendReceiptInput":        baton.AppendReceiptInput{},
		"PrepareAssemblyInput":      baton.PrepareAssemblyInput{},
		"MergePassedCandidateInput": baton.MergePassedCandidateInput{},
		"ActionResult":              baton.ActionResult{},
		"State":                     baton.State{},
	} {
		typ := reflect.TypeOf(value)
		if typ.Kind() != reflect.Struct {
			t.Fatalf("%s is %s, want struct", name, typ.Kind())
		}
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			if field.Type.Kind() == reflect.Interface {
				t.Fatalf("%s.%s is an open interface", name, field.Name)
			}
		}
	}
}
