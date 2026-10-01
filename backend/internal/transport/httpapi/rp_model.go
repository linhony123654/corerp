package httpapi

import (
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/decision"
	"corerp.local/backend/internal/endpointpolicy"
	"corerp.local/backend/internal/narrative"
)

// Player-supplied model overrides are decoded per request and resolved into
// in-memory providers here. The configuration never enters persisted request
// structs, is never logged, and an invalid or incomplete override is an
// explicit error rather than a silent fallback to the operator default.

func rpOverrideTimeout(seconds int, maximum time.Duration) time.Duration {
	if seconds < 1 {
		return 0
	}
	if seconds >= int(maximum/time.Second) {
		return maximum
	}
	return time.Duration(seconds) * time.Second
}

func resolveRPDecisionOverride(override *core.RPModelOverride, policy endpointpolicy.Policy) (core.RPDecisionProvider, error) {
	if override == nil {
		return nil, nil
	}
	provider, err := decision.NewChatProvider(decision.Config{
		Endpoint:             override.Endpoint,
		Model:                override.Model,
		APIKey:               override.APIKey,
		Timeout:              rpOverrideTimeout(override.TimeoutSeconds, decision.MaxRequestTimeout),
		ReasoningEffort:      override.ReasoningEffort,
		DisableThinking:      override.DisableThinking,
		DecisionMaxTokens:    override.DecisionMaxTokens,
		InteractionMaxTokens: override.InteractionMaxTokens,
		DecisionFormat:       override.DecisionFormat,
		EndpointPolicy:       policy,
	})
	if err != nil {
		return nil, core.WrapError(core.CodeInvalidArgument, "模型配置无效，请在模型与 API 设置中修正或改用系统默认", err)
	}
	return provider, nil
}

func resolveRPNarrativeOverride(override *core.RPModelOverride, policy endpointpolicy.Policy) (core.RPStreamingNarrativeProvider, error) {
	if override == nil {
		return nil, nil
	}
	if override.FullProse {
		prose, err := narrative.NewChatProseProvider(narrative.Config{
			Endpoint:        override.Endpoint,
			Model:           override.Model,
			APIKey:          override.APIKey,
			Timeout:         rpOverrideTimeout(override.TimeoutSeconds, 90*time.Second),
			ReasoningEffort: override.ReasoningEffort,
			DisableThinking: override.DisableThinking,
			EndpointPolicy:  policy,
		})
		if err != nil {
			return nil, core.WrapError(core.CodeInvalidArgument, "模型配置无效，请在模型与 API 设置中修正或改用系统默认", err)
		}
		return prose, nil
	}
	planner, err := narrative.NewChatStylePlanner(narrative.Config{
		Endpoint:        override.Endpoint,
		Model:           override.Model,
		APIKey:          override.APIKey,
		Timeout:         rpOverrideTimeout(override.TimeoutSeconds, 30*time.Second),
		ReasoningEffort: override.ReasoningEffort,
		DisableThinking: override.DisableThinking,
		EndpointPolicy:  policy,
	})
	if err != nil {
		return nil, core.WrapError(core.CodeInvalidArgument, "模型配置无效，请在模型与 API 设置中修正或改用系统默认", err)
	}
	return core.PlannedRPNarrativeProvider{Planner: planner}, nil
}
