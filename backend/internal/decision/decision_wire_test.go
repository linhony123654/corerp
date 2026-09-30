package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

const wirePrivateFixture = `{"intent":"先听清对方的请求","emotion":"平静","relationship_stance":"礼貌","basis_event_ids":["event-player"]}`

func wireDecision(observable string) string {
	return `{"private":` + wirePrivateFixture + `,"observable":` + observable + `}`
}

func TestDecisionWireV3KeepsOneObservableAndPrivateSketch(t *testing.T) {
	in := contextFixture()
	in.InterlocutorEntityID = "player"
	in.LegalActions = append(in.LegalActions, "act")
	in.LegalActivities = []string{"tend_counter"}
	for _, tc := range []struct{ action, observable string }{
		{"respond", `{"action":"respond","text":"你好。","introduce_self":false,"expression_code":"nod"}`},
		{"refuse", `{"action":"refuse","text":"今天不方便。","introduce_self":false,"expression_code":"none"}`},
		{"silence", `{"action":"silence","expression_code":"smile"}`},
		{"wait", `{"action":"wait","expression_code":"none"}`},
		{"leave", `{"action":"leave","destination_place_id":"home"}`},
		{"act", `{"action":"act","activity_code":"tend_counter"}`},
	} {
		t.Run(tc.action, func(t *testing.T) {
			proposal, err := parseProposal(wireDecision(tc.observable))
			if err != nil || proposal.Action != tc.action || proposal.Private == nil || proposal.Private.Intent != "先听清对方的请求" {
				t.Fatalf("typed observable/private contract failed: %+v / %v", proposal, err)
			}
			if err := core.ValidateRPDecisionProposal(in, proposal); err != nil {
				t.Fatal("wire introduced a proposal forbidden by the existing owner contract", err)
			}
			if tc.action == "silence" || tc.action == "wait" || tc.action == "act" || tc.action == "leave" {
				if proposal.Text != "" || proposal.IntroduceSelf {
					t.Fatal("non-speech acquired an utterance or identity declaration")
				}
			}
			if tc.action == "leave" || tc.action == "act" {
				if proposal.ExpressionCode != "" {
					t.Fatal("movement/activity acquired an extra gesture")
				}
			}
		})
	}
}

func TestDecisionWireV3RejectsMixedDuplicateAndExtraEffects(t *testing.T) {
	valid := wireDecision(`{"action":"silence","expression_code":"none"}`)
	for _, raw := range []string{
		strings.Replace(valid, `"observable":`, `"action":"silence","observable":`, 1),
		strings.Replace(valid, `"private":`, `"private":`+wirePrivateFixture+`,"private":`, 1),
		strings.Replace(valid, `"observable":`, `"observable":{"action":"wait","expression_code":"none"},"observable":`, 1),
		wireDecision(`{"action":"silence","action":"wait","expression_code":"none"}`),
		wireDecision(`{"action":"silence","expression_code":"none","text":""}`),
		wireDecision(`{"action":"silence","expression_code":"none","text":"我在。"}`),
		wireDecision(`{"action":"respond","text":"好。","introduce_self":false,"expression_code":"none","destination_place_id":"home"}`),
		wireDecision(`{"action":"leave","destination_place_id":"home","expression_code":"smile"}`),
		wireDecision(`{"action":"act","activity_code":"tend_counter","text":""}`),
		wireDecision(`{"action":"wait","expression_code":""}`),
		wireDecision(`{"action":"wait","expression_code":null}`),
		wireDecision(`{"action":"wait","expression_code":"none","private":{}}`),
		wireDecision(`{"action":"respond","text":"好。","expression_code":"none"}`),
		wireDecision(`{"action":"leave","destination_place_id":null}`),
		wireDecision(`{"action":"wait"}`),
		`{"observable":{"action":"wait","expression_code":"none"}}`,
		`{"private":null,"observable":{"action":"wait","expression_code":"none"}}`,
		wireDecision(`null`),
		valid + ` {}`,
	} {
		if proposal, err := parseProposal(raw); err == nil || proposal.Action != "" {
			t.Errorf("mixed/private/observable effects were accepted or silently dropped: %s / %+v", raw, proposal)
		}
	}
}

func TestDecisionWireV3StillUsesAuthoritativeGroundingAndActions(t *testing.T) {
	in := contextFixture()
	in.LegalActions = append(in.LegalActions, "act")
	in.LegalActivities = []string{"tend_counter"}
	for _, raw := range []string{
		wireDecision(`{"action":"leave","destination_place_id":"secret-vault"}`),
		wireDecision(`{"action":"act","activity_code":"conjure_tea"}`),
		strings.Replace(wireDecision(`{"action":"wait","expression_code":"none"}`), `event-player`, `event-unheard`, 1),
	} {
		proposal, err := parseProposal(raw)
		if err != nil {
			t.Fatal("structurally valid wire could not reach the authoritative gate", err)
		}
		if core.ValidateRPDecisionProposal(in, proposal) == nil {
			t.Fatal("new wire bypassed allowed destinations/activities/knowledge")
		}
	}
}

func TestDecisionSchemaV3OffersOnlyApplicableSourcedFields(t *testing.T) {
	in := contextFixture()
	in.LegalActions = append(in.LegalActions, "act")
	in.LegalActivities = []string{"tend_counter"}
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var captured map[string]any
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Error(err)
		}
		requests <- captured
		modelResponse(w, wireDecision(`{"action":"wait","expression_code":"none"}`), "stop")
	}))
	defer server.Close()
	p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "test", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Propose(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	captured := <-requests
	format := captured["response_format"].(map[string]any)["json_schema"].(map[string]any)
	root := format["schema"].(map[string]any)
	properties := root["properties"].(map[string]any)
	if format["name"] != "corerp_decision_v3" || len(properties) != 2 || root["additionalProperties"] != false || !reflect.DeepEqual(root["required"], []any{"private", "observable"}) {
		t.Fatal("request did not separate private and observable at a closed root")
	}
	variants := properties["observable"].(map[string]any)["anyOf"].([]any)
	want := map[string][]any{
		"respond": {"action", "text", "introduce_self", "expression_code"}, "refuse": {"action", "text", "introduce_self", "expression_code"},
		"silence": {"action", "expression_code"}, "wait": {"action", "expression_code"},
		"leave": {"action", "destination_place_id"}, "act": {"action", "activity_code"},
	}
	for _, raw := range variants {
		variant := raw.(map[string]any)
		fields := variant["properties"].(map[string]any)
		action := fields["action"].(map[string]any)["enum"].([]any)[0].(string)
		if variant["additionalProperties"] != false || !reflect.DeepEqual(variant["required"], want[action]) || len(fields) != len(want[action]) {
			t.Errorf("%s can generate unrelated effect slots", action)
		}
		delete(want, action)
		if action == "leave" && !reflect.DeepEqual(fields["destination_place_id"].(map[string]any)["enum"], []any{"home"}) {
			t.Error("unsourced destination offered by the schema")
		}
		if action == "act" && !reflect.DeepEqual(fields["activity_code"].(map[string]any)["enum"], []any{"tend_counter"}) {
			t.Error("undeclared activity offered by the schema")
		}
	}
	if len(want) != 0 {
		t.Fatal("schema omitted a legal action", want)
	}
}

func TestDecisionSchemaV3OmitsUnusableAffordancesWithoutEmptyEnums(t *testing.T) {
	in := contextFixture()
	in.InterlocutorEntityID = "unseen-speaker"
	in.ReachablePlaceIDs = nil
	in.LegalActions = append(in.LegalActions, "act")
	inspect := func(input core.RPDecisionInput) (map[string]any, bool) {
		t.Helper()
		schema, err := decisionResponseSchema(input)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(schema)
		if err != nil || strings.Contains(string(encoded), `"enum":[]`) || !strings.Contains(string(encoded), `"properties":{"private":`) || strings.Count(string(encoded), `"properties":{"action":`) != 4 {
			t.Fatal("empty domain or ordering broke the response grammar", err)
		}
		var wire map[string]any
		_ = json.Unmarshal(encoded, &wire)
		root := wire["properties"].(map[string]any)
		variants := root["observable"].(map[string]any)["anyOf"].([]any)
		var beckon bool
		for _, variant := range variants {
			fields := variant.(map[string]any)["properties"].(map[string]any)
			action := fields["action"].(map[string]any)["enum"].([]any)[0]
			if action == "leave" || action == "act" {
				t.Fatal("schema offered an action with no authorized argument")
			}
			for _, expression := range fields["expression_code"].(map[string]any)["enum"].([]any) {
				beckon = beckon || expression == "beckon"
			}
		}
		return wire, beckon
	}
	if _, beckon := inspect(in); beckon {
		t.Fatal("unseen interlocutor licensed a directed gesture")
	}
	in.VisibleEntities = []core.RPDecisionVisibleEntity{{EntityID: in.InterlocutorEntityID, DisplayName: "陌生人"}}
	if _, beckon := inspect(in); !beckon {
		t.Fatal("a visible interlocutor's legal beckon was removed")
	}
	in.LegalActions = []string{"leave", "act"}
	if _, err := decisionResponseSchema(in); err == nil {
		t.Fatal("a context without a constructible action was accepted")
	}
}

func TestDecisionPrivateNullFieldsAreNotConvertedToEmptyState(t *testing.T) {
	for _, name := range []string{"intent", "emotion", "relationship_stance", "basis_event_ids"} {
		var private map[string]any
		_ = json.Unmarshal([]byte(wirePrivateFixture), &private)
		private[name] = nil
		raw, err := json.Marshal(private)
		if err != nil {
			t.Fatal(err)
		}
		for _, frame := range []string{
			`{"private":` + string(raw) + `,"observable":{"action":"wait","expression_code":"none"}}`,
			`{"action":"wait","text":"","destination_place_id":"","activity_code":"","introduce_self":false,"expression_code":"none","private":` + string(raw) + `}`,
		} {
			if proposal, err := parseProposal(frame); err == nil || proposal.Action != "" {
				t.Errorf("%s null became invented empty private state: %+v / %v", name, proposal, err)
			}
		}
	}
}

func TestDecisionWireV3FieldConflictHasOnlyOneRepair(t *testing.T) {
	for _, repaired := range []bool{false, true} {
		t.Run(fmt.Sprint(repaired), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Messages []struct{ Content string } }
				_ = json.NewDecoder(r.Body).Decode(&body)
				call := calls.Add(1)
				if call == 2 && (len(body.Messages) != 3 || !strings.Contains(body.Messages[2].Content, "observable")) {
					t.Error("repair failed to describe the actual typed contract")
				}
				observable := `{"action":"silence","expression_code":"none","text":"我在。"}`
				if call == 2 && repaired {
					observable = `{"action":"silence","expression_code":"none"}`
				}
				modelResponse(w, wireDecision(observable), "stop")
			}))
			defer server.Close()
			p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "test", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			trace := &core.RPProviderTrace{}
			proposal, err := p.Propose(core.WithRPProviderTrace(context.Background(), trace), contextFixture())
			if calls.Load() != 2 || trace.AttemptCount() != 2 || (err == nil) != repaired {
				t.Fatalf("field conflict repair escaped its budget: %+v / %v / calls=%d", proposal, err, calls.Load())
			}
		})
	}
}
