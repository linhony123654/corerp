package core

import "testing"

func TestCareerLeaveInputBoundaries(t *testing.T) {
	b := CareerBinding{PrincipalID: "employee", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "leave"}
	r := CareerLeaveRequest{Binding: b, LeaveID: "leave", ContractID: "contract", StartDay: 1, EndDay: 3, Reason: "Personal plans"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*CareerLeaveRequest){
		func(r *CareerLeaveRequest) { r.StartDay = -1 },
		func(r *CareerLeaveRequest) { r.EndDay = r.StartDay },
		func(r *CareerLeaveRequest) { r.EndDay = r.StartDay + 31 },
		func(r *CareerLeaveRequest) { r.Reason = "" },
	} {
		bad := r
		change(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid leave accepted")
		}
	}
	if !HasCode((CareerLeaveReviewRequest{Binding: b, LeaveID: "leave", Decision: "approve", Notice: ""}).Validate(), CodeInvalidArgument) {
		t.Fatal("approval without notice accepted")
	}
	if !HasCode((CareerLeaveReviewRequest{Binding: b, LeaveID: "leave", Decision: "erase_history", Notice: "No"}).Validate(), CodeInvalidArgument) {
		t.Fatal("invalid review action accepted")
	}
}
