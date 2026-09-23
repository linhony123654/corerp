package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareCareerPositionOffer(t *testing.T, s *Store, grade string) (CareerRecord, core.CareerPositionOfferRequest) {
	t.Helper()
	ctx := context.Background()
	accepted, performance := prepareCareerPerformance(t, s)
	if _, err := s.RecordCareerPerformance(ctx, performance); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineCareerGradeScale(ctx, core.CareerGradeScaleRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "scale"), Scale: core.CareerGradeScale{OrganizationID: accepted.Fact.OrganizationID, Grades: []string{"trainee", "junior", "senior"}}}); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	posting.Binding.IdempotencyKey = "target-posting"
	posting.Posting.PositionID, posting.Posting.Grade = "target-position", grade
	posting.Posting.DailyWageMinor = 20
	posting.Posting.RequiredQualifications = []string{"team_coordination"}
	if _, err := s.PostCareerPosition(ctx, posting); err != nil {
		t.Fatal(err)
	}
	return accepted, core.CareerPositionOfferRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "position-offer"), ChangeID: "change-ada", ReviewID: performance.ReviewID, PositionID: posting.Posting.PositionID, EffectiveFromDay: 3, Assessments: []core.CareerQualificationAssessment{{Code: "team_coordination", Passed: true, Reason: "Private manager qualification judgment based on completed-work review"}}, Notice: "Proposed new role from day three, subject to your acceptance."}
}

func TestCareerPositionOfferEvidencePrivacyDeclineRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "position.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	accepted, r := prepareCareerPositionOffer(t, s, "senior")
	before := readCareerTestContext(t, s, M2AgentAdaID)
	for _, change := range []func(*core.CareerPositionOfferRequest){
		func(r *core.CareerPositionOfferRequest) { r.EffectiveFromDay = 2 },
		func(r *core.CareerPositionOfferRequest) { r.EffectiveFromDay = 33 },
		func(r *core.CareerPositionOfferRequest) { r.PositionID = accepted.Fact.Employment.PositionID },
		func(r *core.CareerPositionOfferRequest) { r.Assessments = nil },
		func(r *core.CareerPositionOfferRequest) {
			r.Assessments = []core.CareerQualificationAssessment{{Code: "wrong", Passed: true, Reason: "wrong requirement"}}
		},
		func(r *core.CareerPositionOfferRequest) {
			r.Assessments = []core.CareerQualificationAssessment{{Code: "team_coordination", Passed: false, Reason: "not passed"}}
		},
	} {
		bad := r
		change(&bad)
		if _, err := s.OfferCareerPositionChange(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("invalid offer accepted: %v", err)
		}
	}
	bad := r
	bad.Binding.PrincipalID = M2AgentAdaPrincipal
	if _, err := s.OfferCareerPositionChange(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("self-promotion: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "position offer rollback") }
	if _, err := s.OfferCareerPositionChange(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE json_extract(payload,'$.kind')='position_change'`, nil, 0)
	offer, err := s.OfferCareerPositionChange(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if offer.Fact.PositionChange.Kind != "promotion" || offer.Fact.PositionAssessment.PerformanceEventID == "" || offer.Fact.Employment != nil {
		t.Fatalf("proposal lacks distinction/evidence: %+v", offer)
	}
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{accepted.Fact.Employment.ContractID}, 12)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE json_extract(payload,'$.kind')='career_terms_effective'`, nil, 0)
	market, err := s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range market.Postings {
		if p.Posting.PositionID == r.PositionID && p.AvailableSlots != 1 {
			t.Fatal("unaccepted proposal reserved target")
		}
		if p.Posting.PositionID == accepted.Fact.Employment.PositionID && p.AvailableSlots != 0 {
			t.Fatal("proposal freed current position")
		}
	}
	after := readCareerTestContext(t, s, M2AgentAdaID)
	if after.OwnAssetMinor != before.OwnAssetMinor || after.Life.Employment[0].WageMinor != 12 {
		t.Fatal("proposal changed actual finances")
	}
	for _, principal := range []string{M2AgentAdaPrincipal, M2AgentBoPrincipal} {
		read, err := s.ReadCareerRecruitmentRecord(ctx, principal, r.Binding.InstanceID, r.Binding.BranchID, "position_change", r.ChangeID)
		if err != nil {
			t.Fatal(err)
		}
		if (read.Fact.PositionAssessment != nil) != (principal == M2AgentBoPrincipal) {
			t.Fatal("assessment privacy differs")
		}
		if principal == M2AgentAdaPrincipal {
			raw, _ := json.Marshal(read)
			if strings.Contains(string(raw), r.Assessments[0].Reason) || strings.Contains(string(raw), "performance_event_id") {
				t.Fatal("private evidence leaked")
			}
		}
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, r.Binding.InstanceID, r.Binding.BranchID, "position_change", r.ChangeID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unrelated reader: %v", err)
	}
	d := core.CareerPositionDeclineRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "decline-change"), ChangeID: r.ChangeID, Reason: "I prefer my current role."}
	wrong := d
	wrong.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.DeclineCareerPositionChange(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager impersonated employee: %v", err)
	}
	declined, err := s.DeclineCareerPositionChange(ctx, d)
	if err != nil || declined.Fact.PositionAssessment != nil || declined.Fact.PositionChange.Status != "declined" || declined.Fact.PositionChange.OfferEventID != offer.EventID {
		t.Fatalf("decline: %+v %v", declined, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, retry := range []func() (CareerRecord, error){func() (CareerRecord, error) { return s.OfferCareerPositionChange(ctx, r) }, func() (CareerRecord, error) { return s.DeclineCareerPositionChange(ctx, d) }} {
		got, err := retry()
		if err != nil || !got.Replayed {
			t.Fatalf("retry: %+v %v", got, err)
		}
		if got.EventID == declined.EventID && got.Fact.PositionAssessment != nil {
			t.Fatal("replayed employee result leaked assessment")
		}
	}
	d.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "decline-again")
	if _, err := s.DeclineCareerPositionChange(ctx, d); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("closed proposal changed: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.event_type='RPCareerFactRecorded'`, nil, 0)
}

func TestCareerPositionOfferDirectionsAndStaleReview(t *testing.T) {
	for _, tc := range []struct{ grade, kind string }{{"junior", "transfer"}, {"trainee", "demotion"}} {
		t.Run(tc.kind, func(t *testing.T) {
			s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "direction.db"))
			defer s.Close()
			_, r := prepareCareerPositionOffer(t, s, tc.grade)
			got, err := s.OfferCareerPositionChange(context.Background(), r)
			if err != nil || got.Fact.PositionChange.Kind != tc.kind {
				t.Fatalf("direction: %+v %v", got, err)
			}
		})
	}
	t.Run("superseded-unfavorable-review", func(t *testing.T) {
		ctx := context.Background()
		s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "review.db"))
		defer s.Close()
		accepted, r := prepareCareerPositionOffer(t, s, "senior")
		report, err := s.ReadCareerAttendance(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, accepted.Fact.Employment.ContractID, 1)
		if err != nil {
			t.Fatal(err)
		}
		newReview := core.CareerPerformanceRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "new-review"), ReviewID: "new-review", ContractID: accepted.Fact.Employment.ContractID, EvidenceEventIDs: []string{report.EventID}, Assessment: "needs_improvement", Reason: "Reconsidered work evidence."}
		if _, err := s.RecordCareerPerformance(ctx, newReview); err != nil {
			t.Fatal(err)
		}
		r.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "position-offer")
		if _, err := s.OfferCareerPositionChange(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatalf("stale review accepted: %v", err)
		}
		r.ReviewID = newReview.ReviewID
		if _, err := s.OfferCareerPositionChange(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatalf("unfavorable promotion accepted: %v", err)
		}
	})
}
