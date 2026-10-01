package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

// Build through accepted speech, applied private records and the actual provider
// projection. The catalog must not become a second source/identity pipeline.
func TestRPEvidenceSupportStorageProjectedPacketHistoricalAndBudgetBoundaries(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "support-provider-packet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	session, oldPeer, initial := newRPWaitTestSession(t, ctx, s)
	old, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: oldPeer.PrincipalID, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "support-old-peer", Text: "那段祖先原话只是一项陈述。"}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我听到了。", Private: &core.RPDecisionPrivate{Intent: "过去曾考虑回应旧对象", RelationshipStance: "过去的谨慎", BasisEventIDs: []string{in.SpeechEventID}}}, nil
	}))
	if err != nil || len(old.NPCEventIDs) != 1 {
		t.Fatal("old applied private fixture", old, err)
	}
	var oldTime string
	if err := s.db.QueryRowContext(ctx, `SELECT world_time FROM events WHERE event_id=?`, old.NPCEventIDs[0]).Scan(&oldTime); err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, initial.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: oldPeer.PrincipalID, SessionID: oldPeer.SessionID, ExpectedCursor: old.SettledSequence, TargetWorldTime: at.Add(time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "support-later-time"}); err != nil {
		t.Fatal(err)
	}

	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	otherSession, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	other := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: otherSession.SessionID}
	observed, err := s.ObserveRPSession(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: other.PrincipalID, SessionID: other.SessionID, FromPlaceID: observed.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "support-unknown-arrives"}); err != nil {
		t.Fatal(err)
	}
	var partialEvent string
	// Crowd the old peer's words out of recent dialogue/knowledge using real
	// committed utterances. Keep their independently applied private sketch.
	for i := 0; i < 24; i++ {
		observed, err = s.ObserveRPSession(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		text := fmt.Sprintf("新话题的陈述第%d次。", i)
		if i == 0 {
			text = strings.Repeat("甲种往事", 400)
		}
		turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: other.PrincipalID, SessionID: other.SessionID, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: fmt.Sprintf("support-crowd-%d", i), Text: text}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
			return core.RPDecisionProposal{Action: "respond", Text: "我听着。"}, nil
		}))
		if err != nil {
			t.Fatal("committed crowding fixture", i, err)
		}
		if i == 0 {
			partialEvent = turn.PlayerEventID
		}
	}
	observed, err = s.ObserveRPSession(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: other.PrincipalID, SessionID: other.SessionID, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "support-current-question", Text: "现在的新提问。"})
	if err != nil {
		t.Fatal(err)
	}
	built, err := s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: other.PrincipalID, SessionID: other.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := s.rpDecisionProviderView(ctx, built)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPNPCID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	if packet.InterlocutorEntityID != alias || packet.Presentation == nil || packet.ContextSelection == nil {
		t.Fatal("actual provider projection missing", packet)
	}
	encoded, err := json.Marshal(packet)
	if err != nil || len(encoded) > core.DefaultRPDecisionContextBudgetBytes {
		t.Fatal("provider budget not applied", len(encoded), err)
	}
	catalog := core.RPDecisionEvidenceSupport(packet)
	allowed := core.RPDecisionEvidenceEventIDs(packet)
	for id, ranges := range catalog {
		if !allowed[id] {
			t.Fatal("catalog fetched an unoffered source", id)
		}
		for _, r := range ranges {
			if r.ActorEntityID == M2AgentAdaID || r.TargetEntityID == M2AgentAdaID {
				t.Fatal("catalog unmasked unfamiliar canonical entity", r)
			}
		}
	}
	current := rpStorageSupportRange(t, catalog, speech.EventID, "/player_speech_text")
	if current.ActorEntityID != alias || current.WorldTime != packet.PlayerSpeechWorldTime || current.Kind != "accepted_speech" || current.Status != "said" || !reflect.DeepEqual(current.AllowedUses, []string{"attributed_speech", "exact_quote"}) {
		t.Fatal("current speech became truth or lost projected actor", current)
	}
	privateFound := false
	for _, r := range catalog[old.NPCEventIDs[0]] {
		if r.Kind == "own_private" {
			privateFound = true
			if r.ActorEntityID != M2RPNPCID || r.TargetEntityID != session.ControlledEntityID || r.TargetEntityID == packet.InterlocutorEntityID || r.WorldTime != oldTime || r.WorldTime == packet.WorldTime || r.Status != "historical_sketch" || !reflect.DeepEqual(r.AllowedUses, []string{"private_continuity"}) {
				t.Fatal("old sketch acquired current peer/time/commitment", r)
			}
		}
	}
	if !privateFound {
		t.Fatal("actual applied private sketch missing")
	}
	ancestor := catalog[old.PlayerEventID]
	if len(ancestor) == 0 {
		t.Fatal("private basis provenance missing")
	}
	for _, r := range ancestor {
		if r.Kind != "provenance_only" || !reflect.DeepEqual(r.AllowedUses, []string{"provenance"}) {
			t.Fatal("basis-only ancestor gained assertion authority", r)
		}
	}
	partialFound := false
	for i, excerpt := range packet.HeardPlayerHistory {
		if excerpt.EventID == partialEvent && excerpt.Truncated {
			partialFound = true
			r := rpStorageSupportRange(t, catalog, partialEvent, fmt.Sprintf("/heard_player_history/%d/excerpt", i))
			if r.Completeness != "partial" || rpStorageSupportHasUse(r, "exact_quote") {
				t.Fatal("stored truncated excerpt authorized full quote", r)
			}
		}
	}
	if !partialFound {
		t.Fatal("fixture must retain a real truncated heard excerpt")
	}

	// Apply the same selector to this actual authorized packet at a tighter
	// budget, rather than adding fabricated sources or inflating required data.
	dropped := false
	for budget := len(encoded) - 1024; budget >= 1024; budget -= 1024 {
		selected, err := core.SelectRPDecisionContext(packet, budget)
		if err != nil {
			continue
		}
		selectedSources := core.RPDecisionEvidenceEventIDs(selected)
		for id, ranges := range catalog {
			if selectedSources[id] {
				continue
			}
			wasQuote := false
			for _, r := range ranges {
				wasQuote = wasQuote || rpStorageSupportHasUse(r, "exact_quote")
			}
			if !wasQuote {
				continue
			}
			after := core.RPDecisionEvidenceSupport(selected)
			if _, exists := after[id]; exists {
				t.Fatal("catalog retained a budget-discarded quote source", id)
			}
			dropped = true
			break
		}
		if dropped {
			break
		}
	}
	if !dropped {
		t.Fatal("real packet did not exercise source omission under budget pressure")
	}
}

func TestRPEvidenceSupportStorageWaitProviderHasNoInventedCurrentSpeech(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	observed, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, observed.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := f.s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: observed.ObservationCursor, TargetWorldTime: at.Add(time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "support-wait"})
	if err != nil {
		t.Fatal(err)
	}
	built, err := f.s.BuildRPInitiativeInput(ctx, core.RPInitiativeRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, TriggerEventID: wait.EventID, NPCEntityID: f.ids[0]})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := f.s.rpDecisionProviderView(ctx, built)
	if err != nil {
		t.Fatal(err)
	}
	if packet.SpeechEventID != "" || packet.PlayerSpeechText != "" || packet.PlayerSpeechWorldTime != "" || packet.Trigger == nil || packet.Trigger.SourceEventID != wait.EventID || packet.ContextSelection == nil || packet.Presentation == nil {
		t.Fatal("wait provider packet invented speech or bypassed projection", packet)
	}
	catalog := core.RPDecisionEvidenceSupport(packet)
	for _, ranges := range catalog {
		for _, r := range ranges {
			if r.Locator == "/player_speech_text" {
				t.Fatal("wait catalog invented a current utterance", r)
			}
		}
	}
	ranges := catalog[wait.EventID]
	if len(ranges) == 0 {
		t.Fatal("wait trigger provenance missing")
	}
	for _, r := range ranges {
		if r.Kind != "provenance_only" || !reflect.DeepEqual(r.AllowedUses, []string{"provenance"}) {
			t.Fatal("wait trigger became speech or fulfillment", r)
		}
	}
}

func rpStorageSupportRange(t *testing.T, catalog map[string][]core.RPEvidenceSupportRange, id, locator string) core.RPEvidenceSupportRange {
	t.Helper()
	for _, r := range catalog[id] {
		if r.Locator == locator {
			return r
		}
	}
	t.Fatalf("support range missing for %s at %s", id, locator)
	return core.RPEvidenceSupportRange{}
}

func rpStorageSupportHasUse(r core.RPEvidenceSupportRange, use string) bool {
	for _, candidate := range r.AllowedUses {
		if candidate == use {
			return true
		}
	}
	return false
}
