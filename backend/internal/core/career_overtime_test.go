package core

import "testing"

func TestCareerOvertimeInputBoundaries(t *testing.T) {
	b := CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "overtime"}
	r := CareerOvertimeRequest{Binding: b, OvertimeID: "ot", ContractID: "job", Day: 1, StartHour: 13, EndHour: 15, RateMinorPerHour: 7, Reason: "Additional work"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*CareerOvertimeRequest){
		func(r *CareerOvertimeRequest) { r.Day = -1 },
		func(r *CareerOvertimeRequest) { r.StartHour = -1 },
		func(r *CareerOvertimeRequest) { r.EndHour = 24 },
		func(r *CareerOvertimeRequest) { r.EndHour = 18 },
		func(r *CareerOvertimeRequest) { r.RateMinorPerHour = 0 },
		func(r *CareerOvertimeRequest) { r.RateMinorPerHour = MaxJSONSafeInteger },
	} {
		bad := r
		change(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid overtime terms accepted")
		}
	}
	for _, decision := range []string{"accept", "decline", "cancel"} {
		if err := (CareerOvertimeResponseRequest{Binding: b, OvertimeID: "ot", Decision: decision, Reason: "Explicit response"}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if !HasCode((CareerOvertimeResponseRequest{Binding: b, OvertimeID: "ot", Decision: "force", Reason: "No"}).Validate(), CodeInvalidArgument) {
		t.Fatal("forced overtime response accepted")
	}
}
