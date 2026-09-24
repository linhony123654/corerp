package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRPCultureGiftChangesRealNPCChoiceAndPreservesPrivateHistory(t *testing.T) {
	for _, score := range []int{2, -2} {
		t.Run(map[int]string{2: "welcome", -2: "unwelcome"}[score], func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "culture-life.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
				t.Fatal(err)
			}
			author, actor := M2AgentBoID, M2RPNPCID
			definition, err := s.DefineRPCulture(ctx, CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "culture"), AuthorID: author, Culture: core.RPCulture{CultureID: "gift_custom", ScopeID: "gift_custom", ScopeKind: "community", GroupIdentity: "gift customs", Norms: []core.RPCultureNorm{{NormID: "giving", Action: "gift", Evaluation: score}}}})
			if err != nil {
				t.Fatal(err)
			}
			news, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "news"), SpeakerID: author, DefinitionEventID: definition.EventID})
			if err != nil {
				t.Fatal(err)
			}
			stanceReq := CultureStanceRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "stance"), EntityID: actor, TransmissionEventID: news.EventID, Stance: "accept"}
			stance, err := s.InternalizeRPCulture(ctx, stanceReq)
			if err != nil {
				t.Fatal(err)
			}
			read := allowFixtureControl(t, ctx, s, author)
			gift := socialRequest(t, ctx, s, read, actor, "gift", "gift")
			gift.AmountMinor = 1
			act, err := s.SocialRP(ctx, gift)
			if err != nil {
				t.Fatal(err)
			}
			before := readCareerTestContext(t, s, actor)
			if len(before.Life.CultureExperiences) != 1 || before.Life.CultureExperiences[0].ActionEventID != act.EventID || before.Life.CultureExperiences[0].Evaluations[0].Score != score {
				t.Fatalf("culture experience missing %+v", before.Life)
			}
			if before.Life.CultureExperiences[0].Evaluations[0].Basis.StanceEventID != stance.EventID {
				t.Fatal("stance provenance missing")
			}
			giver := readCareerTestContext(t, s, author)
			if len(giver.Life.CultureExperiences) != 0 {
				t.Fatal("other actor's private evaluation leaked")
			}
			var raw string
			if err := s.db.QueryRow(`SELECT claim_payload FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, author, act.EventID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(raw, stance.EventID) || strings.Contains(raw, "culture_experiences") {
				t.Fatal("private stance copied into giver knowledge")
			}
			// A later change of identity cannot rewrite the experienced earlier gift.
			stanceReq.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "later-rebel")
			stanceReq.Stance = "rebel"
			if _, err := s.InternalizeRPCulture(ctx, stanceReq); err != nil {
				t.Fatal(err)
			}
			after := readCareerTestContext(t, s, actor)
			if !reflect.DeepEqual(before.Life.CultureExperiences, after.Life.CultureExperiences) {
				t.Fatal("later stance rewrote earlier experience")
			}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: "after-gift"})
			if err != nil {
				t.Fatal(err)
			}
			request := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: speech.TurnID, NPCEntityID: actor}
			decision, err := s.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
			if err != nil {
				t.Fatal(err)
			}
			want := "respond"
			if score < 0 {
				want = "refuse"
			}
			if decision.Proposal.Action != want {
				encoded, _ := json.Marshal(decision)
				t.Fatalf("want %s got %s", want, encoded)
			}
			committed, err := s.CommitRPDecision(ctx, request, decision)
			if err != nil || committed.Action != want {
				t.Fatalf("actual decision %+v %v", committed, err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			retry, err := s.CommitRPDecision(ctx, request, decision)
			if err != nil || !retry.Replayed || retry.EventID != committed.EventID {
				t.Fatalf("decision restart %+v %v", retry, err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			recovered := readCareerTestContext(t, s, actor)
			if !reflect.DeepEqual(before.Life.CultureExperiences, recovered.Life.CultureExperiences) {
				t.Fatal("rebuild changed historical evaluation")
			}
			diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
			if err != nil || len(diffs) != 0 {
				t.Fatalf("projection divergence %v %v", diffs, err)
			}
			// The same norm with explicit rebellion now changes the next real
			// gift's evaluation and the next committed choice, not past history.
			gift = socialRequest(t, ctx, s, read, actor, "gift", "second-gift")
			gift.AmountMinor = 1
			if _, err := s.SocialRP(ctx, gift); err != nil {
				t.Fatal(err)
			}
			view, err = s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			speech, err = s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: "after-second-gift"})
			if err != nil {
				t.Fatal(err)
			}
			request.TurnID = speech.TurnID
			decision, err = s.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
			if err != nil {
				t.Fatal(err)
			}
			want = "refuse"
			if score < 0 {
				want = "respond"
			}
			if decision.Proposal.Action != want {
				t.Fatalf("changed identification: want %s got %+v", want, decision)
			}
			if result, err := s.CommitRPDecision(ctx, request, decision); err != nil || result.Action != want {
				t.Fatalf("second real choice %+v %v", result, err)
			}
		})
	}
}
