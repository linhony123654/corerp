package storage

import (
	"context"
	"corerp.local/backend/internal/core"
)

// RPService binds one immutable operator-selected provider to the application
// surface. Store still owns validation, transactions and durable recovery.
type RPService struct {
	*Store
	provider core.RPDecisionProvider
	mode     string
}

func NewRPService(store *Store, provider core.RPDecisionProvider, mode string) (*RPService, error) {
	if store == nil || provider == nil || (mode != "deterministic" && mode != "chat_completions") {
		return nil, core.NewError(core.CodeInvalidArgument, "RP store, provider and supported mode are required")
	}
	return &RPService{Store: store, provider: provider, mode: mode}, nil
}
func (s *RPService) PlayRPTurn(ctx context.Context, request core.RPSpeechRequest) (RPTurnResult, error) {
	return s.Store.RunRPTurn(ctx, request, s.provider)
}
func (s *RPService) PlayResumeRPTurn(ctx context.Context, request RPTurnResumeRequest) (RPTurnResult, error) {
	return s.Store.ResumeRPTurn(ctx, request, s.provider)
}
func (s *RPService) ObserveRPSession(ctx context.Context, request core.RPSessionReadRequest) (RPObservation, error) {
	observation, err := s.Store.ObserveRPSession(ctx, request)
	observation.DecisionMode = s.mode
	return observation, err
}
