package runtime

import (
	"time"

	"corerp/internal/core"
)

func (e *Engine) recordTraceLocked(trace core.TurnTrace) {
	trace = normalizeTurnTraceCompatibility(trace)
	if trace.CreatedAt.IsZero() {
		trace.CreatedAt = time.Now().UTC()
	}
	if trace.Turn > e.turnCount {
		e.turnCount = trace.Turn
	}
	e.turnTraces = append(e.turnTraces, trace)
	if len(e.turnTraces) > 64 {
		e.turnTraces = append([]core.TurnTrace(nil), e.turnTraces[len(e.turnTraces)-64:]...)
	}
}

func (e *Engine) GetLatestTrace() (core.TurnTrace, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.turnTraces) == 0 {
		return core.TurnTrace{}, false
	}
	return e.turnTraces[len(e.turnTraces)-1], true
}

func (e *Engine) GetTraceByTurn(turn int) (core.TurnTrace, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, trace := range e.turnTraces {
		if trace.Turn == turn {
			return trace, true
		}
	}
	return core.TurnTrace{}, false
}
