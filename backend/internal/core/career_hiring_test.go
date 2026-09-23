package core

import "testing"

func TestCareerHiringInputBoundaries(t *testing.T) {
	b := CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "key"}
	evaluation := CareerEvaluationRequest{Binding: b, EvaluationID: "evaluation", InterviewID: "interview", Decision: "advance", Reason: "Reviewed the answer", Assessments: []CareerQualificationAssessment{{Code: "safety", Passed: true, Reason: "Demonstrates the checks"}}}
	if err := evaluation.Validate(); err != nil {
		t.Fatal(err)
	}
	duplicate := evaluation
	duplicate.Assessments = []CareerQualificationAssessment{evaluation.Assessments[0], evaluation.Assessments[0]}
	if !HasCode(duplicate.Validate(), CodeInvalidArgument) {
		t.Fatal("duplicate assessments accepted")
	}
	bad := evaluation
	bad.Decision = "hired"
	if !HasCode(bad.Validate(), CodeInvalidArgument) {
		t.Fatal("evaluation directly hired candidate")
	}
	if !HasCode((CareerInterviewAnswerRequest{Binding: b, InterviewID: "interview", Answer: " "}).Validate(), CodeInvalidArgument) {
		t.Fatal("empty answer accepted")
	}
	offer := CareerOfferRequest{Binding: b, OfferID: "offer", EvaluationID: "evaluation", StartsOnDay: 1, ProbationDays: 7, ExpiresAt: "2026-09-23T07:59:00+08:00"}
	if err := offer.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*CareerOfferRequest){
		func(r *CareerOfferRequest) { r.StartsOnDay = 0 },
		func(r *CareerOfferRequest) { r.ProbationDays = 91 },
		func(r *CareerOfferRequest) { r.ExpiresAt = "never" },
	} {
		invalid := offer
		mutation(&invalid)
		if !HasCode(invalid.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid offer boundary accepted")
		}
	}
	referral := CareerReferralRequest{Binding: b, ReferralID: "referral", PositionID: "position", ReferrerID: "same", CandidateID: "same", SourceEventID: "source", Note: "Recommend myself"}
	if !HasCode(referral.Validate(), CodeInvalidArgument) {
		t.Fatal("self referral accepted")
	}
}
