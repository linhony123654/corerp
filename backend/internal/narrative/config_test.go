package narrative

import "testing"

func TestNarrativeConfigurationIndependentAndFailClosed(t *testing.T) {
	get := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	// Existing decision configuration is not inherited or consumed.
	_, mode, err := FromEnvironment(get(map[string]string{"CORERP_LLM_ENDPOINT": "https://not-narrative.invalid", "CORERP_LLM_API_KEY": "private-decision-key"}))
	if err != nil || mode != "deterministic" {
		t.Fatalf("decision credentials affected narrator: %s %v", mode, err)
	}
	for _, values := range []map[string]string{
		{"CORERP_NARRATIVE_API_KEY": "private"},
		{"CORERP_NARRATIVE_PROVIDER": "unknown"},
		{"CORERP_NARRATIVE_PROVIDER": "style_planner"},
		{"CORERP_NARRATIVE_PROVIDER": "style_planner", "CORERP_NARRATIVE_ENDPOINT": "http://remote.invalid", "CORERP_NARRATIVE_MODEL": "x", "CORERP_NARRATIVE_API_KEY": "private"},
		{"CORERP_NARRATIVE_PROVIDER": "style_planner", "CORERP_NARRATIVE_ENDPOINT": "http://localhost", "CORERP_NARRATIVE_MODEL": "x", "CORERP_NARRATIVE_ATTEMPTS": "4"},
		{"CORERP_NARRATIVE_PROVIDER": "style_planner", "CORERP_NARRATIVE_ENDPOINT": "http://localhost", "CORERP_NARRATIVE_MODEL": "x", "CORERP_NARRATIVE_TIMEOUT": "31s"},
	} {
		if _, _, err := FromEnvironment(get(values)); err == nil {
			t.Fatal("invalid narrative configuration accepted")
		}
	}
	if p, mode, err := FromEnvironment(get(map[string]string{"CORERP_NARRATIVE_PROVIDER": "style_planner", "CORERP_NARRATIVE_ENDPOINT": "http://127.0.0.1:1/v1/chat/completions", "CORERP_NARRATIVE_MODEL": "fixture"})); err != nil || p == nil || mode != "style_planner" {
		t.Fatalf("explicit independent config rejected: %s %v", mode, err)
	}
}
