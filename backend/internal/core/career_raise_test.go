package core

import "testing"

func TestCareerRaiseInputBoundaries(t *testing.T) {
	r := CareerRaiseRequest{Binding: CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "raise"}, ContractID: "job", DailyWageMinor: 20, EffectiveFromDay: 2, Notice: "Higher base wage from day two."}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*CareerRaiseRequest){
		func(r *CareerRaiseRequest) { r.DailyWageMinor = 0 },
		func(r *CareerRaiseRequest) { r.DailyWageMinor = MaxJSONSafeInteger + 1 },
		func(r *CareerRaiseRequest) { r.EffectiveFromDay = 0 },
		func(r *CareerRaiseRequest) { r.Notice = "" },
	} {
		bad := r
		change(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid raise accepted")
		}
	}
}
