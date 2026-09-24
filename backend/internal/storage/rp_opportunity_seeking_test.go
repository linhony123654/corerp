package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPOpportunityExplicitSocialIntentChangesChanceNotDrawIdentity(t *testing.T) {
	for _, mode := range []string{"", "social"} {
		t.Run("intent-"+mode, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "seeking.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			_, read, _ := newRPWaitTestSession(t, ctx, s)
			for _, key := range []string{"seek-gift-one", "seek-gift-two"} {
				gift := socialRequest(t, ctx, s, read, M2RPNPCID, "gift", key)
				gift.AmountMinor = 1
				if _, err := s.SocialRP(ctx, gift); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			in, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2RPNPCID, InterlocutorEntityID: M2RPPlayerID})
			tx.Rollback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var source string
			for _, r := range in.Life.Relationships {
				if r.SubjectEntityID == M2RPPlayerID && len(r.SourceEventIDs) > 0 {
					source = r.SourceEventIDs[0]
					break
				}
			}
			p := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "seek-policy"), Policy: RPOpportunityPolicy{ContactBasisPoints: 1000, CooldownHours: 1, HistoryHours: 24}}
			keyHash, err := core.HashJSON([]string{p.Binding.PrincipalID, p.Binding.IdempotencyKey})
			if err != nil {
				t.Fatal(err)
			}
			idHash, err := core.HashJSON([]string{p.Binding.InstanceID, p.Binding.BranchID, "DefineRPOpportunityPolicy", keyHash})
			if err != nil {
				t.Fatal(err)
			}
			var baseline core.RPOpportunityDraw
			for i := 0; i < 1000; i++ {
				seed := fmt.Sprintf("seeking-fixture-%d", i)
				draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: "event_opportunity_" + idHash[7:], StreamSeed: seed, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2RPNPCID, Kind: "friend_contact", SourceEventID: source, WorldTime: "2026-09-22T03:15:00Z"}, 1000)
				if err != nil {
					t.Fatal(err)
				}
				if draw.RollBasisPoints >= 1000 && draw.RollBasisPoints < 1500 {
					baseline = draw
					p.Policy.StreamSeed = seed
					break
				}
			}
			if _, err := s.DefineRPOpportunityPolicy(ctx, p); err != nil {
				t.Fatal(err)
			}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			r := core.RPWaitRequest{OpportunityIntent: mode, PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:15:00Z", Budget: 100, IdempotencyKey: "seeking-wait"}
			bad := r
			bad.OpportunityIntent = "force_drama"
			if _, err := s.WaitRP(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatalf("invalid intent: %v", err)
			}
			wait, err := s.WaitRP(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			readReceipt := func(id string) rpContactOpportunity {
				t.Helper()
				var raw string
				if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, id).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var event rpWaitEvent
				if err := json.Unmarshal([]byte(raw), &event); err != nil {
					t.Fatal(err)
				}
				for _, receipt := range event.ContactOpportunities {
					if receipt.ActorID == M2RPNPCID {
						return receipt
					}
				}
				t.Fatal("missing source receipt")
				return rpContactOpportunity{}
			}
			first := readReceipt(wait.EventID)
			want := 1000
			if mode == "social" {
				want = 1500
			}
			if first.Seeking != (mode == "social") || first.Draw.ChanceBasisPoints != want || first.Draw.Selected != (mode == "social") {
				t.Fatalf("explicit intent: %+v", first)
			}
			if baseline.IdentityHash != first.Draw.IdentityHash || baseline.RollBasisPoints != first.Draw.RollBasisPoints {
				t.Fatal("intent altered random identity")
			}
			var public string
			if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, wait.EventID).Scan(&public); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(public, "opportunity_intent") || strings.Contains(public, "seeking") {
				t.Fatal("private intent broadcast")
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
			retry, err := s.WaitRP(ctx, r)
			if err != nil || !retry.Replayed || !reflect.DeepEqual(readReceipt(retry.EventID), first) {
				t.Fatalf("intent recovery: %+v %v", retry, err)
			}
			bad = r
			if mode == "" {
				bad.OpportunityIntent = "social"
			} else {
				bad.OpportunityIntent = ""
			}
			if _, err := s.WaitRP(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
				t.Fatalf("changed intent retry: %v", err)
			}
			calls := 0
			provider := rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
				calls++
				return core.RPDecisionProposal{Action: "respond", Text: "你好，又见面了。"}, nil
			})
			out, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}, provider)
			wantCalls := 0
			wantAction := "silence"
			if mode == "social" {
				wantCalls = 1
				wantAction = "respond"
			}
			if err != nil || calls != wantCalls || out.Action != wantAction {
				t.Fatalf("seeking consequence: %+v %v calls=%d", out, err, calls)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, out.EventID}, int64(wantCalls))
			view, err = s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			bad.ExpectedCursor = view.ObservationCursor
			bad.IdempotencyKey = "fresh-mode-same-hour"
			bad.TargetWorldTime = "2026-09-22T03:45:00Z"
			later, err := s.WaitRP(ctx, bad)
			if err != nil || !reflect.DeepEqual(readReceipt(later.EventID), first) {
				t.Fatalf("mode toggling rerolled: %+v %v", later, err)
			}
		})
	}
}
