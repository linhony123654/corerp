package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

type rpDecisionProviderFunc func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error)

func (f rpDecisionProviderFunc) Propose(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
	return f(ctx, input)
}

func TestRPDecisionInputIgnoresUnsourcedLegacyOwnActionWithoutPlace(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-decision-legacy-own-action.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, _, initial := newRPWaitTestSession(t, ctx, store)
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "还好吗？", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "legacy-own-action",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence)
		SELECT ?,event_id,'silence',NULL,NULL,NULL,world_time,NULL,instance_id,branch_id,event_sequence
		FROM events WHERE event_id=?`, M2RPNPCID, speech.EventID); err != nil {
		t.Fatal(err)
	}
	input, err := store.BuildRPDecisionInput(ctx, core.RPDecisionRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, action := range input.OwnActions {
		if action.Action == "silence" && action.PlaceID == "" {
			found = true
		}
	}
	if found {
		t.Fatalf("unsourced legacy row entered NPC memory: %+v", input.OwnActions)
	}
}

func TestRPDecisionInputIsNPCScopedAndProviderCannotCommitWorld(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-decision.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, _, initial := newRPWaitTestSession(t, ctx, store)
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "今天过得怎么样？", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "npc-input"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	input, err := store.BuildRPDecisionInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if input.NPCEntityID != M2RPNPCID || input.PlaceID != M2AgentCafeID || input.PlayerSpeechText != "今天过得怎么样？" || input.GoalCode != "keep_daily_routine" || input.OwnAssetMinor <= 0 || input.CurrencyID == "" {
		t.Fatalf("NPC input omitted own real state: %+v", input)
	}
	if input.ContextVersion != core.RPContextVersion || input.Readiness.Persona != "MISSING" || input.Readiness.RelationshipToInterlocutor != "UNKNOWN" || input.Readiness.AddressToInterlocutor != "UNKNOWN" || input.PersonaSourceEventID != "" {
		t.Fatalf("legacy decision context invented authored relationship: %+v", input)
	}
	if len(input.VisibleEntities) != 1 || input.VisibleEntities[0].EntityID != M2RPPlayerID || len(input.Knowledge) == 0 || input.Knowledge[0].ClaimType != "speaker_said" {
		t.Fatalf("NPC input lacks legal observation/knowledge: %+v", input)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"asset_account_id", "receivable_account_id", "liability_account_id", "principal_id", "audit_records", "creator", "private_memory", M2AgentAdaID, M2AgentBoID} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("NPC provider input leaked %q: %s", forbidden, encoded)
		}
	}
	offsite := request
	offsite.NPCEntityID = M2AgentAdaID
	if _, err := store.BuildRPDecisionInput(ctx, offsite); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("offsite NPC was allowed to read player speech: %v", err)
	}
	player := request
	player.NPCEntityID = M2RPPlayerID
	if _, err := store.BuildRPDecisionInput(ctx, player); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("player was treated as NPC: %v", err)
	}
	var providerInput core.RPDecisionInput
	valid := rpDecisionProviderFunc(func(_ context.Context, got core.RPDecisionInput) (core.RPDecisionProposal, error) {
		providerInput = got
		return core.RPDecisionProposal{Action: "respond", Text: "还不错。"}, nil
	})
	decision, err := store.DecideRP(ctx, request, valid)
	if err != nil || decision.Status != "validated" || decision.Proposal.Action != "respond" || decision.HeadSequence != speech.EventSequence || providerInput.NPCEntityID != M2RPNPCID {
		t.Fatalf("valid proposal failed or mutated world: %+v, %v", decision, err)
	}
	legalLeave := rpDecisionProviderFunc(func(_ context.Context, _ core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: "place_m2_home_ada"}, nil
	})
	if planned, err := store.DecideRP(ctx, request, legalLeave); err != nil || planned.Status != "validated" || planned.Proposal.Action != "leave" {
		t.Fatalf("reachable leave proposal was rejected: %+v, %v", planned, err)
	}
	invalid := rpDecisionProviderFunc(func(_ context.Context, _ core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: "place_m2_hidden_castle"}, nil
	})
	if _, err := store.DecideRP(ctx, request, invalid); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("illegal provider proposal was not rejected: %v", err)
	}
	failure := rpDecisionProviderFunc(func(_ context.Context, _ core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{}, errors.New("provider timeout")
	})
	fallback, err := store.DecideRP(ctx, request, failure)
	if err != nil || fallback.Status != "provider_fallback" || fallback.Proposal.Action != "silence" {
		t.Fatalf("provider failure did not safely no-op: %+v, %v", fallback, err)
	}
	cancelledCtx, cancel := context.WithCancel(ctx)
	timedOut := rpDecisionProviderFunc(func(_ context.Context, _ core.RPDecisionInput) (core.RPDecisionProposal, error) {
		cancel()
		return core.RPDecisionProposal{}, context.Canceled
	})
	cancelled, err := store.DecideRP(cancelledCtx, request, timedOut)
	if err != nil || cancelled.Status != "provider_fallback" || cancelled.Proposal.Action != "silence" {
		t.Fatalf("cancelled provider did not leave audited safe no-op: %+v, %v", cancelled, err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, speech.EventSequence)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_sequence > ? AND instance_id = ? AND branch_id = ?`, []any{speech.EventSequence, M2DemoInstanceID, M2DemoBranchID}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM audit_records WHERE related_event_id = ? AND authority = 'non-authoritative'`, []any{speech.EventID}, 5)
}

func TestRPDeterministicDecisionCanRefuseUsingOwnEconomyAndSchedule(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-decision-refuse.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	move, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "visit-ada"})
	if err != nil || move.ToPlaceID != "place_m2_home_ada" {
		t.Fatalf("could not reach Ada: %+v, %v", move, err)
	}
	seen, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "能借我一些钱吗？", SpeechAct: "request", ExpectedCursor: seen.ObservationCursor, IdempotencyKey: "ask-ada"})
	if err != nil || len(speech.ListenerIDs) != 1 || speech.ListenerIDs[0] != M2AgentAdaID {
		t.Fatalf("Ada did not hear the request: %+v, %v", speech, err)
	}
	request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2AgentAdaID}
	input, err := store.BuildRPDecisionInput(ctx, request)
	if err != nil || input.NextSchedule == nil || input.NextSchedule.ActivityCode != "work" || input.OwnAssetMinor <= 0 {
		t.Fatalf("NPC decision omitted real schedule/economy: %+v, %v", input, err)
	}
	decision, err := store.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || decision.Status != "validated" || decision.Proposal.Action != "refuse" || decision.Proposal.Text == "" {
		t.Fatalf("deterministic provider failed to refuse: %+v, %v", decision, err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, speech.EventSequence)
}
