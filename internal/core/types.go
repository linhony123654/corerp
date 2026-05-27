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
	Location    string   `json:"location" yaml:"location"`
	TimeOfDay   string   `json:"time_of_day" yaml:"time_of_day"`
	Weather     string   `json:"weather" yaml:"weather"`
	Characters  []string `json:"characters" yaml:"characters"`
	Description string   `json:"description" yaml:"description"`
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
	Name           string `json:"name" yaml:"name"`
	Description    string `json:"description" yaml:"description"`
	BoundCharacter string `json:"bound_character" yaml:"bound_character"`
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
	Character string    `json:"character"` // actor/focus persona key; kept for compatibility
	Score     float64   `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// MemorySnapshot is scoped to one focus persona.
// Character is retained as the legacy JSON field name for that focus key.
type MemorySnapshot struct {
	Character      string       `json:"character"`
	FocusCharacter string       `json:"focus_character,omitempty"`
	WorkingMemory  string       `json:"working_memory"`
	Facts          []FactFrame  `json:"facts"`
	Episodic       []EventFrame `json:"episodic"`
	Dialogue       []Message    `json:"dialogue"`
}

// SaveSlot stores a world snapshot plus the focus persona used when the slot was created.
type SaveSlot struct {
	Name           string     `json:"name"`
	Branch         string     `json:"branch"`
	EventID        string     `json:"event_id"`
	Character      string     `json:"character"` // focus persona at save time; kept for compatibility
	FocusCharacter string     `json:"focus_character,omitempty"`
	PlayerRole     PlayerRole `json:"player_role"`
	Note           string     `json:"note"`
	Preview        string     `json:"preview"`
	CreatedAt      time.Time  `json:"created_at"`
	WorldState     WorldState `json:"world_state,omitempty"`
}

// ScenarioPreset stores a reusable scene/world opening plus its default focus persona.
type ScenarioPreset struct {
	Name           string     `json:"name" yaml:"name"`
	Branch         string     `json:"branch" yaml:"branch"`
	Character      string     `json:"character" yaml:"character"` // default focus persona; kept for compatibility
	FocusCharacter string     `json:"focus_character,omitempty" yaml:"focus_character,omitempty"`
	PlayerRole     PlayerRole `json:"player_role" yaml:"player_role"`
	Note           string     `json:"note" yaml:"note"`
	Preview        string     `json:"preview" yaml:"preview"`
	CreatedAt      time.Time  `json:"created_at" yaml:"created_at"`
	Scene          SceneState `json:"scene" yaml:"scene"`
}

// CharacterConfig is the editable definition for a focus persona.
// Character is retained as the legacy identifier field name.
type CharacterConfig struct {
	Character      string    `json:"character"`
	FocusCharacter string    `json:"focus_character,omitempty"`
	Path           string    `json:"path"`
	WorldPath      string    `json:"world_path"`
	Card           Character `json:"card"`
}

type WorldConfig struct {
	Name      string `json:"name"`
	CoreRules string `json:"core_rules"`
	Path      string `json:"path"`
	Format    string `json:"format"`
}

type WorldRule struct {
	ID          string   `json:"id" yaml:"id"`
	Title       string   `json:"title" yaml:"title"`
	Summary     string   `json:"summary" yaml:"summary"`
	Constraints []string `json:"constraints" yaml:"constraints"`
	Effects     []string `json:"effects" yaml:"effects"`
}

type WorldRulesetConfig struct {
	Path  string      `json:"path"`
	Rules []WorldRule `json:"rules"`
}

type WorldSeedConfig struct {
	Path             string                 `json:"path"`
	Premise          string                 `json:"premise" yaml:"premise"`
	CurrentSituation string                 `json:"current_situation" yaml:"current_situation"`
	StartingScene    string                 `json:"starting_scene" yaml:"starting_scene"`
	TimeAnchor       string                 `json:"time_anchor" yaml:"time_anchor"`
	Stability        string                 `json:"stability" yaml:"stability"`
	Variables        map[string]interface{} `json:"variables" yaml:"variables"`
}

type WorldFactionConfig struct {
	ID            string   `json:"id" yaml:"id"`
	Name          string   `json:"name" yaml:"name"`
	Role          string   `json:"role" yaml:"role"`
	Description   string   `json:"description" yaml:"description"`
	Goals         []string `json:"goals" yaml:"goals"`
	Relationships []string `json:"relationships" yaml:"relationships"`
}

type WorldLocationConfig struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Kind        string   `json:"kind" yaml:"kind"`
	Description string   `json:"description" yaml:"description"`
	Controller  string   `json:"controller" yaml:"controller"`
	Tags        []string `json:"tags" yaml:"tags"`
}

type WorldPressureConfig struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Kind        string   `json:"kind" yaml:"kind"`
	Description string   `json:"description" yaml:"description"`
	Intensity   float64  `json:"intensity" yaml:"intensity"`
	Target      string   `json:"target" yaml:"target"`
	Escalates   []string `json:"escalates" yaml:"escalates"`
}

type WorldStructureConfig struct {
	Path      string                `json:"path"`
	Ruleset   WorldRulesetConfig    `json:"ruleset"`
	Seed      WorldSeedConfig       `json:"seed"`
	Factions  []WorldFactionConfig  `json:"factions"`
	Locations []WorldLocationConfig `json:"locations"`
	Pressures []WorldPressureConfig `json:"pressures"`
}

type PopulationAttention struct {
	DirectInteractions int     `json:"direct_interactions" yaml:"direct_interactions"`
	Mentions           int     `json:"mentions" yaml:"mentions"`
	SharedEvents       int     `json:"shared_events" yaml:"shared_events"`
	RelationshipDelta  float64 `json:"relationship_delta" yaml:"relationship_delta"`
	SceneCarryover     int     `json:"scene_carryover" yaml:"scene_carryover"`
	Score              float64 `json:"score" yaml:"score"`
}

type BackgroundNPC struct {
	ID        string              `json:"id" yaml:"id"`
	Name      string              `json:"name" yaml:"name"`
	Role      string              `json:"role" yaml:"role"`
	Location  string              `json:"location" yaml:"location"`
	Faction   string              `json:"faction" yaml:"faction"`
	Traits    []string            `json:"traits" yaml:"traits"`
	Hooks     []string            `json:"hooks" yaml:"hooks"`
	Attention PopulationAttention `json:"attention" yaml:"attention"`
}

type PromotedNPC struct {
	ID           string              `json:"id" yaml:"id"`
	Name         string              `json:"name" yaml:"name"`
	From         string              `json:"from" yaml:"from"`
	Status       string              `json:"status" yaml:"status"`
	IdentityCore string              `json:"identity_core" yaml:"identity_core"`
	Attention    PopulationAttention `json:"attention" yaml:"attention"`
	LastEventID  string              `json:"last_event_id" yaml:"last_event_id"`
}

type IdentityCoreConfig struct {
	ID          string             `json:"id" yaml:"id"`
	Name        string             `json:"name" yaml:"name"`
	Immutable   []string           `json:"immutable" yaml:"immutable"`
	Adaptive    map[string]float64 `json:"adaptive" yaml:"adaptive"`
	SpeechHints []string           `json:"speech_hints" yaml:"speech_hints"`
	Drives      []string           `json:"drives" yaml:"drives"`
}

type PromotionPolicy struct {
	PromoteThreshold   float64 `json:"promote_threshold" yaml:"promote_threshold"`
	MajorThreshold     float64 `json:"major_threshold" yaml:"major_threshold"`
	InteractionWeight  float64 `json:"interaction_weight" yaml:"interaction_weight"`
	MentionWeight      float64 `json:"mention_weight" yaml:"mention_weight"`
	EventWeight        float64 `json:"event_weight" yaml:"event_weight"`
	RelationshipWeight float64 `json:"relationship_weight" yaml:"relationship_weight"`
	SceneWeight        float64 `json:"scene_weight" yaml:"scene_weight"`
}

type PopulationConfig struct {
	Path           string               `json:"path"`
	BackgroundNPCs []BackgroundNPC      `json:"background_npcs"`
	PromotedNPCs   []PromotedNPC        `json:"promoted_npcs"`
	IdentityCores  []IdentityCoreConfig `json:"identity_cores"`
	Policy         PromotionPolicy      `json:"policy"`
}

type PopulationCharacterInsight struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Status        string                  `json:"status,omitempty"`
	IdentityCore  string                  `json:"identity_core,omitempty"`
	Attention     PopulationAttention     `json:"attention"`
	LastEventID   string                  `json:"last_event_id,omitempty"`
	Adaptive      map[string]float64      `json:"adaptive,omitempty"`
	Immutable     []string                `json:"immutable,omitempty"`
	SpeechHints   []string                `json:"speech_hints,omitempty"`
	Drives        []string                `json:"drives,omitempty"`
	WorldPath     string                  `json:"world_path,omitempty"`
	GrowthSummary string                  `json:"growth_summary,omitempty"`
	History       []PopulationGrowthEvent `json:"history,omitempty"`
}

type PopulationInsights struct {
	Path       string                       `json:"path"`
	WorldPath  string                       `json:"world_path"`
	Promoted   []PopulationCharacterInsight `json:"promoted"`
	Background []PopulationCharacterInsight `json:"background"`
}

type PopulationGrowthEvent struct {
	EventID   string             `json:"event_id"`
	Type      string             `json:"type"`
	Summary   string             `json:"summary"`
	Adaptive  map[string]float64 `json:"adaptive,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

type WorldSummary struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Path               string `json:"path"`
	Format             string `json:"format"`
	SceneCount         int    `json:"scene_count"`
	CharacterCount     int    `json:"character_count"`
	LocationCount      int    `json:"location_count"`
	FactionCount       int    `json:"faction_count"`
	ItemCount          int    `json:"item_count"`
	EventCount         int    `json:"event_count"`
	TimelineCount      int    `json:"timeline_count"`
	BackgroundNPCCount int    `json:"background_npc_count"`
	PromotedNPCCount   int    `json:"promoted_npc_count"`
	IdentityCoreCount  int    `json:"identity_core_count"`
	LoadedCharacter    string `json:"loaded_character,omitempty"`
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
	ID             string    `json:"id"`
	Character      string    `json:"character"`
	FocusCharacter string    `json:"focus_character,omitempty"`
	Subject        string    `json:"subject"`
	Predicate      string    `json:"predicate"`
	Object         string    `json:"object"`
	Source         string    `json:"source"`
	Confidence     float64   `json:"confidence"`
	Confirmations  int       `json:"confirmations"`
	CreatedAt      time.Time `json:"created_at"`
}

type DirectorConfig struct {
	Mode        string             `json:"mode" yaml:"mode"`
	MaxSpeakers int                `json:"max_speakers" yaml:"max_speakers"`
	Weights     map[string]float64 `json:"weights,omitempty" yaml:"weights,omitempty"`
}

type ParticipantSummary struct {
	Name       string `json:"name"`
	Kind       string `json:"kind,omitempty"`
	Source     string `json:"source,omitempty"`
	WorldPath  string `json:"world_path,omitempty"`
	Loaded     bool   `json:"loaded"`
	Switchable bool   `json:"switchable"`
	Present    bool   `json:"present"`
	Focus      bool   `json:"focus,omitempty"`
}

type RuntimeInstanceSummary struct {
	ID                 string               `json:"id"`
	Label              string               `json:"label"`
	WorldName          string               `json:"world_name"`
	ActiveCharacter    string               `json:"active_character"`
	FocusCharacter     string               `json:"focus_character,omitempty"`
	LoadedCharacters   []string             `json:"loaded_characters"`
	Participants       []string             `json:"participants,omitempty"`
	ParticipantDetails []ParticipantSummary `json:"participant_details,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	IsDefault          bool                 `json:"is_default"`
	Status             string               `json:"status"`
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
	Mode             string              `json:"mode"`
	Trigger          string              `json:"trigger"`
	PreviousSpeaker  string              `json:"previous_speaker"`
	Selected         []string            `json:"selected"`
	Candidates       []string            `json:"candidates"`
	CandidateDetails []DirectorCandidate `json:"candidate_details,omitempty"`
	Steps            []TurnStep          `json:"steps"`
	Reason           string              `json:"reason"`
	Switched         bool                `json:"switched"`
	CreatedAt        time.Time           `json:"created_at"`
}

type DirectorCandidate struct {
	Name           string             `json:"name"`
	Score          float64            `json:"score"`
	Reason         string             `json:"reason,omitempty"`
	Kind           string             `json:"kind,omitempty"`
	Source         string             `json:"source,omitempty"`
	Loaded         bool               `json:"loaded,omitempty"`
	Switchable     bool               `json:"switchable,omitempty"`
	Mentioned      bool               `json:"mentioned,omitempty"`
	Present        bool               `json:"present,omitempty"`
	LocationMatch  bool               `json:"location_match,omitempty"`
	FactionMatch   bool               `json:"faction_match,omitempty"`
	PressureMatch  bool               `json:"pressure_match,omitempty"`
	HookMatch      bool               `json:"hook_match,omitempty"`
	ScoreBreakdown map[string]float64 `json:"score_breakdown,omitempty"`
	Selected       bool               `json:"selected,omitempty"`
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
	Turn               int                  `json:"turn"`
	Character          string               `json:"character"`
	FocusCharacter     string               `json:"focus_character,omitempty"`
	UserInput          string               `json:"user_input"`
	DirectorPlan       DirectorPlan         `json:"director_plan"`
	ParticipantDetails []ParticipantSummary `json:"participant_details,omitempty"`
	StepTraces         []TurnStepTrace      `json:"step_traces"`
	ActiveGoals        []TraceGoal          `json:"active_goals"`
	AllowedActions     []string             `json:"allowed_actions"`
	Memories           []TraceMemory        `json:"memories"`
	SemanticFacts      []TraceFact          `json:"semantic_facts"`
	EpisodicEvents     []EventFrame         `json:"episodic_events"`
	WorkingMemory      string               `json:"working_memory"`
	ActionFrame        ActionFrame          `json:"action_frame"`
	Validator          ValidatorTrace       `json:"validator"`
	Narrative          string               `json:"narrative"`
	CreatedAt          time.Time            `json:"created_at"`
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
