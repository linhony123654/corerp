package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPWorkReadsAcceptedCareerNotPrivateEvaluations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "work-read-accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	r := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	work, err := s.ReadRPWork(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(work.Jobs) != 1 || work.Jobs[0].ContractID != accepted.Fact.Employment.ContractID || work.Jobs[0].Status != "onboarding" || work.Jobs[0].WageMinor != 12 || work.Jobs[0].PayPeriodDays != 1 {
		t.Fatalf("work read differs from accepted terms: %+v", work.Jobs)
	}
	if work.Jobs[0].WorkplaceName == "" || work.Jobs[0].EmployerName == "" || len(work.Appointments) == 0 {
		t.Fatalf("work metadata/schedule missing: %+v", work)
	}
	encoded, _ := json.Marshal(work)
	for _, forbidden := range []string{"Manager's assessment", "An advisory system recommends", "private_finances", "capabilities", "account_id", "evaluation"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private employer data leaked: %s", forbidden)
		}
	}
	if _, err := s.RunAgentLife(ctx, M2AgentMorningTime, 100); err != nil {
		t.Fatal(err)
	}
	current, err := s.ReadRPWork(ctx, r)
	if err != nil || current.Jobs[0].Status == "onboarding" {
		t.Fatalf("work view did not follow actual start: %+v %v", current, err)
	}
	for _, appointment := range current.Appointments {
		if appointment.WorldTime < current.WorldTime {
			t.Fatal("completed/past schedule presented as upcoming")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s.ReadRPWork(ctx, r)
	if err != nil || !reflect.DeepEqual(current, reopened) {
		t.Fatalf("own work view changed on reopen: %v", err)
	}
	foreign := r
	foreign.PrincipalID = "principal_creator"
	if _, err := s.ReadRPWork(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("foreign principal read work: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND subject_id=? AND capability_id='world.rp.control'`, r.PrincipalID, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPWork(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked control read work: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, current.ObservationCursor)
}

func TestRPWorkScheduleBoundAndEvidencedDelay(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "work-delay.db"))
	defer s.Close()
	read := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	initial, err := s.ReadRPWork(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Appointments) != 20 || !initial.MoreAppointments {
		t.Fatalf("unbounded or silently truncated agenda: %+v", initial)
	}
	for _, entry := range initial.Appointments {
		if entry.OriginalWorldTime != "" {
			t.Fatal("undelayed appointment labeled delayed")
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE schedule_id=? AND agent_id=?`, []any{entry.ScheduleID, M2AgentAdaID}, 1)
	}
	if _, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "work-read-roadworks"), FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_work_ada", StartsAt: "2026-09-23T07:30:00Z", EndsAt: "2026-09-23T09:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T08:30:00Z", 100); err != nil {
		t.Fatal(err)
	}
	delay := readTransitTestDelay(t, ctx, s, M2AgentAdaID)
	work, err := s.ReadRPWork(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range work.Appointments {
		if entry.ScheduleID == delay.ScheduleID {
			found = true
			if entry.WorldTime != delay.Retry.WorldTime || entry.OriginalWorldTime != delay.OriginalWorldTime {
				t.Fatalf("delay not expressed faithfully: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("pending delayed work missing from own agenda")
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, work.ObservationCursor)
	// Isolated projection damage must fail closed, not invent a new appointment.
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET world_time='2026-09-23T09:01:00Z' WHERE scheduler_item_id=?`, delay.Retry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPWork(ctx, read); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("unevidenced delay shown to player: %v", err)
	}
}
