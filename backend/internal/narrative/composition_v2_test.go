package narrative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestFreshModelSelectionUsesDefaultGrammarAndFrozenPublicArtifact(t *testing.T) {
	in := proseFixture()
	in.SourceHead = 18
	in.Facts = append(in.Facts, core.RPNarrativeFact{EventID: "smile-source", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "smile", CompanionEventID: "e2", PlaceName: "咖啡馆"})
	expected, err := (core.DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(expected.Artifact.Plan)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Messages []struct{ Role, Content string }
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
			t.Error("invalid model selection request")
			return
		}
		var payload struct {
			Version string      `json:"composition_version"`
			Head    int64       `json:"source_head"`
			Facts   []proseFact `json:"facts"`
			Choices []struct {
				Refs  []string `json:"fact_refs"`
				Forms []string `json:"forms"`
			} `json:"eligible_beats"`
		}
		if json.Unmarshal([]byte(request.Messages[1].Content), &payload) != nil || payload.Version != core.RPFactCompositionVersionV2 || payload.Head != in.SourceHead || len(payload.Facts) != 3 || payload.Facts[1].Text != in.Facts[1].Text {
			t.Error("actual public view lost source/words/version")
			return
		}
		companion := false
		for _, choice := range payload.Choices {
			if reflect.DeepEqual(choice.Refs, []string{"f1", "f2"}) && len(choice.Forms) == 2 {
				companion = true
			}
		}
		if !companion {
			t.Error("proven companion not available to actual selector")
		}
		for _, forbidden := range []string{`"committed_facts"`, `"controlled_entity_id"`, `"companion_event_id"`, `"persona"`, `"private"`} {
			if strings.Contains(request.Messages[1].Content, forbidden) {
				t.Error("raw identity/private/source channel sent", forbidden)
			}
		}
		if strings.Contains(request.Messages[0].Content, in.Facts[0].Text) {
			t.Error("dialogue moved to instruction channel")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(raw)}}}})
	}))
	defer server.Close()
	p, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture", Timeout: time.Second, Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []core.RPNarrativeChunk
	actual, err := p.RenderStream(context.Background(), in, func(c core.RPNarrativeChunk) error { chunks = append(chunks, c); return nil })
	if err != nil || actual.FallbackReason != "" || calls.Load() != 1 || !reflect.DeepEqual(actual.Lines, expected.Lines) || !reflect.DeepEqual(actual.FactGroups, expected.FactGroups) || actual.Artifact == nil || actual.Artifact.InputSHA256 != expected.Artifact.InputSHA256 {
		t.Fatalf("default/model compiler diverged: %+v %v", actual, err)
	}
	if len(chunks) != 2 || !reflect.DeepEqual(chunks[1].EventIDs, []string{"e2", "smile-source"}) {
		t.Fatalf("fused paragraph attribution wrong: %+v", chunks)
	}
}

func TestFreshV2RejectsV1AndExtraProsodyWithoutRelabel(t *testing.T) {
	in := proseFixture()
	if _, err := renderFreshRPComposition(context.Background(), legacyFactCompositionFixture("f0", "f1"), in); err == nil {
		t.Fatal("fresh v1 accepted as v2")
	}
	plan, err := core.BuildDefaultRPComposition(in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(plan)
	invalid := strings.Replace(string(raw), `"lexical":"plain"`, `"lexical":"plain","voice":"低声"`, 1)
	if _, err := renderFreshRPComposition(context.Background(), invalid, in); err == nil {
		t.Fatal("uncommitted prosody accepted")
	}
	legacy, err := renderComposition(context.Background(), legacyFactCompositionFixture("f0", "f1"), in)
	if err != nil || len(legacy.Lines) != 2 {
		t.Fatal("explicit v1 compatibility helper broken", err)
	}
}
