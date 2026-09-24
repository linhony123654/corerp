package core

import "testing"

func TestRPWaitOpportunityIntentValidationAndLegacyHash(t *testing.T) {
	r := RPWaitRequest{PrincipalID: "player", SessionID: "session", TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 100, ExpectedCursor: 12, IdempotencyKey: "wait"}
	legacy := struct {
		PrincipalID     string `json:"principal_id"`
		SessionID       string `json:"session_id"`
		TargetWorldTime string `json:"target_world_time"`
		Budget          int    `json:"budget"`
		ExpectedCursor  int64  `json:"expected_cursor"`
		IdempotencyKey  string `json:"idempotency_key"`
	}{r.PrincipalID, r.SessionID, r.TargetWorldTime, r.Budget, r.ExpectedCursor, r.IdempotencyKey}
	before, err := HashJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	after, err := HashJSON(r)
	if err != nil || before != after {
		t.Fatalf("legacy wait hash changed: %s %s %v", before, after, err)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.OpportunityIntent = "social"
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	seeking, err := HashJSON(r)
	if err != nil || seeking == after {
		t.Fatal("explicit intent not pinned in request hash")
	}
	for _, intent := range []string{"explore", "force_drama", " social", "SOCIAL"} {
		r.OpportunityIntent = intent
		if !HasCode(r.Validate(), CodeInvalidArgument) {
			t.Fatalf("accepted unsupported intent %q", intent)
		}
	}
}
