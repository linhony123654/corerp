package core

import "testing"

func TestCareerPerformanceInputBoundaries(t *testing.T) {
	b := CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "review"}
	r := CareerPerformanceRequest{Binding: b, ReviewID: "review", ContractID: "contract", EvidenceEventIDs: []string{"actual-work"}, Assessment: "meets_expectations", Reason: "Organization judgment"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*CareerPerformanceRequest){
		func(r *CareerPerformanceRequest) { r.EvidenceEventIDs = nil },
		func(r *CareerPerformanceRequest) { r.EvidenceEventIDs = []string{"same", "same"} },
		func(r *CareerPerformanceRequest) { r.Assessment = "promoted" },
		func(r *CareerPerformanceRequest) { r.Reason = " " },
		func(r *CareerPerformanceRequest) { r.AdvisoryNote = "bad\x00note" },
	} {
		bad := r
		change(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid performance request accepted")
		}
	}
	regularize := CareerRegularizationRequest{Binding: b, ReviewID: "review", Notice: "Your probation is complete."}
	if err := regularize.Validate(); err != nil {
		t.Fatal(err)
	}
	regularize.Notice = ""
	if !HasCode(regularize.Validate(), CodeInvalidArgument) {
		t.Fatal("regularization without addressed notice accepted")
	}
}
