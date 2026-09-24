package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPLawCareerAnnouncementHasRealConsequences(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "law-career.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	job, _ := prepareCareerPositionOffer(t, s, "senior")
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "world"), ScopeKind: "world", ScopeID: M2DemoInstanceID, StewardID: M2AgentBoID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "council"), InstitutionID: "council", ScopeKind: "world", ScopeID: M2DemoInstanceID, TreasuryOrganizationID: job.Fact.OrganizationID, LegislatorID: M2AgentAdaID, EnforcerID: M2AgentAdaID, ReviewerID: M2RPNPCID}); err != nil {
		t.Fatal(err)
	}
	p, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "proposal"), InstitutionID: "council", ProposerID: M2AgentAdaID, Law: LawDefinition{LawID: "quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Quiet applies to announcements too."}})
	if err != nil {
		t.Fatal(err)
	}
	law, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "enact"), InstitutionID: "council", LegislatorID: M2AgentAdaID, ProposalEventID: p.EventID, EffectiveWorldTime: careerTime(3, 12, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	before := readCareerTestContext(t, s, M2AgentBoID).OwnAssetMinor
	news, err := s.SpeakCareerAnnouncement(ctx, core.CareerAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "announcement"), AnnouncementID: "real-job-news", ContractID: job.Fact.Employment.ContractID, SpeakerID: M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	heard := false
	for _, id := range news.Fact.Announcement.ListenerIDs {
		if id == M2AgentAdaID {
			heard = true
		}
	}
	if !heard {
		t.Fatal("test requires actual enforcer hearing")
	}
	if readCareerTestContext(t, s, M2AgentBoID).OwnAssetMinor != before {
		t.Fatal("announcement automatically punished")
	}
	v, err := s.RecordRPLawViolation(ctx, LawViolationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "violation"), InstitutionID: "council", EnforcerID: M2AgentAdaID, EnactmentEventID: law.EventID, ActionEventID: news.EventID})
	if err != nil {
		t.Fatal(err)
	}
	if v.Fact.Violation.ActorID != M2AgentBoID || v.Fact.Violation.ActionWorldTime != careerTime(3, 12, 0) || v.Fact.Violation.PlaceID != news.Fact.Announcement.PlaceID {
		t.Fatalf("wrong career speech attribution %+v", v)
	}
	r := LawEnforcementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "fine"), InstitutionID: "council", EnforcerID: M2AgentAdaID, ViolationEventID: v.EventID}
	fine, err := s.EnforceRPLaw(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if readCareerTestContext(t, s, M2AgentBoID).OwnAssetMinor != before-2 {
		t.Fatal("career speech fine not collected")
	}
	assertM2Value(t, ctx, s, `SELECT SUM(p.amount_minor) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id WHERE j.event_id=?`, []any{fine.EventID}, 0)
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
	retry, err := s.EnforceRPLaw(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != fine.EventID {
		t.Fatalf("career fine retry %+v %v", retry, err)
	}
	if readCareerTestContext(t, s, M2AgentBoID).OwnAssetMinor != before-2 {
		t.Fatal("career fine repeated or lost")
	}
}
