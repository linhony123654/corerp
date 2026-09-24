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

// A wait is the product's explicit time advance, not an invented player speech.
// Its committed roster owns retry identity. Individual effects remain atomic;
// after a lost response, committed actors are replayed without model calls.
func (s *RPService) WaitRP(ctx context.Context, request core.RPWaitRequest) (RPWaitResult, error) {
	result, err := s.Store.WaitRP(ctx, request)
	if err != nil || result.Status != "completed" {
		return result, err
	}
	for _, npc := range result.WarmNPCIDs {
		_, err := s.Store.runRPWarmDecision(ctx, core.RPInitiativeRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, NPCEntityID: npc, TriggerEventID: result.EventID})
		if err != nil && !core.HasCode(err, core.CodeBranchConflict) && !core.HasCode(err, core.CodeNotFound) && !core.HasCode(err, core.CodeCommandInProgress) {
			return result, err
		}
	}
	for _, npc := range result.InitiativeNPCIDs {
		effect, err := s.Store.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, NPCEntityID: npc, TriggerEventID: result.EventID}, s.provider)
		if err != nil {
			// A later user action may legitimately supersede an unfinished old
			// opportunity. Never replay an old initiative into a different scene.
			if core.HasCode(err, core.CodeBranchConflict) || core.HasCode(err, core.CodeNotFound) || core.HasCode(err, core.CodeCommandInProgress) {
				result.Initiatives = append(result.Initiatives, RPInitiativeResult{NPCEntityID: npc, Action: "silence", Status: "superseded"})
				continue
			}
			return result, err
		}
		result.Initiatives = append(result.Initiatives, effect)
	}
	return result, nil
}
func (s *RPService) ObserveRPSession(ctx context.Context, request core.RPSessionReadRequest) (RPObservation, error) {
	observation, err := s.Store.ObserveRPSession(ctx, request)
	observation.DecisionMode = s.mode
	return observation, err
}
