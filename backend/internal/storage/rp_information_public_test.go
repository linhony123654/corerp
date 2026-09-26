package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInformationPublicLawNoticeRequiresActualAccess(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "public-law-notice.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "public-scope"),
		ScopeKind: "world", ScopeID: M2DemoInstanceID, StewardID: M2AgentBoID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "public-council"),
		InstitutionID: "public-council", ScopeKind: "world", ScopeID: M2DemoInstanceID,
		TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID,
		EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID}); err != nil {
		t.Fatal(err)
	}
	proposal, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "public-proposal"),
		InstitutionID: "public-council", ProposerID: M2RPNPCID,
		Law: LawDefinition{LawID: "quiet-square", ProhibitedAction: "speak", FineMinor: 2, Text: "Square must be quiet."}})
	if err != nil {
		t.Fatal(err)
	}
	law, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "public-enact"),
		InstitutionID: "public-council", LegislatorID: M2AgentAdaID,
		ProposalEventID: proposal.EventID, EffectiveWorldTime: M2AgentNoonTime})
	if err != nil {
		t.Fatal(err)
	}
	linSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "public-lin-session"})
	if err != nil {
		t.Fatal(err)
	}
	lin := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: linSession.SessionID}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 0 {
		t.Fatal("law truth automatically became Lin's information", got)
	}
	request := RPPublicNoticePublishRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "public-publish"),
		MessageID: "public-law-quiet-square", LawEventID: law.EventID, SpeakerID: M2AgentAdaID}
	wrong := request
	wrong.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "public-wrong-author")
	if _, err := s.PublishRPPublicNotice(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("other principal published legislator's enactment", err)
	}
	wrong = request
	wrong.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "public-not-enacted")
	wrong.LawEventID = proposal.EventID
	if _, err := s.PublishRPPublicNotice(ctx, wrong); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("unapproved proposal became public law notice", err)
	}
	published, err := s.PublishRPPublicNotice(ctx, request)
	if err != nil || published.Fact.Channel != "public_notice" || published.Fact.Visibility != "public" ||
		published.Fact.RecipientID != "" || published.Fact.SourceLawEventID != law.EventID ||
		!strings.Contains(published.Fact.Text, "Square must be quiet.") {
		t.Fatal("law publication", published, err)
	}
	if replay, err := s.PublishRPPublicNotice(ctx, request); err != nil || !replay.Replayed || replay.EventID != published.EventID {
		t.Fatal("publication exact replay", replay, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + published.EventID}, 0)
	for _, read := range []core.RPSessionReadRequest{lin, cai} {
		list, err := s.ReadRPPublicNotices(ctx, read)
		if err != nil || len(list.Notices) != 1 || list.Notices[0].MessageID != request.MessageID || list.Notices[0].Accessed {
			t.Fatal("public metadata discovery", list, err)
		}
	}
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 0 {
		t.Fatal("discovery taught Cai law", got)
	}
	linView, err := s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	access := RPPublicNoticeAccessRequest{Binding: core.CareerBinding{PrincipalID: lin.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: linView.ObservationCursor,
		IdempotencyKey: "lin-public-read"}, SessionID: lin.SessionID, MessageID: request.MessageID}
	received, err := s.AccessRPPublicNotice(ctx, access)
	if err != nil || received.Fact.RecipientID != M2RPPlayerID || received.Fact.SourceEventID != published.EventID ||
		received.Fact.AudienceContractID != "" || received.Fact.AudienceTermsEventID != "" {
		t.Fatal("public reader delivery", received, err)
	}
	if replay, err := s.AccessRPPublicNotice(ctx, access); err != nil || !replay.Replayed || replay.EventID != received.EventID {
		t.Fatal("public access exact replay", replay, err)
	}
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 1 ||
		got[0].Channel != "public_notice" || got[0].ClaimedReliability != "official_statement" || got[0].Stance != "" {
		t.Fatal("actual reader's uncommitted belief", got)
	}
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 0 {
		t.Fatal("unaccessed public news taught Cai", got)
	}
	linView, err = s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRPInformationStance(ctx, RPInformationStanceRequest{Binding: core.CareerBinding{
		PrincipalID: lin.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: linView.ObservationCursor, IdempotencyKey: "lin-public-doubt"},
		SessionID: lin.SessionID, MessageID: request.MessageID, Stance: "doubt"}); err != nil {
		t.Fatal("recipient public claim stance", err)
	}
	caiView, err := s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	caiAccess := RPPublicNoticeAccessRequest{Binding: core.CareerBinding{PrincipalID: cai.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: caiView.ObservationCursor,
		IdempotencyKey: "cai-public-read"}, SessionID: cai.SessionID, MessageID: request.MessageID}
	if _, err := s.AccessRPPublicNotice(ctx, caiAccess); err != nil {
		t.Fatal("second reader accesses the same notice", err)
	}
	caiView, err = s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRPInformationStance(ctx, RPInformationStanceRequest{Binding: core.CareerBinding{
		PrincipalID: cai.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: caiView.ObservationCursor, IdempotencyKey: "cai-public-believe"},
		SessionID: cai.SessionID, MessageID: request.MessageID, Stance: "believe"}); err != nil {
		t.Fatal("second reader's independent interpretation", err)
	}
	linMemory := readCareerTestContext(t, s, M2RPPlayerID).Life.Information
	caiMemory := readCareerTestContext(t, s, M2RPNPCID).Life.Information
	boMemory := readCareerTestContext(t, s, M2AgentBoID).Life.Information
	if len(linMemory) != 1 || linMemory[0].Stance != "doubt" || len(caiMemory) != 1 ||
		caiMemory[0].Stance != "believe" || len(boMemory) != 0 {
		t.Fatal("public audience did not retain two beliefs and one unaware observer", linMemory, caiMemory, boMemory)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("law notice source or projection diverged", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE observation_records SET claim_payload='{"claim_type":"message_received","text":"forged"}' WHERE source_event_id=?`, received.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("forged public observation escaped audit", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("repair public notice projection", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("public receipt repair did not converge", diff, err)
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
		t.Fatal("reopened public notice source diverged", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.text','forged public claim') WHERE event_id=?`, published.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("forged public claim escaped law source audit", diff, err)
	}
}
