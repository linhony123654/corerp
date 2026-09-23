package decision

import (
	"strconv"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// FromEnvironment reads only explicit CoreRP configuration, not other tools'
// credentials. A partially configured model is an error, not a silent fallback.
func FromEnvironment(get func(string) string) (core.RPDecisionProvider, string, error) {
	mode := strings.TrimSpace(get("CORERP_DECISION_PROVIDER"))
	if mode == "" {
		mode = "deterministic"
	}
	if mode == "deterministic" {
		for _, key := range []string{"CORERP_LLM_ENDPOINT", "CORERP_LLM_MODEL", "CORERP_LLM_API_KEY", "CORERP_LLM_TIMEOUT", "CORERP_LLM_ATTEMPTS"} {
			if get(key) != "" {
				return nil, "", failure("LLM settings require explicit chat_completions mode")
			}
		}
		return core.DeterministicRPDecisionProvider{}, mode, nil
	}
	if mode != "chat_completions" {
		return nil, "", failure("unknown provider mode")
	}
	config := Config{Endpoint: get("CORERP_LLM_ENDPOINT"), Model: get("CORERP_LLM_MODEL"), APIKey: get("CORERP_LLM_API_KEY")}
	if value := get("CORERP_LLM_TIMEOUT"); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return nil, "", failure("invalid timeout configuration")
		}
		config.Timeout = timeout
	}
	if value := get("CORERP_LLM_ATTEMPTS"); value != "" {
		count, err := strconv.Atoi(value)
		if err != nil || count < 1 {
			return nil, "", failure("invalid attempt configuration")
		}
		config.Attempts = count
	}
	provider, err := NewChatProvider(config)
	if err != nil {
		return nil, "", err
	}
	return provider, mode, nil
}
