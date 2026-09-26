package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedPublicNoticeAccessRequiresHumanAndRecoversRecipientOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-public-access.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "shared-public-scope"),
		ScopeKind: "world", ScopeID: M2DemoInstanceID, StewardID: M2AgentBoID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "shared-public-council"),
		InstitutionID: "shared-public-council", ScopeKind: "world", ScopeID: M2DemoInstanceID,
		TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID,
		EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID}); err != nil {
		t.Fatal(err)
	}
	proposal, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "shared-public-proposal"),
		InstitutionID: "shared-public-council", ProposerID: M2RPNPCID,
		Law: LawDefinition{LawID: "shared-quiet-square", ProhibitedAction: "speak", FineMinor: 2, Text: "Square must be quiet."}})
	if err != nil {
		t.Fatal(err)
	}
	law, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "shared-public-enact"),
		InstitutionID: "shared-public-council", LegislatorID: M2AgentAdaID,
		ProposalEventID: proposal.EventID, EffectiveWorldTime: M2AgentNoonTime})
	if err != nil {
		t.Fatal(err)
	}
	const messageID = "shared-public-law-message"
	published, err := s.PublishRPPublicNotice(ctx, RPPublicNoticePublishRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "shared-public-publish"),
		MessageID: messageID, LawEventID: law.EventID, SpeakerID: M2AgentAdaID})
	if err != nil {
		t.Fatal(err)
	}
	const otherMessageID = "shared-public-other-message"
	if _, err := s.PublishRPPublicNotice(ctx, RPPublicNoticePublishRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "shared-public-other-publish"),
		MessageID: otherMessageID, LawEventID: law.EventID, SpeakerID: M2AgentAdaID}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct{ id, kind string }{{"principal_shared_public_operator", "operator"}, {"principal_shared_public_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var n int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_shared_public_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("shared-public-enroll"),
		EntityID: M2RPNPCID, ControllerPrincipalID: "principal_shared_public_service", ControllerInstanceID: "shared-public-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("shared-public-assign"),
		EntityID: M2RPNPCID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	humanSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "shared-public-human"})
	if err != nil {
		t.Fatal(err)
	}
	externalSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_shared_public_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPNPCID,
		POV: "second_person", IdempotencyKey: "shared-public-external"})
	if err != nil {
		t.Fatal(err)
	}
	human := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: humanSession.SessionID}
	external := core.RPSessionReadRequest{PrincipalID: "principal_shared_public_service", SessionID: externalSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	if list, err := s.ReadRPPublicNotices(ctx, external); err != nil || len(list.Notices) != 2 || list.Notices[0].Accessed || list.Notices[1].Accessed {
		t.Fatal("actual public board discovery", list, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + published.EventID}, 0)
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("shared-public-round"),
		HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := head()
	action := RPSharedPublicNoticeAccessRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID,
		RoundID: round.RoundID, MessageID: messageID, IdempotencyKey: "shared-public-access"}
	unknown := action
	unknown.MessageID = "not-published"
	unknown.IdempotencyKey = "shared-public-unknown"
	if _, err := s.SubmitRPSharedPublicNoticeAccess(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("unpublished notice proposed", err)
	}
	foreign := action
	foreign.PrincipalID = human.PrincipalID
	if _, err := s.SubmitRPSharedPublicNoticeAccess(ctx, foreign); err == nil {
		t.Fatal("foreign principal proposed another reader's notice")
	}
	if submitted, err := s.SubmitRPSharedPublicNoticeAccess(ctx, action); err != nil || submitted.Submitted != 1 {
		t.Fatal("public access proposal", submitted, err)
	}
	if head() != baseline {
		t.Fatal("proposal wrote a delivery Event")
	}
	if replay, err := s.SubmitRPSharedPublicNoticeAccess(ctx, action); err != nil || !replay.Replayed {
		t.Fatal("exact proposal retry", replay, err)
	}
	changed := action
	changed.MessageID = "changed-public-notice"
	if _, err := s.SubmitRPSharedPublicNoticeAccess(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed proposal reused key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: external.PrincipalID, SessionID: external.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external reader bypassed Human", err)
	}
	direct := RPPublicNoticeAccessRequest{Binding: core.CareerBinding{PrincipalID: external.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: baseline,
		IdempotencyKey: "direct-public-bypass"}, SessionID: external.SessionID, MessageID: messageID}
	if _, err := s.AccessRPPublicNotice(ctx, direct); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct public access bypassed shared round", err)
	}
	var now string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&now); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID,
		RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "shared-public-human-wait"}); err != nil {
		t.Fatal(err)
	}
	selectedFailure := errors.New("interrupt after public access selection")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectedFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectedFailure) || head() != baseline {
		t.Fatal("selection alone taught the reader", err)
	}
	direct.Binding.IdempotencyKey = "shared_action_" + round.RoundID
	direct.MessageID = otherMessageID
	if _, err := s.AccessRPPublicNotice(ctx, direct); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("changed selected child accessed a notice", err)
	}
	direct.MessageID = messageID
	direct.SessionID = human.SessionID
	if _, err := s.AccessRPPublicNotice(ctx, direct); err == nil {
		t.Fatal("selected child accessed for another session", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationDelivered' AND json_extract(payload,'$.message_id')=?`, []any{messageID}, 0)
	eventFailure := errors.New("interrupt after durable public receipt Event")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "information_public_access_event_committed" {
			return eventFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, eventFailure) || head() != baseline+1 {
		t.Fatal("public delivery Event-before-receipt stage", err, head())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationDelivered' AND json_extract(payload,'$.message_id')=?`, []any{messageID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != baseline+1 {
		t.Fatal("recover shared public access", settled, err)
	}
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 1 || got[0].Channel != "public_notice" || got[0].Stance != "" {
		t.Fatal("public reader Knowledge or stance", got)
	}
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 0 {
		t.Fatal("Human wait learned another reader's notice", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared public access projection differed", diff, err)
	}
}
