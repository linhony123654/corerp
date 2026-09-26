package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInformationCareerLayoffOrganizationNoticeRequiresActualEmployeeAccess(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "organization-notice.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 12)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{
		Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "organization-notice-accept"),
		OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := accepted.Fact.Employment
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	privateNotice := "Ada 的合同、工资与个人评估不得外泄。"
	layoff, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{
		Binding:    careerTestBinding(t, s, M2AgentBoPrincipal, "organization-layoff"),
		ContractID: job.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: privateNotice})
	if err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	lin := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal}
	linSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: lin.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "organization-lin-session"})
	if err != nil {
		t.Fatal(err)
	}
	lin.SessionID = linSession.SessionID
	before, err := s.ReadRPOrganizationNotices(ctx, ada)
	if err != nil || len(before.Notices) != 0 {
		t.Fatal("unpublished layoff appeared as notice", before, err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 0 {
		t.Fatal("private Career source became information automatically", got)
	}
	publication := RPOrganizationNoticePublishRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "publish-organization-layoff"),
		MessageID: "org-layoff-day-two", CareerEventID: layoff.EventID, SpeakerID: M2AgentBoID}
	wrong := publication
	wrong.Binding.PrincipalID = M2AgentAdaPrincipal
	wrong.Binding.IdempotencyKey = "unauthorized-organization-publish"
	if _, err := s.PublishRPOrganizationNotice(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("nonmanager published private layoff", err)
	}
	wrong = publication
	wrong.CareerEventID = accepted.EventID
	wrong.Binding.IdempotencyKey = "non-layoff-organization-publish"
	if _, err := s.PublishRPOrganizationNotice(ctx, wrong); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("non-layoff Career fact used as announcement", err)
	}
	published, err := s.PublishRPOrganizationNotice(ctx, publication)
	if err != nil || published.Fact.Channel != "organization_announcement" || published.Fact.RecipientID != "" ||
		published.Fact.Visibility != "organization" || published.Fact.SourceCareerEventID != layoff.EventID ||
		strings.Contains(published.Fact.Text, privateNotice) || strings.Contains(published.Fact.Text, M2AgentAdaID) ||
		strings.Contains(published.Fact.Text, job.ContractID) {
		t.Fatal("redacted Career publication", published, err)
	}
	if replay, err := s.PublishRPOrganizationNotice(ctx, publication); err != nil || !replay.Replayed || replay.EventID != published.EventID {
		t.Fatal("publication exact replay", replay, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + published.EventID}, 0)
	available, err := s.ReadRPOrganizationNotices(ctx, ada)
	if err != nil || len(available.Notices) != 1 || available.Notices[0].MessageID != publication.MessageID || available.Notices[0].Accessed {
		t.Fatal("employee notice discovery", available, err)
	}
	if unavailable, err := s.ReadRPOrganizationNotices(ctx, lin); err != nil || len(unavailable.Notices) != 0 {
		t.Fatal("nonemployee discovered private organization notice", unavailable, err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 0 {
		t.Fatal("listing mutated employee knowledge", got)
	}
	linView, err := s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	access := RPOrganizationNoticeAccessRequest{Binding: core.CareerBinding{PrincipalID: lin.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: linView.ObservationCursor,
		IdempotencyKey: "lin-guessed-organization-notice"}, SessionID: lin.SessionID, MessageID: publication.MessageID}
	if _, err := s.AccessRPOrganizationNotice(ctx, access); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("nonemployee accessed private organization notice", err)
	}
	adaView, err := s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	access.Binding.ExpectedHead = adaView.ObservationCursor
	access.Binding.IdempotencyKey = "ada-reads-organization-notice"
	access.SessionID = ada.SessionID
	received, err := s.AccessRPOrganizationNotice(ctx, access)
	if err != nil || received.Fact.RecipientID != M2AgentAdaID || received.Fact.SourceEventID != published.EventID ||
		received.Fact.DeliverWorldTime != received.WorldTime || received.Fact.AudienceContractID != job.ContractID ||
		received.Fact.AudienceTermsEventID == "" {
		t.Fatal("actual employee access delivery", received, err)
	}
	if replay, err := s.AccessRPOrganizationNotice(ctx, access); err != nil || !replay.Replayed || replay.EventID != received.EventID {
		t.Fatal("employee access exact replay", replay, err)
	}
	view, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range view.Facts {
		found = found || fact.Kind == "message_received" && fact.MessageID == publication.MessageID &&
			fact.Channel == "organization_announcement" && fact.Reliability == "official_statement" &&
			fact.Text == published.Fact.Text
	}
	if !found {
		t.Fatal("employee context lacks actual notice", view.Facts)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 1 ||
		got[0].Channel != "organization_announcement" || got[0].ClaimedReliability != "official_statement" ||
		strings.Contains(got[0].Text, privateNotice) {
		t.Fatal("employee model notice is absent or private Career content leaked", got)
	}
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 0 {
		t.Fatal("unaddressed nonemployee learned notice", got)
	}
	available, err = s.ReadRPOrganizationNotices(ctx, ada)
	if err != nil || len(available.Notices) != 1 || !available.Notices[0].Accessed {
		t.Fatal("notice list did not mark actual access", available, err)
	}
	adaView, err = s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRPInformationStance(ctx, RPInformationStanceRequest{Binding: core.CareerBinding{
		PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: adaView.ObservationCursor, IdempotencyKey: "ada-doubts-official-notice",
	}, SessionID: ada.SessionID, MessageID: publication.MessageID, Stance: "doubt"}); err != nil {
		t.Fatal("employee interpretation of official notice", err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 1 ||
		got[0].Stance != "doubt" || got[0].ClaimedReliability != "official_statement" {
		t.Fatal("official claim became automatic belief or lost stance", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("organization notice projection diverged", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE observation_records SET claim_payload='{"claim_type":"message_received","text":"forged"}' WHERE source_event_id=?`, received.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("forged organization observation escaped source audit", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild organization receipt from Events", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("organization observation repair did not converge", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("reopened notice lost source recovery", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.text',?) WHERE event_id=?`, privateNotice, published.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("tampered publication could leak private Career notice", diff, err)
	}
	if _, err := s.ReadRPOrganizationNotices(ctx, ada); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("tampered publication listed to employee", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.text',?) WHERE event_id=?`, published.Fact.Text, published.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("restored publication source still diverged", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.audience_terms_event_id','forged') WHERE event_id=?`, received.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("forged employee eligibility escaped source audit", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.audience_terms_event_id',?) WHERE event_id=?`, received.Fact.AudienceTermsEventID, received.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal("activate real layoff", err)
	}
	if list, err := s.ReadRPOrganizationNotices(ctx, ada); err != nil || len(list.Notices) != 0 {
		t.Fatal("former employee kept fresh organization access", list, err)
	}
	if _, err := s.AccessRPOrganizationNotice(ctx, access); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("former employee replayed private organization receipt", err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 1 || got[0].Stance != "doubt" {
		t.Fatal("past received notice vanished after employment ended", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("historical audience proof failed after lawful exit", diff, err)
	}
}

func TestRPInformationOrganizationNoticeSeparatesTwoEmployeesAndOutsider(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "organization-audience.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Keep the funded Career world but omit the optional fixed demo routine:
	// Bo's real second-day contract must not collide with that separate schedule.
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	for _, command := range []core.MaterializeCohortCommand{
		m2AgentMaterialization("rp_cai", M2RPNPCID, "Cai", 1, 400, 3, 0, 60, 0, "2026-09-22T07:01:00Z"),
		m2AgentMaterialization("rp_lin", M2RPPlayerID, "Lin", 1, 300, 3, 0, 45, 0, "2026-09-22T07:02:00Z"),
	} {
		command.ExpectedHead = careerTestBinding(t, s, "principal_creator", "cursor").ExpectedHead
		if _, err := s.MaterializeCohort(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.setupRPParticipantsAt(ctx, rpLifeSetupTime); err != nil {
		t.Fatal(err)
	}
	if _, err := s.setupRPTravelAt(ctx, rpLifeSetupTime); err != nil {
		t.Fatal(err)
	}
	org := careerTestOrg(t, s)
	org.Organization.ManagerPrincipalID = M2RPPlayerPrincipal
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	post := careerTestPosting(t, s)
	post.Binding.PrincipalID = M2RPPlayerPrincipal
	post.Posting.Capacity = 2
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	jobs := map[string]CareerRecord{}
	for _, employee := range []struct{ id, principal string }{
		{M2AgentAdaID, M2AgentAdaPrincipal}, {M2AgentBoID, M2AgentBoPrincipal},
	} {
		id, principal := employee.id, employee.principal
		appID, interviewID, evaluationID, offerID := "app_"+id, "interview_"+id, "evaluation_"+id, "offer_"+id
		if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{
			Binding: careerTestBinding(t, s, principal, "apply_"+id), ApplicationID: appID,
			PositionID: post.Posting.PositionID, CandidateID: id, Statement: "Apply for the cooperative role.",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{
			Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "invite_"+id), InterviewID: interviewID,
			ApplicationID: appID, Question: "Explain safe work.",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{
			Binding: careerTestBinding(t, s, principal, "answer_"+id), InterviewID: interviewID,
			Answer: "Inspect equipment and exits.",
		}); err != nil {
			t.Fatal(err)
		}
		evaluation := careerTestEvaluation(t, s, evaluationID, true)
		evaluation.Binding.PrincipalID, evaluation.InterviewID = M2RPPlayerPrincipal, interviewID
		if _, err := s.EvaluateCareerApplication(ctx, evaluation); err != nil {
			t.Fatal(err)
		}
		offer := careerTestOffer(t, s, offerID, evaluationID)
		offer.Binding.PrincipalID, offer.StartsOnDay = M2RPPlayerPrincipal, 2
		if _, err := s.OfferCareerEmployment(ctx, offer); err != nil {
			t.Fatal(err)
		}
		accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{
			Binding: careerTestBinding(t, s, principal, "accept_"+id), OfferID: offerID,
			AfterWorkPlaceID: M2AgentCafeID,
		})
		if err != nil {
			t.Fatal(err)
		}
		jobs[id] = accepted
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 8, 0), 200); err != nil {
		t.Fatal(err)
	}
	layoff, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{
		Binding:    careerTestBinding(t, s, M2RPPlayerPrincipal, "layoff_bo"),
		ContractID: jobs[M2AgentBoID].Fact.Employment.ContractID, Kind: "layoff",
		EffectiveFromDay: 3, Notice: "Bo's individual assessment and contract are private.",
	})
	if err != nil {
		t.Fatal(err)
	}
	publication := RPOrganizationNoticePublishRequest{
		Binding:   careerTestBinding(t, s, M2RPPlayerPrincipal, "publish_two_employees"),
		MessageID: "organization-change-day-three", CareerEventID: layoff.EventID, SpeakerID: M2RPPlayerID,
	}
	published, err := s.PublishRPOrganizationNotice(ctx, publication)
	if err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	bo := allowFixtureControl(t, ctx, s, M2AgentBoID)
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	for _, read := range []core.RPSessionReadRequest{ada, bo} {
		list, err := s.ReadRPOrganizationNotices(ctx, read)
		if err != nil || len(list.Notices) != 1 || list.Notices[0].MessageID != publication.MessageID || list.Notices[0].Accessed {
			t.Fatal("active employee could not discover unopened notice", read.SessionID, list, err)
		}
	}
	if list, err := s.ReadRPOrganizationNotices(ctx, cai); err != nil || len(list.Notices) != 0 {
		t.Fatal("outsider discovered organization notice", list, err)
	}
	for _, id := range []string{M2AgentAdaID, M2AgentBoID, M2RPNPCID} {
		if got := readCareerTestContext(t, s, id).Life.Information; len(got) != 0 {
			t.Fatal("publication/listing taught someone without access", id, got)
		}
	}
	access := func(read core.RPSessionReadRequest, key string) (RPInformationRecord, error) {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		return s.AccessRPOrganizationNotice(ctx, RPOrganizationNoticeAccessRequest{
			Binding: core.CareerBinding{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID,
				BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: key},
			SessionID: read.SessionID, MessageID: publication.MessageID,
		})
	}
	if _, err := access(cai, "outsider_access"); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("outsider accessed organization notice", err)
	}
	adaReceipt, err := access(ada, "ada_access")
	if err != nil {
		t.Fatal(err)
	}
	if got := readCareerTestContext(t, s, M2AgentBoID).Life.Information; len(got) != 0 {
		t.Fatal("Ada's access taught Bo", got)
	}
	boReceipt, err := access(bo, "bo_access")
	if err != nil || boReceipt.Fact.RecipientID != M2AgentBoID ||
		adaReceipt.Fact.RecipientID != M2AgentAdaID ||
		adaReceipt.Fact.AudienceContractID == boReceipt.Fact.AudienceContractID ||
		adaReceipt.Fact.SourceEventID != published.EventID || boReceipt.Fact.SourceEventID != published.EventID {
		t.Fatal("two actual employee deliveries lost distinct audience evidence", adaReceipt, boReceipt, err)
	}
	for _, employee := range []struct {
		read   core.RPSessionReadRequest
		id     string
		stance string
	}{
		{ada, M2AgentAdaID, "doubt"}, {bo, M2AgentBoID, "believe"},
	} {
		view, err := s.ObserveRPSession(ctx, employee.read)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.RecordRPInformationStance(ctx, RPInformationStanceRequest{
			Binding: core.CareerBinding{PrincipalID: employee.read.PrincipalID, InstanceID: M2DemoInstanceID,
				BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor,
				IdempotencyKey: "stance_" + employee.id},
			SessionID: employee.read.SessionID, MessageID: publication.MessageID, Stance: employee.stance,
		}); err != nil {
			t.Fatal(err)
		}
		got := readCareerTestContext(t, s, employee.id).Life.Information
		if len(got) != 1 || got[0].Stance != employee.stance || got[0].Text != published.Fact.Text {
			t.Fatal("employee did not retain own interpretation", employee.id, got)
		}
	}
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 0 {
		t.Fatal("outsider learned employee notice", got)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + published.EventID}, 2)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("multi-employee audience projection diverged", diff, err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 200); err != nil {
		t.Fatal("activate Bo's sourced layoff", err)
	}
	if list, err := s.ReadRPOrganizationNotices(ctx, bo); err != nil || len(list.Notices) != 0 {
		t.Fatal("former employee retained fresh notice access", list, err)
	}
	if _, err := access(bo, "bo_after_layoff"); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("former employee accessed notice again", err)
	}
	if list, err := s.ReadRPOrganizationNotices(ctx, ada); err != nil || len(list.Notices) != 1 || !list.Notices[0].Accessed {
		t.Fatal("remaining employee lost organization notice", list, err)
	}
	if got := readCareerTestContext(t, s, M2AgentBoID).Life.Information; len(got) != 1 || got[0].Stance != "believe" {
		t.Fatal("lawful exit erased Bo's historical learning", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("multi-employee audience history diverged after layoff", diff, err)
	}
}
