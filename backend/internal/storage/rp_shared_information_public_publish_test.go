package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedPublicNoticePublishPreservesSourceActorAndHumanGate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-public-publish.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "pub-share-territory"),
		ScopeKind: "world", ScopeID: M2DemoInstanceID, StewardID: M2AgentBoID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "pub-share-institution"),
		InstitutionID: "pub-share-council", ScopeKind: "world", ScopeID: M2DemoInstanceID,
		TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID,
		EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID}); err != nil {
		t.Fatal(err)
	}
	proposal, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "pub-share-propose"),
		InstitutionID: "pub-share-council", ProposerID: M2RPNPCID,
		Law: LawDefinition{LawID: "pub-share-quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Square must be quiet."}})
	if err != nil {
		t.Fatal(err)
	}
	law, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "pub-share-enact"),
		InstitutionID: "pub-share-council", LegislatorID: M2AgentAdaID,
		ProposalEventID: proposal.EventID, EffectiveWorldTime: M2AgentNoonTime})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct{ id, kind string }{{"principal_pub_share_operator", "operator"}, {"principal_pub_share_service", "service"}} {
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
		return core.CareerBinding{PrincipalID: "principal_pub_share_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("pub-share-enroll"),
		EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_pub_share_service", ControllerInstanceID: "pub-share-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("pub-share-assign"),
		EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	humanSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "pub-share-human"})
	if err != nil {
		t.Fatal(err)
	}
	serviceSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_pub_share_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID,
		POV: "second_person", IdempotencyKey: "pub-share-service"})
	if err != nil {
		t.Fatal(err)
	}
	human := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: humanSession.SessionID}
	service := core.RPSessionReadRequest{PrincipalID: "principal_pub_share_service", SessionID: serviceSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, service} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := s.ReadRPNoticePublicationSources(ctx, RPNoticePublicationSourceReadRequest{
		PrincipalID: service.PrincipalID, SessionID: service.SessionID, Channel: "public_notice"})
	if err != nil || len(sources.Sources) != 1 || sources.Sources[0].SourceHandle == "" ||
		strings.Contains(sources.Sources[0].SourceHandle, law.EventID) {
		t.Fatal("legislator lacks opaque own law source", sources, err)
	}
	if other, err := s.ReadRPNoticePublicationSources(ctx, RPNoticePublicationSourceReadRequest{
		PrincipalID: human.PrincipalID, SessionID: human.SessionID, Channel: "public_notice"}); err != nil || len(other.Sources) != 0 {
		t.Fatal("other Person discovered legislative source", other, err)
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("pub-share-round"),
		HumanSessionID: human.SessionID, ExternalSessionIDs: []string{service.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := head()
	action := RPSharedPublicNoticePublishRequest{PrincipalID: service.PrincipalID, SessionID: service.SessionID,
		RoundID: round.RoundID, MessageID: "pub-share-message", SourceHandle: sources.Sources[0].SourceHandle, IdempotencyKey: "pub-share-submit"}
	wrong := action
	wrong.PrincipalID, wrong.SessionID, wrong.IdempotencyKey = human.PrincipalID, human.SessionID, "pub-share-wrong"
	if _, err := s.SubmitRPSharedPublicNoticePublish(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("non-legislator proposed law publication", err)
	}
	wrong = action
	wrong.SourceHandle, wrong.IdempotencyKey = "pns_unenacted", "pub-share-unenacted"
	if _, err := s.SubmitRPSharedPublicNoticePublish(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("unknown source handle entered shared publication", err)
	}
	if pending, err := s.SubmitRPSharedPublicNoticePublish(ctx, action); err != nil || pending.Submitted != 1 || head() != baseline {
		t.Fatal("proposal wrote an Event", pending, err)
	}
	if replay, err := s.SubmitRPSharedPublicNoticePublish(ctx, action); err != nil || !replay.Replayed {
		t.Fatal("proposal exact retry", replay, err)
	}
	changed := action
	changed.MessageID = "pub-share-other"
	if _, err := s.SubmitRPSharedPublicNoticePublish(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed proposal reused key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: service.PrincipalID, SessionID: service.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("publication bypassed Human answer", err)
	}
	direct := RPPublicNoticePublishRequest{Binding: core.CareerBinding{PrincipalID: M2AgentAdaPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: baseline,
		IdempotencyKey: "pub-share-direct"}, MessageID: "pub-share-direct-message", LawEventID: law.EventID, SpeakerID: M2AgentAdaID}
	if _, err := s.PublishRPPublicNotice(ctx, direct); err == nil {
		t.Fatal("direct publication bypassed active shared round")
	}
	var at string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID,
		RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "pub-share-human-wait"}); err != nil {
		t.Fatal(err)
	}
	selectedPause := errors.New("interrupt after public publication selection")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectedPause
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectedPause) || head() != baseline {
		t.Fatal("selection changed world before publication", err)
	}
	var childJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=?`,
		round.RoundID, service.SessionID).Scan(&childJSON); err != nil {
		t.Fatal(err)
	}
	var pinned RPPublicNoticePublishRequest
	if err := json.Unmarshal([]byte(childJSON), &pinned); err != nil {
		t.Fatal(err)
	}
	changedChild := pinned
	changedChild.MessageID = "pub-share-forged-child"
	if _, err := s.PublishRPPublicNotice(ctx, changedChild); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("changed selected child crossed public publication window", err)
	}
	changedChild = pinned
	changedChild.ControllerPrincipalID = human.PrincipalID
	if _, err := s.PublishRPPublicNotice(ctx, changedChild); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("other controller stole selected public publication", err)
	}
	if head() != baseline {
		t.Fatal("rejected selected child changed world")
	}
	interrupted := errors.New("interrupt after public publication Event")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "information_public_publish_event_committed" {
			return interrupted
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, interrupted) || head() != baseline+1 {
		t.Fatal("selected publication did not commit before interrupted receipt", err)
	}
	s.afterRPSharedActionStage = nil
	if pending, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || pending.Status != "advancing" {
		t.Fatal("interrupted selected publication not recoverable", pending, err)
	}
	if _, err := s.PublishRPPublicNotice(ctx, direct); err == nil {
		t.Fatal("direct publication crossed advancing round")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.EventSequence != baseline+1 || settled.OwnDisposition != "action_accepted" {
		t.Fatal("restarted selected publication did not settle", settled, err)
	}
	var actor, payload string
	if err := s.db.QueryRowContext(ctx, `SELECT actor_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence=? AND event_type='RPInformationSent'`,
		M2DemoInstanceID, M2DemoBranchID, baseline+1).Scan(&actor, &payload); err != nil {
		t.Fatal(err)
	}
	if actor != M2AgentAdaPrincipal || head() != baseline+1 {
		t.Fatal("controller replaced legislator source actor or duplicated Event", actor, head())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key IN (SELECT 'information:'||event_id FROM events WHERE instance_id=? AND branch_id=? AND event_sequence=?)`,
		[]any{M2DemoInstanceID, M2DemoBranchID, baseline + 1}, 0)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared public publication source diverged", diff, err)
	}
}
