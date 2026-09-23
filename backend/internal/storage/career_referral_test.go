package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerReferralNeedsKnownCandidateAndTheirOwnApplication(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "referral.db"))
	defer s.Close()
	if _, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s)); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	if _, err := s.PostCareerPosition(ctx, posting); err != nil {
		t.Fatal(err)
	}
	referral := core.CareerReferralRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "referral"), ReferralID: "referral_ada", PositionID: posting.Posting.PositionID, ReferrerID: M2AgentBoID, CandidateID: M2AgentAdaID, SourceEventID: "invented_observation", Note: "I know Ada and recommend an interview, not automatic employment."}
	if _, err := s.ReferCareerCandidate(ctx, referral); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("unobserved candidate referral: %v", err)
	}
	// Their actual scheduled lunch encounter creates the source observation.
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT source_event_id FROM observation_records WHERE observer_agent_id=? AND subject_agent_id=? ORDER BY observed_world_time DESC LIMIT 1`, M2AgentBoID, M2AgentAdaID).Scan(&referral.SourceEventID); err != nil {
		t.Fatal(err)
	}
	referral.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "referral")
	referred, err := s.ReferCareerCandidate(ctx, referral)
	if err != nil || referred.Fact.Referral.ObservationEventID == "" {
		t.Fatalf("sourced referral: %+v %v", referred, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='application'`, nil, 0)
	application := core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "forged-application"), ApplicationID: "ada_referred_application", PositionID: posting.Posting.PositionID, CandidateID: M2AgentAdaID, Statement: "Candidate still chooses whether to apply.", ReferralID: referral.ReferralID}
	if _, err := s.ApplyForCareerPosition(ctx, application); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("referrer applied for candidate: %v", err)
	}
	wrong := application
	wrong.CandidateID, wrong.Binding = M2RPPlayerID, careerTestBinding(t, s, M2RPPlayerPrincipal, "wrong-referral")
	if _, err := s.ApplyForCareerPosition(ctx, wrong); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("another candidate stole referral: %v", err)
	}
	application.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "own-referred-application")
	applied, err := s.ApplyForCareerPosition(ctx, application)
	if err != nil || applied.Fact.Application.ReferralEventID != referred.EventID {
		t.Fatalf("candidate application: %+v %v", applied, err)
	}
	for _, principal := range []string{M2AgentAdaPrincipal, M2AgentBoPrincipal} {
		if _, err := s.ReadCareerRecruitmentRecord(ctx, principal, M2DemoInstanceID, M2DemoBranchID, "referral", referral.ReferralID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "referral", referral.ReferralID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("referral leaked: %v", err)
	}
	if got, err := s.ReferCareerCandidate(ctx, referral); err != nil || !got.Replayed || got.EventID != referred.EventID {
		t.Fatalf("referral retry: %+v %v", got, err)
	}
}
