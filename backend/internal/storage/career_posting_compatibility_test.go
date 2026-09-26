package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerPostingTermsCompatibility(t *testing.T) {
	base := core.CareerPostingDefinition{PositionID: "position", OrganizationID: "org", Title: "Assistant", OccupationID: "operations", Grade: "junior", Capacity: 1, DailyWageMinor: 12}
	for _, tc := range []struct {
		name       string
		edit       func(*core.CareerPostingDefinition)
		compatible bool
	}{
		{"capacity", func(p *core.CareerPostingDefinition) { p.Capacity = 2 }, true},
		{"status", func(p *core.CareerPostingDefinition) { p.Status = "frozen" }, true},
		{"wage", func(p *core.CareerPostingDefinition) { p.DailyWageMinor++ }, false},
		{"position", func(p *core.CareerPostingDefinition) { p.PositionID = "other" }, false},
		{"organization", func(p *core.CareerPostingDefinition) { p.OrganizationID = "other" }, false},
		{"title", func(p *core.CareerPostingDefinition) { p.Title = "Manager" }, false},
		{"occupation", func(p *core.CareerPostingDefinition) { p.OccupationID = "other" }, false},
		{"grade", func(p *core.CareerPostingDefinition) { p.Grade = "senior" }, false},
		{"qualification", func(p *core.CareerPostingDefinition) { p.RequiredQualifications = []string{"training"} }, false},
		{"credential", func(p *core.CareerPostingDefinition) {
			p.RequiredCredentials = []core.CredentialRequirement{{Code: "license", IssuerID: "issuer"}}
		}, false},
		{"capability", func(p *core.CareerPostingDefinition) { p.Capabilities = []string{"manage"} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			tc.edit(&changed)
			if got := careerPostingTermsEqual(base, changed); got != tc.compatible {
				t.Fatalf("compatibility=%v, want %v", got, tc.compatible)
			}
		})
	}
}

func TestOrganizationAgencyReviewPreservesHiringEvidence(t *testing.T) {
	for _, stage := range []string{"application", "offer"} {
		for _, freeze := range []bool{false, true} {
			name := stage + "_expand"
			if freeze {
				name = stage + "_freeze"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "hiring.db"))
				defer s.Close()
				app := prepareCareerApplicant(t, s)
				if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "invite"), InterviewID: "interview_ada", ApplicationID: app.Fact.RecordID, Question: "Explain safe opening checks."}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "answer"), InterviewID: "interview_ada", Answer: "Inspect equipment and exits."}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.EvaluateCareerApplication(ctx, careerTestEvaluation(t, s, "evaluation", true)); err != nil {
					t.Fatal(err)
				}
				if stage == "offer" {
					if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "offer", "evaluation")); err != nil {
						t.Fatal(err)
					}
				}
				policy := core.OrganizationAgencyPolicy{PolicyID: "policy_hiring", OrganizationID: "actor_m2_coop_employer", ManagerPrincipalID: M2AgentBoPrincipal, ReviewFrequencyHours: 1, HiringThresholdMinor: 10, TargetPositionID: "position_coop_assistant", DefaultCapacity: 2, Status: "active"}
				if freeze {
					policy.ReserveTargetMinor = 1300
				}
				if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "policy"), Policy: policy}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "review"), OrganizationID: policy.OrganizationID, PolicyID: policy.PolicyID}); err != nil {
					t.Fatal(err)
				}
				var err error
				if stage == "application" {
					_, err = s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "offer", "evaluation"))
				} else {
					_, err = s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: "offer", AfterWorkPlaceID: M2AgentCafeID})
				}
				if freeze {
					if !core.HasCode(err, core.CodeBranchConflict) {
						t.Fatalf("frozen recruitment allowed: %v", err)
					}
				} else if err != nil {
					t.Fatalf("capacity review invalidated hiring evidence: %v", err)
				}
				if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
					t.Fatalf("projection differences: %+v %v", differences, err)
				}
			})
		}
	}
}
