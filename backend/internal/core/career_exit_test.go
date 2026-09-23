package core

import "testing"

func TestCareerExitInputBoundaries(t *testing.T) {
	r := CareerExitRequest{Binding: CareerBinding{PrincipalID: "employee", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "exit"}, ContractID: "job", Kind: "resignation", EffectiveFromDay: 2, Notice: "My last work day is day one."}
	for _, kind := range []string{"resignation", "termination", "layoff"} {
		r.Kind = kind
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, change := range []func(*CareerExitRequest){func(r *CareerExitRequest) { r.Kind = "retirement" }, func(r *CareerExitRequest) { r.EffectiveFromDay = 0 }, func(r *CareerExitRequest) { r.EffectiveFromDay = 36501 }, func(r *CareerExitRequest) { r.Notice = "" }, func(r *CareerExitRequest) { r.ContractID = "" }} {
		bad := r
		change(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid exit accepted")
		}
	}
}
