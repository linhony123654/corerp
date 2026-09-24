package narrative

import (
	"corerp.local/backend/internal/core"
	"strconv"
	"strings"
	"time"
)

// Never borrow DecisionProvider or another tool's endpoint/credentials.
func FromEnvironment(get func(string) string) (core.RPStreamingNarrativeProvider, string, error) {
	mode := strings.TrimSpace(get("CORERP_NARRATIVE_PROVIDER"))
	if mode == "" {
		mode = "deterministic"
	}
	if mode == "deterministic" {
		for _, suffix := range []string{"ENDPOINT", "MODEL", "API_KEY", "TIMEOUT", "ATTEMPTS"} {
			if get("CORERP_NARRATIVE_"+suffix) != "" {
				return nil, "", failure("narrative settings require explicit style_planner mode")
			}
		}
		return core.DeterministicRPNarrativeProvider{}, mode, nil
	}
	if mode != "style_planner" {
		return nil, "", failure("unknown provider mode")
	}
	c := Config{Endpoint: get("CORERP_NARRATIVE_ENDPOINT"), Model: get("CORERP_NARRATIVE_MODEL"), APIKey: get("CORERP_NARRATIVE_API_KEY")}
	if value := get("CORERP_NARRATIVE_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return nil, "", failure("invalid timeout configuration")
		}
		c.Timeout = duration
	}
	if value := get("CORERP_NARRATIVE_ATTEMPTS"); value != "" {
		attempts, err := strconv.Atoi(value)
		if err != nil || attempts < 1 {
			return nil, "", failure("invalid attempts configuration")
		}
		c.Attempts = attempts
	}
	planner, err := NewChatStylePlanner(c)
	if err != nil {
		return nil, "", err
	}
	return core.PlannedRPNarrativeProvider{Planner: planner}, mode, nil
}
