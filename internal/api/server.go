package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"corerp/internal/agents"
	"corerp/internal/auth"
	"corerp/internal/core"
	"corerp/internal/events"
	"corerp/internal/goalexpr"
	"corerp/internal/llm"
	"corerp/internal/narrative"
	worldpkg "corerp/internal/world"
)

var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
	BuildTime    = "unknown"
)

var (
	errInstanceNotFound = errors.New("instance not found")
	errInstanceConflict = errors.New("instance conflict")
)

var WorldCatalogRoot = "worlds"

// RuntimeEngine is the interface the server needs from the runtime engine.
type RuntimeEngine interface {
	ProcessTurn(userInput string) (<-chan string, error)
	GetInstanceID() string
	InstanceSummary() core.RuntimeInstanceSummary
	GetState() core.WorldState
	GetFocusDefinition() (core.Character, bool)
	GetCharacter() (core.Character, bool) // compatibility accessor for current focus persona
	GetCharacterName() string
	GetFocusCharacter() string
	GetPlayerRole() core.PlayerRole
	UpdatePlayerRole(role core.PlayerRole) (core.PlayerRole, error)
	GetFocusDefinitionConfig(name string) (core.CharacterConfig, error)
	UpdateFocusDefinitionConfig(name string, card core.Character) (core.CharacterConfig, error)
	GetCharacterConfig(name string) (core.CharacterConfig, error)
	UpdateCharacterConfig(name string, card core.Character) (core.CharacterConfig, error)
	GetWorldConfig() (core.WorldConfig, error)
	UpdateWorldConfig(cfg core.WorldConfig) (core.WorldConfig, error)
	GetWorldStructureConfig() (core.WorldStructureConfig, error)
	UpdateWorldStructureConfig(cfg core.WorldStructureConfig) (core.WorldStructureConfig, error)
	ListSceneConfigs() (core.SceneConfigList, error)
	UpdateSceneConfig(scene core.SceneConfig) (core.SceneConfig, error)
	GetCanonFactsConfig() (core.CanonFactsConfig, error)
	UpdateCanonFactsConfig(cfg core.CanonFactsConfig) (core.CanonFactsConfig, error)
	GetPopulationConfig() (core.PopulationConfig, error)
	GetPopulationInsights() (core.PopulationInsights, error)
	UpdatePopulationConfig(cfg core.PopulationConfig) (core.PopulationConfig, error)
	GetDirectorConfig() core.DirectorConfig
	UpdateDirectorConfig(cfg core.DirectorConfig) core.DirectorConfig
	GetDirectorPlan() core.DirectorPlan
	GetLatestTrace() (core.TurnTrace, bool)
	ListTurnTraces(limit int) []core.TurnTrace
	GetTraceByTurn(turn int) (core.TurnTrace, bool)
	ListQuarantineEvents(character string, limit int) ([]core.Event, error)
	PromoteQuarantineEvent(eventID string) error
	RejectQuarantineEvent(eventID string) error
	ListPendingFacts(character string, limit int) ([]core.PendingFact, map[string]interface{}, error)
	ConfirmPendingFact(eventID string) error
	DeletePendingFact(eventID string) error
	PromotePendingFact(eventID string) error
	GetLoadedCharacters() []string
	GetSceneParticipants() []string
	SwitchCharacter(name string) error
	EnterWorld(path string) (core.ScenarioPreset, error)
	GetWorldName() string
	GetWorldPaths() map[string]string
	GetFocusMemorySnapshot(character string, factLimit, episodicLimit, dialogueLimit int) (core.MemorySnapshot, error)
	GetMemorySnapshot(character string, factLimit, episodicLimit, dialogueLimit int) (core.MemorySnapshot, error)
	ListSaveSlots() ([]core.SaveSlot, error)
	CreateSaveSlot(name, branch, note string) (core.SaveSlot, error)
	LoadSaveSlot(name string) (core.SaveSlot, error)
	CompareSaveSlots(saveA, saveB string) (core.WorldStateDiff, error)
	ListCheckpoints() ([]core.SaveSlot, error)
	CreateCheckpoint(name, branch, note string) (core.SaveSlot, error)
	LoadCheckpoint(name string) (core.SaveSlot, error)
	ListScenarioPresets() ([]core.ScenarioPreset, error)
	CreateScenarioPreset(name, branch, note string) (core.ScenarioPreset, error)
	ApplyScenarioPreset(name string) (core.ScenarioPreset, error)
	GetNPCActions(name string, sinceTick int) []agents.NPCActionLog
	GetCausalityChain(eventID string, depth int) (interface{}, error)
	GetCausalityChainNarrative(eventID string, depth int) (interface{}, error)
	GetCausalitySummary(eventID string, depth int) (string, error)
	GetCausalitySummaryNarrative(eventID string, depth int) (string, error)
	ReplayTo(eventID string) (core.WorldState, error)
	ReplayAtTime(hour, minute, day int) (core.WorldState, error)
	ForkTimeline(eventID, branchName string) error
	GetTimeline(branch string, limit int) ([]events.EventTimeline, error)
	ListBranches() ([]string, error)
	CompareBranchesDetailed(branchA, branchB string, index int) (core.WorldStateDiff, error)
	MergeBranchState(sourceBranch, targetBranch string, mergeFlags, mergeVariables bool) (core.BranchMergeResult, error)
	CompressEvents(from, to int) (*narrative.CompressionResult, error)
	CompressionStats() map[string]interface{}
	LLMRoutes() map[string]interface{}
	SwitchLLM(name, endpoint, apiKey, model string)
	GetDialogueLimit(limit int) []core.Message
	ResetDialogue()
	DebugInfo() map[string]interface{}
	SetTension(v float64)
	QueryActionLog(character string, firedOnly, blockedOnly bool, limit int) []interface{}
	ActionLogStats() map[string]interface{}
	TickStatus() map[string]interface{}
	ManualTick()
	PauseTick()
	ResumeTick()
	GetSceneParticipantDetails() []core.ParticipantSummary
}

type InstanceResolver interface {
	DefaultInstanceID() string
	ResolveInstance(id string) (RuntimeEngine, error)
	ListInstances() []core.RuntimeInstanceSummary
	InstanceStatus(id string) (core.RuntimeInstanceSummary, error)
	SetDefaultInstance(id string) error
	StopInstance(id string) (core.RuntimeInstanceSummary, error)
	DeleteInstance(id string) error
	CreateInstance(sourceID, id, label, focusCharacter string) (core.RuntimeInstanceSummary, error)
}

type Server struct {
	engine   RuntimeEngine
	resolver InstanceResolver
}

func NewServer(engine RuntimeEngine, resolver ...InstanceResolver) *Server {
	s := &Server{engine: engine}
	if len(resolver) > 0 {
		s.resolver = resolver[0]
	}
	return s
}

func (s *Server) Register(mux *http.ServeMux) {
	a := func(h http.HandlerFunc) http.HandlerFunc { return auth.Middleware(h) }
	mux.HandleFunc("/login", auth.HandleLogin)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/ready", s.handleReady)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/change-password", s.handleChangePassword)
	mux.HandleFunc("/api/chat", a(s.handleChat))
	mux.HandleFunc("/api/state", a(s.handleState))
	mux.HandleFunc("/api/character", a(s.handleFocusDefinitionCompat))
	mux.HandleFunc("/api/player-role", a(s.handlePlayerRole))
	mux.HandleFunc("/api/instances", a(s.handleInstances))
	mux.HandleFunc("/api/instances/status", a(s.handleInstanceStatus))
	mux.HandleFunc("/api/instances/create", a(s.handleInstanceCreate))
	mux.HandleFunc("/api/instances/default", a(s.handleInstanceDefault))
	mux.HandleFunc("/api/instances/stop", a(s.handleInstanceStop))
	mux.HandleFunc("/api/instances/delete", a(s.handleInstanceDelete))
	mux.HandleFunc("/api/character-config", a(s.handleFocusDefinitionConfigCompat))
	mux.HandleFunc("/api/characters", a(s.handleSceneParticipants))
	mux.HandleFunc("/api/switch", a(s.handleFocusSwitch))
	mux.HandleFunc("/api/world", a(s.handleWorld))
	mux.HandleFunc("/api/worlds", a(s.handleWorlds))
	mux.HandleFunc("/api/world-config", a(s.handleWorldConfig))
	mux.HandleFunc("/api/world-structure", a(s.handleWorldStructure))
	mux.HandleFunc("/api/scenes", a(s.handleScenes))
	mux.HandleFunc("/api/canon-facts", a(s.handleCanonFacts))
	mux.HandleFunc("/api/population", a(s.handlePopulation))
	mux.HandleFunc("/api/population-insights", a(s.handlePopulationInsights))
	mux.HandleFunc("/api/director-config", a(s.handleDirectorConfig))
	mux.HandleFunc("/api/trace", a(s.handleTrace))
	mux.HandleFunc("/api/traces", a(s.handleTraces))
	mux.HandleFunc("/api/trace/latest", a(s.handleTrace))
	mux.HandleFunc("/api/quarantine", a(s.handleQuarantine))
	mux.HandleFunc("/api/quarantine/promote", a(s.handleQuarantinePromote))
	mux.HandleFunc("/api/quarantine/reject", a(s.handleQuarantineReject))
	mux.HandleFunc("/api/pending-facts", a(s.handlePendingFacts))
	mux.HandleFunc("/api/pending-facts/confirm", a(s.handlePendingFactsConfirm))
	mux.HandleFunc("/api/pending-facts/delete", a(s.handlePendingFactsDelete))
	mux.HandleFunc("/api/pending-facts/promote", a(s.handlePendingFactsPromote))
	mux.HandleFunc("/api/memory", a(s.handleMemory))
	mux.HandleFunc("/api/export", a(s.handleExport))
	mux.HandleFunc("/api/saves", a(s.handleSaves))
	mux.HandleFunc("/api/saves/load", a(s.handleSaveLoad))
	mux.HandleFunc("/api/saves/diff", a(s.handleSavesDiff))
	mux.HandleFunc("/api/checkpoints", a(s.handleCheckpoints))
	mux.HandleFunc("/api/checkpoints/load", a(s.handleCheckpointLoad))
	mux.HandleFunc("/api/presets", a(s.handlePresets))
	mux.HandleFunc("/api/presets/apply", a(s.handlePresetApply))
	mux.HandleFunc("/api/npc-actions", a(s.handleNPCActions))
	mux.HandleFunc("/api/npc-action-log", a(s.handleActionLog))
	mux.HandleFunc("/api/causality", a(s.handleCausality))
	mux.HandleFunc("/api/replay", a(s.handleReplay))
	mux.HandleFunc("/api/fork", a(s.handleFork))
	mux.HandleFunc("/api/timeline", a(s.handleTimeline))
	mux.HandleFunc("/api/branches", a(s.handleBranches))
	mux.HandleFunc("/api/branches/diff", a(s.handleBranchesDiff))
	mux.HandleFunc("/api/branches/merge", a(s.handleBranchesMerge))
	mux.HandleFunc("/api/compress", a(s.handleCompress))
	mux.HandleFunc("/api/compression-stats", a(s.handleCompressionStats))
	mux.HandleFunc("/api/usage", a(s.handleUsage))
	mux.HandleFunc("/api/llm-configs", a(s.handleLLMConfigs))
	mux.HandleFunc("/api/llm-configs/", a(s.handleLLMConfigs))
	mux.HandleFunc("/api/llm-active", a(s.handleLLMActive))
	mux.HandleFunc("/api/llm-models", a(s.handleLLMModels))
	mux.HandleFunc("/api/llm-routes", a(s.handleLLMRoutes))
	mux.HandleFunc("/api/dialogue", a(s.handleDialogue))
	mux.HandleFunc("/api/dialogue/reset", a(s.handleDialogueReset))
	mux.HandleFunc("/api/debug/memory", a(s.handleDebugMemory))
	mux.HandleFunc("/api/director", a(s.handleDirector))
	mux.HandleFunc("/api/sim/status", a(s.handleSimStatus))
	mux.HandleFunc("/api/sim/tick", a(s.handleSimTick))
	mux.HandleFunc("/api/sim/pause", a(s.handleSimPause))
	mux.HandleFunc("/api/sim/resume", a(s.handleSimResume))
	mux.HandleFunc("/", a(s.handleStatic))
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeJSONWithETag marshals payload, computes a SHA256 ETag, and returns 304
// if the client's If-None-Match header matches. This avoids re-transferring
// unchanged JSON on poll-heavy endpoints.
func writeJSONWithETag(w http.ResponseWriter, r *http.Request, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hash := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(hash[:8]) + `"`

	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag)
	w.Write(body)
}

func writeInstanceError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, errInstanceNotFound), strings.HasPrefix(err.Error(), "instance not found:"):
		status = http.StatusNotFound
	case errors.Is(err, errInstanceConflict),
		strings.Contains(err.Error(), "cannot delete default instance"),
		strings.Contains(err.Error(), "cannot delete the only instance"):
		status = http.StatusConflict
	case strings.Contains(err.Error(), "instance id required"):
		status = http.StatusBadRequest
	}
	http.Error(w, err.Error(), status)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"status":  "ok",
		"service": "corerp",
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.engine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ok":     false,
			"status": "unavailable",
			"reason": "runtime unavailable",
		})
		return
	}
	instanceID := s.engine.GetInstanceID()
	if s.resolver != nil {
		instanceID = s.resolver.DefaultInstanceID()
		if _, err := s.resolver.ResolveInstance(instanceID); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"ok":           false,
				"status":       "unavailable",
				"reason":       err.Error(),
				"default":      instanceID,
				"instances":    len(s.resolver.ListInstances()),
				"build":        BuildVersion,
				"build_commit": BuildCommit,
				"build_time":   BuildTime,
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"status":  "ready",
		"default": instanceID,
		"instances": func() int {
			if s.resolver != nil {
				return len(s.resolver.ListInstances())
			}
			return 1
		}(),
		"build":        BuildVersion,
		"build_commit": BuildCommit,
		"build_time":   BuildTime,
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service": "corerp",
		"version": BuildVersion,
		"commit":  BuildCommit,
		"time":    BuildTime,
	})
}

func (s *Server) engineForRequest(r *http.Request) (RuntimeEngine, string, error) {
	if s.resolver == nil {
		return s.engine, s.engine.GetInstanceID(), nil
	}
	id := strings.TrimSpace(r.URL.Query().Get("instance_id"))
	engine, err := s.resolver.ResolveInstance(id)
	if err != nil {
		return nil, "", err
	}
	if id == "" {
		id = s.resolver.DefaultInstanceID()
	}
	return engine, id, nil
}

type chatRequest struct {
	Message string `json:"message"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ch, err := engine.ProcessTurn(req.Message)
	if err != nil {
		fmt.Fprintf(w, "data: [ERROR] %v\n\n", err)
		flusher.Flush()
		return
	}

	for chunk := range ch {
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		flusher.Flush()
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	engine, instanceID, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	state := engine.GetState()
	payload := struct {
		core.WorldState
		InstanceID     string                      `json:"instance_id"`
		Instance       core.RuntimeInstanceSummary `json:"instance"`
		DirectorConfig core.DirectorConfig         `json:"director_config"`
		DirectorPlan   core.DirectorPlan           `json:"director_plan"`
		LatestTrace    *core.TurnTrace             `json:"latest_trace,omitempty"`
	}{
		WorldState:     state,
		InstanceID:     instanceID,
		Instance:       engine.InstanceSummary(),
		DirectorConfig: engine.GetDirectorConfig(),
		DirectorPlan:   engine.GetDirectorPlan(),
	}
	if trace, ok := engine.GetLatestTrace(); ok {
		payload.LatestTrace = &trace
	}
	writeJSONWithETag(w, r, payload)
}

// handleFocusDefinitionCompat serves the legacy /api/character path while returning
// the current focus persona definition.
func (s *Server) handleFocusDefinitionCompat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	char, ok := engine.GetFocusDefinition()
	if !ok {
		http.Error(w, "focus definition not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(char)
}

func (s *Server) handlePlayerRole(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(engine.GetPlayerRole())
	case http.MethodPost:
		var req core.PlayerRole
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		role, err := engine.UpdatePlayerRole(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(role)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := struct {
		Default   string                        `json:"default"`
		Instances []core.RuntimeInstanceSummary `json:"instances"`
	}{
		Default:   s.engine.GetInstanceID(),
		Instances: []core.RuntimeInstanceSummary{s.engine.InstanceSummary()},
	}
	if s.resolver != nil {
		resp.Default = s.resolver.DefaultInstanceID()
		resp.Instances = s.resolver.ListInstances()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleInstanceCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		http.Error(w, "instance manager unavailable", http.StatusNotImplemented)
		return
	}
	var req struct {
		ID              string `json:"id"`
		Label           string `json:"label"`
		SourceID        string `json:"source_id"`
		FocusCharacter  string `json:"focus_character"`
		ActiveCharacter string `json:"active_character"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	focusCharacter := strings.TrimSpace(req.FocusCharacter)
	if focusCharacter == "" {
		focusCharacter = strings.TrimSpace(req.ActiveCharacter)
	}
	summary, err := s.resolver.CreateInstance(req.SourceID, req.ID, req.Label, focusCharacter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleInstanceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.engine.InstanceSummary())
		return
	}
	summary, err := s.resolver.InstanceStatus(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleInstanceDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		http.Error(w, "instance manager unavailable", http.StatusNotImplemented)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.resolver.SetDefaultInstance(req.ID); err != nil {
		writeInstanceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "default": req.ID})
}

func (s *Server) handleInstanceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		http.Error(w, "instance manager unavailable", http.StatusNotImplemented)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	summary, err := s.resolver.StopInstance(req.ID)
	if err != nil {
		writeInstanceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleInstanceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		http.Error(w, "instance manager unavailable", http.StatusNotImplemented)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.resolver.DeleteInstance(req.ID); err != nil {
		writeInstanceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "deleted": req.ID})
}

// handleFocusDefinitionConfigCompat serves the legacy /api/character-config path
// for reading and writing the current focus persona definition.
func (s *Server) handleFocusDefinitionConfigCompat(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	name := r.URL.Query().Get("focus_character")
	if name == "" {
		name = r.URL.Query().Get("character")
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := engine.GetFocusDefinitionConfig(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	case http.MethodPost:
		var req struct {
			FocusCharacter string         `json:"focus_character"`
			Character      string         `json:"character"`
			Card           core.Character `json:"card"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.FocusCharacter != "" {
			name = req.FocusCharacter
		} else if req.Character != "" {
			name = req.Character
		}
		if err := goalexpr.ValidateCharacter(req.Card); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg, err := engine.UpdateFocusDefinitionConfig(name, req.Card)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSceneParticipants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	chars := engine.GetSceneParticipants()
	if len(chars) == 0 {
		chars = engine.GetLoadedCharacters()
	}
	details := engine.GetSceneParticipantDetails()
	focus := engine.GetFocusCharacter()
	writeJSONWithETag(w, r, map[string]interface{}{
		"active":              focus,
		"focus_character":     focus,
		"characters":          chars,
		"participants":        chars,
		"participant_details": details,
	})
}

func (s *Server) handleWorlds(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		worlds, err := worldpkg.ListCatalog(WorldCatalogRoot, engine.GetWorldPaths())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONWithETag(w, r, map[string]interface{}{
			"active":      engine.GetWorldName(),
			"active_path": activeWorldPath(engine),
			"worlds":      worlds,
		})
	case http.MethodPost:
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		preset, err := engine.EnterWorld(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":              true,
			"world":           engine.GetWorldName(),
			"character":       engine.GetFocusCharacter(),
			"focus_character": engine.GetFocusCharacter(),
			"preset":          preset,
		})
	case http.MethodPut:
		var req struct {
			Name      string `json:"name"`
			CoreRules string `json:"core_rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dir, err := worldpkg.CreateWorld(WorldCatalogRoot, req.Name, req.CoreRules)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":   true,
			"name": req.Name,
			"path": dir,
		})
	case http.MethodPatch:
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dir, err := worldpkg.ConvertToDir(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"old_path": req.Path,
			"new_path": dir,
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func activeWorldPath(engine RuntimeEngine) string {
	focus := engine.GetFocusCharacter()
	if focus == "" {
		return ""
	}
	return engine.GetWorldPaths()[focus]
}

// handleFocusSwitch serves the legacy /api/switch path while changing the
// current focus persona/scene viewpoint.
func (s *Server) handleFocusSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		FocusCharacter string `json:"focus_character"`
		Character      string `json:"character"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.FocusCharacter = strings.TrimSpace(req.FocusCharacter)
	req.Character = strings.TrimSpace(req.Character)
	name := req.FocusCharacter
	if name == "" {
		name = req.Character
	}
	if name == "" {
		http.Error(w, "focus_character is required", http.StatusBadRequest)
		return
	}
	req.Character = name

	if req.Character != engine.GetFocusCharacter() {
		for _, participant := range engine.GetSceneParticipantDetails() {
			if participant.Name != req.Character {
				continue
			}
			if !participant.Switchable {
				http.Error(w, fmt.Sprintf("participant '%s' is not switchable (%s)", req.Character, participant.Kind), http.StatusBadRequest)
				return
			}
			break
		}
	}

	if err := engine.SwitchCharacter(req.Character); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Include recent NPC actions for "while you were away" summary
	npcActions := engine.GetNPCActions(req.Character, 0)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":          true,
		"character":   req.Character,
		"npc_actions": npcActions,
	})
}

func (s *Server) handleWorld(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"name": engine.GetWorldName(),
	})
}

func (s *Server) handleWorldConfig(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := engine.GetWorldConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	case http.MethodPost:
		var req core.WorldConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg, err := engine.UpdateWorldConfig(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorldStructure(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := engine.GetWorldStructureConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPost:
		var req core.WorldStructureConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg, err := engine.UpdateWorldStructureConfig(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleScenes(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		scenes, err := engine.ListSceneConfigs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scenes)
	case http.MethodPost:
		var req core.SceneConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		scene, err := engine.UpdateSceneConfig(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scene)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePopulation(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := engine.GetPopulationConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPost:
		var req core.PopulationConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg, err := engine.UpdatePopulationConfig(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePopulationInsights(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	insights, err := engine.GetPopulationInsights()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, insights)
}

func (s *Server) handleCanonFacts(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := engine.GetCanonFactsConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	case http.MethodPost:
		var req core.CanonFactsConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg, err := engine.UpdateCanonFactsConfig(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDirectorConfig(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"config": engine.GetDirectorConfig(),
			"plan":   engine.GetDirectorPlan(),
		})
	case http.MethodPost:
		var req core.DirectorConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg := engine.UpdateDirectorConfig(req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"config": cfg,
			"plan":   engine.GetDirectorPlan(),
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var (
		trace core.TurnTrace
		ok    bool
	)
	if turnRaw := r.URL.Query().Get("turn"); turnRaw != "" {
		var turn int
		fmt.Sscanf(turnRaw, "%d", &turn)
		trace, ok = engine.GetTraceByTurn(turn)
	} else {
		trace, ok = engine.GetLatestTrace()
	}
	if !ok {
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trace)
}

func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	limit := 20
	if n := r.URL.Query().Get("limit"); n != "" {
		fmt.Sscanf(n, "%d", &limit)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"traces": engine.ListTurnTraces(limit),
	})
}

func (s *Server) handleQuarantine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	character := r.URL.Query().Get("focus_character")
	if character == "" {
		character = r.URL.Query().Get("character")
	}
	limit := 50
	if n := r.URL.Query().Get("n"); n != "" {
		fmt.Sscanf(n, "%d", &limit)
	}
	events, err := engine.ListQuarantineEvents(character, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("include_noise") != "1" {
		filtered := events[:0]
		for _, event := range events {
			if event.Type == "observe" {
				continue
			}
			filtered = append(filtered, event)
		}
		events = filtered
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events":          events,
		"count":           len(events),
		"focus_character": character,
		"character":       character,
	})
}

func (s *Server) handleQuarantinePromote(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.handleQuarantineAction(w, r, engine.PromoteQuarantineEvent)
}

func (s *Server) handleQuarantineReject(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.handleQuarantineAction(w, r, engine.RejectQuarantineEvent)
}

func (s *Server) handlePendingFacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	character := r.URL.Query().Get("focus_character")
	if character == "" {
		character = r.URL.Query().Get("character")
	}
	limit := 50
	if n := r.URL.Query().Get("n"); n != "" {
		fmt.Sscanf(n, "%d", &limit)
	}
	items, stats, err := engine.ListPendingFacts(character, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"facts":           items,
		"count":           len(items),
		"stats":           stats,
		"focus_character": character,
		"character":       character,
	})
}

func (s *Server) handlePendingFactsConfirm(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.handleFactAction(w, r, engine.ConfirmPendingFact)
}

func (s *Server) handlePendingFactsDelete(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.handleFactAction(w, r, engine.DeletePendingFact)
}

func (s *Server) handlePendingFactsPromote(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.handleFactAction(w, r, engine.PromotePendingFact)
}

func (s *Server) handleQuarantineAction(w http.ResponseWriter, r *http.Request, fn func(string) error) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := fn(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "id": req.ID})
}

func (s *Server) handleFactAction(w http.ResponseWriter, r *http.Request, fn func(string) error) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := fn(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "id": req.ID})
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	character := r.URL.Query().Get("focus_character")
	if character == "" {
		character = r.URL.Query().Get("character")
	}
	facts := 50
	episodic := 20
	dialogue := 20
	if n := r.URL.Query().Get("facts"); n != "" {
		fmt.Sscanf(n, "%d", &facts)
	}
	if n := r.URL.Query().Get("episodic"); n != "" {
		fmt.Sscanf(n, "%d", &episodic)
	}
	if n := r.URL.Query().Get("dialogue"); n != "" {
		fmt.Sscanf(n, "%d", &dialogue)
	}
	snapshot, err := engine.GetFocusMemorySnapshot(character, facts, episodic, dialogue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	focusCharacter := snapshot.FocusCharacter
	if strings.TrimSpace(focusCharacter) == "" {
		focusCharacter = strings.TrimSpace(snapshot.Character)
	}
	writeJSONWithETag(w, r, map[string]interface{}{
		"character":       snapshot.Character,
		"focus_character": focusCharacter,
		"working_memory":  snapshot.WorkingMemory,
		"facts":           snapshot.Facts,
		"episodic":        snapshot.Episodic,
		"dialogue":        snapshot.Dialogue,
	})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	limit := 50
	if n := r.URL.Query().Get("limit"); n != "" {
		fmt.Sscanf(n, "%d", &limit)
	}

	focusDefinition, _ := engine.GetFocusDefinition()
	dialogue := engine.GetDialogueLimit(limit)
	timeline, _ := engine.GetTimeline("main", limit)
	payload := map[string]interface{}{
		"exported_at":      time.Now().UTC(),
		"character":        focusDefinition,
		"focus_definition": focusDefinition,
		"focus_character":  engine.GetFocusCharacter(),
		"world":            engine.GetWorldName(),
		"state":            engine.GetState(),
		"dialogue":         dialogue,
		"timeline":         timeline,
	}

	filename := fmt.Sprintf("corerp-%s-%s", engine.GetFocusCharacter(), time.Now().UTC().Format("20060102T150405Z"))
	switch format {
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.md\"", filename))
		fmt.Fprintf(w, "# CoreRP Session Export\n\n")
		fmt.Fprintf(w, "- Character: %s\n", focusDefinition.Identity.Name)
		fmt.Fprintf(w, "- World: %s\n", engine.GetWorldName())
		fmt.Fprintf(w, "- Exported: %s\n\n", time.Now().UTC().Format(time.RFC3339))
		state := engine.GetState()
		fmt.Fprintf(w, "## Scene\n\n")
		fmt.Fprintf(w, "- Day %d %02d:%02d\n", state.Clock.Day, state.Clock.Hour, state.Clock.Minute)
		fmt.Fprintf(w, "- Location: %s\n", state.Scene.Location)
		fmt.Fprintf(w, "- Weather: %s\n", state.Scene.Weather)
		fmt.Fprintf(w, "- Tension: %.2f\n\n", state.Tension)
		fmt.Fprintf(w, "## Dialogue\n\n")
		for _, msg := range dialogue {
			fmt.Fprintf(w, "- **%s**: %s\n", msg.Role, strings.ReplaceAll(msg.Content, "\n", " "))
		}
		fmt.Fprintf(w, "\n## Timeline\n\n")
		for _, item := range timeline {
			fmt.Fprintf(w, "- #%d `%s` %s -> %s\n", item.EventIndex, item.Event.Type, item.Event.Actor, item.Event.Target)
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.json\"", filename))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(payload)
	}
}

func (s *Server) handleSaves(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		slots, err := engine.ListSaveSlots()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"saves": slots})
	case http.MethodPost:
		var req struct {
			Name   string `json:"name"`
			Branch string `json:"branch"`
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slot, err := engine.CreateSaveSlot(req.Name, req.Branch, req.Note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(slot)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSaveLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slot, err := engine.LoadSaveSlot(req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(slot)
}

func (s *Server) handleSavesDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	saveA := r.URL.Query().Get("a")
	saveB := r.URL.Query().Get("b")
	diff, err := engine.CompareSaveSlots(saveA, saveB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(diff)
}

func (s *Server) handleCheckpoints(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		slots, err := engine.ListCheckpoints()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"checkpoints": slots})
	case http.MethodPost:
		var req struct {
			Name   string `json:"name"`
			Branch string `json:"branch"`
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slot, err := engine.CreateCheckpoint(req.Name, req.Branch, req.Note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(slot)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCheckpointLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slot, err := engine.LoadCheckpoint(req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(slot)
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		presets, err := engine.ListScenarioPresets()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"presets": presets})
	case http.MethodPost:
		var req struct {
			Name   string `json:"name"`
			Branch string `json:"branch"`
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		preset, err := engine.CreateScenarioPreset(req.Name, req.Branch, req.Note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(preset)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePresetApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	preset, err := engine.ApplyScenarioPreset(req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preset)
}

func (s *Server) handleNPCActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	name := r.URL.Query().Get("focus_character")
	if name == "" {
		name = r.URL.Query().Get("character")
	}
	if name == "" {
		name = engine.GetFocusCharacter()
	}

	actions := engine.GetNPCActions(name, 0)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"focus_character": name,
		"character":       name,
		"actions":         actions,
	})
}

func (s *Server) handleActionLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	character := q.Get("character")
	firedOnly := q.Get("mode") == "fired"
	blockedOnly := q.Get("mode") == "blocked"
	limit := 50
	if n := q.Get("n"); n != "" {
		fmt.Sscanf(n, "%d", &limit)
	}
	if q.Get("stats") == "1" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(engine.ActionLogStats())
		return
	}
	entries := engine.QueryActionLog(character, firedOnly, blockedOnly, limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"count":   len(entries),
	})
}

func (s *Server) handleCausality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	eventID := r.URL.Query().Get("id")
	if eventID == "" {
		http.Error(w, "Missing 'id' query parameter", http.StatusBadRequest)
		return
	}

	depth := 3
	if d := r.URL.Query().Get("depth"); d != "" {
		fmt.Sscanf(d, "%d", &depth)
	}
	if depth > 10 {
		depth = 10
	}

	narrativeOnly := r.URL.Query().Get("mode") == "narrative"
	var chain interface{}
	var summary string
	if narrativeOnly {
		chain, err = engine.GetCausalityChainNarrative(eventID, depth)
		if err == nil {
			summary, _ = engine.GetCausalitySummaryNarrative(eventID, depth)
		}
	} else {
		chain, err = engine.GetCausalityChain(eventID, depth)
		if err == nil {
			summary, _ = engine.GetCausalitySummary(eventID, depth)
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"event_id": eventID,
		"depth":    depth,
		"chain":    chain,
		"summary":  summary,
	})
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	eventID := r.URL.Query().Get("id")
	timeParam := r.URL.Query().Get("time")

	w.Header().Set("Content-Type", "application/json")

	if eventID != "" {
		state, err := engine.ReplayTo(eventID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(state)
		return
	}

	if timeParam != "" {
		var h, m, d int
		fmt.Sscanf(timeParam, "%d:%d:%d", &d, &h, &m)
		state, err := engine.ReplayAtTime(h, m, d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(state)
		return
	}

	http.Error(w, "Missing 'id' or 'time' query parameter", http.StatusBadRequest)
}

func (s *Server) handleFork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		EventID string `json:"event_id"`
		Branch  string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := engine.ForkTimeline(req.EventID, req.Branch); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":       true,
		"event_id": req.EventID,
		"branch":   req.Branch,
	})
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "main"
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	timeline, err := engine.GetTimeline(branch, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"branch":   branch,
		"timeline": timeline,
	})
}

func (s *Server) handleBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	branches, err := engine.ListBranches()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"branches": branches,
	})
}

func (s *Server) handleBranchesDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	branchA := r.URL.Query().Get("a")
	branchB := r.URL.Query().Get("b")
	index := -1
	if n := r.URL.Query().Get("index"); n != "" {
		fmt.Sscanf(n, "%d", &index)
	}
	diff, err := engine.CompareBranchesDetailed(branchA, branchB, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(diff)
}

func (s *Server) handleBranchesMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		Source         string `json:"source"`
		Target         string `json:"target"`
		MergeFlags     bool   `json:"merge_flags"`
		MergeVariables bool   `json:"merge_variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := engine.MergeBranchState(req.Source, req.Target, req.MergeFlags, req.MergeVariables)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleCompress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := engine.CompressEvents(req.From, req.To)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleCompressionStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	stats := engine.CompressionStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := llm.ReadUsageStats("data/llm_usage.jsonl")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_calls":       stats.TotalCalls,
		"total_tokens":      stats.TotalTokens,
		"prompt_tokens":     stats.TotalPromptTokens,
		"completion_tokens": stats.TotalCompTokens,
		"estimated_cost":    stats.EstimatedCost(),
		"by_task":           stats.ByTask,
		"by_model":          stats.ByModel,
		"by_day":            stats.ByDay,
		"by_week":           stats.ByWeek,
		"by_month":          stats.ByMonth,
	})
}

func (s *Server) handleLLMConfigs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store := llm.GetConfigStore()

	// Extract item name from path: /api/llm-configs/<name>
	name := strings.TrimPrefix(r.URL.Path, "/api/llm-configs/")
	if name == "" || name == r.URL.Path {
		// List all or create
		switch r.Method {
		case "GET":
			json.NewEncoder(w).Encode(store.List())
		case "POST":
			var cfg llm.APIConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := store.Add(cfg); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// Single item operations
	switch r.Method {
	case "GET":
		cfg, err := store.Get(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if len(cfg.APIKey) > 8 {
			cfg.APIKey = cfg.APIKey[:4] + "****" + cfg.APIKey[len(cfg.APIKey)-4:]
		}
		json.NewEncoder(w).Encode(cfg)
	case "POST":
		current, err := store.Get(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		updated := *current
		var incoming llm.APIConfig
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if incoming.Name != "" {
			updated.Name = incoming.Name
		}
		if incoming.Endpoint != "" {
			updated.Endpoint = incoming.Endpoint
		}
		if incoming.Model != "" {
			updated.Model = incoming.Model
		}
		if incoming.APIKey != "" && !strings.Contains(incoming.APIKey, "****") {
			updated.APIKey = incoming.APIKey
		}
		if incoming.PromptPrice > 0 {
			updated.PromptPrice = incoming.PromptPrice
		}
		if incoming.CompletionPrice > 0 {
			updated.CompletionPrice = incoming.CompletionPrice
		}
		if err := store.Add(updated); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if active := llm.GetActiveConfig(); active.Name == name {
			llm.SetActiveConfigFull(updated)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	case "DELETE":
		if err := store.Remove(name); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLLMModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	configName := r.URL.Query().Get("config")
	store := llm.GetConfigStore()
	var cfg *llm.APIConfig
	if configName != "" {
		c, err := store.Get(configName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		cfg = c
	} else {
		c := llm.GetActiveConfig()
		cfg = &c
	}

	// Proxy the models list request
	req, _ := http.NewRequest("GET", strings.TrimRight(cfg.Endpoint, "/")+"/models", nil)
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to fetch models: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		http.Error(w, "Failed to parse models response", http.StatusBadGateway)
		return
	}
	var models []string
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"models": models})
}

func (s *Server) handleLLMActive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "POST" {
		engine, _, err := s.engineForRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var cfg llm.APIConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		store := llm.GetConfigStore()
		if cfg.Name != "" && (cfg.Endpoint == "" || strings.Contains(cfg.APIKey, "****")) {
			stored, err := store.Get(cfg.Name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			cfg = *stored
		}
		if cfg.Name == "" || cfg.Endpoint == "" {
			http.Error(w, "invalid active config", http.StatusBadRequest)
			return
		}
		if err := store.Add(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		llm.SetActiveConfigFull(cfg)
		engine.SwitchLLM(cfg.Name, cfg.Endpoint, cfg.APIKey, cfg.Model)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "switched": cfg.Model})
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := llm.GetActiveConfig()
	// Mask key
	if len(cfg.APIKey) > 8 {
		cfg.APIKey = cfg.APIKey[:4] + "****" + cfg.APIKey[len(cfg.APIKey)-4:]
	}
	json.NewEncoder(w).Encode(cfg)
}

func (s *Server) handleLLMRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	routes := engine.LLMRoutes()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

func (s *Server) handleDialogue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	dialogue := engine.GetDialogueLimit(limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": dialogue,
	})
}

func (s *Server) handleDialogueReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	engine.ResetDialogue()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) handleDebugMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	info := engine.DebugInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *Server) handleDirector(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		Action string  `json:"action"`
		Value  float64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch req.Action {
	case "set_tension":
		engine.SetTension(req.Value)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"action": req.Action,
			"value":  req.Value,
			"state":  engine.GetState().Tension,
		})
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
	}
}

func (s *Server) handleSimStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, 200, engine.TickStatus())
}

func (s *Server) handleSimTick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	engine.ManualTick()
	writeJSON(w, 200, map[string]interface{}{"ok": true, "tick_status": engine.TickStatus()})
}

func (s *Server) handleSimPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	engine.PauseTick()
	writeJSON(w, 200, map[string]interface{}{"ok": true, "paused": true})
}

func (s *Server) handleSimResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	engine.ResumeTick()
	writeJSON(w, 200, map[string]interface{}{"ok": true, "paused": false})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	// Sanitize: prevent directory traversal
	path = strings.TrimPrefix(path, "/")
	path = filepath.Join("web", path)

	// Reject paths that escape web directory
	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	absWeb, _ := filepath.Abs("web")
	if !strings.HasPrefix(absPath, absWeb) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contentType := "application/octet-stream"
	if strings.HasSuffix(path, ".html") {
		contentType = "text/html; charset=utf-8"
	} else if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript"
	} else if strings.HasSuffix(path, ".json") {
		contentType = "application/json"
	} else if strings.HasSuffix(path, ".css") {
		contentType = "text/css"
	}
	w.Header().Set("Content-Type", contentType)
	if strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".js") {
		w.Header().Set("Cache-Control", "no-store, max-age=0")
	}
	w.Write(data)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := auth.ChangePassword(req.Old, req.New); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}
