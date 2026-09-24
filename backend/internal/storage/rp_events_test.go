package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPClientEventsLateSettlementRefreshesHistoryWithoutNewWorldEvent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "late-history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	other, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "late-other-client"})
	if err != nil {
		t.Fatal(err)
	}
	interruption := errors.New("pause before settled narrative view")
	s.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effects_committed" {
			return interruption
		}
		return nil
	}
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "late-history", Text: "稍后完成的共同见闻。"}
	if _, err := s.PlayRPTurn(ctx, request); !errors.Is(err, interruption) {
		t.Fatalf("wrong interruption: %v", err)
	}
	r := RPEventsReadRequest{PrincipalID: read.PrincipalID, SessionID: other.SessionID}
	before, err := s.ReadRPEvents(ctx, r)
	if err != nil || len(before.Events) == 0 || before.HistoryRevision != 0 {
		t.Fatalf("before settlement: %+v %v", before, err)
	}
	otherRead := core.RPSessionReadRequest{PrincipalID: read.PrincipalID, SessionID: other.SessionID}
	view, err := s.ObserveRPSession(ctx, otherRead)
	if err != nil || len(view.RecentTurns) != 0 {
		t.Fatalf("unsettled history published: %+v %v", view, err)
	}
	s.afterRPTurnStage = nil
	settled, err := s.PlayRPTurn(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	r.After = before.NextSequence
	after, err := s.ReadRPEvents(ctx, r)
	if err != nil || after.HeadSequence != before.HeadSequence || after.NextSequence != before.NextSequence || len(after.Events) != 0 || after.HistoryRevision != before.HistoryRevision+1 {
		t.Fatalf("late settlement notification missing: before=%+v after=%+v err=%v", before, after, err)
	}
	view, err = s.ObserveRPSession(ctx, otherRead)
	if err != nil || len(view.RecentTurns) != 1 || view.RecentTurns[0].TurnRunID != settled.TurnRunID || view.RecentTurns[0].CanRegenerate {
		t.Fatalf("late shared view: %+v %v", view, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := s.ReadRPEvents(ctx, r)
	if err != nil || !reflect.DeepEqual(after, rebuilt) {
		t.Fatalf("unstable history revision after rebuild: %v", err)
	}
}

func TestRPClientEventsEvidencePagingAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-events.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我听到了。"}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "event-speech", Text: "你好。"}, provider); err != nil {
		t.Fatal(err)
	}
	gift := socialRequest(t, ctx, s, read, M2RPNPCID, "gift", "event-gift")
	gift.AmountMinor = 1
	given, err := s.SocialRP(ctx, gift)
	if err != nil {
		t.Fatal(err)
	}
	r := RPEventsReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, After: initial.ObservationCursor}
	before, err := s.ReadRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.ReadRPEvents(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if all.ProtocolVersion != RPClientProtocolVersion || all.HeadSequence != given.EventSequence || all.NextSequence != all.HeadSequence || all.MoreEvents || len(all.Events) < 3 {
		t.Fatalf("unexpected event page: %+v", all)
	}
	own, heard, social := false, false, false
	for _, event := range all.Events {
		if event.Sequence <= r.After || event.Facts == nil {
			t.Fatalf("invalid event: %+v", event)
		}
		own = own || event.OwnAction != nil && event.OwnAction.Kind == "speech" && event.OwnAction.Text == "你好。"
		for _, fact := range event.Facts {
			if fact.SourceEventID != event.EventID || fact.SubjectEntityID != M2RPNPCID {
				t.Fatalf("foreign evidence: %+v", fact)
			}
			heard = heard || fact.Kind == "speaker_said" && fact.Text == "我听到了。"
			social = social || event.EventID == given.EventID && fact.Action == "gift"
		}
	}
	if !own || !heard || !social {
		t.Fatalf("missing sourced evidence: %+v", all)
	}
	r.Limit = 1
	var paged []RPClientEvent
	for i := 0; i < 20; i++ {
		page, err := s.ReadRPEvents(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Events) > 1 || page.NextSequence <= r.After {
			t.Fatalf("pagination stalled: %+v", page)
		}
		paged = append(paged, page.Events...)
		r.After = page.NextSequence
		if !page.MoreEvents {
			break
		}
	}
	if !reflect.DeepEqual(paged, all.Events) {
		t.Fatal("pagination skipped/duplicated evidence")
	}
	empty, err := s.ReadRPEvents(ctx, r)
	if err != nil || len(empty.Events) != 0 || empty.MoreEvents || empty.NextSequence != all.HeadSequence {
		t.Fatalf("tail: %+v %v", empty, err)
	}
	after, err := s.ReadRPSession(ctx, read)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("event reads mutated session")
	}
	r.After, r.Limit = initial.ObservationCursor, 0
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := s.ReadRPEvents(ctx, r)
	if err != nil || !reflect.DeepEqual(all, recovered) {
		t.Fatalf("rebuild/reopen changed events: %v", err)
	}
	second, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "first_person", IdempotencyKey: "events-second-client"})
	if err != nil {
		t.Fatal(err)
	}
	r.SessionID = second.SessionID
	shared, err := s.ReadRPEvents(ctx, r)
	if err != nil || !reflect.DeepEqual(shared.Events, all.Events) {
		t.Fatal("same character split across clients")
	}
	r.SessionID = read.SessionID
	for _, limit := range []int{-1, 51} {
		r.Limit = limit
		if _, err := s.ReadRPEvents(ctx, r); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("bad limit: %v", err)
		}
	}
	r.Limit = 0
	r.After = -1
	if _, err := s.ReadRPEvents(ctx, r); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("negative cursor: %v", err)
	}
	r.After = all.HeadSequence + 1
	if _, err := s.ReadRPEvents(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("future cursor: %v", err)
	}
	r.After = 0
	r.PrincipalID = "principal_creator"
	if _, err := s.ReadRPEvents(ctx, r); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("foreign session: %v", err)
	}
	r.PrincipalID = read.PrincipalID
	// Disposable observation projection: extra private keys must not serialize.
	if _, err := s.db.ExecContext(ctx, `UPDATE observation_records SET claim_payload=json_set(claim_payload,'$.private_goal','secret-evaluation') WHERE observer_agent_id=?`, M2RPPlayerID); err != nil {
		t.Fatal(err)
	}
	private, err := s.ReadRPEvents(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(private)
	if err != nil || strings.Contains(string(raw), "private_goal") || strings.Contains(string(raw), "secret-evaluation") || strings.Contains(string(raw), "listener_ids") {
		t.Fatal("raw private data exposed")
	}
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPEvents(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("closed session: %v", err)
	}
}

func TestRPClientEventsRetainEncountersAndNarrowWait(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "events-encounters.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	from := M2AgentCafeID
	var encounters []string
	for index, to := range []string{"place_m2_home_ada", M2AgentCafeID, "place_m2_home_ada", M2AgentCafeID} {
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		moved, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: from, ToPlaceID: to, IdempotencyKey: fmt.Sprintf("encounter-%d", index)})
		if err != nil {
			t.Fatal(err)
		}
		if to == "place_m2_home_ada" {
			encounters = append(encounters, moved.EventID)
		}
		from = to
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 1, IdempotencyKey: "event-quiet-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("wait: %+v %v", wait, err)
	}
	r := RPEventsReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, After: initial.ObservationCursor}
	page, err := s.ReadRPEvents(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	waitSeen := false
	for _, event := range page.Events {
		for _, fact := range event.Facts {
			if fact.Kind == "agent_presence" && fact.SubjectEntityID == M2AgentAdaID {
				seen = append(seen, event.EventID)
			}
		}
		if event.EventID == wait.EventID {
			waitSeen = event.OwnAction != nil && event.OwnAction.Kind == "wait" && event.OwnAction.TargetWorldTime == wait.CurrentWorldTime
		}
	}
	if !reflect.DeepEqual(seen, encounters) || !waitSeen {
		t.Fatalf("lost encounter or wait: seen=%v want=%v wait=%v", seen, encounters, waitSeen)
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"warm_candidates", "contact_opportunities", "listener_ids", "initiative_npc_ids", "processed_items", "decision_input", M2AgentBoID} {
		if strings.Contains(string(raw), hidden) {
			t.Fatalf("private/remote field exposed: %s", hidden)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id='world.rp.control'`, read.PrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPEvents(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked controller: %v", err)
	}
}
