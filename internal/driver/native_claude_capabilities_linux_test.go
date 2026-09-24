package driver

import "testing"

func TestExactClaudeCapabilitiesAdmitsReviewedAndRefusesUnknown(t *testing.T) {
	list := func(names ...string) any {
		values := make([]any, len(names))
		for index, name := range names {
			values[index] = name
		}
		return values
	}
	cases := []struct {
		name  string
		value any
		want  bool
	}{
		{"2.1.241 set", list("interrupt_receipt_v1", "interrupt_cancel_queued_v1", "msg_lifecycle_v1"), true},
		{"2.1.280 set", list("interrupt_receipt_v1", "interrupt_cancel_queued_v1", "msg_lifecycle_v1", "mcp_read_resource_v1", "mcp_tool_ui_meta_v1"), true},
		{"order is irrelevant", list("mcp_tool_ui_meta_v1", "msg_lifecycle_v1", "interrupt_cancel_queued_v1", "interrupt_receipt_v1"), true},
		{"a required capability missing", list("interrupt_receipt_v1", "msg_lifecycle_v1", "mcp_read_resource_v1"), false},
		{"an unknown capability", list("interrupt_receipt_v1", "interrupt_cancel_queued_v1", "msg_lifecycle_v1", "remote_shell_v1"), false},
		{"a duplicate", list("interrupt_receipt_v1", "interrupt_cancel_queued_v1", "msg_lifecycle_v1", "msg_lifecycle_v1"), false},
		{"not a string", []any{"interrupt_receipt_v1", "interrupt_cancel_queued_v1", "msg_lifecycle_v1", 7}, false},
		{"not an array", "interrupt_receipt_v1", false},
	}
	for _, testCase := range cases {
		if got := exactClaudeCapabilities(testCase.value); got != testCase.want {
			t.Fatalf("%s: exactClaudeCapabilities = %v, want %v", testCase.name, got, testCase.want)
		}
	}
}
