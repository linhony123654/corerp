package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func readTransitTestDelay(t *testing.T, ctx context.Context, s *Store, actor string) rpTransitDelay {
	t.Helper()
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='AgentTravelDelayed' AND json_extract(payload,'$.agent_id')=? ORDER BY event_sequence DESC LIMIT 1`, actor).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var d rpTransitDelay
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRPTransitRepeatedDelayPreservesChainAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "transit-chain.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "first-edge"), FromPlaceID: "place_m2_home_ada", ToPlaceID: M2AgentCafeID, StartsAt: "2026-09-23T07:30:00Z", EndsAt: "2026-09-23T09:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T08:15:00Z", 100); err != nil {
		t.Fatal(err)
	}
	first := readTransitTestDelay(t, ctx, s, M2AgentAdaID)
	if first.Retry.WorldTime != "2026-09-23T09:00:00Z" {
		t.Fatalf("first delay: %+v", first)
	}
	secondWorks, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "second-edge"), FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_work_ada", StartsAt: "2026-09-23T08:30:00Z", EndsAt: "2026-09-23T10:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T09:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	second := readTransitTestDelay(t, ctx, s, M2AgentAdaID)
	if second.Previous.ID != first.Retry.ID || second.Retry.ID == first.Retry.ID || second.OriginalWorldTime != first.OriginalWorldTime || second.ScheduleID != first.ScheduleID || second.Retry.WorldTime != "2026-09-23T10:00:00Z" || len(second.DelaySourceEventIDs) != 1 || second.DelaySourceEventIDs[0] != secondWorks.EventID {
		t.Fatalf("delay chain: %+v -> %+v", first, second)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE schedule_id=?`, []any{first.ScheduleID}, 0)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("chain projection: %+v %v", differences, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET status='pending' WHERE scheduler_item_id=?`, first.Retry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_schedule_entries SET scheduler_item_id=? WHERE schedule_id=?`, first.Retry.ID, first.ScheduleID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id=? AND status='completed'`, []any{first.Retry.ID}, 1)
	if _, err := s.RunAgentLife(ctx, "2026-09-23T10:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE schedule_id=? AND world_time='2026-09-23T10:00:00Z'`, []any{first.ScheduleID}, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("chain arrival: %+v %v", differences, err)
	}
}

func TestRPTransitMidnightDelayRetainsOriginalAppointmentAndAdvancesDay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "transit-midnight.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	const actor = "entity_transit_night"
	binding := careerTestBinding(t, s, "principal_creator", "night-materialize")
	materialized, err := s.MaterializeCohort(ctx, m2AgentMaterialization("transit_night", actor, "Night visitor", 1, 150, 0, 0, 30, binding.ExpectedHead, rpLifeSetupTime))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.MaterializeRPBackground(ctx, core.RPBackgroundRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: actor, ExpectedHead: materialized.LastSequence, IdempotencyKey: "night-routine", AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID, Schedule: []core.RPBackgroundSchedule{{WorldTime: "2026-09-22T23:50:00Z", PlaceID: "place_m2_home_bo", ActivityCode: "home"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "midnight-works"), FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_bo", StartsAt: "2026-09-22T23:45:00Z", EndsAt: "2026-09-23T00:30:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-22T23:59:00Z", 100); err != nil {
		t.Fatal(err)
	}
	delay := readTransitTestDelay(t, ctx, s, actor)
	var payload agentSchedulePayload
	if err := json.Unmarshal([]byte(delay.Retry.Payload), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Day != 1 || delay.OriginalWorldTime != "2026-09-22T23:50:00Z" || delay.Retry.WorldTime != "2026-09-23T00:30:00Z" {
		t.Fatalf("wrong midnight calendar: %+v %+v", delay, payload)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{actor, M2AgentCafeID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T00:30:00Z", 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT current_day FROM world_clocks WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND schedule_id=? AND world_time='2026-09-23T00:30:00Z'`, []any{actor, delay.ScheduleID}, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("midnight recovery: %+v %v", differences, err)
	}
}

func TestRPTransitDoesNotExtendCareerDeadlinesOrRewriteEarnedPay(t *testing.T) {
	for _, kind := range []string{"leave", "resignation"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "transit-career.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			offer := prepareCareerEmploymentOffer(t, s)
			accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
			if err != nil {
				t.Fatal(err)
			}
			job := accepted.Fact.Employment
			if _, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "career-works"), FromPlaceID: M2AgentCafeID, ToPlaceID: job.WorkplaceID, StartsAt: careerTime(1, 7, 30), EndsAt: careerTime(1, 9, 0)}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
				t.Fatal(err)
			}
			delay := readTransitTestDelay(t, ctx, s, job.EmployeeID)
			late := core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "late-leave"), LeaveID: "late-leave", ContractID: job.ContractID, StartDay: 1, EndDay: 2, Reason: "Already past original shift start"}
			if _, err := s.RequestCareerLeave(ctx, late); !core.HasCode(err, core.CodeBranchConflict) {
				t.Fatalf("delay moved original leave deadline: %v", err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id=? AND status='pending'`, []any{delay.Retry.ID}, 1)
			if kind == "leave" {
				future := late
				future.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "future-leave")
				future.LeaveID = "future-leave"
				future.StartDay = 2
				future.EndDay = 3
				if _, err := s.RequestCareerLeave(ctx, future); err != nil {
					t.Fatal(err)
				}
				if _, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "approve-future-leave"), LeaveID: future.LeaveID, Decision: "approve", Notice: "Future leave approved"}); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "exit"), ContractID: job.ContractID, Kind: "resignation", EffectiveFromDay: 2, Notice: "Prior earned wages remain due"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(2, 12, 0), 200); err != nil {
				t.Fatal(err)
			}
			attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1)
			if err != nil || attendance.Attendance.RecordedSeconds != int64(job.WorkEndHour-job.WorkStartHour-1)*3600 || attendance.BaseEarnedMinor != job.DailyWageMinor {
				t.Fatalf("delay fabricated attendance or new pay penalty: %+v %v", attendance, err)
			}
			assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, job.DailyWageMinor)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND activity_code='work' AND world_time>=? AND world_time<?`, []any{job.EmployeeID, careerTime(2, 0, 0), careerTime(3, 0, 0)}, 0)
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("career transit recovery: %+v %v", differences, err)
			}
		})
	}
}
