package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPContextSelectionBuilderRecoversOldExchangeUnderPressureAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "selection.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	session, read, _ := newRPWaitTestSession(t, ctx, s)
	// A genuinely heard, unintroduced third speaker makes identity masking grow
	// the packet. The externally controlled actor does not get autonomous calls.
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	ada, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	adaRead := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}
	adaView, err := s.ObserveRPSession(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID, FromPlaceID: adaView.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "selection-unknown-visitor"}); err != nil {
		t.Fatal(err)
	}
	const question = "明天把那本蓝皮诗集带来，好吗？"
	const answer = "蓝皮诗集可以带来，但是缺了末页。"
	var oldPlayer, oldNPC string
	for i := 0; i < 28; i++ {
		if i > 0 && i%2 == 0 {
			adaView, err := s.ObserveRPSession(ctx, adaRead)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: fmt.Sprintf("selection-bystander-%d", i), Text: fmt.Sprintf("旁听的第%d轮，", i) + strings.Repeat("只是第三人的另一话题。", 60)}); err != nil {
				t.Fatal(err)
			}
		}
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		speech, reply := fmt.Sprintf("第%d轮，", i)+strings.Repeat("只是别的话题。", 120), fmt.Sprintf("第%d次回答，", i)+strings.Repeat("这段是别的话题。", 120)
		if i == 0 {
			speech, reply = question, answer
		}
		turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, Text: speech, IdempotencyKey: fmt.Sprintf("selection-pressure-%d", i)}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
			return core.RPDecisionProposal{Action: "respond", Text: reply, Private: &core.RPDecisionPrivate{Intent: "我私下打算耐心把眼前这次话听完。"}}, nil
		}))
		if err != nil || len(turn.NPCEventIDs) != 1 {
			t.Fatal(i, turn, err)
		}
		if i == 0 {
			oldNPC = turn.NPCEventIDs[0]
			if err := s.db.QueryRowContext(ctx, `SELECT player_event_id FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&oldPlayer); err != nil {
				t.Fatal(err)
			}
		}
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	const recall = "那本蓝皮诗集，你当时怎么答应我的？"
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, Text: recall, IdempotencyKey: "selection-recall"})
	if err != nil {
		t.Fatal(err)
	}
	req := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	var packet core.RPDecisionInput
	check := func() string {
		t.Helper()
		packet, err = s.BuildRPDecisionInput(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(packet)
		if len(encoded) > core.DefaultRPDecisionContextBudgetBytes || packet.ContextSelection == nil || packet.ContextSelection.EncodedBytes != len(encoded) {
			t.Fatal("real builder has no effective unified budget", len(encoded), packet.ContextSelection)
		}
		if packet.ContextSelection.Omitted.OwnActions == 0 {
			t.Fatal("fixture did not create context pressure", packet.ContextSelection)
		}
		for _, d := range packet.RecentDialogue {
			if d.EventID == oldNPC || d.EventID == oldPlayer {
				t.Fatal("old exchange never left the recent window")
			}
		}
		oldWords, complete := core.ResolveRPDecisionSpeech(packet, oldNPC, M2RPNPCID)
		if !complete || oldWords != answer {
			t.Fatal("old heard response/provenance lost under pressure")
		}
		oldWords, complete = core.ResolveRPDecisionSpeech(packet, oldPlayer, M2RPPlayerID)
		if !complete || oldWords != question {
			t.Fatal("old player request was not recalled as complete words")
		}
		if strings.Count(string(encoded), recall) != 1 || packet.PlayerSpeechWorldTime != speech.WorldTime || len(packet.RecentPrivateDecisions) != 3 {
			t.Fatalf("current stimulus/private continuity: copies=%d private=%d packet_time=%s source_time=%s selection=%+v", strings.Count(string(encoded), recall), len(packet.RecentPrivateDecisions), packet.PlayerSpeechWorldTime, speech.WorldTime, packet.ContextSelection)
		}
		hash, _ := core.HashJSON(packet)
		return hash
	}
	before := check()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if after := check(); after != before {
		t.Fatal("same-head restart changed selection/hash")
	}
	providerView, err := s.rpDecisionProviderView(ctx, packet)
	if err != nil {
		t.Fatal(err)
	}
	encodedProvider, _ := json.Marshal(providerView)
	if len(encodedProvider) > core.DefaultRPDecisionContextBudgetBytes || strings.Contains(string(encodedProvider), M2AgentAdaID) {
		t.Fatal("actual view bypassed byte budget or unfamiliar identity masking")
	}
	decision, err := s.DecideRP(ctx, req, rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		encoded, _ := json.Marshal(input)
		if len(encoded) > core.DefaultRPDecisionContextBudgetBytes {
			t.Fatal("NPC alias view exceeded budget")
		}
		return core.RPDecisionProposal{Action: "respond", Text: "我当时说过可以带来，但缺末页。", Private: &core.RPDecisionPrivate{Intent: "私下继续提醒缺页。", BasisEventIDs: []string{oldNPC}}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitRPDecision(ctx, req, decision); err != nil {
		t.Fatal(err)
	}
	public, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(public)
	if strings.Contains(string(encoded), "我私下打算") || strings.Contains(string(encoded), "私下继续提醒") || strings.Contains(string(encoded), "context_selection") {
		t.Fatal("NPC-private context/report leaked to player")
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal(diffs, err)
	}
}

func TestRPContextSelectionProviderMaskRechecksBudgetAndBasis(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "selection-mask.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _, view := newRPWaitTestSession(t, ctx, s)
	// A synthetic packet isolates the critical byte boundary of presentation
	// masking; it is not used as author canon or submitted as a world fact.
	input := core.RPDecisionInput{ContextVersion: core.RPContextVersion, NPCEntityID: M2RPNPCID, InterlocutorEntityID: M2RPPlayerID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, HeadSequence: view.ObservationCursor, WorldTime: view.WorldTime, PlaceID: view.PlaceID, Persona: "临界字节测试的明确设定。", LegalActions: []string{"respond", "silence"}, Knowledge: []core.RPDecisionKnowledge{{ClaimType: "speaker_said", SubjectEntityID: M2AgentAdaID, SourceEventID: "view-candidate", Text: "这是一条待选择的原话。"}}}
	raw, err := core.SelectRPDecisionContext(input, core.DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	input.Persona += strings.Repeat("x", core.DefaultRPDecisionContextBudgetBytes-raw.ContextSelection.EncodedBytes-2)
	raw, err = core.SelectRPDecisionContext(input, core.DefaultRPDecisionContextBudgetBytes)
	if err != nil || len(raw.Knowledge) != 1 {
		t.Fatal("raw fixture did not retain the candidate", err)
	}
	providerView, err := s.rpDecisionProviderView(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(providerView)
	if len(encoded) > core.DefaultRPDecisionContextBudgetBytes || len(providerView.Knowledge) != 0 || providerView.ContextSelection.Omitted.Knowledge != 1 || providerView.Persona != input.Persona {
		t.Fatal("mask growth changed required canon or exceeded budget", len(encoded), providerView.ContextSelection)
	}
	proposal := core.RPDecisionProposal{Action: "respond", Text: "回答。", Private: &core.RPDecisionPrivate{Intent: "引用被省略的内容。", BasisEventIDs: []string{"view-candidate"}}}
	if err := core.ValidateRPDecisionProposal(raw, proposal); err != nil {
		t.Fatal("raw packet did not authorize the source", err)
	}
	if reason, err := core.ValidateRPDecisionProposalEvidence(providerView, proposal); reason != "ungrounded_decision" || !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("actual-view validation accepted unoffered evidence", reason, err)
	}
}

func TestRPContextSelectionInitiativeSharesTheContractWithoutInventingSpeech(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := f.s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at.Add(time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "selection-wait"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := f.s.BuildRPInitiativeInput(ctx, core.RPInitiativeRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, TriggerEventID: wait.EventID, NPCEntityID: f.ids[0]})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(input)
	if input.ContextSelection == nil || input.ContextSelection.EncodedBytes != len(encoded) || input.PlayerSpeechText != "" || input.PlayerSpeechWorldTime != "" || input.Trigger == nil || input.Trigger.SourceEventID != wait.EventID || input.Readiness.Persona != "MISSING" {
		t.Fatal("initiative gained invented speech/persona or bypassed selection", input)
	}
}
