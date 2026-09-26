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

func TestRPSharedOrganizationNoticePublishKeepsLayoffPrivateAndRecovers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-org-publish.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 12)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "org-pub-accept"),
		OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	privateNotice := "Ada 的工资与个人评估不得公开。"
	layoff, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "org-pub-layoff"),
		ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: privateNotice})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct{ id, kind string }{{"principal_org_pub_operator", "operator"}, {"principal_org_pub_service", "service"}} {
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
		return core.CareerBinding{PrincipalID: "principal_org_pub_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("org-pub-enroll"),
		EntityID: M2AgentBoID, ControllerPrincipalID: "principal_org_pub_service", ControllerInstanceID: "org-pub-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("org-pub-assign"),
		EntityID: M2AgentBoID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	humanSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "org-pub-human"})
	if err != nil {
		t.Fatal(err)
	}
	serviceSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_org_pub_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID,
		POV: "second_person", IdempotencyKey: "org-pub-service"})
	if err != nil {
		t.Fatal(err)
	}
	human := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: humanSession.SessionID}
	service := core.RPSessionReadRequest{PrincipalID: "principal_org_pub_service", SessionID: serviceSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, service} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := s.ReadRPNoticePublicationSources(ctx, RPNoticePublicationSourceReadRequest{
		PrincipalID: service.PrincipalID, SessionID: service.SessionID, Channel: "organization_announcement"})
	if err != nil || len(sources.Sources) != 1 || sources.Sources[0].SourceHandle == "" ||
		strings.Contains(sources.Sources[0].Summary, privateNotice) ||
		strings.Contains(sources.Sources[0].Summary, accepted.Fact.Employment.ContractID) ||
		strings.Contains(sources.Sources[0].SourceHandle, layoff.EventID) {
		t.Fatal("manager source handle leaked private layoff", sources, err)
	}
	if other, err := s.ReadRPNoticePublicationSources(ctx, RPNoticePublicationSourceReadRequest{
		PrincipalID: human.PrincipalID, SessionID: human.SessionID, Channel: "organization_announcement"}); err != nil || len(other.Sources) != 0 {
		t.Fatal("nonmanager discovered private Career source", other, err)
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("org-pub-round"),
		HumanSessionID: human.SessionID, ExternalSessionIDs: []string{service.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := head()
	action := RPSharedOrganizationNoticePublishRequest{PrincipalID: service.PrincipalID, SessionID: service.SessionID,
		RoundID: round.RoundID, MessageID: "org-pub-day-two", SourceHandle: sources.Sources[0].SourceHandle, IdempotencyKey: "org-pub-submit"}
	wrong := action
	wrong.PrincipalID, wrong.SessionID, wrong.IdempotencyKey = human.PrincipalID, human.SessionID, "org-pub-wrong"
	if _, err := s.SubmitRPSharedOrganizationNoticePublish(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("nonmanager proposed layoff publication", err)
	}
	wrong = action
	wrong.SourceHandle, wrong.IdempotencyKey = "pns_not_layoff", "org-pub-not-layoff"
	if _, err := s.SubmitRPSharedOrganizationNoticePublish(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("unknown private source handle entered shared publication", err)
	}
	if pending, err := s.SubmitRPSharedOrganizationNoticePublish(ctx, action); err != nil || pending.Submitted != 1 || head() != baseline {
		t.Fatal("organization proposal wrote Event", pending, err)
	}
	if replay, err := s.SubmitRPSharedOrganizationNoticePublish(ctx, action); err != nil || !replay.Replayed {
		t.Fatal("organization proposal exact retry", replay, err)
	}
	changed := action
	changed.MessageID = "org-pub-other"
	if _, err := s.SubmitRPSharedOrganizationNoticePublish(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed organization proposal reused key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: service.PrincipalID, SessionID: service.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("organization publication bypassed Human answer", err)
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
		IdempotencyKey: "org-pub-human-wait"}); err != nil {
		t.Fatal(err)
	}
	selectedPause := errors.New("interrupt after organization publication selection")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectedPause
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectedPause) || head() != baseline {
		t.Fatal("organization selection changed world", err)
	}
	var childJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=?`,
		round.RoundID, service.SessionID).Scan(&childJSON); err != nil {
		t.Fatal(err)
	}
	var pinned RPOrganizationNoticePublishRequest
	if err := json.Unmarshal([]byte(childJSON), &pinned); err != nil {
		t.Fatal(err)
	}
	changedChild := pinned
	changedChild.MessageID = "org-pub-forged-child"
	if _, err := s.PublishRPOrganizationNotice(ctx, changedChild); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("changed selected organization child crossed window", err)
	}
	interrupted := errors.New("interrupt after organization publication Event")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "information_organization_publish_event_committed" {
			return interrupted
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, interrupted) || head() != baseline+1 {
		t.Fatal("selected organization publication did not commit before interruption", err)
	}
	s.afterRPSharedActionStage = nil
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
		t.Fatal("restarted organization publication did not settle", settled, err)
	}
	var actor, raw string
	if err := s.db.QueryRowContext(ctx, `SELECT actor_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence=? AND event_type='RPInformationSent'`,
		M2DemoInstanceID, M2DemoBranchID, baseline+1).Scan(&actor, &raw); err != nil {
		t.Fatal(err)
	}
	if actor != M2AgentBoPrincipal || head() != baseline+1 || strings.Contains(raw, privateNotice) ||
		strings.Contains(raw, accepted.Fact.Employment.ContractID) || strings.Contains(raw, M2AgentAdaID) {
		t.Fatal("organization announcement leaked private Career source or changed actor", actor, raw)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key IN (SELECT 'information:'||event_id FROM events WHERE instance_id=? AND branch_id=? AND event_sequence=?)`,
		[]any{M2DemoInstanceID, M2DemoBranchID, baseline + 1}, 0)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared organization publication source diverged", diff, err)
	}
}
