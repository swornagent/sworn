package driver

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestNoUnlabelledContinuationInvalidConstructor(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	labelledCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := entry.Name()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", path, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if ident, isIdent := call.Fun.(*ast.Ident); isIdent {
					if ident.Name == "fail" && len(call.Args) == 1 {
						if lit, isLit := call.Args[0].(*ast.BasicLit); isLit {
							if lit.Value == `"CONTINUATION_INVALID"` {
								pos := fset.Position(call.Pos())
								t.Errorf("unlabelled fail(\"CONTINUATION_INVALID\") in %s:%d", path, pos.Line)
							}
						}
					}
					if ident.Name == "failContinuation" && len(call.Args) == 1 {
						labelledCount++
						if lit, isLit := call.Args[0].(*ast.BasicLit); isLit {
							val := strings.Trim(lit.Value, `"`)
							if val == "" {
								pos := fset.Position(call.Pos())
								t.Errorf("empty site label in failContinuation at %s:%d", path, pos.Line)
							}
						} else {
							pos := fset.Position(call.Pos())
							t.Errorf("non-literal site label in failContinuation at %s:%d", path, pos.Line)
						}
					}
				}
			}
			if comp, ok := n.(*ast.CompositeLit); ok {
				if path == "continuation_labels_test.go" {
					return true
				}
				var typeName string
				if ident, ok := comp.Type.(*ast.Ident); ok {
					typeName = ident.Name
				}
				if typeName == "ContractError" {
					hasContinuationInvalid := false
					hasDetail := false
					for _, elt := range comp.Elts {
						if kve, ok := elt.(*ast.KeyValueExpr); ok {
							if kIdent, ok := kve.Key.(*ast.Ident); ok {
								if kIdent.Name == "Code" {
									if lit, ok := kve.Value.(*ast.BasicLit); ok && lit.Value == `"CONTINUATION_INVALID"` {
										hasContinuationInvalid = true
									}
								}
								if kIdent.Name == "Detail" {
									if lit, ok := kve.Value.(*ast.BasicLit); ok && strings.Trim(lit.Value, `"`) != "" {
										hasDetail = true
									} else if ident, ok := kve.Value.(*ast.Ident); ok && ident.Name != "" {
										hasDetail = true
									}
								}
							}
						}
					}
					if hasContinuationInvalid && !hasDetail {
						pos := fset.Position(comp.Pos())
						t.Errorf("unlabelled ContractError literal with CONTINUATION_INVALID in %s:%d", path, pos.Line)
					}
				}
			}
			return true
		})
	}

	if labelledCount == 0 {
		t.Fatal("expected labelled continuation constructors to be found, got 0")
	}
	t.Logf("verified %d labelled continuation exit constructors across package driver", labelledCount)
}

func TestResponsesFunctionCallDiscreteLabels(t *testing.T) {
	t.Parallel()

	validItem := func() map[string]any {
		return map[string]any{
			"type":      "function_call",
			"call_id":   "call-001",
			"name":      "Read",
			"arguments": `{"path":"a.txt"}`,
			"status":    "completed",
		}
	}

	tests := []struct {
		name      string
		modify    func(map[string]any, *continuationLedger)
		wantLabel string
	}{
		{
			name: "item type mismatch",
			modify: func(item map[string]any, _ *continuationLedger) {
				item["type"] = "message"
			},
			wantLabel: "continuation.toolcall_decode.item_type_mismatch",
		},
		{
			name: "missing call id",
			modify: func(item map[string]any, _ *continuationLedger) {
				delete(item, "call_id")
			},
			wantLabel: "continuation.toolcall_decode.missing_call_id",
		},
		{
			name: "missing name",
			modify: func(item map[string]any, _ *continuationLedger) {
				delete(item, "name")
			},
			wantLabel: "continuation.toolcall_decode.missing_name",
		},
		{
			name: "missing arguments",
			modify: func(item map[string]any, _ *continuationLedger) {
				delete(item, "arguments")
			},
			wantLabel: "continuation.toolcall_decode.missing_arguments",
		},
		{
			name: "status incomplete",
			modify: func(item map[string]any, _ *continuationLedger) {
				item["status"] = "in_progress"
			},
			wantLabel: "continuation.toolcall_decode.status_incomplete",
		},
		{
			name: "correlate reuse",
			modify: func(item map[string]any, ledger *continuationLedger) {
				_ = ledger.correlate("call-001")
			},
			wantLabel: "continuation.toolcall_decode.correlate_reuse",
		},
		{
			name: "invalid name pattern",
			modify: func(item map[string]any, _ *continuationLedger) {
				item["name"] = "invalid tool name!"
			},
			wantLabel: "continuation.toolcall_decode.invalid_name_pattern",
		},
		{
			name: "empty arguments length",
			modify: func(item map[string]any, _ *continuationLedger) {
				item["arguments"] = ""
			},
			wantLabel: "continuation.toolcall_decode.arguments_length_out_of_bounds",
		},
		{
			name: "arguments length over limit",
			modify: func(item map[string]any, _ *continuationLedger) {
				item["arguments"] = strings.Repeat("x", MaxToolArgumentBytes+1)
			},
			wantLabel: "continuation.toolcall_decode.arguments_length_out_of_bounds",
		},
		{
			name: "arguments json invalid",
			modify: func(item map[string]any, _ *continuationLedger) {
				item["arguments"] = "{malformed json"
			},
			wantLabel: "continuation.toolcall_decode.arguments_json_invalid",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ledger := newContinuationLedger()
			item := validItem()
			tt.modify(item, ledger)

			call, err := responsesFunctionCall(ledger, make(map[string]struct{}), item)
			if err == nil {
				t.Fatalf("expected error, got tool call: %#v", call)
			}
			if !IsCode(err, "CONTINUATION_INVALID") {
				t.Fatalf("expected CONTINUATION_INVALID, got: %v", err)
			}
			var contractErr *ContractError
			if !asContractError(err, &contractErr) || contractErr.Detail != tt.wantLabel {
				t.Fatalf("expected detail %q, got: %#v", tt.wantLabel, err)
			}
		})
	}

	t.Run("valid function call succeeds", func(t *testing.T) {
		t.Parallel()
		ledger := newContinuationLedger()
		item := validItem()
		call, err := responsesFunctionCall(ledger, make(map[string]struct{}), item)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if call.ID != "call-001" || call.Name != "Read" || string(call.Arguments) != `{"path":"a.txt"}` {
			t.Fatalf("unexpected call result: %#v", call)
		}
	})
}

func TestContinuationMechanismLabels_BudgetExhaustion(t *testing.T) {
	t.Parallel()

	t.Run("step budget exhausted", func(t *testing.T) {
		ledger := newContinuationLedger()
		ledger.steps = MaxContinuationSteps
		_, err := ledger.retain(opaqueField{kind: opaqueText, body: []byte("step")})
		if !IsCode(err, "CONTINUATION_INVALID") {
			t.Fatalf("expected CONTINUATION_INVALID, got: %v", err)
		}
		var contractErr *ContractError
		if !asContractError(err, &contractErr) || contractErr.Detail != "continuation.ledger.step_budget_exhausted" {
			t.Fatalf("expected detail continuation.ledger.step_budget_exhausted, got %#v", err)
		}
	})

	t.Run("field bytes exhausted", func(t *testing.T) {
		ledger := newContinuationLedger()
		oversized := make([]byte, MaxOpaqueFieldBytes+1)
		for i := range oversized {
			oversized[i] = 'a'
		}
		_, err := ledger.retain(opaqueField{kind: opaqueText, body: oversized})
		if !IsCode(err, "CONTINUATION_INVALID") {
			t.Fatalf("expected CONTINUATION_INVALID, got: %v", err)
		}
		var contractErr *ContractError
		if !asContractError(err, &contractErr) || contractErr.Detail != "continuation.ledger.field_bytes_exhausted" {
			t.Fatalf("expected detail continuation.ledger.field_bytes_exhausted, got %#v", err)
		}
	})

	t.Run("step bytes exhausted", func(t *testing.T) {
		ledger := newContinuationLedger()
		fieldSize := MaxOpaqueFieldBytes
		fieldCount := (MaxOpaqueStepBytes / fieldSize) + 1
		fields := make([]opaqueField, fieldCount)
		for i := 0; i < fieldCount; i++ {
			body := make([]byte, fieldSize)
			for j := range body {
				body[j] = 'b'
			}
			fields[i] = opaqueField{kind: opaqueText, body: body}
		}
		_, err := ledger.retain(fields...)
		if !IsCode(err, "CONTINUATION_INVALID") {
			t.Fatalf("expected CONTINUATION_INVALID, got: %v", err)
		}
		var contractErr *ContractError
		if !asContractError(err, &contractErr) || contractErr.Detail != "continuation.ledger.step_bytes_exhausted" {
			t.Fatalf("expected detail continuation.ledger.step_bytes_exhausted, got %#v", err)
		}
	})
}

func TestContinuationMechanismLabels_UnrecognisedDialect(t *testing.T) {
	t.Parallel()

	adapter := &loopAdapter{
		identity: AdapterIdentity{Key: "openai", ID: "sworn.openai", Version: "1.0.0"},
		family:   ProfileOpenAIHTTP,
		surface:  ProfileSurfaceOpenAIChat,
		dialect:  providerDialectOpenAIChat,
	}

	state := &apiContinuationState{
		conversation: nil,
		dialect:      providerDialectGoogleChat, // mismatched dialect
		mode:         ContinuationModeOpaqueReplay,
		bytes:        100,
	}

	invocation := Invocation{
		Selected: SelectedProfile{
			Profile: ProfileConfig{Key: "openai", Network: NetworkRequired},
			Adapter: adapter.identity,
		},
		Request: Request{
			Workspace: Workspace{Access: ReadWrite},
		},
	}

	_, _, err := adapter.resumeProviderContinuation(
		context.Background(),
		invocation,
		state,
		false,
		false,
	)
	if !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("expected CONTINUATION_INVALID, got: %v", err)
	}
	var contractErr *ContractError
	if !asContractError(err, &contractErr) || contractErr.Detail != "continuation.provider.resume_state_invalid" {
		t.Fatalf("expected detail continuation.provider.resume_state_invalid, got: %#v", err)
	}
}

func TestContinuationMechanismLabels_ToolCallDecodeRefusal(t *testing.T) {
	t.Parallel()

	ledger := newContinuationLedger()
	_ = ledger.correlate("call-1")

	item := map[string]any{
		"type":      "function_call",
		"call_id":   "call-1",
		"name":      "Read",
		"arguments": `{"path":"a.txt"}`,
		"status":    "completed",
	}

	_, err := responsesFunctionCall(ledger, make(map[string]struct{}), item)
	if !IsCode(err, "CONTINUATION_INVALID") {
		t.Fatalf("expected CONTINUATION_INVALID, got: %v", err)
	}
	var contractErr *ContractError
	if !asContractError(err, &contractErr) || contractErr.Detail != "continuation.toolcall_decode.correlate_reuse" {
		t.Fatalf("expected detail continuation.toolcall_decode.correlate_reuse, got: %#v", err)
	}
}

func TestContractErrorDetailFormatting(t *testing.T) {
	t.Parallel()

	unlabelled := &ContractError{Code: "CONTINUATION_INVALID"}
	if got := unlabelled.Error(); got != "driver contract: CONTINUATION_INVALID" {
		t.Fatalf("unlabelled Error() = %q, want %q", got, "driver contract: CONTINUATION_INVALID")
	}

	labelled := &ContractError{Code: "CONTINUATION_INVALID", Detail: "continuation.toolcall_decode.correlate_reuse"}
	if got := labelled.Error(); got != "driver contract: CONTINUATION_INVALID: continuation.toolcall_decode.correlate_reuse" {
		t.Fatalf("labelled Error() = %q, want %q", got, "driver contract: CONTINUATION_INVALID: continuation.toolcall_decode.correlate_reuse")
	}

	if !IsCode(unlabelled, "CONTINUATION_INVALID") || !IsCode(labelled, "CONTINUATION_INVALID") {
		t.Fatal("IsCode must return true for both labelled and unlabelled errors")
	}
}

func asContractError(err error, target **ContractError) bool {
	if err == nil {
		return false
	}
	if ce, ok := err.(*ContractError); ok {
		*target = ce
		return true
	}
	return false
}
