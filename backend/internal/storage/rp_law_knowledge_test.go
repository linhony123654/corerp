package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

type deliberateLawBreakingProvider struct{}

func (deliberateLawBreakingProvider) Propose(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
	return core.RPDecisionProposal{Action: "respond", Text: "我知道这里要求安静，但我还是选择说话。"}, nil
}

func TestRPLawKnowledgeTimingAndExecutableNoncompliance(t *testing.T) {
	for _, scenario := range []struct {
		name, reviewer, principal, decision string
		conflicted                          bool
	}{
		{"reverse", M2AgentAdaID, M2AgentAdaPrincipal, "reverse", false},
		{"uphold", M2AgentAdaID, M2AgentAdaPrincipal, "uphold", false},
		{"defendant-reviewer", M2RPNPCID, M2RPNPCPrincipal, "reverse", true},
		{"enforcer-reviewer", M2AgentBoID, M2AgentBoPrincipal, "reverse", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			testRPLawCaseLifecycle(t, scenario.reviewer, scenario.principal, scenario.decision, scenario.conflicted)
		})
	}
}

func testRPLawCaseLifecycle(t *testing.T, reviewer, reviewerPrincipal, decision string, conflicted bool) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "law-knowledge.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "scope"), ScopeKind: "region", ScopeID: "cafe", StewardID: M2AgentBoID, PlaceIDs: []string{M2AgentCafeID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "institution"), InstitutionID: "quiet-council", ScopeKind: "region", ScopeID: "cafe", TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID, EnforcerID: M2AgentBoID, ReviewerID: reviewer}); err != nil {
		t.Fatal(err)
	}
	proposed, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "proposal"), InstitutionID: "quiet-council", ProposerID: M2RPNPCID, Law: LawDefinition{LawID: "quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Quiet in the cafe."}})
	if err != nil {
		t.Fatal(err)
	}
	effective := "2026-09-23T13:00:00Z"
	enacted, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "enact"), InstitutionID: "quiet-council", LegislatorID: M2AgentAdaID, ProposalEventID: proposed.EventID, EffectiveWorldTime: effective})
	if err != nil {
		t.Fatal(err)
	}
	if readCareerTestContext(t, s, M2RPNPCID).Law != nil || readCareerTestContext(t, s, M2AgentBoID).Law != nil {
		t.Fatal("proposal/enforcement role acquired enacted law omnisciently")
	}
	unseen := LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "unseen"), SpeakerID: M2AgentBoID, EnactmentEventID: enacted.EventID}
	if _, err := s.AnnounceRPLaw(ctx, unseen); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unseen relay: %v", err)
	}
	// Legislator's offsite announcement cannot inform the cafe's inhabitants.
	if _, err := s.AnnounceRPLaw(ctx, LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "offsite"), SpeakerID: M2AgentAdaID, EnactmentEventID: enacted.EventID}); err != nil {
		t.Fatal(err)
	}
	if readCareerTestContext(t, s, M2RPNPCID).Law != nil {
		t.Fatal("absent observer learned offsite announcement")
	}
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	news, err := s.AnnounceRPLaw(ctx, LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "news"), SpeakerID: M2AgentAdaID, EnactmentEventID: enacted.EventID})
	if err != nil {
		t.Fatal(err)
	}
	read := allowFixtureControl(t, ctx, s, M2AgentBoID)
	decisionEvents := map[string]string{}
	decide := func(key string, provider core.RPDecisionProvider, want string) {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: key})
		if err != nil {
			t.Fatal(err)
		}
		request := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
		input, err := s.BuildRPDecisionInput(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if input.Law == nil || len(input.Law.KnownLaws) != 1 || input.Law.KnownLaws[0].KnowledgeEventID != news.EventID {
			t.Fatal("known law source missing")
		}
		lawful, executable := false, false
		for _, a := range input.LegalActions {
			if a == "respond" {
				executable = true
			}
		}
		for _, a := range input.Law.LawfulActions {
			if a == "respond" {
				lawful = true
			}
		}
		if !executable || lawful != (key == "before-effective") {
			t.Fatalf("execution/law distinction %+v", input.Law)
		}
		choice, err := s.DecideRP(ctx, request, provider)
		if err != nil || choice.Proposal.Action != want {
			t.Fatalf("choice %+v %v", choice, err)
		}
		result, err := s.CommitRPDecision(ctx, request, choice)
		if err != nil || result.Action != want {
			t.Fatalf("committed choice %+v %v", result, err)
		}
		decisionEvents[key] = result.EventID
	}
	decide("before-effective", core.DeterministicRPDecisionProvider{}, "respond")
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: effective, Budget: 100, IdempotencyKey: "reach-law-effective"})
	if err != nil || wait.Status != "completed" || wait.CurrentWorldTime != effective {
		t.Fatalf("effective clock %+v %v", wait, err)
	}
	decide("lawful", core.DeterministicRPDecisionProvider{}, "silence")
	before := readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor
	decide("deliberate", deliberateLawBreakingProvider{}, "respond")
	// Spoken announcements/transmissions are not exempt merely because their
	// immutable Event belongs to another domain.
	spokenLaw, err := s.AnnounceRPLaw(ctx, LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "spoken-law"), SpeakerID: M2RPNPCID, EnactmentEventID: enacted.EventID})
	if err != nil {
		t.Fatal(err)
	}
	culture, err := s.DefineRPCulture(ctx, CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "speech-culture"), AuthorID: M2RPNPCID, Culture: core.RPCulture{CultureID: "speech-culture", ScopeKind: "community", ScopeID: "speech-culture", GroupIdentity: "sharing", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	spokenCulture, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "spoken-culture"), SpeakerID: M2RPNPCID, DefinitionEventID: culture.EventID})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventID := range []string{spokenLaw.EventID, spokenCulture.EventID} {
		v, err := s.RecordRPLawViolation(ctx, LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "charge-"+eventID), InstitutionID: "quiet-council", EnforcerID: M2AgentBoID, EnactmentEventID: enacted.EventID, ActionEventID: eventID})
		if err != nil || v.Fact.Violation.ActorID != M2RPNPCID || v.Fact.Violation.PlaceID != M2AgentCafeID {
			t.Fatalf("domain speech violation %+v %v", v, err)
		}
	}
	if _, err := s.RecordRPLawViolation(ctx, LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "not-spoken-definition"), InstitutionID: "quiet-council", EnforcerID: M2AgentBoID, EnactmentEventID: enacted.EventID, ActionEventID: culture.EventID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("private definition charged as speech: %v", err)
	}
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before {
		t.Fatal("law violation automatically fined actor")
	}
	violationRequest := LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "violation"), InstitutionID: "quiet-council", EnforcerID: M2AgentBoID, EnactmentEventID: enacted.EventID, ActionEventID: decisionEvents["before-effective"]}
	if _, err := s.RecordRPLawViolation(ctx, violationRequest); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("retroactive violation: %v", err)
	}
	violationRequest.ActionEventID = decisionEvents["lawful"]
	if _, err := s.RecordRPLawViolation(ctx, violationRequest); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("silence charged as speech: %v", err)
	}
	violationRequest.ActionEventID = decisionEvents["deliberate"]
	violation, err := s.RecordRPLawViolation(ctx, violationRequest)
	if err != nil {
		t.Fatal(err)
	}
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before {
		t.Fatal("recording violation automatically punished")
	}
	enforce := LawEnforcementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "fine"), InstitutionID: "quiet-council", EnforcerID: M2AgentAdaID, ViolationEventID: violation.EventID}
	if _, err := s.EnforceRPLaw(ctx, enforce); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("review role executed fine: %v", err)
	}
	enforce.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "fine")
	enforce.EnforcerID = M2AgentBoID
	if !conflicted && decision == "reverse" {
		var actorAccount string
		if err := s.db.QueryRow(`SELECT asset_account_id FROM materialized_entities WHERE entity_id=?`, M2RPNPCID).Scan(&actorAccount); err != nil {
			t.Fatal(err)
		}
		attempt := func() error { _, err := s.EnforceRPLaw(ctx, enforce); return err }
		assertLawMoneyBoundary(t, s, actorAccount, 0, attempt)
		assertLawMoneyBoundary(t, s, m2EconomyEmployerCash, core.MaxJSONSafeInteger, attempt)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "fine rollback") }
	if _, err := s.EnforceRPLaw(ctx, enforce); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("fine rollback: %v", err)
	}
	s.beforeCommit = nil
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before {
		t.Fatal("failed fine leaked funds")
	}
	var treasuryBefore int64
	if err := s.db.QueryRow(`SELECT balance_minor FROM account_balances WHERE account_id=?`, m2EconomyEmployerCash).Scan(&treasuryBefore); err != nil {
		t.Fatal(err)
	}
	fine, err := s.EnforceRPLaw(ctx, enforce)
	if err != nil {
		t.Fatal(err)
	}
	if fine.Fact.Enforcement.FineMinor != 2 || readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-2 {
		t.Fatal("actual fine did not affect owned funds")
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, treasuryBefore+2)
	assertM2Value(t, ctx, s, `SELECT SUM(p.amount_minor) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id WHERE j.event_id=?`, []any{fine.EventID}, 0)
	again := enforce
	again.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "duplicate-fine")
	if _, err := s.EnforceRPLaw(ctx, again); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("case fined twice: %v", err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current, err := readEffectiveInstitutionLaws(ctx, conn, M2DemoInstanceID, M2DemoBranchID, effective, M2AgentCafeID)
	conn.Close()
	if err != nil || len(current) != 1 || current[0].EnactmentEventID != enacted.EventID {
		t.Fatalf("authoritative law %+v %v", current, err)
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
	retry, err := s.EnforceRPLaw(ctx, enforce)
	if err != nil || !retry.Replayed || retry.EventID != fine.EventID {
		t.Fatalf("fine restart retry %+v %v", retry, err)
	}
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-2 {
		t.Fatal("fine duplicated or lost across rebuild")
	}
	recovered := readCareerTestContext(t, s, M2RPNPCID).Law
	if recovered == nil || len(recovered.KnownLaws) != 1 || recovered.KnownLaws[0].KnowledgeEventID != news.EventID {
		t.Fatal("law belief lost across recovery")
	}
	cases := readCareerTestContext(t, s, M2RPNPCID).Life.LawCases
	queried, err := s.ReadOwnRPLawCases(ctx, LawCasesRequest{PrincipalID: M2RPNPCPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2RPNPCID})
	if err != nil || len(queried) != 1 || queried[0].EnforcementEventID != fine.EventID {
		t.Fatalf("own case read %+v %v", queried, err)
	}
	if _, err := s.ReadOwnRPLawCases(ctx, LawCasesRequest{PrincipalID: M2AgentBoPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2RPNPCID}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("enforcer queried private defendant cases: %v", err)
	}
	if len(cases) != 1 || cases[0].EnforcementEventID != fine.EventID || cases[0].Status != "enforced" {
		t.Fatalf("own fine receipt missing %+v", cases)
	}
	if len(readCareerTestContext(t, s, M2AgentBoID).Life.LawCases) != 0 {
		t.Fatal("private defendant case appeared as another actor's own receipt")
	}
	disputeRequest := LawDisputeRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "dispute"), InstitutionID: "quiet-council", ActorID: M2AgentBoID, EnforcementEventID: fine.EventID, Statement: "Please review this fine."}
	if _, err := s.DisputeRPLaw(ctx, disputeRequest); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("another actor disputed defendant's fine: %v", err)
	}
	disputeRequest.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "dispute")
	disputeRequest.ActorID = M2RPNPCID
	dispute, err := s.DisputeRPLaw(ctx, disputeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-2 {
		t.Fatal("filing dispute automatically refunded fine")
	}
	reviewRequest := LawReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "review"), InstitutionID: "quiet-council", ReviewerID: M2AgentBoID, DisputeEventID: dispute.EventID, Decision: "reverse", Reason: "Grant a reasoned discretionary reversal for this first incident."}
	if _, err := s.ReviewRPLawDispute(ctx, reviewRequest); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("enforcer reviewed own enforcement: %v", err)
	}
	reviewRequest.Binding = careerTestBinding(t, s, reviewerPrincipal, "review")
	reviewRequest.ReviewerID = reviewer
	reviewRequest.Decision = decision
	forwardedReceipt := ""
	if conflicted {
		// Verify a real sourced role and addressed filing before testing conflict.
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = authorizeInstitutionRole(ctx, conn, reviewRequest.Binding, "quiet-council", reviewer, institutionReview)
		conn.Close()
		if err != nil {
			t.Fatalf("conflicted reviewer lacks actual role: %v", err)
		}
		if len(dispute.Fact.Dispute.ReviewerIDs) != 1 || dispute.Fact.Dispute.ReviewerIDs[0] != reviewer {
			t.Fatal("filing was not addressed to actual reviewer")
		}
		if _, err := s.ReviewRPLawDispute(ctx, reviewRequest); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("interested role holder reviewed own case: %v", err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_review'`, nil, 0)
		assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, treasuryBefore+2)
		cases = readCareerTestContext(t, s, M2RPNPCID).Life.LawCases
		if len(cases) != 1 || cases[0].Status != "disputed" || readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-2 {
			t.Fatal("rejected conflicted review changed case or funds")
		}
		if reviewer == M2AgentBoID {
			return
		}
		if _, err := s.ChangeRPInstitutionAuthority(ctx, InstitutionAuthorityRequest{Binding: careerTestBinding(t, s, "principal_creator", "new-reviewer"), InstitutionID: "quiet-council", CapabilityID: institutionReview, Decision: "appoint", EntityID: M2AgentAdaID}); err != nil {
			t.Fatal(err)
		}
		reviewerPrincipal = M2AgentAdaPrincipal
		reviewRequest.ReviewerID = M2AgentAdaID
		reviewRequest.Binding = careerTestBinding(t, s, reviewerPrincipal, "successor-review")
		if _, err := s.ReviewRPLawDispute(ctx, reviewRequest); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("successor learned private filing by appointment: %v", err)
		}
		forward := LawDisputeForwardRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "forward"), InstitutionID: "quiet-council", ActorID: M2AgentBoID, DisputeEventID: dispute.EventID}
		if _, err := s.ForwardRPLawDispute(ctx, forward); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("another actor forwarded private filing: %v", err)
		}
		forward.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "forward")
		forward.ActorID = M2RPNPCID
		s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "forward rollback") }
		if _, err := s.ForwardRPLawDispute(ctx, forward); !core.HasCode(err, core.CodeInjectedFailure) {
			t.Fatalf("forward rollback: %v", err)
		}
		s.beforeCommit = nil
		if _, err := s.ReviewRPLawDispute(ctx, reviewRequest); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("failed forward leaked filing: %v", err)
		}
		sent, err := s.ForwardRPLawDispute(ctx, forward)
		if err != nil {
			t.Fatal(err)
		}
		if sent.Fact.DisputeForward.ReviewerID != M2AgentAdaID {
			t.Fatal("wrong handover recipient")
		}
		forwardedReceipt = sent.EventID
		duplicate := forward
		duplicate.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "duplicate-forward")
		if _, err := s.ForwardRPLawDispute(ctx, duplicate); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatalf("duplicate handover: %v", err)
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
		retry, err := s.ForwardRPLawDispute(ctx, forward)
		if err != nil || !retry.Replayed || retry.EventID != sent.EventID {
			t.Fatalf("handover retry %+v %v", retry, err)
		}
		reviewRequest.Binding = careerTestBinding(t, s, reviewerPrincipal, "successor-review")
	}
	if !conflicted && decision == "reverse" {
		attempt := func() error { _, err := s.ReviewRPLawDispute(ctx, reviewRequest); return err }
		assertLawMoneyBoundary(t, s, fine.Fact.Enforcement.TreasuryAccountID, 0, attempt)
		assertLawMoneyBoundary(t, s, fine.Fact.Enforcement.ActorAccountID, core.MaxJSONSafeInteger, attempt)
		pending, err := s.ReadOwnRPLawCases(ctx, LawCasesRequest{PrincipalID: M2RPNPCPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2RPNPCID})
		if err != nil || len(pending) != 1 || pending[0].Status != "disputed" || pending[0].RefundedMinor != 0 {
			t.Fatalf("failed refund finalized case %+v %v", pending, err)
		}
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "review rollback") }
	if _, err := s.ReviewRPLawDispute(ctx, reviewRequest); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("refund rollback: %v", err)
	}
	s.beforeCommit = nil
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-2 {
		t.Fatal("failed review leaked refund")
	}
	reviewed, err := s.ReviewRPLawDispute(ctx, reviewRequest)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Fact.Review.ReceiptEventID != forwardedReceipt {
		t.Fatal("review lost received filing provenance")
	}
	if forwardedReceipt != "" {
		if _, err := s.ForwardRPLawDispute(ctx, LawDisputeForwardRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "forward-final-case"), InstitutionID: "quiet-council", ActorID: M2RPNPCID, DisputeEventID: dispute.EventID}); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatalf("final case forwarded: %v", err)
		}
	}
	refund := int64(0)
	if decision == "reverse" {
		refund = 2
	}
	if reviewed.Fact.Review.RefundedMinor != refund || readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-2+refund {
		t.Fatal("review outcome moved incorrect funds")
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, treasuryBefore+2-refund)
	assertM2Value(t, ctx, s, `SELECT COALESCE(SUM(p.amount_minor),0) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id WHERE j.event_id=?`, []any{reviewed.EventID}, 0)
	if decision == "uphold" {
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM journal_entries WHERE event_id=?`, []any{reviewed.EventID}, 0)
	}
	duplicateReview := reviewRequest
	duplicateReview.Binding = careerTestBinding(t, s, reviewerPrincipal, "double-review")
	duplicateReview.Decision = "reverse"
	if _, err := s.ReviewRPLawDispute(ctx, duplicateReview); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("fine reversed twice: %v", err)
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
	finalRetry, err := s.ReviewRPLawDispute(ctx, reviewRequest)
	if err != nil || !finalRetry.Replayed || finalRetry.EventID != reviewed.EventID {
		t.Fatalf("review retry %+v %v", finalRetry, err)
	}
	cases = readCareerTestContext(t, s, M2RPNPCID).Life.LawCases
	if len(cases) != 1 || cases[0].Status != decision || cases[0].RefundedMinor != refund || cases[0].ReviewEventID != reviewed.EventID || readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-2+refund {
		t.Fatal("case/refund lost or repeated across recovery")
	}
}

// Deliberate projection fault injection in an isolated test DB. This verifies
// financial guards/atomic rejection, not a simulated authoritative spending
// history. Restore from the real ledger before continuing the success path.
func assertLawMoneyBoundary(t *testing.T, s *Store, account string, balance int64, attempt func() error) {
	t.Helper()
	ctx := context.Background()
	var original int64
	if err := s.db.QueryRow(`SELECT balance_minor FROM account_balances WHERE account_id=?`, account).Scan(&original); err != nil {
		t.Fatal(err)
	}
	snapshot := func() [4]int64 {
		var state [4]int64
		if err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM events),(SELECT COUNT(*) FROM journal_entries),(SELECT COUNT(*) FROM postings),(SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?)`, M2DemoInstanceID, M2DemoBranchID).Scan(&state[0], &state[1], &state[2], &state[3]); err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := snapshot()
	if _, err := s.db.Exec(`UPDATE account_balances SET balance_minor=? WHERE account_id=?`, balance, account); err != nil {
		t.Fatal(err)
	}
	if err := attempt(); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("financial boundary error: %v", err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("failed money command changed history/ledger: %v -> %v", before, after)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{account}, balance)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{account}, original)
}
