package decision

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sort"
	"strings"

	"corerp.local/backend/internal/core"
)

const decisionProposalFunction = "propose_rp_decision"

// Native calls serialize a proposal; they never dispatch a function. Only the
// one declared name and complete v3 arguments are accepted at this boundary.
func decisionFunctionArguments(raw json.RawMessage) (string, error) {
	fields, err := decisionObjectFields(string(raw))
	if err != nil {
		return "", err
	}
	for name := range fields {
		if name != "id" && name != "index" && name != "type" && name != "function" {
			return "", failure("invalid proposal schema")
		}
	}
	var kind string
	if json.Unmarshal(fields["type"], &kind) != nil || kind != "function" {
		return "", failure("invalid proposal schema")
	}
	function, err := decisionObjectFields(string(fields["function"]))
	if err != nil || len(function) != 2 {
		return "", failure("invalid proposal schema")
	}
	var name, arguments string
	if json.Unmarshal(function["name"], &name) != nil || name != decisionProposalFunction || json.Unmarshal(function["arguments"], &arguments) != nil || strings.TrimSpace(arguments) == "" {
		return "", failure("invalid proposal schema")
	}
	return arguments, nil
}

func parseRequestedDecisionProposal(raw string, requireV3 bool) (core.RPDecisionProposal, error) {
	if requireV3 {
		fields, err := decisionObjectFields(raw)
		if err != nil {
			return core.RPDecisionProposal{}, err
		}
		if len(fields) != 2 || fields["private"] == nil || fields["observable"] == nil {
			return core.RPDecisionProposal{}, failure("invalid proposal schema")
		}
	}
	return parseProposal(raw)
}

// The wire describes a single proposed observable, not an action plus a bag
// of possible effects. Internal core proposals and owner validation stay unchanged.
var decisionObservableFields = map[string][]string{
	"respond": {"action", "text", "introduce_self", "expression_code"},
	"refuse":  {"action", "text", "introduce_self", "expression_code"},
	"silence": {"action", "expression_code"},
	"wait":    {"action", "expression_code"},
	"leave":   {"action", "destination_place_id"},
	"act":     {"action", "activity_code"},
}

func decisionResponseSchema(input core.RPDecisionInput) (map[string]any, error) {
	destinations := decisionWireChoices(input.ReachablePlaceIDs)
	activities := decisionWireChoices(input.LegalActivities)
	expressions := []string{"none", "smile", "nod", "shake_head", "turn_away", "frown"}
	if input.InterlocutorEntityID != "" && slices.ContainsFunc(input.VisibleEntities, func(actor core.RPDecisionVisibleEntity) bool { return actor.EntityID == input.InterlocutorEntityID }) {
		expressions = append(expressions, "beckon")
	}
	values := map[string]any{
		"text":                 map[string]any{"type": "string", "minLength": 1, "maxLength": 2000},
		"introduce_self":       map[string]any{"type": "boolean"},
		"expression_code":      map[string]any{"type": "string", "enum": expressions},
		"destination_place_id": map[string]any{"type": "string", "enum": destinations},
		"activity_code":        map[string]any{"type": "string", "enum": activities},
	}
	variants := make([]map[string]any, 0, len(decisionObservableFields))
	for _, action := range []string{"respond", "refuse", "silence", "wait", "leave", "act"} {
		if !slices.Contains(input.LegalActions, action) || action == "leave" && len(destinations) == 0 || action == "act" && len(activities) == 0 {
			continue
		}
		fields := decisionObservableFields[action]
		// Reuse the existing ordered schema encoder: action precedes its
		// arguments, and the private sketch precedes the observable decision.
		properties := interactionSchemaProperties{{"action", map[string]any{"type": "string", "enum": []string{action}}}}
		for _, field := range fields[1:] {
			properties = append(properties, interactionSchemaProperty{field, values[field]})
		}
		variants = append(variants, map[string]any{"type": "object", "additionalProperties": false, "required": fields, "properties": properties})
	}
	if len(variants) == 0 {
		return nil, failure("no legal action")
	}
	refs := make([]string, 0)
	for _, source := range decisionSourceRefs(input) {
		refs = append(refs, source.Ref)
	}
	basisItems := map[string]any{"type": "string"}
	if len(refs) > 0 {
		basisItems["enum"] = refs
	}
	private := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"intent", "emotion", "relationship_stance", "basis_event_ids"},
		"properties": interactionSchemaProperties{
			{"intent", decisionPrivateTextSchema("Current motivation", 160)},
			{"emotion", decisionPrivateTextSchema("Current emotional stance", 80)},
			{"relationship_stance", decisionPrivateTextSchema("Stance toward the current interlocutor", 80)},
			{"basis_event_ids", map[string]any{"type": "array", "maxItems": min(8, len(refs)), "items": basisItems, "description": "Distinct ref values from grounding_sources in this request, each at most once. The server binds them to actual Event IDs. Use [] if uncertain; do not copy, invent or approximate long IDs."}},
		},
	}
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"private", "observable"},
		"properties": interactionSchemaProperties{{"private", private}, {"observable", map[string]any{"anyOf": variants}}},
	}, nil
}

func decisionPrivateTextSchema(meaning string, limit int) map[string]any {
	return map[string]any{
		"type": "string", "maxLength": limit,
		"description": meaning + ": one short single-line phrase, not reasoning or a list. No NUL, CR or LF characters. Use an empty string if unspecified.",
	}
}

func decisionWireChoices(values []string) []string {
	choices := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			choices = append(choices, value)
		}
	}
	sort.Strings(choices)
	return slices.Compact(choices)
}

// Read before choosing a wire version so duplicates and mixed roots cannot
// be hidden by ordinary map decoding or a permissive fallback.
func decisionObjectFields(raw string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, failure("invalid proposal schema")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, failure("invalid proposal schema")
		}
		name, ok := token.(string)
		if !ok {
			return nil, failure("invalid proposal schema")
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, failure("duplicate proposal field")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || strings.TrimSpace(string(value)) == "null" {
			return nil, failure("invalid proposal schema")
		}
		fields[name] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, failure("invalid proposal schema")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, failure("trailing proposal data")
	}
	return fields, nil
}

func parseProposal(raw string) (core.RPDecisionProposal, error) {
	var empty core.RPDecisionProposal
	fields, err := decisionObjectFields(raw)
	if err != nil {
		return empty, err
	}
	observable, compact := fields["observable"]
	if !compact {
		// Strict v1/v2 responses remain readable. Do not remove bad fields or
		// turn one into another action; the same owner gate must accept it.
		return parseLegacyProposal(raw)
	}
	if len(fields) != 2 || fields["private"] == nil {
		return empty, failure("invalid proposal schema")
	}
	private, err := parsePrivateDecision(fields["private"])
	if err != nil {
		return empty, err
	}
	proposal, err := parseDecisionObservable(string(observable))
	if err != nil {
		return empty, err
	}
	proposal.Private = &private
	return proposal, nil
}

func parseDecisionObservable(raw string) (core.RPDecisionProposal, error) {
	var empty core.RPDecisionProposal
	fields, err := decisionObjectFields(raw)
	if err != nil {
		return empty, err
	}
	var action string
	if fields["action"] == nil || json.Unmarshal(fields["action"], &action) != nil {
		return empty, failure("invalid proposal schema")
	}
	expected, known := decisionObservableFields[action]
	if !known {
		return empty, failure("invalid proposal schema")
	}
	if len(fields) != len(expected) {
		return empty, &Error{Kind: "illegal proposal", Detail: "observable_schema_mismatch"}
	}
	for _, name := range expected {
		if fields[name] == nil {
			return empty, &Error{Kind: "illegal proposal", Detail: "observable_schema_mismatch"}
		}
	}
	proposal := core.RPDecisionProposal{Action: action}
	for _, field := range []struct {
		name string
		out  *string
	}{
		{"text", &proposal.Text}, {"destination_place_id", &proposal.DestinationPlaceID},
		{"activity_code", &proposal.ActivityCode}, {"expression_code", &proposal.ExpressionCode},
	} {
		if raw, present := fields[field.name]; present && json.Unmarshal(raw, field.out) != nil {
			return empty, failure("invalid proposal schema")
		}
	}
	if raw, present := fields["introduce_self"]; present && json.Unmarshal(raw, &proposal.IntroduceSelf) != nil {
		return empty, failure("invalid proposal schema")
	}
	if _, present := fields["expression_code"]; present {
		switch proposal.ExpressionCode {
		case "none":
			proposal.ExpressionCode = ""
		case "smile", "nod", "shake_head", "turn_away", "frown", "beckon":
		default:
			return empty, failure("invalid proposal schema")
		}
	}
	return proposal, nil
}
