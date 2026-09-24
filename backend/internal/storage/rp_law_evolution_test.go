package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPLawEvolutionPreservesHistoryAndIncompleteKnowledge(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "law-evolution.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "territory"), ScopeKind: "region", ScopeID: "cafe", StewardID: M2AgentBoID, PlaceIDs: []string{M2AgentCafeID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "institution"), InstitutionID: "council", ScopeKind: "region", ScopeID: "cafe", TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID, EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID}); err != nil {
		t.Fatal(err)
	}
	proposal := func(key, previous string, repeal bool, fine int64) InstitutionRecord {
		t.Helper()
		r, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, key), InstitutionID: "council", ProposerID: M2AgentAdaID, PreviousEnactmentEventID: previous, Repealed: repeal, Law: LawDefinition{LawID: "quiet", ProhibitedAction: "speak", FineMinor: fine, Text: "Versioned quiet rule."}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	enactRequest := func(key string, p InstitutionRecord, at string) LawEnactmentRequest {
		return LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, key), InstitutionID: "council", LegislatorID: M2AgentAdaID, ProposalEventID: p.EventID, EffectiveWorldTime: at}
	}
	v1Proposal := proposal("proposal1", "", false, 2)
	v1, err := s.EnactRPLaw(ctx, enactRequest("enact1", v1Proposal, "2026-09-23T13:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	unknown := LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "unknown-predecessor"), InstitutionID: "council", ProposerID: M2RPNPCID, PreviousEnactmentEventID: v1.EventID, Law: v1.Fact.Enactment.Law}
	if _, err := s.ProposeRPLaw(ctx, unknown); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unknown amendment: %v", err)
	}
	v2Proposal := proposal("proposal2", v1.EventID, false, 5)
	if _, err := s.EnactRPLaw(ctx, enactRequest("nonmonotonic", v2Proposal, "2026-09-23T13:00:00Z")); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("nonmonotonic effective time: %v", err)
	}
	v2, err := s.EnactRPLaw(ctx, enactRequest("enact2", v2Proposal, "2026-09-23T14:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	stale := proposal("stale-proposal", v1.EventID, false, 9)
	if _, err := s.EnactRPLaw(ctx, enactRequest("stale-enact", stale, "2026-09-23T16:00:00Z")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale enactment: %v", err)
	}
	repealProposal := proposal("repeal-proposal", v2.EventID, true, 0)
	repealRequest := enactRequest("repeal", repealProposal, "2026-09-23T15:00:00Z")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "repeal rollback") }
	if _, err := s.EnactRPLaw(ctx, repealRequest); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("repeal rollback: %v", err)
	}
	s.beforeCommit = nil
	repeal, err := s.EnactRPLaw(ctx, repealRequest)
	if err != nil {
		t.Fatal(err)
	}
	checkHistory := func() {
		t.Helper()
		for _, tc := range []struct {
			at, id string
			fine   int64
		}{
			{"2026-09-23T12:59:59Z", "", 0},
			{"2026-09-23T13:00:00Z", v1.EventID, 2},
			{"2026-09-23T13:59:59Z", v1.EventID, 2},
			{"2026-09-23T14:00:00Z", v2.EventID, 5},
			{"2026-09-23T15:00:00Z", "", 0},
		} {
			conn, err := s.db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			laws, err := readEffectiveInstitutionLaws(ctx, conn, M2DemoInstanceID, M2DemoBranchID, tc.at, M2AgentCafeID)
			conn.Close()
			if err != nil {
				t.Fatal(err)
			}
			if tc.id == "" {
				if len(laws) != 0 {
					t.Fatalf("unexpected law at %s: %+v", tc.at, laws)
				}
				continue
			}
			if len(laws) != 1 || laws[0].EnactmentEventID != tc.id || laws[0].FineMinor != tc.fine {
				t.Fatalf("history at %s: %+v", tc.at, laws)
			}
		}
	}
	checkHistory()
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	announce := func(key, id string) {
		t.Helper()
		if _, err := s.AnnounceRPLaw(ctx, LawAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, key), SpeakerID: M2AgentAdaID, EnactmentEventID: id}); err != nil {
			t.Fatal(err)
		}
	}
	community := func(start, at string) []rpCommunityChangeSource {
		t.Helper()
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readRPCommunityChangeSources(ctx, conn, M2DemoInstanceID, M2DemoBranchID, M2RPNPCID, start, at)
		conn.Close()
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := community(careerTime(0, 0, 0), M2AgentNoonTime); len(got) != 0 {
		t.Fatalf("unheard community changes exposed: %+v", got)
	}
	checkBelief := func(wantID string) {
		t.Helper()
		input := readCareerTestContext(t, s, M2RPNPCID)
		if input.Law == nil {
			t.Fatal("missing heard law")
		}
		laws := core.EffectiveRPLaws(input.Law.KnownLaws, "2026-09-23T15:00:00Z", M2AgentCafeID)
		if wantID == "" {
			if len(laws) != 0 {
				t.Fatalf("heard repeal did not end belief: %+v", laws)
			}
			return
		}
		if len(laws) != 1 || laws[0].EnactmentEventID != wantID {
			t.Fatalf("belief %+v, want %s", laws, wantID)
		}
	}
	announce("hear1", v1.EventID)
	checkBelief(v1.EventID) // Authoritative amendment/repeal do not reveal themselves.
	announce("hear2", v2.EventID)
	checkBelief(v2.EventID)
	var firstV2Knowledge string
	for _, law := range readCareerTestContext(t, s, M2RPNPCID).Law.KnownLaws {
		if law.EnactmentEventID == v2.EventID {
			firstV2Knowledge = law.KnowledgeEventID
		}
	}
	announce("hear-repeal", repeal.EventID)
	checkBelief("")
	announce("hear-old-again", v1.EventID)
	checkBelief("")
	known := readCareerTestContext(t, s, M2RPNPCID).Law.KnownLaws
	for _, tc := range []struct {
		at    string
		count int
	}{{"2026-09-23T14:00:00Z", 1}, {"2026-09-23T15:00:00Z", 2}} {
		view := core.BuildRPLawContext(known, tc.at, M2AgentCafeID, []string{"respond", "silence"})
		if len(view.LawfulActions) != tc.count {
			t.Fatalf("future repeal / candidate boundary %+v", view)
		}
	}
	// Advance the actual world clock and commit actual NPC speech in each era.
	read := allowFixtureControl(t, ctx, s, M2AgentBoID)
	speakAt := func(key, at string, lawful bool) string {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 100, IdempotencyKey: key + "-wait"})
		if err != nil || wait.Status != "completed" || wait.CurrentWorldTime != at {
			t.Fatalf("real time advance %+v %v", wait, err)
		}
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		spoken, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: key + "-speech"})
		if err != nil {
			t.Fatal(err)
		}
		req := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: spoken.TurnID, NPCEntityID: M2RPNPCID}
		input, err := s.BuildRPDecisionInput(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		canSpeak := false
		if input.Law == nil {
			t.Fatal("missing known version context")
		}
		for _, action := range input.Law.LawfulActions {
			if action == "respond" {
				canSpeak = true
			}
		}
		if canSpeak != lawful {
			t.Fatalf("actual-time candidates %+v", input.Law)
		}
		var provider core.RPDecisionProvider = deliberateLawBreakingProvider{}
		if lawful {
			provider = core.DeterministicRPDecisionProvider{}
		}
		choice, err := s.DecideRP(ctx, req, provider)
		if err != nil {
			t.Fatal(err)
		}
		if choice.Proposal.Action != "respond" && choice.Proposal.Action != "refuse" {
			t.Fatalf("expected actual speech, got %+v", choice)
		}
		committed, err := s.CommitRPDecision(ctx, req, choice)
		if err != nil {
			t.Fatal(err)
		}
		return committed.EventID
	}
	oldAct := speakAt("old-law", "2026-09-23T13:00:00Z", false)
	if got := community(careerTime(0, 0, 0), careerTime(1, 13, 0)); len(got) != 0 {
		t.Fatalf("future/initial rule treated as community revision: %+v", got)
	}
	newAct := speakAt("new-law", "2026-09-23T14:00:00Z", false)
	if got := community(careerTime(0, 0, 0), careerTime(1, 14, 0)); len(got) != 1 || got[0].Law.EnactmentEventID != v2.EventID || got[0].Law.KnowledgeEventID != firstV2Knowledge || got[0].AvailableSince != careerTime(1, 14, 0) {
		t.Fatalf("actual local revision source: %+v", got)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 14, 30), Budget: 1000, IdempotencyKey: "repeat-news-clock"}); err != nil {
		t.Fatal(err)
	}
	announce("repeat-change", v2.EventID)
	if got := community(careerTime(1, 14, 15), careerTime(1, 14, 30)); len(got) != 0 {
		t.Fatalf("repeat announcement refreshed expired change: %+v", got)
	}
	if got := community(careerTime(0, 0, 0), careerTime(1, 14, 30)); len(got) != 1 || got[0].Law.KnowledgeEventID != firstV2Knowledge {
		t.Fatalf("first knowledge identity changed: %+v", got)
	}
	freeAct := speakAt("repealed-law", "2026-09-23T15:00:00Z", true)
	if got := community(careerTime(0, 0, 0), careerTime(1, 15, 0)); len(got) != 1 || got[0].Law.EnactmentEventID != repeal.EventID || !got[0].Law.Repealed {
		t.Fatalf("repeal lost as real community change: %+v", got)
	}
	charge := func(key, action, version string) (InstitutionRecord, error) {
		return s.RecordRPLawViolation(ctx, LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, key), InstitutionID: "council", EnforcerID: M2AgentBoID, EnactmentEventID: version, ActionEventID: action})
	}
	for _, tc := range []struct{ key, action, version string }{
		{"retroactive-new", oldAct, v2.EventID},
		{"obsolete-old", newAct, v1.EventID},
		{"post-repeal-old", freeAct, v1.EventID},
		{"post-repeal-new", freeAct, v2.EventID},
		{"repeal-is-not-prohibition", freeAct, repeal.EventID},
	} {
		if _, err := charge(tc.key, tc.action, tc.version); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("invalid historical charge %s: %v", tc.key, err)
		}
	}
	before := readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor
	var treasuryBefore int64
	if err := s.db.QueryRow(`SELECT balance_minor FROM account_balances WHERE account_id=?`, m2EconomyEmployerCash).Scan(&treasuryBefore); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key, action, version string
		fine                 int64
	}{
		{"historical-old", oldAct, v1.EventID, 2},
		{"historical-new", newAct, v2.EventID, 5},
	} {
		violation, err := charge(tc.key, tc.action, tc.version)
		if err != nil {
			t.Fatal(err)
		}
		fine, err := s.EnforceRPLaw(ctx, LawEnforcementRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, tc.key+"-fine"), InstitutionID: "council", EnforcerID: M2AgentBoID, ViolationEventID: violation.EventID})
		if err != nil || fine.Fact.Enforcement.FineMinor != tc.fine {
			t.Fatalf("historical fine %+v %v", fine, err)
		}
		assertM2Value(t, ctx, s, `SELECT SUM(p.amount_minor) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id WHERE j.event_id=?`, []any{fine.EventID}, 0)
	}
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-7 {
		t.Fatal("historical penalties changed with repeal")
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, treasuryBefore+7)
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
	retry, err := s.EnactRPLaw(ctx, repealRequest)
	if err != nil || !retry.Replayed || retry.EventID != repeal.EventID {
		t.Fatalf("repeal retry %+v %v", retry, err)
	}
	checkHistory()
	checkBelief("")
	if got := community(careerTime(0, 0, 0), careerTime(1, 15, 0)); len(got) != 1 || got[0].Law.EnactmentEventID != repeal.EventID || got[0].AvailableSince != careerTime(1, 15, 0) {
		t.Fatalf("community source changed on recovery: %+v", got)
	}
	if readCareerTestContext(t, s, M2RPNPCID).OwnAssetMinor != before-7 {
		t.Fatal("historical penalties lost after rebuild")
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, treasuryBefore+7)
}
