package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPRequestRetirementScopeAndImmutability(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, submit := requestRetirementFixture(t, ctx, s, "social")
	foreign := r
	foreign.PrincipalID = "principal_creator"
	if _, err := s.RetireRPRequest(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("foreign scope: %v", err)
	}
	for _, change := range []func(*RPRequestRetireRequest){
		func(r *RPRequestRetireRequest) { r.Operation = "unsupported" },
		func(r *RPRequestRetireRequest) { r.SessionID = "" },
		func(r *RPRequestRetireRequest) { r.IdempotencyKey = " " },
		func(r *RPRequestRetireRequest) { r.Operation = "open" },
	} {
		bad := r
		change(&bad)
		if _, err := s.RetireRPRequest(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("invalid request accepted: %v", err)
		}
	}
	// Same key in another operation and another principal is independent.
	other := r
	other.Operation = "move"
	if _, err := s.RetireRPRequest(ctx, other); err != nil {
		t.Fatal(err)
	}
	open := RPRequestRetireRequest{PrincipalID: "principal_creator", Operation: "open", IdempotencyKey: r.IdempotencyKey}
	if _, err := s.RetireRPRequest(ctx, open); err != nil {
		t.Fatal(err)
	}
	if err := submit(s); err != nil {
		t.Fatalf("different scope blocked: %v", err)
	}
	for _, query := range []string{`DELETE FROM rp_request_retirements`, `UPDATE rp_request_retirements SET idempotency_key='changed'`} {
		if _, err := s.db.ExecContext(ctx, query); err == nil {
			t.Fatal("fence was mutable")
		}
	}
	if _, err := s.CloseRPSession(ctx, core.RPSessionReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID}); err != nil {
		t.Fatal(err)
	}
	if out, err := s.RetireRPRequest(ctx, r); err != nil || out.Status != "completed" {
		t.Fatalf("closed receipt: %+v %v", out, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id='world.rp.control'`, r.PrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetireRPRequest(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked permission: %v", err)
	}
}

func TestRPRequestRetirementUpgrade026RetainsWorldAndReceipts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	r, submit := requestRetirementFixture(t, ctx, s, "dialogue")
	if err := submit(s); err != nil {
		t.Fatal(err)
	}
	before := requestWorldSnapshot(t, ctx, s)
	beforeReplay, err := s.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, before.Head)
	if err != nil {
		t.Fatal(err)
	}
	// Remove only the new empty application schema in this disposable fixture
	// to recreate026; the old world, history, styles and receipts are untouched.
	if _, err := s.db.ExecContext(ctx, `DROP TABLE rp_request_retirements`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version=?`, RPRequestSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if before != requestWorldSnapshot(t, ctx, s) {
		t.Fatal("upgrade changed world")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPRequestSchemaVersion}, 1)
	if out, err := s.RetireRPRequest(ctx, r); err != nil || out.Status != "completed" {
		t.Fatalf("legacy receipt lost: %+v %v", out, err)
	}
	if err := submit(s); err != nil {
		t.Fatal(err)
	}
	if before != requestWorldSnapshot(t, ctx, s) {
		t.Fatal("legacy replay duplicated")
	}
	afterReplay, err := s.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, before.Head)
	if err != nil || beforeReplay.StateHash != afterReplay.StateHash {
		t.Fatalf("upgrade changed replay hash: %s %s %v", beforeReplay.StateHash, afterReplay.StateHash, err)
	}
	// Unknown future principal is rejected without a dangling FK fence.
	if _, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: "missing", Operation: "open", IdempotencyKey: "absent"}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("unknown principal: %v", err)
	}
	var missing string
	if err := s.db.QueryRowContext(ctx, `SELECT principal_id FROM rp_request_retirements LIMIT 1`).Scan(&missing); err != sql.ErrNoRows {
		t.Fatalf("unexpected fence: %v", err)
	}
}

func requestRetirementFixture(t *testing.T, ctx context.Context, s *Store, operation string) (RPRequestRetireRequest, func(*Store) error) {
	t.Helper()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	r := RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Operation: operation, IdempotencyKey: "request-fence"}
	switch operation {
	case "open":
		r.SessionID = ""
		return r, func(s *Store) error {
			_, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: r.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "first_person", IdempotencyKey: r.IdempotencyKey})
			return err
		}
	case "dialogue":
		return r, func(s *Store) error {
			_, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好。", IdempotencyKey: r.IdempotencyKey})
			return err
		}
	case "wait":
		return r, func(s *Store) error {
			_, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 1, IdempotencyKey: r.IdempotencyKey})
			return err
		}
	case "move":
		return r, func(s *Store) error {
			_, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", IdempotencyKey: r.IdempotencyKey})
			return err
		}
	case "social":
		return r, func(s *Store) error {
			_, err := s.SocialRP(ctx, core.RPSocialRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, ExpectedCursor: view.ObservationCursor, TargetEntityID: M2RPNPCID, Action: "greet", IdempotencyKey: r.IdempotencyKey})
			return err
		}
	default:
		t.Fatal("unknown test operation")
		return r, nil
	}
}

type retirementWorldSnapshot struct {
	Head, Events, Commands, Sessions, Turns, Waits int64
	WorldTime                                      string
}

func requestWorldSnapshot(t *testing.T, ctx context.Context, s *Store) retirementWorldSnapshot {
	t.Helper()
	var out retirementWorldSnapshot
	err := s.db.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,
 (SELECT COUNT(*) FROM events),(SELECT COUNT(*) FROM commands),(SELECT COUNT(*) FROM rp_sessions),
 (SELECT COUNT(*) FROM rp_turn_runs),(SELECT COUNT(*) FROM rp_wait_intents)
 FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
 WHERE b.instance_id=? AND b.branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&out.Head, &out.WorldTime, &out.Events, &out.Commands, &out.Sessions, &out.Turns, &out.Waits)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRPRequestRetirementAllOwnersBothOrdersAndRecovery(t *testing.T) {
	ctx := context.Background()
	for _, operation := range []string{"open", "dialogue", "wait", "move", "social"} {
		for _, acceptedFirst := range []bool{false, true} {
			name := operation + "/retire-first"
			if acceptedFirst {
				name = operation + "/accept-first"
			}
			t.Run(name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "retirement.db")
				s, err := Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { s.Close() }()
				r, submit := requestRetirementFixture(t, ctx, s, operation)
				want := "retired"
				if acceptedFirst {
					if err := submit(s); err != nil {
						t.Fatal(err)
					}
					want = "completed"
				}
				before := requestWorldSnapshot(t, ctx, s)
				out, err := s.RetireRPRequest(ctx, r)
				if err != nil || out.Status != want || out.ProtocolVersion != RPClientProtocolVersion {
					t.Fatalf("%+v %v", out, err)
				}
				if operation == "open" && acceptedFirst && out.SessionID == "" {
					t.Fatal("open receipt missing session")
				}
				if after := requestWorldSnapshot(t, ctx, s); before != after {
					t.Fatalf("retirement changed authority: %+v %+v", before, after)
				}
				if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
					t.Fatal(err)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s, err = Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				recovered, err := s.RetireRPRequest(ctx, r)
				if err != nil || !reflect.DeepEqual(out, recovered) {
					t.Fatalf("retirement lost: %+v %v", recovered, err)
				}
				err = submit(s)
				if acceptedFirst && err != nil || !acceptedFirst && !core.HasCode(err, core.CodeRequestRetired) {
					t.Fatalf("late request/replay: %v", err)
				}
				if after := requestWorldSnapshot(t, ctx, s); before != after {
					t.Fatalf("late duplicate/effect: %+v %+v", before, after)
				}
				if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
					t.Fatalf("projection divergence: %v %v", diff, err)
				}
			})
		}
	}
}

func TestRPRequestRetirementConcurrentSeparateConnections(t *testing.T) {
	ctx := context.Background()
	for _, operation := range []string{"open", "dialogue", "wait", "move", "social"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "race.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			r, submit := requestRetirementFixture(t, ctx, s, operation)
			other, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			start := make(chan struct{})
			done := make(chan error, 1)
			go func() { <-start; done <- submit(other) }()
			close(start)
			out, err := s.RetireRPRequest(ctx, r)
			commandErr := <-done
			if err != nil {
				t.Fatal(err)
			}
			if out.Status == "retired" {
				if !core.HasCode(commandErr, core.CodeRequestRetired) {
					t.Fatalf("retired yet accepted: %v", commandErr)
				}
			} else if commandErr != nil {
				t.Fatal(commandErr)
			}
			stable, err := s.RetireRPRequest(ctx, r)
			if err != nil || stable.Status != "retired" && stable.Status != "completed" {
				t.Fatalf("unstable: %+v %v", stable, err)
			}
		})
	}
}

func TestRPRequestRetirementPreservesPartialOwners(t *testing.T) {
	ctx := context.Background()
	for _, operation := range []string{"dialogue", "wait"} {
		t.Run(operation, func(t *testing.T) {
			s, err := Open(ctx, filepath.Join(t.TempDir(), "partial.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			_, read, view := newRPWaitTestSession(t, ctx, s)
			r := RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Operation: operation, IdempotencyKey: "partial"}
			var recoverOriginal func() error
			if operation == "dialogue" {
				speech := core.RPSpeechRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: r.IdempotencyKey, Text: "你好。"}
				s.afterRPTurnStage = func(stage string) error {
					if stage == "player_event_committed" {
						return core.NewError(core.CodeInjectedFailure, "lost response")
					}
					return nil
				}
				if _, err := s.PlayRPTurn(ctx, speech); !core.HasCode(err, core.CodeInjectedFailure) {
					t.Fatalf("missing interruption: %v", err)
				}
				s.afterRPTurnStage = nil
				recoverOriginal = func() error { _, err := s.PlayRPTurn(ctx, speech); return err }
			} else {
				wait := core.RPWaitRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: r.IdempotencyKey, TargetWorldTime: "2026-09-23T08:00:00Z", Budget: 1}
				first, err := s.WaitRP(ctx, wait)
				if err != nil || first.Status != "budget_exhausted" {
					t.Fatalf("missing partial wait: %+v %v", first, err)
				}
				recoverOriginal = func() error {
					for i := 0; i < 20; i++ {
						result, err := s.WaitRP(ctx, wait)
						if err != nil {
							return err
						}
						if result.Status == "completed" {
							return nil
						}
					}
					t.Fatal("wait never completed")
					return nil
				}
			}
			before := requestWorldSnapshot(t, ctx, s)
			out, err := s.RetireRPRequest(ctx, r)
			if err != nil || out.Status != "in_progress" {
				t.Fatalf("partial discarded: %+v %v", out, err)
			}
			if before != requestWorldSnapshot(t, ctx, s) {
				t.Fatal("partial owner changed")
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_request_retirements`, nil, 0)
			if err := recoverOriginal(); err != nil {
				t.Fatal(err)
			}
			out, err = s.RetireRPRequest(ctx, r)
			if err != nil || out.Status != "completed" {
				t.Fatalf("recovery: %+v %v", out, err)
			}
		})
	}
}
