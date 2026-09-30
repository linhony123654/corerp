package core

import (
	"context"
	"sync/atomic"
)

// RPProviderTrace counts actual outbound adapter requests in one invocation.
// It contains no endpoint, request body, credential or remote error.
type RPProviderTrace struct{ attempts atomic.Int32 }

type rpProviderTraceKey struct{}

func WithRPProviderTrace(ctx context.Context, trace *RPProviderTrace) context.Context {
	return context.WithValue(ctx, rpProviderTraceKey{}, trace)
}

// RecordRPProviderHTTPAttempt is called immediately before an adapter hands a
// request to HTTP. A constructor, configured model or proposal alone is not
// evidence that any network request was attempted.
func RecordRPProviderHTTPAttempt(ctx context.Context) {
	if trace, ok := ctx.Value(rpProviderTraceKey{}).(*RPProviderTrace); ok && trace != nil {
		trace.attempts.Add(1)
	}
}

func (trace *RPProviderTrace) AttemptCount() int {
	if trace == nil {
		return 0
	}
	return int(trace.attempts.Load())
}
