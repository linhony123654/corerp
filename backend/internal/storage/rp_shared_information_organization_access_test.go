package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedOrganizationNoticeAccessRequiresEmployeeHumanAndRetainsHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-organization-access.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 12)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "shared-org-accept"),
		OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	privateNotice := "Ada 的工资与个人评估不得外泄。"
	layoff, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "shared-org-layoff"),
		ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: privateNotice})
	if err != nil {
		t.Fatal(err)
	}
	const messageID, otherMessageID = "shared-org-day-two", "shared-org-day-two-other"
	published, err := s.PublishRPOrganizationNotice(ctx, RPOrganizationNoticePublishRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "shared-org-publish"),
		MessageID: messageID, CareerEventID: layoff.EventID, SpeakerID: M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishRPOrganizationNotice(ctx, RPOrganizationNoticePublishRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "shared-org-publish-other"),
		MessageID: otherMessageID, CareerEventID: layoff.EventID, SpeakerID: M2AgentBoID}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(published.Fact.Text, privateNotice) || strings.Contains(published.Fact.Text, accepted.Fact.Employment.ContractID) {
		t.Fatal("private Career details appeared in organization claim")
	}
	for _, p := range []struct{ id, kind string }{{"principal_shared_org_operator", "operator"}, {"principal_shared_org_service", "service"}} {
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
		return core.CareerBinding{PrincipalID: "principal_shared_org_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("shared-org-enroll"),
		EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_shared_org_service", ControllerInstanceID: "shared-org-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("shared-org-assign"),
		EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	humanSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "shared-org-human"})
	if err != nil {
		t.Fatal(err)
	}
	externalSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_shared_org_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID,
		POV: "second_person", IdempotencyKey: "shared-org-external"})
	if err != nil {
		t.Fatal(err)
	}
	human := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: humanSession.SessionID}
	external := core.RPSessionReadRequest{PrincipalID: "principal_shared_org_service", SessionID: externalSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	if list, err := s.ReadRPOrganizationNotices(ctx, external); err != nil || len(list.Notices) != 2 || list.Notices[0].Accessed || list.Notices[1].Accessed {
		t.Fatal("eligible service resident sees only metadata", list, err)
	}
	if list, err := s.ReadRPOrganizationNotices(ctx, human); err != nil || len(list.Notices) != 0 {
		t.Fatal("nonemployee discovered announcement", list, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + published.EventID}, 0)
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("shared-org-round"),
		HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := head()
	action := RPSharedOrganizationNoticeAccessRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID,
		RoundID: round.RoundID, MessageID: messageID, IdempotencyKey: "shared-org-proposal"}
	outsider := action
	outsider.PrincipalID, outsider.SessionID, outsider.IdempotencyKey = human.PrincipalID, human.SessionID, "shared-org-outsider"
	if _, err := s.SubmitRPSharedOrganizationNoticeAccess(ctx, outsider); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("nonemployee entered organization access round", err)
	}
	unknown := action
	unknown.MessageID, unknown.IdempotencyKey = "not-published", "shared-org-unknown"
	if _, err := s.SubmitRPSharedOrganizationNoticeAccess(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("unpublished announcement entered round", err)
	}
	if pending, err := s.SubmitRPSharedOrganizationNoticeAccess(ctx, action); err != nil || pending.Submitted != 1 || head() != baseline {
		t.Fatal("eligible proposal wrote an Event", pending, err)
	}
	if replay, err := s.SubmitRPSharedOrganizationNoticeAccess(ctx, action); err != nil || !replay.Replayed {
		t.Fatal("exact organization proposal replay", replay, err)
	}
	changed := action
	changed.MessageID = otherMessageID
	if _, err := s.SubmitRPSharedOrganizationNoticeAccess(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed organization proposal reused key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: external.PrincipalID, SessionID: external.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("service access bypassed Human", err)
	}
	direct := RPOrganizationNoticeAccessRequest{Binding: core.CareerBinding{PrincipalID: external.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: baseline,
		IdempotencyKey: "shared-org-direct-bypass"}, SessionID: external.SessionID, MessageID: messageID}
	if _, err := s.AccessRPOrganizationNotice(ctx, direct); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct organization access bypassed shared round", err)
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
		IdempotencyKey: "shared-org-human-wait"}); err != nil {
		t.Fatal(err)
	}
	selectedFailure := errors.New("interrupt after organization access selection")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectedFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectedFailure) || head() != baseline {
		t.Fatal("selection alone taught employee", err)
	}
	direct.Binding.IdempotencyKey = "shared_action_" + round.RoundID
	direct.MessageID = otherMessageID
	if _, err := s.AccessRPOrganizationNotice(ctx, direct); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed valid announcement", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationDelivered' AND json_extract(payload,'$.message_id')=?`, []any{messageID}, 0)
	eventFailure := errors.New("interrupt after organization receipt Event")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "information_organization_access_event_committed" {
			return eventFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, eventFailure) || head() != baseline+1 {
		t.Fatal("organization delivery Event-before-receipt stage", err, head())
	}
	// Scheduled employment exit may lawfully happen after the delivery Event
	// but before the shared-round receipt is settled. Recovery must use that
	// accepted historical Event rather than demand fresh employee access.
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal("activate sourced layoff before shared receipt recovery", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	childKey, err := core.HashJSON([]string{external.PrincipalID, "shared_action_" + round.RoundID})
	if err != nil {
		t.Fatal(err)
	}
	var originalHash string
	if err := s.db.QueryRowContext(ctx, `SELECT request_hash FROM commands WHERE command_type='AccessRPOrganizationNotice' AND idempotency_key=?`, childKey).Scan(&originalHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE commands SET request_hash=? WHERE command_type='AccessRPOrganizationNotice' AND idempotency_key=?`, "sha256:"+strings.Repeat("0", 64), childKey); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("mismatched committed child hash settled organization round", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE commands SET request_hash=? WHERE command_type='AccessRPOrganizationNotice' AND idempotency_key=?`, originalHash, childKey); err != nil {
		t.Fatal(err)
	}
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != baseline+1 {
		t.Fatal("recover selected organization access", settled, err)
	}
	var audienceContract, audienceTerms string
	if err := s.db.QueryRowContext(ctx, `SELECT json_extract(payload,'$.audience_contract_id'),json_extract(payload,'$.audience_terms_event_id')
	 FROM events WHERE instance_id=? AND branch_id=? AND event_sequence=? AND event_type='RPInformationDelivered'`,
		M2DemoInstanceID, M2DemoBranchID, settled.EventSequence).Scan(&audienceContract, &audienceTerms); err != nil ||
		audienceContract != accepted.Fact.Employment.ContractID || audienceTerms == "" {
		t.Fatal("selected organization delivery lost employment lineage", audienceContract, audienceTerms, err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 1 ||
		got[0].Channel != "organization_announcement" || got[0].Stance != "" || strings.Contains(got[0].Text, privateNotice) {
		t.Fatal("employee's private model notice", got)
	}
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 0 {
		t.Fatal("nonemployee Human learned organization claim", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared organization access projection differed", diff, err)
	}
	if list, err := s.ReadRPOrganizationNotices(ctx, external); err != nil || len(list.Notices) != 0 {
		t.Fatal("former employee retained fresh announcement listing", list, err)
	}
	direct.MessageID = otherMessageID
	direct.Binding.IdempotencyKey = "shared-org-former-employee"
	direct.Binding.ExpectedHead = head()
	if _, err := s.AccessRPOrganizationNotice(ctx, direct); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("former employee gained fresh announcement", err)
	}
	if receipt, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || receipt.EventSequence != settled.EventSequence {
		t.Fatal("historical round receipt changed after layoff", receipt, err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 1 || got[0].Channel != "organization_announcement" {
		t.Fatal("historical employee Knowledge vanished", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("historical audience proof diverged after layoff", diff, err)
	}
}
