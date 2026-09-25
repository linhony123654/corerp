package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPConditionMinorIllnessTruthSymptomsProgressionAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "condition.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f5_operator','operator','F5 local operator','active')`); err != nil {
		t.Fatal(err)
	}
	operatorBinding := func(key string) core.CareerBinding { return careerTestBinding(t, s, "principal_f5_operator", key) }
	onsetRequest := RPConditionOnsetRequest{Binding: operatorBinding("illness-onset"),
		ConditionKey: "ada-illness-one", EntityID: M2AgentAdaID, Kind: "minor_illness", Severity: 2}
	bad := onsetRequest
	bad.Binding.PrincipalID = M2AgentAdaPrincipal
	if _, err := s.StartRPConditionLocal(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("subject forged world condition truth", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "condition rollback") }
	if _, err := s.StartRPConditionLocal(ctx, onsetRequest); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("condition onset did not roll back", err)
	}
	s.beforeCommit = nil
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPConditionStarted'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rolled-back condition remains", count, err)
	}
	onset, err := s.StartRPConditionLocal(ctx, onsetRequest)
	if err != nil || onset.Fact.Kind != "minor_illness" || onset.Fact.Status != "active" ||
		onset.Fact.OnsetEventID != onset.EventID || onset.Fact.Severity != 2 {
		t.Fatal("sourced condition onset", onset, err)
	}
	if replay, err := s.StartRPConditionLocal(ctx, onsetRequest); err != nil || !replay.Replayed || replay.EventID != onset.EventID {
		t.Fatal("condition onset exact retry", replay, err)
	}
	if _, err := s.StartRPConditionLocal(ctx, RPConditionOnsetRequest{Binding: operatorBinding("same-kind"),
		ConditionKey: "ada-illness-two", EntityID: M2AgentAdaID, Kind: "minor_illness", Severity: 1}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("parallel same-kind condition accepted", err)
	}
	self := readCareerTestContext(t, s, M2AgentAdaID)
	if self.Life.Health == nil || self.Life.Health.ConditionImpact != "consider_rest_or_leave" ||
		len(self.Life.Health.Symptoms) != 1 || self.Life.Health.Symptoms[0] != "malaise" {
		t.Fatal("subject lacks bounded symptoms", self.Life.Health)
	}
	encoded, _ := json.Marshal(self)
	for _, private := range []string{onset.EventID, onset.Fact.ConditionID, "minor_illness", "severity", "onset_event_id"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("condition truth escaped actor model context", private)
		}
	}
	other := readCareerTestContext(t, s, M2AgentBoID)
	if other.Life.Health != nil {
		t.Fatal("coworker learned private condition without observation", other.Life.Health)
	}
	player, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID,
		BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "condition-clock"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: player.SessionID}
	wait := func(key, target string) {
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID,
			TargetWorldTime: target, Budget: 1000, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key})
		if err != nil || result.Status != "completed" || result.CurrentWorldTime != target {
			t.Fatal("condition clock advance", result, err)
		}
	}
	wait("condition-recovery-time", "2026-09-22T07:30:00Z")
	progress := RPConditionChangeRequest{Binding: operatorBinding("illness-recovering"),
		ConditionID: onset.Fact.ConditionID, Status: "recovering", Severity: 1, Reason: "Symptoms are easing"}
	if _, err := s.ChangeRPConditionLocal(ctx, RPConditionChangeRequest{Binding: operatorBinding("invalid-treatment"),
		ConditionID: onset.Fact.ConditionID, Status: "recovering", Severity: 1, Reason: "Treatment", TreatmentEventID: onset.EventID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("non-treatment source accepted as treatment", err)
	}
	changed, err := s.ChangeRPConditionLocal(ctx, progress)
	if err != nil || changed.Fact.PreviousEventID != onset.EventID || changed.Fact.OnsetEventID != onset.EventID ||
		changed.Fact.Severity != 1 || changed.Fact.Status != "recovering" {
		t.Fatal("condition progression source", changed, err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID); got.Life.Health == nil || got.Life.Health.ConditionImpact != "consider_rest_or_leave" {
		t.Fatal("recovering condition vanished prematurely", got.Life.Health)
	}
	wait("condition-resolved-time", "2026-09-22T08:30:00Z")
	resolve := RPConditionChangeRequest{Binding: operatorBinding("illness-resolve"),
		ConditionID: onset.Fact.ConditionID, Status: "resolved", Severity: 0, Reason: "Symptoms resolved"}
	resolved, err := s.ChangeRPConditionLocal(ctx, resolve)
	if err != nil || resolved.Fact.PreviousEventID != changed.EventID || resolved.Fact.Severity != 0 {
		t.Fatal("condition recovery source", resolved, err)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID); got.Life.Health != nil {
		t.Fatal("resolved condition still appears as own symptom", got.Life.Health)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := s.ChangeRPConditionLocal(ctx, resolve); err != nil || !retry.Replayed || retry.EventID != resolved.EventID {
		t.Fatal("recovery exact retry after reopen", retry, err)
	}
	if _, err := s.ChangeRPConditionLocal(ctx, RPConditionChangeRequest{Binding: operatorBinding("change-resolved"),
		ConditionID: onset.Fact.ConditionID, Status: "active", Severity: 1, Reason: "Invalid revival"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("resolved condition was revived without new onset", err)
	}
	var original string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, resolved.EventID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.previous_event_id','event_missing') WHERE event_id=?`, resolved.EventID); err != nil {
		t.Fatal(err)
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{NPCEntityID: M2AgentAdaID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID})
	tx.Rollback(ctx)
	if !core.HasCode(readErr, core.CodeProjectionDiverged) {
		t.Fatal("broken condition source lineage was trusted", readErr)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=? WHERE event_id=?`, original, resolved.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("condition source changed unrelated projections", diff, err)
	}
}
