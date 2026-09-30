package decision

import (
	"strconv"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

// FromEnvironment reads only explicit CoreRP configuration, not other tools'
// credentials. A partially configured model is an error, not a silent fallback.
func FromEnvironment(get func(string) string) (core.RPDecisionProvider, string, error) {
	mode := strings.TrimSpace(get("CORERP_DECISION_PROVIDER"))
	if mode == "" {
		mode = "deterministic"
	}
	if mode == "deterministic" {
		for _, key := range []string{"CORERP_LLM_ENDPOINT", "CORERP_LLM_MODEL", "CORERP_LLM_API_KEY", "CORERP_LLM_TIMEOUT", "CORERP_LLM_ATTEMPTS", "CORERP_LLM_REASONING_EFFORT", "CORERP_LLM_DISABLE_THINKING", "CORERP_LLM_PROPOSAL_REPAIRS", "CORERP_LLM_INTERACTION_MAX_TOKENS", "CORERP_LLM_DECISION_MAX_TOKENS", "CORERP_LLM_DECISION_FORMAT"} {
			if get(key) != "" {
				return nil, "", failure("LLM settings require explicit chat_completions mode")
			}
		}
		return core.DeterministicRPDecisionProvider{}, mode, nil
	}
	if mode != "chat_completions" {
		return nil, "", failure("unknown provider mode")
	}
	config := Config{Endpoint: get("CORERP_LLM_ENDPOINT"), Model: get("CORERP_LLM_MODEL"), APIKey: get("CORERP_LLM_API_KEY"), ReasoningEffort: get("CORERP_LLM_REASONING_EFFORT"), DecisionFormat: get("CORERP_LLM_DECISION_FORMAT")}
	policy, err := endpointpolicy.FromEnvironment(get("CORERP_PROVIDER_ALLOWLIST"), get("CORERP_PROVIDER_LOCAL_ALLOWLIST"))
	if err != nil {
		return nil, "", failure("invalid endpoint allowlist")
	}
	config.EndpointPolicy = policy.WithOperatorEndpoint(config.Endpoint)
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
	if value := get("CORERP_LLM_PROPOSAL_REPAIRS"); value != "" {
		count, err := strconv.Atoi(value)
		if err != nil || count < 0 || count > 1 {
			return nil, "", failure("invalid proposal repair configuration")
		}
		config.ProposalRepairs = count
	}
	if value := get("CORERP_LLM_INTERACTION_MAX_TOKENS"); value != "" {
		count, err := strconv.Atoi(value)
		if err != nil || count < 512 || count > 4096 {
			return nil, "", failure("invalid interaction token budget")
		}
		config.InteractionMaxTokens = count
	}
	if value := get("CORERP_LLM_DECISION_MAX_TOKENS"); value != "" {
		count, err := strconv.Atoi(value)
		if err != nil || count < 512 || count > 8192 {
			return nil, "", failure("invalid decision token budget")
		}
		config.DecisionMaxTokens = count
	}
	if value := get("CORERP_LLM_DISABLE_THINKING"); value != "" {
		if value != "true" && value != "false" {
			return nil, "", failure("invalid thinking configuration")
		}
		config.DisableThinking = value == "true"
	}
	provider, err := NewChatProvider(config)
	if err != nil {
		return nil, "", err
	}
	return provider, mode, nil
}
