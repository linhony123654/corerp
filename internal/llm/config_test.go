package llm

import "testing"

func TestSetActiveConfigFullPreservesPricing(t *testing.T) {
	SetActiveConfigFull(APIConfig{
		Name:            "priced",
		Endpoint:        "http://example.test/v1",
		APIKey:          "secret",
		Model:           "demo",
		PromptPrice:     2.5,
		CompletionPrice: 9.5,
	})

	cfg := GetActiveConfig()
	if cfg.PromptPrice != 2.5 || cfg.CompletionPrice != 9.5 {
		t.Fatalf("pricing = (%v,%v), want (2.5,9.5)", cfg.PromptPrice, cfg.CompletionPrice)
	}
}

func TestSetActiveConfigDefaultsPricing(t *testing.T) {
	SetActiveConfig("default", "http://example.test/v1", "secret", "demo")

	cfg := GetActiveConfig()
	if cfg.PromptPrice != 1.0 || cfg.CompletionPrice != 4.0 {
		t.Fatalf("pricing = (%v,%v), want defaults (1.0,4.0)", cfg.PromptPrice, cfg.CompletionPrice)
	}
}
