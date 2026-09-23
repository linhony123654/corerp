package core

import "testing"

func TestCareerAggregateExitNoticeValidation(t *testing.T) {
	r := CareerAggregateExitRequest{Binding: CareerBinding{PrincipalID: "worker", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "notice"}, CandidateID: "worker", ContractID: "aggregate", FinalEarnedDay: 2, Notice: "Leaving after this period."}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*CareerAggregateExitRequest){func(r *CareerAggregateExitRequest) { r.FinalEarnedDay = 0 }, func(r *CareerAggregateExitRequest) { r.FinalEarnedDay = 31 }, func(r *CareerAggregateExitRequest) { r.CandidateID = "" }, func(r *CareerAggregateExitRequest) { r.Notice = "" }, func(r *CareerAggregateExitRequest) { r.Binding.ExpectedHead = 0 }} {
		bad := r
		change(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid notice accepted")
		}
	}
}
