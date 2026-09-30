package core

// RPProviderMetadata identifies the selected adapter without exposing an
// endpoint, credential or request body to presentation and diagnostics.
type RPProviderMetadata struct {
	Kind  string `json:"kind"`
	Model string `json:"model,omitempty"`
}

// RPModelOverride carries a player-supplied model configuration for a single
// request. It travels only as a decoded HTTP field and a resolved in-memory
// provider; it must never be added to request structs that are pinned or
// persisted (speech/wait/interaction requests are stored for recovery).
type RPModelOverride struct {
	Endpoint       string `json:"endpoint"`
	Model          string `json:"model"`
	APIKey         string `json:"api_key"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	// Optional decision/interpretation tuning. Omitted values retain adapter
	// defaults; provider-specific switches are never inferred from a model name.
	ReasoningEffort      string `json:"reasoning_effort,omitempty"`
	DisableThinking      bool   `json:"disable_thinking,omitempty"`
	DecisionMaxTokens    int    `json:"decision_max_tokens,omitempty"`
	InteractionMaxTokens int    `json:"interaction_max_tokens,omitempty"`
	// Transport format only for NPC decisions; does not alter world validation
	// or select a different context, input interpreter, or narrator.
	DecisionFormat string `json:"decision_format,omitempty"`
	// FullProse selects the novel-paragraph narrative renderer instead of the
	// style planner for this request's presentation.
	FullProse bool `json:"full_prose,omitempty"`
}
