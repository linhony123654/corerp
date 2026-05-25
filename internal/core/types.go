package core

import "time"

// --- Event Sourcing ---

type Event struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Actor         string                 `json:"actor"`
	Target        string                 `json:"target"`
	Payload       map[string]interface{} `json:"payload"`
	Causes        []Cause                `json:"causes"`      // TODO(P3)
	Effects       []StateEffect          `json:"effects"`     // TODO(P3)
	Canonical     bool                   `json:"canonical"`
	Confidence    float64                `json:"confidence"`  // TODO(P2)
	Confirmations int                    `json:"confirmations"` // TODO(P2)
	SceneID       string                 `json:"scene_id"`
	SessionID     string                 `json:"session_id"`
	CreatedAt     time.Time              `json:"created_at"`
}

type Cause struct {
	EventID string  `json:"event_id"`
	Weight  float64 `json:"weight"`
}

type StateEffect struct {
	Path      string  `json:"path"`
	Delta     float64 `json:"delta"`
	Condition string  `json:"condition"`
}

// --- World State ---

type WorldTime struct {
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
	Day    int `json:"day"`
}

type SceneState struct {
	Location    string   `json:"location"`
	TimeOfDay   string   `json:"time_of_day"`
	Weather     string   `json:"weather"`
	Characters  []string `json:"characters"`
	Description string   `json:"description"`
}

type Relationship struct {
	Trust     float64 `json:"trust"`
	Intimacy  float64 `json:"intimacy"`
	Fear      float64 `json:"fear"`
	Respect   float64 `json:"respect"`
	Debt      float64 `json:"debt"`
	LastScene string  `json:"last_scene"`
}

type WorldState struct {
	Clock         WorldTime                `json:"clock"`
	Scene         SceneState               `json:"scene"`
	Relationships map[string]Relationship  `json:"relationships"`
	Variables     map[string]interface{}   `json:"variables"`
	Flags         map[string]bool          `json:"flags"`
	Tension       float64                  `json:"tension"`
}

// --- Snapshot ---

type PersonaFrame struct {
	Name        string            `json:"name"`
	Immutable   []string          `json:"immutable"`
	Adaptive    map[string]float64 `json:"adaptive"`
	Forbidden   []string          `json:"forbidden"`
	VoiceStyle  string            `json:"voice_style"`
	VoiceRhythm string            `json:"voice_rhythm"`
}

type GoalFrame struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
	Type     string `json:"type"` // primary, secondary, hidden
	Target   string `json:"target"`
	Condition string `json:"condition"`
}

type FactFrame struct {
	Subject   string  `json:"subject"`
	Predicate string  `json:"predicate"`
	Object    string  `json:"object"`
	Confidence float64 `json:"confidence"`
}

type EventFrame struct {
	EventID     string  `json:"event_id"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
	EmotionalWeight float64 `json:"emotional_weight"`
}

type Message struct {
	Role    string `json:"role"` // user, assistant
	Content string `json:"content"`
}

type WorldSnapshot struct {
	CoreRules      string        `json:"core_rules"`
	PersonaState   PersonaFrame  `json:"persona_state"`
	SceneState     SceneState    `json:"scene_state"`
	ActiveGoals    []GoalFrame   `json:"active_goals"`
	WorkingMemory  string        `json:"working_memory"`
	SemanticFacts  []FactFrame   `json:"semantic_facts"`
	EpisodicEvents []EventFrame  `json:"episodic_events"`
	RecentDialogue []Message     `json:"recent_dialogue"`
	AllowedActions []string      `json:"allowed_actions"`

	TokenBudget    int           `json:"token_budget"`
	UsedTokens     int           `json:"used_tokens"`
}

// --- Action ---

type EmotionState struct {
	Primary   string  `json:"primary"`
	Secondary string  `json:"secondary"`
	Intensity float64 `json:"intensity"`
}

type ActionFrame struct {
	Actor         string        `json:"actor"`
	Action        string        `json:"action"`
	Target        string        `json:"target"`
	Intensity     int           `json:"intensity"`
	Emotion       EmotionState  `json:"emotion"`
	Intent        string        `json:"intent"`
	SuggestedLine string        `json:"suggested_line"`
	Effects       []StateEffect `json:"effects"`
}

// --- Memory ---

type Memory struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"` // short_term, working, semantic, episodic
	Content   string  `json:"content"`
	Character string  `json:"character"`
	Score     float64 `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Agent / Identity ---

type IdentityEnvelope struct {
	Name      string            `json:"name"`
	Immutable []string          `json:"immutable"`
	Adaptive  map[string]float64 `json:"adaptive"`
	Forbidden []string          `json:"forbidden"`
	Voice     VoiceConfig       `json:"voice"`
}

type VoiceConfig struct {
	Style  string `json:"style"`
	Rhythm string `json:"rhythm"`
}

type Goal struct {
	ID              string `json:"id"`
	Priority        int    `json:"priority"`
	Type            string `json:"type"` // primary, secondary, hidden
	Target          string `json:"target"`
	Condition       string `json:"condition"`
	KnownBy         []string `json:"known_by"`
	RevealCondition string `json:"reveal_condition"`
}

type Character struct {
	Identity IdentityEnvelope `json:"identity"`
	Goals    []Goal           `json:"goals"`
}

// --- LLM ---

type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMRequest struct {
	Model       string       `json:"model"`
	Messages    []LLMMessage `json:"messages"`
	Stream      bool         `json:"stream"`
	Temperature float64      `json:"temperature"`
	MaxTokens   int          `json:"max_tokens"`
}

type LLMStreamChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}
