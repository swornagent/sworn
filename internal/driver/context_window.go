package driver

import "strconv"

// contextWindowClamp computes the output-token ceiling one request may
// carry, given the profile's fixed configured ceiling, its declared total
// context window, and the previous turn's reported input tokens
// (S6-context-window-clamp A2). contextWindowTokens <= 0 disables the
// clamp entirely (ceiling is returned unchanged: byte-identical to today).
// lastInputTokens == nil means no turn has been accepted yet - the first
// request of a dispatch - so the clamp does not apply there either,
// exactly as the contract requires. Otherwise the room left in the window,
// after the previous turn's input and a fixed safety margin, becomes the
// ceiling whenever it is stricter than the configured ceiling (or when no
// ceiling was configured at all, ceiling == 0 meaning "unbounded" today);
// this only ever lowers what would have been sent, never raises it.
// exhausted reports whether that room falls below the declared minimal
// output: the caller must refuse the turn instead of sending it.
func contextWindowClamp(
	ceiling, contextWindowTokens int64,
	lastInputTokens *int64,
) (clamped int64, exhausted bool) {
	if contextWindowTokens <= 0 || lastInputTokens == nil {
		return ceiling, false
	}
	room := contextWindowTokens - *lastInputTokens - contextWindowSafetyMarginTokens
	if room < contextWindowMinimumOutputTokens {
		return 0, true
	}
	if ceiling > 0 && ceiling < room {
		return ceiling, false
	}
	return room, false
}

// contextWindowExhaustedDetail renders the bounded ECONOMY_CONTEXT_EXHAUSTED
// detail naming every fact A3 requires (the window, the last input tokens,
// and the ceiling) plus the knob that fixes it, well inside
// maxProviderErrorDetailBytes.
func contextWindowExhaustedDetail(contextWindowTokens, lastInputTokens, ceiling int64) string {
	return "context_window_tokens=" + strconv.FormatInt(contextWindowTokens, 10) +
		" last_input_tokens=" + strconv.FormatInt(lastInputTokens, 10) +
		" ceiling=" + strconv.FormatInt(ceiling, 10) +
		" fix: raise context_window_tokens or lower max_output_tokens in the driver config"
}
