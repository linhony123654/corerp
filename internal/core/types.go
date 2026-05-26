package core

import "time"

// --- Event Sourcing ---

type Event struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Actor         string                 `json:"actor"`
	Target        string                 `json:"target"`
	Payload       map[string]interface{} `json:"payload"`
	Causes        []Cause                `json:"causes"`
	Effects       []StateEffect          `json:"effects"`
	Canonical     bool                   `json:"canonical"`
	Confidence    float64                `json:"confidence"`
	Confirmations int                    `json:"confirmations"`
	SceneID       string                 `json:"scene_id"`
	SessionID     string                 `json:"session_id"`
	Branch        string                 `json:"branch"`
	Hash          string                 `json:"hash"` // chained event hash for tamper detection
	Tag           string                 `json:"tag"`  // narrative / system / tick / maintenance
	CreatedAt     time.Time              `json:"created_at"`
}

// EventTag constants for filtering causal graphs and timeline views.
const (
	TagNarrative   = "narrative"   // player/NPC action-driven
	TagSystem      = "system"      // clock advance, scene init
	TagTick        = "tick"        // simulation tick events (decay, pressure)
	TagMaintenance = "maintenance" // compression, snapshot
	TagUser        = "user"        // direct user input
)

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
	Clock         WorldTime               `json:"clock"`
	Scene         SceneState              `json:"scene"`
	Relationships map[string]Relationship `json:"relationships"`
	Variables     map[string]interface{}  `json:"variables"`
	Flags         map[string]bool         `json:"flags"`
	Tension       float64                 `json:"tension"`
}

type PlayerRole struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	BoundCharacter string `json:"bound_character"`
}

// --- Snapshot ---

type PersonaFrame struct {
	Name         string             `json:"name"`
	Immutable    []string           `json:"immutable"`
	Adaptive     map[string]float64 `json:"adaptive"`
	Forbidden    []string           `json:"forbidden"`
	VoiceStyle   string             `json:"voice_style"`
	VoiceRhythm  string             `json:"voice_rhythm"`
	WritingGuide string             `json:"writing_guide"`
}

type GoalFrame struct {
	ID        string `json:"id"`
	Priority  int    `json:"priority"`
	Type      string `json:"type"` // primary, secondary, hidden
	Target    string `json:"target"`
	Condition string `json:"condition"`
}

type FactFrame struct {
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	Confidence float64 `json:"confidence"`
}

type EventFrame struct {
	EventID         string  `json:"event_id"`
	Type            string  `json:"type"`
	Description     string  `json:"description"`
	EmotionalWeight float64 `json:"emotional_weight"`
}

type Message struct {
	Role    string `json:"role"` // user, assistant
	Content string `json:"content"`
}

type WorldSnapshot struct {
	CoreRules      string       `json:"core_rules"`
	PersonaState   PersonaFrame `json:"persona_state"`
	PlayerRole     PlayerRole   `json:"player_role"`
	SceneState     SceneState   `json:"scene_state"`
	ActiveGoals    []GoalFrame  `json:"active_goals"`
	WorkingMemory  string       `json:"working_memory"`
	SemanticFacts  []FactFrame  `json:"semantic_facts"`
	EpisodicEvents []EventFrame `json:"episodic_events"`
	RecentDialogue []Message    `json:"recent_dialogue"`
	AllowedActions []string     `json:"allowed_actions"`

	TokenBudget int `json:"token_budget"`
	UsedTokens  int `json:"used_tokens"`
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
	ID        string    `json:"id"`
	Type      string    `json:"type"` // short_term, working, semantic, episodic
	Content   string    `json:"content"`
	Character string    `json:"character"`
	Score     float64   `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

type MemorySnapshot struct {
	Character     string       `json:"character"`
	WorkingMemory string       `json:"working_memory"`
	Facts         []FactFrame  `json:"facts"`
	Episodic      []EventFrame `json:"episodic"`
	Dialogue      []Message    `json:"dialogue"`
}

type SaveSlot struct {
	Name       string     `json:"name"`
	Branch     string     `json:"branch"`
	EventID    string     `json:"event_id"`
	Character  string     `json:"character"`
	PlayerRole PlayerRole `json:"player_role"`
	Note       string     `json:"note"`
	Preview    string     `json:"preview"`
	CreatedAt  time.Time  `json:"created_at"`
	WorldState WorldState `json:"world_state,omitempty"`
}

type ScenarioPreset struct {
	Name       string     `json:"name" yaml:"name"`
	Branch     string     `json:"branch" yaml:"branch"`
	Character  string     `json:"character" yaml:"character"`
	PlayerRole PlayerRole `json:"player_role" yaml:"player_role"`
	Note       string     `json:"note" yaml:"note"`
	Preview    string     `json:"preview" yaml:"preview"`
	CreatedAt  time.Time  `json:"created_at" yaml:"created_at"`
	Scene      SceneState `json:"scene" yaml:"scene"`
}

type CharacterConfig struct {
	Character string    `json:"character"`
	Path      string    `json:"path"`
	WorldPath string    `json:"world_path"`
	Card      Character `json:"card"`
}

type WorldConfig struct {
	Name      string `json:"name"`
	CoreRules string `json:"core_rules"`
	Path      string `json:"path"`
	Format    string `json:"format"`
}

type SceneConfig struct {
	Name  string     `json:"name"`
	Path  string     `json:"path"`
	Scene SceneState `json:"scene"`
}

type SceneConfigList struct {
	Selected string        `json:"selected"`
	Scenes   []SceneConfig `json:"scenes"`
}

type CanonFactsConfig struct {
	Path  string      `json:"path"`
	Facts []FactFrame `json:"facts"`
}

type PendingFact struct {
	ID            string    `json:"id"`
	Character     string    `json:"character"`
	Subject       string    `json:"subject"`
	Predicate     string    `json:"predicate"`
	Object        string    `json:"object"`
	Source        string    `json:"source"`
	Confidence    float64   `json:"confidence"`
	Confirmations int       `json:"confirmations"`
	CreatedAt     time.Time `json:"created_at"`
}

type DirectorConfig struct {
	Mode        string `json:"mode"`
	MaxSpeakers int    `json:"max_speakers"`
}

type RuntimeInstanceSummary struct {
	ID               string    `json:"id"`
	Label            string    `json:"label"`
	WorldName        string    `json:"world_name"`
	ActiveCharacter  string    `json:"active_character"`
	LoadedCharacters []string  `json:"loaded_characters"`
	CreatedAt        time.Time `json:"created_at"`
	IsDefault        bool      `json:"is_default"`
	Status           string    `json:"status"`
}

type TurnStep struct {
	ID         string `json:"id"`
	Index      int    `json:"index"`
	Speaker    string `json:"speaker"`
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	BudgetMode string `json:"budget_mode"`
}

type DirectorPlan struct {
	Mode            string     `json:"mode"`
	Trigger         string     `json:"trigger"`
	PreviousSpeaker string     `json:"previous_speaker"`
	Selected        []string   `json:"selected"`
	Candidates      []string   `json:"candidates"`
	Steps           []TurnStep `json:"steps"`
	Reason          string     `json:"reason"`
	Switched        bool       `json:"switched"`
	CreatedAt       time.Time  `json:"created_at"`
}

type StateDiffEntry struct {
	A interface{} `json:"a"`
	B interface{} `json:"b"`
}

type WorldStateDiff struct {
	BranchA       string                    `json:"branch_a,omitempty"`
	BranchB       string                    `json:"branch_b,omitempty"`
	SaveA         string                    `json:"save_a,omitempty"`
	SaveB         string                    `json:"save_b,omitempty"`
	Scene         map[string]StateDiffEntry `json:"scene,omitempty"`
	Clock         *StateDiffEntry           `json:"clock,omitempty"`
	Tension       *StateDiffEntry           `json:"tension,omitempty"`
	Flags         map[string]StateDiffEntry `json:"flags,omitempty"`
	Variables     map[string]StateDiffEntry `json:"variables,omitempty"`
	Relationships map[string]StateDiffEntry `json:"relationships,omitempty"`
}

type BranchMergeResult struct {
	SourceBranch    string `json:"source_branch"`
	TargetBranch    string `json:"target_branch"`
	FlagsMerged     int    `json:"flags_merged"`
	VariablesMerged int    `json:"variables_merged"`
	EventsAppended  int    `json:"events_appended"`
}

type TraceMemory struct {
	Type    string  `json:"type"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type TraceFact struct {
	Subject   string  `json:"subject"`
	Predicate string  `json:"predicate"`
	Object    string  `json:"object"`
	Score     float64 `json:"score"`
}

type TraceGoal struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Priority  int    `json:"priority"`
	Condition string `json:"condition"`
}

type ValidatorTrace struct {
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason"`
}

type TraceEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor"`
	Target    string    `json:"target"`
	Canonical bool      `json:"canonical"`
	Branch    string    `json:"branch"`
	CreatedAt time.Time `json:"created_at"`
}

type StepHandoff struct {
	FromSpeaker    string       `json:"from_speaker"`
	StepIndex      int          `json:"step_index"`
	Kind           string       `json:"kind"`
	Action         string       `json:"action"`
	Target         string       `json:"target"`
	OutcomeSummary string       `json:"outcome_summary"`
	Narrative      string       `json:"narrative"`
	Events         []TraceEvent `json:"events"`
}

type TurnStepTrace struct {
	Step           TurnStep       `json:"step"`
	Character      string         `json:"character"`
	Handoff        *StepHandoff   `json:"handoff,omitempty"`
	ActiveGoals    []TraceGoal    `json:"active_goals"`
	AllowedActions []string       `json:"allowed_actions"`
	Memories       []TraceMemory  `json:"memories"`
	SemanticFacts  []TraceFact    `json:"semantic_facts"`
	EpisodicEvents []EventFrame   `json:"episodic_events"`
	WorkingMemory  string         `json:"working_memory"`
	ActionFrame    ActionFrame    `json:"action_frame"`
	Validator      ValidatorTrace `json:"validator"`
	Narrative      string         `json:"narrative"`
	Events         []TraceEvent   `json:"events"`
	TokenBudget    int            `json:"token_budget"`
	UsedTokens     int            `json:"used_tokens"`
	Error          string         `json:"error,omitempty"`
}

type TurnTrace struct {
	Turn           int             `json:"turn"`
	Character      string          `json:"character"`
	UserInput      string          `json:"user_input"`
	DirectorPlan   DirectorPlan    `json:"director_plan"`
	StepTraces     []TurnStepTrace `json:"step_traces"`
	ActiveGoals    []TraceGoal     `json:"active_goals"`
	AllowedActions []string        `json:"allowed_actions"`
	Memories       []TraceMemory   `json:"memories"`
	SemanticFacts  []TraceFact     `json:"semantic_facts"`
	EpisodicEvents []EventFrame    `json:"episodic_events"`
	WorkingMemory  string          `json:"working_memory"`
	ActionFrame    ActionFrame     `json:"action_frame"`
	Validator      ValidatorTrace  `json:"validator"`
	Narrative      string          `json:"narrative"`
	CreatedAt      time.Time       `json:"created_at"`
}

// --- Agent / Identity ---

type IdentityEnvelope struct {
	Name         string             `json:"name"`
	Immutable    []string           `json:"immutable"`
	Adaptive     map[string]float64 `json:"adaptive"`
	Forbidden    []string           `json:"forbidden"`
	Voice        VoiceConfig        `json:"voice"`
	WritingGuide string             `json:"writing_guide"`
}

type VoiceConfig struct {
	Style  string `json:"style"`
	Rhythm string `json:"rhythm"`
}

type Goal struct {
	ID              string   `json:"id"`
	Priority        int      `json:"priority"`
	Type            string   `json:"type"` // primary, secondary, hidden
	Target          string   `json:"target"`
	Condition       string   `json:"condition"`
	CooldownTurns   int      `json:"cooldown_turns,omitempty"`
	KnownBy         []string `json:"known_by"`
	RevealCondition string   `json:"reveal_condition"`
}

type Character struct {
	WorldPath string           `json:"world_path,omitempty"`
	Identity  IdentityEnvelope `json:"identity"`
	Goals     []Goal           `json:"goals"`
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
