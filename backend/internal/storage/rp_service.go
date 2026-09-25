package storage

import (
	"context"
	"corerp.local/backend/internal/core"
)

// RPService binds one immutable operator-selected provider to the application
// surface. Store still owns validation, transactions and durable recovery.
type RPService struct {
	*Store
	provider  core.RPDecisionProvider
	mode      string
	narrative core.RPStreamingNarrativeProvider
}

func NewRPService(store *Store, provider core.RPDecisionProvider, mode string) (*RPService, error) {
	return NewRPServiceWithNarrative(store, provider, mode, core.DeterministicRPNarrativeProvider{})
}

// The operator supplies immutable, independent decision and presentation
// providers. Requests/styles cannot select a remote endpoint or credentials.
// Canonical settlement always keeps its original deterministic record.
func NewRPServiceWithNarrative(store *Store, provider core.RPDecisionProvider, mode string, narrative core.RPStreamingNarrativeProvider) (*RPService, error) {
	if store == nil || provider == nil || (mode != "deterministic" && mode != "chat_completions") {
		return nil, core.NewError(core.CodeInvalidArgument, "RP store, provider and supported mode are required")
	}
	if narrative == nil {
		return nil, core.NewError(core.CodeInvalidArgument, "RP narrative provider is required")
	}
	return &RPService{Store: store, provider: provider, mode: mode, narrative: narrative}, nil
}
func (s *RPService) ReadRPNarrative(ctx context.Context, r RPNarrativeReadRequest) (RPNarrativeReadResult, error) {
	return s.StreamRPNarrative(ctx, r, nil)
}
func (s *RPService) StreamRPNarrative(ctx context.Context, r RPNarrativeReadRequest, emit func(core.RPNarrativeChunk) error) (RPNarrativeReadResult, error) {
	return s.Store.streamRPNarrativeWithProvider(ctx, r, emit, s.narrative)
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
	return s.finishRPWaitPresentation(ctx, request, result)
}

// Shared-round authority is settled by Store first. The product service then
// drains the same warm/initiative work as an ordinary Human wait; a retry
// replays the single wait Event and resumes any unfinished derived work.
func (s *RPService) AdvanceRPSharedRound(ctx context.Context, request RPSharedRoundAdvanceRequest) (RPSharedRound, error) {
	result, err := s.Store.advanceRPSharedRoundWithProvider(ctx, request, s.provider)
	if err != nil || result.Status != "settled" {
		return result, err
	}
	// An action boundary has no Human wait or post-wait roster to drain.
	var settlementKind string
	if err := s.Store.db.QueryRowContext(ctx, `SELECT settlement_kind FROM rp_shared_rounds WHERE round_id=?`, request.RoundID).Scan(&settlementKind); err != nil {
		return result, err
	}
	if settlementKind != "wait" {
		return result, nil
	}
	var wait core.RPWaitRequest
	if err := s.Store.db.QueryRowContext(ctx, `SELECT p.principal_id,r.human_session_id,r.advance_target,r.baseline_head,r.wait_key FROM rp_shared_rounds r JOIN rp_shared_round_participants p ON p.round_id=r.round_id AND p.role='human' WHERE r.round_id=? AND r.status='settled'`, request.RoundID).Scan(&wait.PrincipalID, &wait.SessionID, &wait.TargetWorldTime, &wait.ExpectedCursor, &wait.IdempotencyKey); err != nil {
		return result, err
	}
	wait.Budget = request.Budget
	completed, err := s.Store.waitRPForSharedRound(ctx, wait, request.RoundID)
	if err != nil {
		return result, err
	}
	_, err = s.finishRPWaitPresentation(ctx, wait, completed)
	return result, err
}

func (s *RPService) finishRPWaitPresentation(ctx context.Context, request core.RPWaitRequest, result RPWaitResult) (RPWaitResult, error) {
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
	observation.NarrativeMode = "custom"
	if mode, ok := s.narrative.(interface{ NarrativeMode() string }); ok {
		observation.NarrativeMode = mode.NarrativeMode()
	}
	return observation, err
}
