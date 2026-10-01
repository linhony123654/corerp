package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/endpointpolicy"
)

func TestDecisionWireV4SpeechToneAndArchivedV3(t *testing.T) {
	for _, action := range []string{"respond", "refuse"} {
		for _, tone := range decisionSpeechTones {
			raw := wireDecision(`{"action":"` + action + `","text":"好。","speech_tone":"` + tone + `","introduce_self":false,"expression_code":"none"}`)
			proposal, err := parseRequestedDecisionProposal(raw, true)
			want := tone
			if tone == "none" {
				want = ""
			}
			if err != nil || proposal.SpeechTone != want || proposal.Text != "好。" || proposal.Private == nil {
				t.Fatalf("tone %s/%s: %+v %v", action, tone, proposal, err)
			}
		}
		archived := wireDecision(`{"action":"` + action + `","text":"旧记录。","introduce_self":false,"expression_code":"none"}`)
		proposal, err := parseProposal(archived)
		if err != nil || proposal.Text != "旧记录。" || proposal.SpeechTone != "" {
			t.Fatalf("archived v3 changed: %+v %v", proposal, err)
		}
		if proposal, err := parseRequestedDecisionProposal(archived, true); err == nil || proposal.Action != "" {
			t.Fatal("live request accepted missing v4 tone")
		}
	}
	legacy := `{"action":"respond","text":"旧记录。","destination_place_id":"","activity_code":"","introduce_self":false}`
	if proposal, err := parseProposal(legacy); err != nil || proposal.Text != "旧记录。" || proposal.SpeechTone != "" {
		t.Fatalf("legacy flat record changed: %+v %v", proposal, err)
	}
	if proposal, err := parseRequestedDecisionProposal(legacy, true); err == nil || proposal.Action != "" {
		t.Fatal("live v4 accepted legacy root")
	}
}

func TestDecisionWireV4ToneRejectsUnknownMissingDuplicateAndPrivateFields(t *testing.T) {
	valid := wireDecision(`{"action":"respond","text":"好。","speech_tone":"gentle","introduce_self":false,"expression_code":"none"}`)
	for _, raw := range []string{
		strings.Replace(valid, `"speech_tone":"gentle",`, "", 1),
		strings.Replace(valid, `"speech_tone":"gentle"`, `"speech_tone":null`, 1),
		strings.Replace(valid, `"speech_tone":"gentle"`, `"speech_tone":""`, 1),
		strings.Replace(valid, `"speech_tone":"gentle"`, `"speech_tone":"angry"`, 1),
		strings.Replace(valid, `"speech_tone":"gentle"`, `"speech_tone":true`, 1),
		strings.Replace(valid, `"speech_tone":"gentle"`, `"speech_tone":"gentle","speech_tone":"firm"`, 1),
		strings.Replace(valid, `"emotion":"平静"`, `"emotion":"平静","speech_tone":"gentle"`, 1),
		strings.Replace(valid, `"observable":`, `"speech_tone":"gentle","observable":`, 1),
		strings.Replace(valid, `"speech_tone":"gentle"`, `"speech_tone":"gentle","emotion":"平静"`, 1),
		wireDecision(`{"action":"silence","expression_code":"none","speech_tone":"none"}`),
		wireDecision(`{"action":"wait","expression_code":"none","speech_tone":"gentle"}`),
		wireDecision(`{"action":"act","activity_code":"read","speech_tone":"flat"}`),
		wireDecision(`{"action":"leave","destination_place_id":"home","speech_tone":"firm"}`),
	} {
		if proposal, err := parseRequestedDecisionProposal(raw, true); err == nil || proposal.Action != "" {
			t.Fatalf("tone crossed contract: %+v %v / %s", proposal, err, raw)
		}
	}
}

func TestDecisionV4AllTransportsRequireAndPreserveSpeechTone(t *testing.T) {
	for _, format := range []string{"json_schema", "json_object", "tool_call"} {
		for _, missing := range []bool{false, true} {
			t.Run(format+map[bool]string{false: "/valid", true: "/missing"}[missing], func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var body struct{ Messages []struct{ Content string } }
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) < 2 {
						t.Error("missing request", err)
						return
					}
					var packet decisionContext
					if err := json.Unmarshal([]byte(body.Messages[1].Content), &packet); err != nil || packet.Version != "corerp.decision.v4" {
						t.Error("request is not v4", err)
					}
					raw := wireDecision(`{"action":"respond","text":"我在。","speech_tone":"gentle","introduce_self":false,"expression_code":"none"}`)
					if missing {
						raw = strings.Replace(raw, `"speech_tone":"gentle",`, "", 1)
					}
					if format == "tool_call" {
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"tool_calls": []any{proposalTool(raw)}}}}})
					} else {
						modelResponse(w, raw, "stop")
					}
				}))
				defer server.Close()
				p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: format, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := p.Propose(context.Background(), contextFixture())
				if missing {
					if err == nil || proposal.Action != "" || calls > 2 {
						t.Fatalf("transport accepted omitted tone: %+v %v calls=%d", proposal, err, calls)
					}
				} else if err != nil || proposal.SpeechTone != "gentle" || proposal.Text != "我在。" || calls != 1 {
					t.Fatalf("transport changed tone: %+v %v calls=%d", proposal, err, calls)
				}
			})
		}
	}
}

func TestDecisionV4SchemaOffersDeliveryOnlyOnSpeech(t *testing.T) {
	schema, err := decisionResponseSchema(contextFixture())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(encoded, &root); err != nil {
		t.Fatal(err)
	}
	variants := root["properties"].(map[string]any)["observable"].(map[string]any)["anyOf"].([]any)
	for _, raw := range variants {
		variant := raw.(map[string]any)
		fields := variant["properties"].(map[string]any)
		action := fields["action"].(map[string]any)["enum"].([]any)[0].(string)
		tone, present := fields["speech_tone"]
		if present != (action == "respond" || action == "refuse") {
			t.Fatalf("tone field on %s", action)
		}
		if present && !reflect.DeepEqual(tone.(map[string]any)["enum"], []any{"none", "gentle", "firm", "teasing", "hesitant", "flat"}) {
			t.Fatal("schema delivery enum drift")
		}
	}
}
