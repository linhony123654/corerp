package narrative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestProseExpressionRecipientRequiresSameObservable(t *testing.T) {
	base := core.RPNarrativeInput{ControlledEntityID: "player", Style: core.RPStyleProfile{POV: "second_person"}, Facts: []core.RPNarrativeFact{
		{EventID: "player", ActorID: "player", ActorName: "宝玉", Action: "speak"},
		{EventID: "nod", ActorID: "baochai", ActorName: "宝钗", Action: "expression", ExpressionCode: "nod"},
		{EventID: "other", ActorID: "xiren", ActorName: "袭人", Action: "silence"},
	}}
	for _, draft := range []string{"宝钗对你点头。", "宝钗向你点了点头。", "宝钗朝袭人点头。", "宝钗向窗外点头。"} {
		t.Run(draft, func(t *testing.T) {
			if err := validateProseSceneClaims(draft, base); err == nil {
				t.Fatal("untargeted expression acquired a new recipient")
			}
		})
	}
	if err := validateProseSceneClaims("宝钗点头。", base); err != nil {
		t.Fatal("sourced undirected expression was rejected", err)
	}
	player := base
	player.Facts = append([]core.RPNarrativeFact(nil), base.Facts...)
	player.Facts[1].TargetActorID, player.Facts[1].TargetActorName = "player", "宝玉"
	for _, draft := range []string{"宝钗对你点头。", "宝钗朝着宝玉点了点头。", "宝钗保持沉默，向你点头。"} {
		if err := validateProseSceneClaims(draft, player); err != nil {
			t.Fatalf("approved recipient was rejected: %q: %v", draft, err)
		}
	}
	if err := validateProseSceneClaims("宝钗向袭人点头。", player); err == nil {
		t.Fatal("approved player recipient was changed to another NPC")
	}
	other := base
	other.Facts = append([]core.RPNarrativeFact(nil), base.Facts...)
	other.Facts[1].TargetActorID, other.Facts[1].TargetActorName = "xiren", "袭人"
	if err := validateProseSceneClaims("宝钗向袭人点头。", other); err != nil {
		t.Fatal("explicitly witnessed NPC recipient was rejected", err)
	}
	if err := validateProseSceneClaims("宝钗对你点头。", other); err == nil {
		t.Fatal("another NPC's gesture was redirected to the player")
	}
	borrowed := base
	borrowed.Facts = append(append([]core.RPNarrativeFact(nil), base.Facts...), core.RPNarrativeFact{
		EventID: "beckon", ActorID: "baochai", ActorName: "宝钗", Action: "expression", ExpressionCode: "beckon", TargetActorID: "player", TargetActorName: "宝玉",
	})
	if err := validateProseSceneClaims("宝钗向你点头。", borrowed); err == nil {
		t.Fatal("nod borrowed recipient from a different expression")
	}
	first := player
	first.Style.POV = "first_person"
	if err := validateProseSceneClaims("宝钗朝我点头。", first); err != nil {
		t.Fatal("first-person approved recipient was rejected", err)
	}
	if err := validateProseSceneClaims("宝钗向她点头。", player); err == nil {
		t.Fatal("ambiguous NPC pronoun was accepted as a player recipient")
	}
}

func TestChatProseProviderCarriesAndRepairsObservableRecipient(t *testing.T) {
	in := proseFixture()
	in.Facts = append(in.Facts,
		core.RPNarrativeFact{EventID: "other", ActorID: "mei", ActorName: "Mei", Action: "silence"},
		core.RPNarrativeFact{EventID: "nod", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "nod", TargetActorID: in.ControlledEntityID, TargetActorName: in.Facts[0].ActorName},
	)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) < 2 {
			t.Error("invalid prose request", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload struct {
			Facts []struct{ Actor, Action, Target string } `json:"facts"`
		}
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &payload); err != nil || len(payload.Facts) != len(in.Facts) || payload.Facts[len(payload.Facts)-1].Target != "你" {
			t.Error("model request lost the sourced recipient", err)
		}
		draft := "你问：「今晚有空吗？」Cai 回答：「有啊，坐这儿吧。」Cai向Mei点头。"
		if calls.Add(1) == 2 {
			if len(body.Messages) != 4 || !strings.Contains(body.Messages[3].Content, "fact_ref") || !strings.Contains(body.Messages[3].Content, "禁止所有额外字段") {
				t.Error("repair feedback lost the recipient constraint")
			}
			draft = factCompositionFixture("f0", "f1", "f2", "f3")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": draft}}}})
	}))
	defer server.Close()
	p, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	view, err := p.Render(context.Background(), in)
	if err != nil || view.FallbackReason != "" || calls.Load() != 2 || !strings.Contains(strings.Join(view.Lines, ""), "Cai 向你点了点头") {
		t.Fatalf("approved recipient repair failed: %+v / %v / calls=%d", view, err, calls.Load())
	}
	if len(view.EventIDs) != len(in.Facts) || strings.Contains(strings.Join(view.Lines, ""), "Cai 向Mei") {
		t.Fatalf("repair lost facts or changed observed recipient: %+v", view)
	}
	if proseValidationCategory(failure("uncommitted expression target in prose")) != "expression_target_not_committed" {
		t.Fatal("recipient violation lost its safe diagnostic category")
	}
}

func TestProseMovementKindAndPlaceMustMatchCommittedFact(t *testing.T) {
	for _, controlled := range []bool{false, true} {
		actorID, actorName, prefix := "cai", "Cai", "Cai "
		if controlled {
			actorID, actorName, prefix = "player", "宝玉", "你"
		}
		for _, action := range []string{"arrive", "depart"} {
			in := core.RPNarrativeInput{ControlledEntityID: "player", Style: core.RPStyleProfile{POV: "second_person"}, Facts: []core.RPNarrativeFact{
				{EventID: "move", ActorID: actorID, ActorName: actorName, Action: action, PlaceName: "咖啡馆"},
			}}
			valid, wrongDirection := "来到咖啡馆。", "离开咖啡馆。"
			if action == "depart" {
				valid, wrongDirection = wrongDirection, valid
			}
			if err := validateProse(prefix+valid, in); err != nil {
				t.Fatalf("committed %s was rejected: %v", action, err)
			}
			for _, extra := range []string{wrongDirection, "坐下。", "站起来。", "走到后花园。", "走过去抱住了你。"} {
				if err := validateProse(prefix+extra, in); err == nil {
					t.Errorf("%s licensed a different movement or posture: %q", action, prefix+extra)
				}
			}
		}
	}
}

func TestProseNPCWaitDoesNotAdvanceWorldTime(t *testing.T) {
	in := core.RPNarrativeInput{ControlledEntityID: "player", Style: core.RPStyleProfile{POV: "second_person"}, Facts: []core.RPNarrativeFact{
		{EventID: "player", ActorID: "player", ActorName: "宝玉", Action: "speak", WorldTime: "2026-09-30T00:00:00Z"},
		{EventID: "wait", ActorID: "npc", ActorName: "Cai", Action: "wait", WorldTime: "2026-09-30T00:00:00Z"},
	}}
	for _, draft := range []string{"转眼过了三天。", "过了10分钟。"} {
		if err := validateProseSceneClaims(draft, in); err == nil {
			t.Errorf("wait stance licensed an unsupported clock advance: %q", draft)
		}
	}
	if err := validateProseSceneClaims("Cai选择等待。", in); err != nil {
		t.Fatal("committed waiting stance was rejected", err)
	}
}
