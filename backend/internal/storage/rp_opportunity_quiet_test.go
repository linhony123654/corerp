package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPOpportunityQuietWindowMaturesWithoutEventGuarantee(t *testing.T) {
	for _, base := range []int{0, 1000} {
		t.Run(fmt.Sprint(base), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "quiet.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			_, read, _ := newRPWaitTestSession(t, ctx, s)
			for _, key := range []string{"quiet-gift-one", "quiet-gift-two"} {
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
				if relation.SubjectEntityID == M2RPPlayerID && relation.Trust >= 2 && len(relation.SourceEventIDs) > 0 {
					source = relation.SourceEventIDs[0]
					break
				}
			}
			if source == "" {
				t.Fatal("missing friendship source")
			}
			now, err := time.Parse(time.RFC3339, input.WorldTime)
			if err != nil {
				t.Fatal(err)
			}
			early := now.Add(15 * time.Minute).Format(time.RFC3339)
			late := now.Add(2 * time.Hour).Format(time.RFC3339)
			r := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "quiet-policy"), Policy: RPOpportunityPolicy{ContactBasisPoints: base, CooldownHours: 1, HistoryHours: 1}}
			keyHash, err := core.HashJSON([]string{r.Binding.PrincipalID, r.Binding.IdempotencyKey})
			if err != nil {
				t.Fatal(err)
			}
			idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, "DefineRPOpportunityPolicy", keyHash})
			if err != nil {
				t.Fatal(err)
			}
			// Select misses explicitly: this proves quiet does not guarantee a
			// reaction, not a statistical estimate of long-run frequency.
			for i := 0; i < 1000; i++ {
				seed := fmt.Sprintf("quiet-miss-%d", i)
				miss := true
				for _, at := range []string{early, late} {
					chance := base
					if at == late {
						chance += base / 4
					}
					draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: "event_opportunity_" + idHash[7:], StreamSeed: seed, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2RPNPCID, Kind: "friend_contact", SourceEventID: source, WorldTime: at}, chance)
					if err != nil {
						t.Fatal(err)
					}
					miss = miss && !draw.Selected
				}
				if miss {
					r.Policy.StreamSeed = seed
					break
				}
			}
			policy, err := s.DefineRPOpportunityPolicy(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			// Context-reader boundary probes use existing immutable sources;
			// they do not mutate world history to manufacture a quiet interval.
			tx, err = beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			for _, probe := range []struct {
				delta         time.Duration
				recent, major int
				want          bool
			}{
				{time.Hour - time.Second, 0, 0, false},
				{time.Hour, 0, 0, true},
				{time.Hour, 1, 0, false},
				{time.Hour, 0, 1, false},
			} {
				quiet, err := readRPQuietOpportunityHistory(ctx, tx.conn, input, policy, now.Add(probe.delta), probe.recent, probe.major)
				if err != nil || quiet != probe.want {
					tx.Rollback(ctx)
					t.Fatalf("quiet boundary %+v: %t %v", probe, quiet, err)
				}
			}
			wrongScope := input
			wrongScope.BranchID = "other-branch"
			quiet, err := readRPQuietOpportunityHistory(ctx, tx.conn, wrongScope, policy, now.Add(2*time.Hour), 0, 0)
			tx.Rollback(ctx)
			if err != nil || quiet {
				t.Fatalf("foreign routine counted as mature: %t %v", quiet, err)
			}
			waitAt := func(at, key string) (RPWaitResult, rpContactOpportunity) {
				t.Helper()
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 100, IdempotencyKey: key})
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
				for _, receipt := range event.ContactOpportunities {
					if receipt.ActorID == M2RPNPCID {
						return wait, receipt
					}
				}
				t.Fatal("missing contact evaluation")
				return wait, rpContactOpportunity{}
			}
			_, first := waitAt(early, "early")
			if first.Quiet || first.Draw.ChanceBasisPoints != base || first.Draw.Selected {
				t.Fatalf("new policy counted as quiet: %+v", first)
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
			wait, mature := waitAt(late, "mature")
			if !mature.Quiet || mature.Draw.ChanceBasisPoints != base+base/4 || mature.Draw.Selected {
				t.Fatalf("quiet bonus/floor: %+v", mature)
			}
			calls := 0
			provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
				calls++
				return core.RPDecisionProposal{Action: "silence"}, nil
			})
			out, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}, provider)
			if err != nil || out.Action != "silence" || out.Status != "opportunity_quiet" || calls != 0 {
				t.Fatalf("quiet forced provider/action: %+v %v calls=%d", out, err, calls)
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("quiet recovery: %+v %v", differences, err)
			}
		})
	}
}
