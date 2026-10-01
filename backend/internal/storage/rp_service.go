package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"errors"
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

// The operator supplies the default decision and presentation providers.
// The transport layer may additionally resolve a player-supplied
// core.RPModelOverride into an in-memory per-request provider (the With
// methods); overrides are validated at construction, never persisted, and an
// invalid override is an explicit error rather than a silent fallback.
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
	return s.StreamRPNarrativeWith(ctx, r, emit, nil)
}

// The With variants accept a per-request override provider resolved by the
// transport layer from RPModelOverride. A nil provider keeps the immutable
// operator-selected default; overrides are never persisted.
func (s *RPService) StreamRPNarrativeWith(ctx context.Context, r RPNarrativeReadRequest, emit func(core.RPNarrativeChunk) error, narrative core.RPStreamingNarrativeProvider) (RPNarrativeReadResult, error) {
	if narrative == nil {
		narrative = s.narrative
	}
	return s.Store.streamRPNarrativeWithProvider(ctx, r, emit, narrative)
}
func (s *RPService) PlayRPTurn(ctx context.Context, request core.RPSpeechRequest) (RPTurnResult, error) {
	return s.PlayRPTurnWith(ctx, request, nil)
}
func (s *RPService) PlayRPTurnWith(ctx context.Context, request core.RPSpeechRequest, provider core.RPDecisionProvider) (RPTurnResult, error) {
	if provider == nil {
		provider = s.provider
	}
	return s.Store.RunRPTurn(ctx, request, provider)
}
func (s *RPService) PlayResumeRPTurn(ctx context.Context, request RPTurnResumeRequest) (RPTurnResult, error) {
	return s.PlayResumeRPTurnWith(ctx, request, nil)
}
func (s *RPService) PlayResumeRPTurnWith(ctx context.Context, request RPTurnResumeRequest, provider core.RPDecisionProvider) (RPTurnResult, error) {
	if provider == nil {
		provider = s.provider
	}
	return s.Store.ResumeRPTurn(ctx, request, provider)
}

func (s *RPService) NonverbalRP(ctx context.Context, request core.RPNonverbalRequest) (RPNonverbalResult, error) {
	return s.NonverbalRPWith(ctx, request, nil)
}

func (s *RPService) NonverbalRPWith(ctx context.Context, request core.RPNonverbalRequest, provider core.RPDecisionProvider) (RPNonverbalResult, error) {
	if err := request.Validate(); err != nil {
		return RPNonverbalResult{}, err
	}
	// Pre-upgrade raw-action receipts retain their exact retry outcome. Do
	// not add retroactive reactions to an action that already completed alone.
	session, err := loadRPSessionRecord(ctx, s.Store.db, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPNonverbalResult{}, err
	}
	var legacy int
	if err := s.Store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands c WHERE c.instance_id=? AND c.branch_id=? AND c.command_type='RPNonverbalAction' AND c.idempotency_key=? AND NOT EXISTS (SELECT 1 FROM rp_turn_runs r WHERE r.session_id=? AND r.idempotency_key=?)`, session.InstanceID, session.BranchID, "rp_nonverbal:"+session.SessionID+":"+request.IdempotencyKey, session.SessionID, request.IdempotencyKey).Scan(&legacy); err != nil {
		return RPNonverbalResult{}, err
	}
	if legacy != 0 {
		return s.Store.NonverbalRP(ctx, request)
	}
	if provider == nil {
		provider = s.provider
	}
	turn, err := s.Store.RunRPNonverbalTurn(ctx, request, provider)
	if errors.Is(err, errRPNonverbalRawReceipt) {
		return s.Store.NonverbalRP(ctx, request)
	}
	if err != nil {
		return RPNonverbalResult{}, err
	}
	action, err := s.Store.NonverbalRP(ctx, request)
	if err != nil {
		return RPNonverbalResult{}, err
	}
	action.Replayed, action.Turn = turn.Replayed, &turn
	return action, nil
}

// A wait is the product's explicit time advance, not an invented player speech.
// Its committed roster owns retry identity. Individual effects remain atomic;
// after a lost response, committed actors are replayed without model calls.
func (s *RPService) WaitRP(ctx context.Context, request core.RPWaitRequest) (RPWaitResult, error) {
	return s.WaitRPWith(ctx, request, nil)
}
func (s *RPService) WaitRPWith(ctx context.Context, request core.RPWaitRequest, provider core.RPDecisionProvider) (RPWaitResult, error) {
	result, err := s.Store.WaitRP(ctx, request)
	if err != nil || result.Status != "completed" {
		return result, err
	}
	return s.finishRPWaitPresentation(ctx, request, result, provider)
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
	_, err = s.finishRPWaitPresentation(ctx, wait, completed, nil)
	return result, err
}

func (s *RPService) finishRPWaitPresentation(ctx context.Context, request core.RPWaitRequest, result RPWaitResult, provider core.RPDecisionProvider) (RPWaitResult, error) {
	if provider == nil {
		provider = s.provider
	}
	for _, npc := range result.WarmNPCIDs {
		_, err := s.Store.runRPWarmDecision(ctx, core.RPInitiativeRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, NPCEntityID: npc, TriggerEventID: result.EventID})
		if err != nil && !core.HasCode(err, core.CodeBranchConflict) && !core.HasCode(err, core.CodeNotFound) && !core.HasCode(err, core.CodeCommandInProgress) {
			return result, err
		}
	}
	for _, npc := range result.InitiativeNPCIDs {
		effect, err := s.Store.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, NPCEntityID: npc, TriggerEventID: result.EventID}, provider)
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
