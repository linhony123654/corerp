package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

// Same-world causal integration before the actual multi-client long-run test.
// Actor commands are explicit fixture choices using their real own authority;
// NPC dialogue proposals come unchanged from the production deterministic provider.
func TestFinalWorldSourcedCareerCultureLawAndRelationshipStory(t *testing.T) {
	runFinalWorldStory(t, false)
}

func runFinalWorldStory(t *testing.T, month bool) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "final-story.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	f := prepareFinalWorld(t, s, careerTime(2, 12, 0))
	const nora = "entity_final_nora"
	initial := readCareerTestContext(t, s, nora)
	if initial.Life.Background == nil || initial.Life.Background.MaterializationEventID == "" || initial.Life.LiabilityMinor != 30 {
		t.Fatal("emergent identity/debt source missing")
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "final-story-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	var lastTurn RPTurnResult
	turn := func(key, text, expectedAction string) core.RPDecisionInput {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		var seen core.RPDecisionInput
		var action string
		provider := rpDecisionProviderFunc(func(ctx context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
			p, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, in)
			if in.NPCEntityID == nora {
				seen, action = in, p.Action
			}
			return p, err
		})
		r, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: text, IdempotencyKey: key}, provider)
		if err != nil || r.Status != "settled" || seen.NPCEntityID != nora || action != expectedAction {
			t.Fatalf("%s turn: status=%s nora=%s action=%s want=%s err=%v", key, r.Status, seen.NPCEntityID, action, expectedAction, err)
		}
		lastTurn = r
		return seen
	}
	wait := func(at, key string) {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
		if err != nil {
			t.Fatal(err)
		}
		r, err := service.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: key})
		if err != nil || r.Status != "completed" || r.CurrentWorldTime != at {
			t.Fatalf("wait %s: %+v %v", key, r, err)
		}
	}
	gift := func(key string, amount int64) RPSocialResult {
		t.Helper()
		r := socialRequest(t, ctx, s, read, nora, "gift", key)
		r.AmountMinor = amount
		act, err := s.SocialRP(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		return act
	}
	pressure := turn("initial-pressure", "今天有空聊聊吗？", "refuse")
	need := false
	for _, n := range pressure.Life.Needs {
		need = need || n.Code == "cash_security" && len(n.SourceEventIDs) > 0
	}
	if !need {
		t.Fatal("refusal lacks sourced cash pressure")
	}
	// The manager cannot replace aggregate employment by silently adding salary.
	offer := offerExistingCareerPositionTo(t, s, nora, f.NoraPrincipal, 2)
	accept := core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, f.NoraPrincipal, "final-accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}
	if _, err := s.AcceptCareerOffer(ctx, accept); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("overlapping wage accepted: %v", err)
	}
	notice, err := s.RequestCareerAggregateExit(ctx, core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, f.NoraPrincipal, "final-exit"), CandidateID: nora, ContractID: m2EconomyContractID, FinalEarnedDay: 1, Notice: "I choose to leave the pooled job after my earned first period."})
	if err != nil {
		t.Fatal(err)
	}
	wait(M2AgentNoonTime, "first-noon")
	news, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "final-culture-news"), SpeakerID: M2AgentBoID, DefinitionEventID: f.CultureEventID})
	if err != nil {
		t.Fatal(err)
	}
	stance := CultureStanceRequest{Binding: careerTestBinding(t, s, f.NoraPrincipal, "final-rebel"), EntityID: nora, TransmissionEventID: news.EventID, Stance: "rebel"}
	rebel, err := s.InternalizeRPCulture(ctx, stance)
	if err != nil {
		t.Fatal(err)
	}
	firstGift := gift("final-help", 200)
	refusal := turn("culture-refusal", "现在可以聊聊街区里的赠礼习惯吗？", "refuse")
	if refusal.OwnAssetMinor < 290 || len(refusal.Life.CultureExperiences) == 0 {
		t.Fatal("gift did not relieve cash pressure or carry culture evidence")
	}
	experience := refusal.Life.CultureExperiences[len(refusal.Life.CultureExperiences)-1]
	if experience.ActionEventID != firstGift.EventID || experience.Evaluations[0].Basis.StanceEventID != rebel.EventID || experience.Evaluations[0].Score >= 0 {
		t.Fatal("cultural refusal has no actual gift/stance lineage")
	}
	stance.Binding = careerTestBinding(t, s, f.NoraPrincipal, "final-accept-culture")
	stance.Stance = "accept"
	if _, err := s.InternalizeRPCulture(ctx, stance); err != nil {
		t.Fatal(err)
	}
	secondGift := gift("final-friendly-gift", 1)
	friendly := turn("relationship-response", "又见面了，谢谢你说明自己的想法。", "respond")
	trusted := false
	for _, relation := range friendly.Life.Relationships {
		// The unwelcome gift adds tension, not trust; only the welcome gift
		// contributes trust. Both consequences retain their actual sources.
		trusted = trusted || relation.SubjectEntityID == M2RPPlayerID && relation.Trust == 1 && relation.Tension == 2 && len(relation.SourceEventIDs) >= 2
	}
	if !trusted {
		t.Fatalf("two gifts did not create sourced trust: %+v", friendly.Life.Relationships)
	}
	if !reflect.DeepEqual(experience, friendly.Life.CultureExperiences[0]) {
		t.Fatal("new stance rewrote earlier experience")
	}
	// Finish the earned aggregate period and real exit activation before acceptance.
	wait(careerTime(1, 12, 1), "earned-period-and-exit")
	accept.Binding = careerTestBinding(t, s, f.NoraPrincipal, "final-accept")
	job, err := s.AcceptCareerOffer(ctx, accept)
	if err != nil {
		t.Fatal(err)
	}
	if job.Fact.Employment == nil || job.Fact.Employment.EmployeeID != nora {
		t.Fatal("offer did not create same person's contract")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	wait(careerTime(2, 12, 0), "first-real-workday")
	// Knowledge is acquired by hearing a real announcement at the actual place.
	if _, err := s.AnnounceRPLaw(ctx, LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "final-law-news"), SpeakerID: M2AgentAdaID, EnactmentEventID: f.LawEventID}); err != nil {
		t.Fatal(err)
	}
	lawInput := turn("law-restraint", "工作结束了，现在还能继续交谈吗？", "silence")
	if lawInput.Law == nil {
		t.Fatal("law did not reach candidate decision")
	}
	// Nora declines to speak, while Lin's actual question is still an accepted
	// player act. An observing enforcer must explicitly record and punish it.
	beforeFine := readCareerTestContext(t, s, M2RPPlayerID).OwnAssetMinor
	violation, err := s.RecordRPLawViolation(ctx, LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "final-violation"), InstitutionID: "final-council", EnforcerID: M2AgentBoID, EnactmentEventID: f.LawEventID, ActionEventID: lastTurn.PlayerEventID})
	if err != nil {
		t.Fatal(err)
	}
	fineRequest := LawEnforcementRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "final-fine"), InstitutionID: "final-council", EnforcerID: M2AgentBoID, ViolationEventID: violation.EventID}
	fine, err := s.EnforceRPLaw(ctx, fineRequest)
	if err != nil {
		t.Fatal(err)
	}
	if violation.Fact.Violation.ActorID != M2RPPlayerID || readCareerTestContext(t, s, M2RPPlayerID).OwnAssetMinor != beforeFine-2 {
		t.Fatal("law consequence not debited")
	}
	wait(careerTime(3, 0, 1), "first-independent-payroll")
	attendance, err := s.ReadCareerAttendance(ctx, f.NoraPrincipal, M2DemoInstanceID, M2DemoBranchID, job.Fact.Employment.ContractID, 2)
	if err != nil || attendance.Attendance.RecordedSeconds != 14400 {
		t.Fatalf("actual work: %+v %v", attendance, err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=2`, []any{job.Fact.Employment.ContractID}, 12)
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
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("story replay: %+v %v", diffs, err)
	}
	retry, err := s.EnforceRPLaw(ctx, fineRequest)
	if err != nil || !retry.Replayed || retry.EventID != fine.EventID {
		t.Fatalf("fine replay: %+v %v", retry, err)
	}
	recovered := readCareerTestContext(t, s, nora)
	if recovered.Life.Background.MaterializationEventID != initial.Life.Background.MaterializationEventID || !reflect.DeepEqual(recovered.Life.CultureExperiences, friendly.Life.CultureExperiences) {
		t.Fatal("restart changed identity or historical experience")
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "evaluation", "eval_"+nora); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player learned private interview judgment: %v", err)
	}
	if month {
		runFinalWorldMonth(t, &s, path, read, nora)
	}
	t.Logf("same-world sourced chain: materialization=%s pressure/refusal; culture gift=%s/%s; exit=%s job=%s attendance=%s fine=%s; two reopens/rebuild; long clients/rare still pending", initial.Life.Background.MaterializationEventID, firstGift.EventID, secondGift.EventID, notice.EventID, job.EventID, attendance.EventID, fine.EventID)
}
