package narrative

import (
	"context"
	"corerp.local/backend/internal/core"
	"strings"
	"unicode/utf8"
)

// Fresh model selection and the no-network default compile the same grammar.
// Legacy v1 helpers/readers retain their version and cannot be accepted here.
func renderFreshRPComposition(ctx context.Context, draft string, in core.RPNarrativeInput) (*core.RPNarrativeView, error) {
	if !strings.HasPrefix(strings.TrimSpace(draft), "{") {
		legacy, err := expandProseSpeechTokens(draft, in)
		if err != nil {
			return nil, err
		}
		if err = validateProse(legacy, in); err != nil {
			return nil, err
		}
		return nil, failure("invalid composition plan")
	}
	plan, err := core.DecodeRPCompositionPlan([]byte(draft), in)
	if err != nil {
		return nil, failure("invalid composition plan")
	}
	view, err := core.RenderRPComposition(ctx, in, plan, nil)
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(strings.Join(view.Lines, "\n")) > maxProseRunes {
		return nil, failure("prose empty or exceeds budget")
	}
	return &view, nil
}
