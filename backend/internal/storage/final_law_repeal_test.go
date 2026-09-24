package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestFinalWorldTemporaryLawNeedsHeardRepealAndPreservesFine(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "final-temporary-law.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	f := prepareFinalWorld(t, s, M2AgentNoonTime)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "temporary-law-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	advance := func(at, key string) {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
		if err != nil {
			t.Fatal(err)
		}
		wait, err := service.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: key})
		if err != nil || wait.Status != "completed" || wait.CurrentWorldTime != at {
			t.Fatalf("advance %+v %v", wait, err)
		}
	}
	turn := func(key, action string) RPTurnResult {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "现在可以聊聊今天的生活吗？", IdempotencyKey: key})
		if err != nil || r.Status != "settled" {
			t.Fatalf("turn %+v %v", r, err)
		}
		var committed, eventID, eventType string
		if err := s.db.QueryRowContext(ctx, `SELECT d.action,e.event_id,e.event_type FROM rp_npc_decisions d JOIN events e ON e.event_id=d.event_id WHERE d.session_id=? AND d.parent_turn_id=? AND d.npc_entity_id='entity_final_nora' AND e.actor_id=d.npc_entity_id`, read.SessionID, r.PlayerTurnID).Scan(&committed, &eventID, &eventType); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, id := range r.NPCEventIDs {
			found = found || id == eventID
		}
		if !found || committed != action || action == "respond" && eventType != "RPSpeechAccepted" {
			t.Fatalf("%s committed=%s event=%s returned=%v want=%s", key, committed, eventType, found, action)
		}
		return r
	}
	advance(M2AgentNoonTime, "law-effective")
	gift := socialRequest(t, ctx, s, read, "entity_final_nora", "gift", "remove-independent-cash-pressure")
	gift.AmountMinor = 200
	if _, err := s.SocialRP(ctx, gift); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnnounceRPLaw(ctx, LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "temporary-law-news"), SpeakerID: M2AgentAdaID, EnactmentEventID: f.LawEventID}); err != nil {
		t.Fatal(err)
	}
	question := turn("during-quiet-law", "silence")
	violation, err := s.RecordRPLawViolation(ctx, LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "temporary-violation"), InstitutionID: "final-council", EnforcerID: M2AgentBoID, EnactmentEventID: f.LawEventID, ActionEventID: question.PlayerEventID})
	if err != nil {
		t.Fatal(err)
	}
	fineRequest := LawEnforcementRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "temporary-fine"), InstitutionID: "final-council", EnforcerID: M2AgentBoID, ViolationEventID: violation.EventID}
	fine, err := s.EnforceRPLaw(ctx, fineRequest)
	if err != nil {
		t.Fatal(err)
	}
	before := readCareerTestContext(t, s, M2RPPlayerID).OwnAssetMinor
	proposal, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "temporary-repeal-proposal"), InstitutionID: "final-council", ProposerID: M2AgentAdaID, PreviousEnactmentEventID: f.LawEventID, Repealed: true, Law: LawDefinition{LawID: "final-quiet", ProhibitedAction: "speak", FineMinor: 0, Text: "Temporary quiet period is over; ordinary conversation may resume."}})
	if err != nil {
		t.Fatal(err)
	}
	repeal, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "temporary-repeal"), InstitutionID: "final-council", LegislatorID: M2AgentAdaID, ProposalEventID: proposal.EventID, EffectiveWorldTime: careerTime(1, 13, 0)})
	if err != nil {
		t.Fatal(err)
	}
	advance(careerTime(1, 13, 0), "repeal-effective")
	// No telepathic legal update: the world rule is gone, but Nora still only
	// knows the old announcement and retains her cautious response.
	unheard := turn("unheard-repeal", "silence")
	if _, err := s.RecordRPLawViolation(ctx, LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "obsolete-law-case"), InstitutionID: "final-council", EnforcerID: M2AgentBoID, EnactmentEventID: f.LawEventID, ActionEventID: unheard.PlayerEventID}); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("obsolete law punished new speech: %v", err)
	}
	if _, err := s.AnnounceRPLaw(ctx, LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "temporary-repeal-news"), SpeakerID: M2AgentAdaID, EnactmentEventID: repeal.EventID}); err != nil {
		t.Fatal(err)
	}
	turn("heard-repeal", "respond")
	if readCareerTestContext(t, s, M2RPPlayerID).OwnAssetMinor != before {
		t.Fatal("repeal silently refunded or recharged past fine")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	turn("recovered-repeal", "respond")
	retry, err := s.EnforceRPLaw(ctx, fineRequest)
	if err != nil || !retry.Replayed || retry.EventID != fine.EventID || readCareerTestContext(t, s, M2RPPlayerID).OwnAssetMinor != before {
		t.Fatalf("historical fine recovery %+v %v", retry, err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("repeal replay %+v %v", diffs, err)
	}
	t.Logf("law=%s repeal=%s fine=%s: actual silence→still-uninformed silence→heard response→recovered response; old fine preserved", f.LawEventID, repeal.EventID, fine.EventID)
}
