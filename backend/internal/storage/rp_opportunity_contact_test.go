package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

// Fixture seeds deliberately exercise both branches. This is an integration
// test of recorded opportunities, not a statistical frequency or long-run test.
func TestRPOpportunityContactHitAndMissCommitDifferentConsequences(t *testing.T) {
	for _, hit := range []bool{false, true} {
		t.Run(fmt.Sprintf("selected-%t", hit), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "contact-action.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			_, read, _ := newRPWaitTestSession(t, ctx, s)
			for _, key := range []string{"first-gift", "second-gift"} {
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
			input, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2RPNPCID, InterlocutorEntityID: M2RPPlayerID})
			tx.Rollback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var source string
			for _, relation := range input.Life.Relationships {
				if relation.SubjectEntityID == M2RPPlayerID && len(relation.SourceEventIDs) > 0 {
					source = relation.SourceEventIDs[0]
				}
			}
			if source == "" {
				t.Fatal("missing actual relationship")
			}
			r := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "contact-policy"), Policy: RPOpportunityPolicy{ContactBasisPoints: 5000, CooldownHours: 6, HistoryHours: 24}}
			keyHash, err := core.HashJSON([]string{r.Binding.PrincipalID, r.Binding.IdempotencyKey})
			if err != nil {
				t.Fatal(err)
			}
			idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, "DefineRPOpportunityPolicy", keyHash})
			if err != nil {
				t.Fatal(err)
			}
			policyID := "event_opportunity_" + idHash[7:]
			for i := 0; i < 1000; i++ {
				seed := fmt.Sprintf("fixture-contact-%d", i)
				draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policyID, StreamSeed: seed, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2RPNPCID, Kind: "friend_contact", SourceEventID: source, WorldTime: "2026-09-22T03:15:00Z"}, 5000)
				if err != nil {
					t.Fatal(err)
				}
				if draw.Selected == hit {
					r.Policy.StreamSeed = seed
					break
				}
			}
			policy, err := s.DefineRPOpportunityPolicy(ctx, r)
			if err != nil || policy.EventID != policyID {
				t.Fatalf("fixture stream installation: %+v %v", policy, err)
			}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:15:00Z", Budget: 100, IdempotencyKey: "contact"})
			if err != nil {
				t.Fatal(err)
			}
			initiative := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}
			calls := 0
			provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				calls++
				if in.ContactOpportunity == nil || !in.ContactOpportunity.Selected || in.ContactOpportunity.SourceEventID != source {
					t.Fatal("provider contact lacks own source")
				}
				return core.RPDecisionProposal{Action: "respond", Text: "又见面了，最近过得怎么样？"}, nil
			})
			out, err := s.RunRPInitiative(ctx, initiative, provider)
			if err != nil {
				t.Fatal(err)
			}
			wantAction, wantStatus, wantCalls, wantHeard := "silence", "opportunity_quiet", 0, int64(0)
			if hit {
				wantAction, wantStatus, wantCalls, wantHeard = "respond", "validated", 1, 1
			}
			if out.Action != wantAction || out.Status != wantStatus || calls != wantCalls {
				t.Fatalf("contact result: %+v calls=%d", out, calls)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, out.EventID}, wantHeard)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND event_type='RPSpeechAccepted'`, []any{out.EventID}, wantHeard)
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
			retry, err := s.RunRPInitiative(ctx, initiative, provider)
			if err != nil || !retry.Replayed || retry.EventID != out.EventID || calls != wantCalls {
				t.Fatalf("contact retry: %+v calls=%d %v", retry, calls, err)
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("contact replay differences: %+v %v", differences, err)
			}
			if hit {
				// Original draw at03:15: six-hour cooldown must still hold at09:00,
				// even after restart. Rounding to the draw window would expire early.
				for _, at := range []string{"2026-09-22T09:00:00Z", "2026-09-22T10:00:00Z"} {
					view, err := s.ObserveRPSession(ctx, read)
					if err != nil {
						t.Fatal(err)
					}
					wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 100, IdempotencyKey: at})
					if err != nil {
						t.Fatal(err)
					}
					var raw string
					if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, wait.EventID).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					var event rpWaitEvent
					if err := json.Unmarshal([]byte(raw), &event); err != nil {
						t.Fatal(err)
					}
					found := false
					for _, receipt := range event.ContactOpportunities {
						if receipt.ActorID != M2RPNPCID {
							continue
						}
						found = true
						wantCooling := at == "2026-09-22T09:00:00Z"
						if receipt.CoolingDown != wantCooling || receipt.RecentChanges != 1 {
							t.Fatalf("cooldown/density at%s: %+v", at, receipt)
						}
						if wantCooling && (receipt.Draw.Selected || receipt.Draw.ChanceBasisPoints != 0) {
							t.Fatal("cooldown still offered contact")
						}
						if !wantCooling && receipt.Draw.ChanceBasisPoints != 2500 {
							t.Fatalf("cooldown did not expire or density lost: %+v", receipt)
						}
					}
					if !found {
						t.Fatal("missing post-restart contact receipt")
					}
				}
			}
		})
	}
}
