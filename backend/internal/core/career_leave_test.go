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

func TestCareerLeaveConsiderPolicyBoundaries(t *testing.T) {
	for _, delay := range []int{-1, 1441} {
		if !HasCode((CareerLeaveReviewPolicy{MaxConcurrentEmployees: 1, AutoReviewDelayMinutes: delay}).Validate(), CodeInvalidArgument) {
			t.Fatalf("invalid automatic review delay %d", delay)
		}
	}
	for _, limit := range []int{-1, 0, 101} {
		if !HasCode((CareerLeaveReviewPolicy{MaxConcurrentEmployees: limit}).Validate(), CodeInvalidArgument) {
			t.Fatalf("invalid capacity %d", limit)
		}
	}
	for _, limit := range []int{1, 100} {
		if err := (CareerLeaveReviewPolicy{MaxConcurrentEmployees: limit}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	r := CareerLeaveReviewRequest{Binding: CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "consider"}, LeaveID: "leave", Decision: "consider", Notice: "Policy review"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCareerOrganizationWithoutLeavePolicyRetainsHash(t *testing.T) {
	legacy := struct {
		OrganizationID     string `json:"organization_id"`
		DisplayName        string `json:"display_name"`
		ManagerPrincipalID string `json:"manager_principal_id"`
		WorkplaceID        string `json:"workplace_id"`
	}{"org", "Co-op", "manager", "work"}
	current := CareerOrganizationDefinition{OrganizationID: "org", DisplayName: "Co-op", ManagerPrincipalID: "manager", WorkplaceID: "work"}
	before, err := HashJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	after, err := HashJSON(current)
	if err != nil || before != after {
		t.Fatalf("default-off policy changed old hash: %s %s %v", before, after, err)
	}
	current.LeaveReviewPolicy = &CareerLeaveReviewPolicy{MaxConcurrentEmployees: 1}
	enabled, err := HashJSON(current)
	if err != nil || enabled == before {
		t.Fatalf("policy not bound to source hash: %v", err)
	}
}
