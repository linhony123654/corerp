package core

import "testing"

func TestCareerPositionInputBoundaries(t *testing.T) {
	r := CareerPositionOfferRequest{Binding: CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "offer"}, ChangeID: "change", ReviewID: "review", PositionID: "position", EffectiveFromDay: 3, Assessments: []CareerQualificationAssessment{{Code: "skill", Passed: true, Reason: "Judgment"}}, Notice: "An offer, not a completed change."}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CareerPositionOfferRequest){
		func(r *CareerPositionOfferRequest) { r.EffectiveFromDay = 0 },
		func(r *CareerPositionOfferRequest) { r.EffectiveFromDay = 36501 },
		func(r *CareerPositionOfferRequest) { r.ChangeID = "" },
		func(r *CareerPositionOfferRequest) { r.Notice = "" },
		func(r *CareerPositionOfferRequest) { r.Assessments = append(r.Assessments, r.Assessments[0]) },
		func(r *CareerPositionOfferRequest) {
			r.Assessments = []CareerQualificationAssessment{{Code: "skill", Reason: ""}}
		},
	} {
		bad := r
		mutate(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid position proposal accepted")
		}
	}
	d := CareerPositionDeclineRequest{Binding: r.Binding, ChangeID: r.ChangeID, Reason: "No thanks"}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.Reason = ""
	if !HasCode(d.Validate(), CodeInvalidArgument) {
		t.Fatal("empty decline reason accepted")
	}
}
