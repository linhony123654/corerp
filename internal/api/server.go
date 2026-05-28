package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"corerp/internal/agents"
	"corerp/internal/auth"
	"corerp/internal/core"
	dclpkg "corerp/internal/dcl"
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
var DCLRoot = "mods"

type runtimeInstancePayload struct {
	ID                 string                    `json:"id"`
	Label              string                    `json:"label"`
	WorldName          string                    `json:"world_name"`
	FocusCharacter     string                    `json:"focus_character,omitempty"`
	Participants       []string                  `json:"participants,omitempty"`
	ParticipantDetails []core.ParticipantSummary `json:"participant_details,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
	IsDefault          bool                      `json:"is_default"`
	Status             string                    `json:"status"`
}

type experimentReplayPayload struct {
	ReportName        string                           `json:"report_name"`
	WorldName         string                           `json:"world_name,omitempty"`
	CurrentCheckpoint string                           `json:"current_checkpoint,omitempty"`
	CompareCheckpoint string                           `json:"compare_checkpoint,omitempty"`
	CurrentInstance   *runtimeInstancePayload          `json:"current_instance,omitempty"`
	CompareInstance   *runtimeInstancePayload          `json:"compare_instance,omitempty"`
	CurrentEvidence   *experimentReplayEvidencePayload `json:"current_evidence,omitempty"`
	CompareEvidence   *experimentReplayEvidencePayload `json:"compare_evidence,omitempty"`
	CreatedAt         time.Time                        `json:"created_at"`
}

type experimentReplayEvidencePayload struct {
	SimStatus    map[string]interface{}   `json:"sim_status,omitempty"`
	LatestTrace  *core.TurnTrace          `json:"latest_trace,omitempty"`
	Population   *core.PopulationInsights `json:"population,omitempty"`
	AuditSummary []string                 `json:"audit_summary,omitempty"`
}

type experimentReplayBatchItem struct {
	ReportName string                   `json:"report_name"`
	WorldName  string                   `json:"world_name,omitempty"`
	Replay     *experimentReplayPayload `json:"replay,omitempty"`
	Error      string                   `json:"error,omitempty"`
}

type experimentReplayAdvanceEntry struct {
	ReportName        string `json:"report_name"`
	WorldName         string `json:"world_name,omitempty"`
	CurrentInstanceID string `json:"current_instance_id"`
	CompareInstanceID string `json:"compare_instance_id,omitempty"`
}

type experimentReplayBatchPayload struct {
	Mode      string                      `json:"mode"`
	WorldName string                      `json:"world_name,omitempty"`
	Count     int                         `json:"count,omitempty"`
	Total     int                         `json:"total"`
	Successes []string                    `json:"successes,omitempty"`
	Failures  []experimentReplayBatchItem `json:"failures,omitempty"`
	Results   []experimentReplayBatchItem `json:"results,omitempty"`
	CreatedAt time.Time                   `json:"created_at"`
}

type runtimeAuditPayload struct {
	InstanceID         string                    `json:"instance_id"`
	Instance           runtimeInstancePayload    `json:"instance"`
	State              core.WorldState           `json:"state"`
	PlayerRole         core.PlayerRole           `json:"player_role"`
	FocusCharacter     string                    `json:"focus_character"`
	Participants       []string                  `json:"participants,omitempty"`
	ParticipantDetails []core.ParticipantSummary `json:"participant_details,omitempty"`
	SimStatus          map[string]interface{}    `json:"sim_status,omitempty"`
	DirectorConfig     core.DirectorConfig       `json:"director_config"`
	DirectorPlan       core.DirectorPlan         `json:"director_plan"`
	LatestTrace        *core.TurnTrace           `json:"latest_trace,omitempty"`
	RecentTraces       []core.TurnTrace          `json:"recent_traces,omitempty"`
	Population         core.PopulationInsights   `json:"population,omitempty"`
	Checkpoints        []core.SaveSlot           `json:"checkpoints,omitempty"`
	Presets            []core.ScenarioPreset     `json:"presets,omitempty"`
	ExperimentReports  []core.ExperimentReport   `json:"experiment_reports,omitempty"`
	AuditSummary       []string                  `json:"audit_summary,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
}

type proofAuditFilePayload struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type proofAuditPayload struct {
	Name           string                  `json:"name"`
	CreatedAt      time.Time               `json:"created_at"`
	Overall        string                  `json:"overall,omitempty"`
	SummaryPath    string                  `json:"summary_path,omitempty"`
	SummaryPreview string                  `json:"summary_preview,omitempty"`
	Files          []proofAuditFilePayload `json:"files,omitempty"`
}

func toRuntimeInstancePayload(summary core.RuntimeInstanceSummary) runtimeInstancePayload {
	participants := append([]string(nil), summary.Participants...)
	if len(participants) == 0 && len(summary.ParticipantDetails) > 0 {
		seen := map[string]struct{}{}
		for _, item := range summary.ParticipantDetails {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			participants = append(participants, name)
		}
	}
	return runtimeInstancePayload{
		ID:                 summary.ID,
		Label:              summary.Label,
		WorldName:          summary.WorldName,
		FocusCharacter:     strings.TrimSpace(summary.FocusCharacter),
		Participants:       participants,
		ParticipantDetails: summary.ParticipantDetails,
		CreatedAt:          summary.CreatedAt,
		IsDefault:          summary.IsDefault,
		Status:             summary.Status,
	}
}

// RuntimeEngine is the interface the server needs from the runtime engine.
type RuntimeEngine interface {
	ProcessTurn(userInput string) (<-chan string, error)
	GetInstanceID() string
	InstanceSummary() core.RuntimeInstanceSummary
	GetState() core.WorldState
	GetFocusDefinition() (core.Character, bool)
	GetFocusCharacter() string
	GetPlayerRole() core.PlayerRole
	UpdatePlayerRole(role core.PlayerRole) (core.PlayerRole, error)
	GetFocusDefinitionConfig(name string) (core.CharacterConfig, error)
	UpdateFocusDefinitionConfig(name string, card core.Character) (core.CharacterConfig, error)
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
	GetSceneParticipants() []string
	SwitchFocusCharacter(name string) error
	EnterWorld(path string) (core.ScenarioPreset, error)
	GetWorldName() string
	GetWorldPaths() map[string]string
	GetFocusMemorySnapshot(character string, factLimit, episodicLimit, dialogueLimit int) (core.MemorySnapshot, error)
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
	ListExperimentReports() ([]core.ExperimentReport, error)
	CreateExperimentReport(report core.ExperimentReport) (core.ExperimentReport, error)
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
	engine         RuntimeEngine
	resolver       InstanceResolver
	proofAuditRoot string
}

func normalizeMemorySnapshotCompatibility(snapshot core.MemorySnapshot) core.MemorySnapshot {
	snapshot.FocusCharacter = strings.TrimSpace(snapshot.FocusCharacter)
	snapshot.Character = strings.TrimSpace(snapshot.Character)
	if snapshot.FocusCharacter == "" {
		snapshot.FocusCharacter = snapshot.Character
	}
	return snapshot
}

func normalizePendingFactCompatibility(fact core.PendingFact) core.PendingFact {
	fact.FocusCharacter = strings.TrimSpace(fact.FocusCharacter)
	fact.Character = strings.TrimSpace(fact.Character)
	if fact.FocusCharacter == "" {
		fact.FocusCharacter = fact.Character
	}
	return fact
}

func normalizeSaveSlotCompatibility(slot core.SaveSlot) core.SaveSlot {
	slot.FocusCharacter = strings.TrimSpace(slot.FocusCharacter)
	slot.Character = strings.TrimSpace(slot.Character)
	if slot.FocusCharacter == "" {
		slot.FocusCharacter = slot.Character
	}
	return slot
}

func normalizeScenarioPresetCompatibility(preset core.ScenarioPreset) core.ScenarioPreset {
	preset.FocusCharacter = strings.TrimSpace(preset.FocusCharacter)
	preset.Character = strings.TrimSpace(preset.Character)
	if preset.FocusCharacter == "" {
		preset.FocusCharacter = preset.Character
	}
	return preset
}

func normalizeExperimentSnapshotCompatibility(snapshot core.ExperimentSnapshot) core.ExperimentSnapshot {
	snapshot.FocusCharacter = strings.TrimSpace(snapshot.FocusCharacter)
	if snapshot.LatestTrace != nil {
		trace := normalizeTurnTraceCompatibility(*snapshot.LatestTrace)
		snapshot.LatestTrace = &trace
		if snapshot.FocusCharacter == "" {
			snapshot.FocusCharacter = trace.FocusCharacter
		}
	}
	return snapshot
}

func normalizeExperimentReportCompatibility(report core.ExperimentReport) core.ExperimentReport {
	report.CurrentCheckpoint = strings.TrimSpace(report.CurrentCheckpoint)
	report.CompareCheckpoint = strings.TrimSpace(report.CompareCheckpoint)
	report.Current = normalizeExperimentSnapshotCompatibility(report.Current)
	if report.Compare != nil {
		compare := normalizeExperimentSnapshotCompatibility(*report.Compare)
		report.Compare = &compare
	}
	return report
}

func normalizeTurnTraceCompatibility(trace core.TurnTrace) core.TurnTrace {
	trace.FocusCharacter = strings.TrimSpace(trace.FocusCharacter)
	trace.Character = strings.TrimSpace(trace.Character)
	if trace.FocusCharacter == "" {
		trace.FocusCharacter = trace.Character
	}
	trace.Character = ""
	for i := range trace.StepTraces {
		trace.StepTraces[i] = normalizeTurnStepTraceCompatibility(trace.StepTraces[i])
	}
	return trace
}

func normalizeTurnStepTraceCompatibility(stepTrace core.TurnStepTrace) core.TurnStepTrace {
	stepTrace.Character = strings.TrimSpace(stepTrace.Character)
	if stepTrace.Step.Speaker == "" {
		stepTrace.Step.Speaker = stepTrace.Character
	}
	stepTrace.Character = ""
	return stepTrace
}

func NewServer(engine RuntimeEngine, resolver ...InstanceResolver) *Server {
	s := &Server{engine: engine}
	if len(resolver) > 0 {
		s.resolver = resolver[0]
	}
	return s
}

func (s *Server) SetProofAuditRoot(root string) {
	s.proofAuditRoot = strings.TrimSpace(root)
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
	mux.HandleFunc("/api/focus-definition", a(s.handleFocusDefinition))
	mux.HandleFunc("/api/character", a(s.handleFocusDefinitionCompat))
	mux.HandleFunc("/api/player-role", a(s.handlePlayerRole))
	mux.HandleFunc("/api/instances", a(s.handleInstances))
	mux.HandleFunc("/api/instances/status", a(s.handleInstanceStatus))
	mux.HandleFunc("/api/instances/create", a(s.handleInstanceCreate))
	mux.HandleFunc("/api/instances/default", a(s.handleInstanceDefault))
	mux.HandleFunc("/api/instances/stop", a(s.handleInstanceStop))
	mux.HandleFunc("/api/instances/delete", a(s.handleInstanceDelete))
	mux.HandleFunc("/api/focus-definition-config", a(s.handleFocusDefinitionConfig))
	mux.HandleFunc("/api/character-config", a(s.handleFocusDefinitionConfigCompat))
	mux.HandleFunc("/api/characters", a(s.handleSceneParticipants))
	mux.HandleFunc("/api/switch", a(s.handleFocusSwitch))
	mux.HandleFunc("/api/world", a(s.handleWorld))
	mux.HandleFunc("/api/worlds", a(s.handleWorlds))
	mux.HandleFunc("/api/dcl", a(s.handleDCL))
	mux.HandleFunc("/api/dcl/install", a(s.handleDCLInstall))
	mux.HandleFunc("/api/dcl/upload", a(s.handleDCLUpload))
	mux.HandleFunc("/api/dcl/remove", a(s.handleDCLRemove))
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
	mux.HandleFunc("/api/experiment-reports", a(s.handleExperimentReports))
	mux.HandleFunc("/api/experiment-reports/replay", a(s.handleExperimentReportReplay))
	mux.HandleFunc("/api/experiment-reports/replay-batch", a(s.handleExperimentReportReplayBatch))
	mux.HandleFunc("/api/experiment-reports/replay-advance", a(s.handleExperimentReportReplayAdvance))
	mux.HandleFunc("/api/proof-audits", a(s.handleProofAudits))
	mux.HandleFunc("/api/runtime-audit", a(s.handleRuntimeAudit))
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

func markLegacyCompatRoute(w http.ResponseWriter, replacement string) {
	w.Header().Set("Deprecation", "true")
	if strings.TrimSpace(replacement) != "" {
		w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"successor-version\"", replacement))
	}
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
		InstanceID         string                    `json:"instance_id"`
		Instance           runtimeInstancePayload    `json:"instance"`
		FocusCharacter     string                    `json:"focus_character"`
		Participants       []string                  `json:"participants,omitempty"`
		ParticipantDetails []core.ParticipantSummary `json:"participant_details,omitempty"`
		DirectorConfig     core.DirectorConfig       `json:"director_config"`
		DirectorPlan       core.DirectorPlan         `json:"director_plan"`
		LatestTrace        *core.TurnTrace           `json:"latest_trace,omitempty"`
	}{
		WorldState:         state,
		InstanceID:         instanceID,
		Instance:           toRuntimeInstancePayload(engine.InstanceSummary()),
		FocusCharacter:     engine.GetFocusCharacter(),
		Participants:       engine.GetSceneParticipants(),
		ParticipantDetails: engine.GetSceneParticipantDetails(),
		DirectorConfig:     engine.GetDirectorConfig(),
		DirectorPlan:       engine.GetDirectorPlan(),
	}
	if trace, ok := engine.GetLatestTrace(); ok {
		trace = normalizeTurnTraceCompatibility(trace)
		payload.LatestTrace = &trace
	}
	writeJSONWithETag(w, r, payload)
}

func (s *Server) handleFocusDefinition(w http.ResponseWriter, r *http.Request) {
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

// handleFocusDefinitionCompat serves the legacy /api/character path.
func (s *Server) handleFocusDefinitionCompat(w http.ResponseWriter, r *http.Request) {
	markLegacyCompatRoute(w, "/api/focus-definition")
	s.handleFocusDefinition(w, r)
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
		Default   string                   `json:"default"`
		Instances []runtimeInstancePayload `json:"instances"`
	}{
		Default:   s.engine.GetInstanceID(),
		Instances: []runtimeInstancePayload{toRuntimeInstancePayload(s.engine.InstanceSummary())},
	}
	if s.resolver != nil {
		resp.Default = s.resolver.DefaultInstanceID()
		summaries := s.resolver.ListInstances()
		resp.Instances = make([]runtimeInstancePayload, 0, len(summaries))
		for _, summary := range summaries {
			resp.Instances = append(resp.Instances, toRuntimeInstancePayload(summary))
		}
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
		ID             string `json:"id"`
		Label          string `json:"label"`
		SourceID       string `json:"source_id"`
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	focusCharacter := strings.TrimSpace(req.FocusCharacter)
	summary, err := s.resolver.CreateInstance(req.SourceID, req.ID, req.Label, focusCharacter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toRuntimeInstancePayload(summary))
}

func (s *Server) handleInstanceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toRuntimeInstancePayload(s.engine.InstanceSummary()))
		return
	}
	summary, err := s.resolver.InstanceStatus(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toRuntimeInstancePayload(summary))
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
	json.NewEncoder(w).Encode(toRuntimeInstancePayload(summary))
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

func (s *Server) handleFocusDefinitionConfig(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	name := r.URL.Query().Get("focus_character")
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
			Card           core.Character `json:"card"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.FocusCharacter != "" {
			name = req.FocusCharacter
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

// handleFocusDefinitionConfigCompat serves the legacy /api/character-config path.
func (s *Server) handleFocusDefinitionConfigCompat(w http.ResponseWriter, r *http.Request) {
	markLegacyCompatRoute(w, "/api/focus-definition-config")
	s.handleFocusDefinitionConfig(w, r)
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
	details := engine.GetSceneParticipantDetails()
	focus := engine.GetFocusCharacter()
	writeJSONWithETag(w, r, map[string]interface{}{
		"focus_character":     focus,
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
			"ok":                  true,
			"world":               engine.GetWorldName(),
			"focus_character":     engine.GetFocusCharacter(),
			"participants":        engine.GetSceneParticipants(),
			"participant_details": engine.GetSceneParticipantDetails(),
			"preset":              preset,
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

func (s *Server) handleDCL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mods, err := dclpkg.List(DCLRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONWithETag(w, r, map[string]interface{}{"mods": mods})
}

func (s *Server) handleDCLInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID        string `json:"id"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := dclpkg.Install(DCLRoot, req.ID, WorldCatalogRoot, dclpkg.InstallOptions{Overwrite: req.Overwrite})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "result": result})
}

func (s *Server) handleDCLUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "dcl-upload-*.zip")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	overwrite := strings.EqualFold(strings.TrimSpace(r.FormValue("overwrite")), "true")
	result, err := dclpkg.UploadZip(DCLRoot, tmp.Name(), overwrite)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "result": result})
}

func (s *Server) handleDCLRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID            string `json:"id"`
		DeleteWorld   bool   `json:"delete_world"`
		DeletePackage bool   `json:"delete_package"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	removed, err := dclpkg.Remove(DCLRoot, req.ID, req.DeleteWorld)
	if err != nil {
		if !req.DeletePackage {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		deletedPath, deleteErr := dclpkg.DeletePackage(DCLRoot, req.ID)
		if deleteErr != nil {
			http.Error(w, deleteErr.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "deleted_package": deletedPath})
		return
	}
	result := map[string]interface{}{"ok": true, "removed": removed}
	if req.DeletePackage {
		deletedPath, deleteErr := dclpkg.DeletePackage(DCLRoot, req.ID)
		if deleteErr != nil {
			http.Error(w, deleteErr.Error(), http.StatusBadRequest)
			return
		}
		result["deleted_package"] = deletedPath
	}
	writeJSON(w, http.StatusOK, result)
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.FocusCharacter)
	if name == "" {
		http.Error(w, "focus_character is required", http.StatusBadRequest)
		return
	}

	if name != engine.GetFocusCharacter() {
		for _, participant := range engine.GetSceneParticipantDetails() {
			if participant.Name != name {
				continue
			}
			if !participant.Switchable {
				http.Error(w, fmt.Sprintf("participant '%s' is not switchable (%s)", name, participant.Kind), http.StatusBadRequest)
				return
			}
			break
		}
	}

	if err := engine.SwitchFocusCharacter(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Include recent NPC actions for "while you were away" summary
	npcActions := engine.GetNPCActions(name, 0)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":              true,
		"focus_character": name,
		"npc_actions":     npcActions,
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
	trace = normalizeTurnTraceCompatibility(trace)
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
	traces := engine.ListTurnTraces(limit)
	for i := range traces {
		traces[i] = normalizeTurnTraceCompatibility(traces[i])
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"traces": traces,
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
	character := strings.TrimSpace(r.URL.Query().Get("focus_character"))
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
	if character == "" {
		character = strings.TrimSpace(engine.GetFocusCharacter())
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events":          events,
		"count":           len(events),
		"focus_character": character,
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
	limit := 50
	if n := r.URL.Query().Get("n"); n != "" {
		fmt.Sscanf(n, "%d", &limit)
	}
	items, stats, err := engine.ListPendingFacts(character, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	focusCharacter := strings.TrimSpace(character)
	if focusCharacter == "" {
		focusCharacter = strings.TrimSpace(engine.GetFocusCharacter())
	}
	for i := range items {
		items[i] = normalizePendingFactCompatibility(items[i])
		if focusCharacter == "" {
			focusCharacter = items[i].FocusCharacter
		}
		items[i].Character = ""
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"facts":           items,
		"count":           len(items),
		"stats":           stats,
		"focus_character": focusCharacter,
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
	snapshot = normalizeMemorySnapshotCompatibility(snapshot)
	writeJSONWithETag(w, r, map[string]interface{}{
		"focus_character": snapshot.FocusCharacter,
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
		"exported_at":         time.Now().UTC(),
		"focus_definition":    focusDefinition,
		"focus_character":     engine.GetFocusCharacter(),
		"participants":        engine.GetSceneParticipants(),
		"participant_details": engine.GetSceneParticipantDetails(),
		"world":               engine.GetWorldName(),
		"state":               engine.GetState(),
		"dialogue":            dialogue,
		"timeline":            timeline,
	}

	filename := fmt.Sprintf("corerp-%s-%s", engine.GetFocusCharacter(), time.Now().UTC().Format("20060102T150405Z"))
	switch format {
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.md\"", filename))
		fmt.Fprintf(w, "# CoreRP Session Export\n\n")
		fmt.Fprintf(w, "- Focus Character: %s\n", focusDefinition.Identity.Name)
		fmt.Fprintf(w, "- World: %s\n", engine.GetWorldName())
		fmt.Fprintf(w, "- Exported: %s\n\n", time.Now().UTC().Format(time.RFC3339))
		state := engine.GetState()
		fmt.Fprintf(w, "## Scene\n\n")
		fmt.Fprintf(w, "- Day %d %02d:%02d\n", state.Clock.Day, state.Clock.Hour, state.Clock.Minute)
		fmt.Fprintf(w, "- Location: %s\n", state.Scene.Location)
		fmt.Fprintf(w, "- Weather: %s\n", state.Scene.Weather)
		fmt.Fprintf(w, "- Tension: %.2f\n\n", state.Tension)
		participants := engine.GetSceneParticipants()
		if len(participants) > 0 {
			fmt.Fprintf(w, "- Participants: %s\n\n", strings.Join(participants, ", "))
		}
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
		slots = sanitizeSaveSlotsForAPI(slots)
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
		slot = sanitizeSaveSlotForAPI(slot)
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
	slot = sanitizeSaveSlotForAPI(slot)
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
		presets = sanitizeScenarioPresetsForAPI(presets)
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
		preset = sanitizeScenarioPresetForAPI(preset)
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
	preset = sanitizeScenarioPresetForAPI(preset)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preset)
}

func (s *Server) handleExperimentReports(w http.ResponseWriter, r *http.Request) {
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		reports, err := engine.ListExperimentReports()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range reports {
			reports[i] = normalizeExperimentReportCompatibility(reports[i])
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"reports": reports})
	case http.MethodPost:
		var report core.ExperimentReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		report = normalizeExperimentReportCompatibility(report)
		saved, err := engine.CreateExperimentReport(report)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved = normalizeExperimentReportCompatibility(saved)
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExperimentReportReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		http.Error(w, "instance manager unavailable", http.StatusNotImplemented)
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
	reportName := strings.TrimSpace(req.Name)
	if reportName == "" {
		http.Error(w, "experiment report name is required", http.StatusBadRequest)
		return
	}

	reports, err := engine.ListExperimentReports()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var report *core.ExperimentReport
	for i := range reports {
		normalized := normalizeExperimentReportCompatibility(reports[i])
		if normalized.Name == reportName {
			report = &normalized
			break
		}
	}
	if report == nil {
		http.Error(w, "experiment report not found", http.StatusNotFound)
		return
	}
	if strings.TrimSpace(report.CurrentCheckpoint) == "" {
		http.Error(w, "experiment report is missing current checkpoint anchor", http.StatusBadRequest)
		return
	}
	if report.Compare != nil && strings.TrimSpace(report.CompareCheckpoint) == "" {
		http.Error(w, "experiment report is missing compare checkpoint anchor", http.StatusBadRequest)
		return
	}

	result, err := s.replayExperimentReport(*report)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) replayExperimentReport(report core.ExperimentReport) (experimentReplayPayload, error) {
	report = normalizeExperimentReportCompatibility(report)
	if strings.TrimSpace(report.Name) == "" {
		return experimentReplayPayload{}, fmt.Errorf("experiment report name is required")
	}
	if strings.TrimSpace(report.CurrentCheckpoint) == "" {
		return experimentReplayPayload{}, fmt.Errorf("experiment report is missing current checkpoint anchor")
	}
	if report.Compare != nil && strings.TrimSpace(report.CompareCheckpoint) == "" {
		return experimentReplayPayload{}, fmt.Errorf("experiment report is missing compare checkpoint anchor")
	}

	now := time.Now().UTC()
	stamp := now.Format("20060102-150405")
	result := experimentReplayPayload{
		ReportName:        report.Name,
		WorldName:         strings.TrimSpace(report.Current.WorldName),
		CurrentCheckpoint: report.CurrentCheckpoint,
		CompareCheckpoint: report.CompareCheckpoint,
		CreatedAt:         now,
	}

	currentSummary, err := s.spawnExperimentReplayInstance(report, false, stamp)
	if err != nil {
		return experimentReplayPayload{}, err
	}
	currentPayload := toRuntimeInstancePayload(currentSummary)
	result.CurrentInstance = &currentPayload
	if evidence, err := s.buildExperimentReplayEvidence(currentPayload.ID); err == nil {
		result.CurrentEvidence = evidence
	}

	if report.Compare != nil {
		compareSummary, err := s.spawnExperimentReplayInstance(report, true, stamp)
		if err != nil {
			return experimentReplayPayload{}, err
		}
		comparePayload := toRuntimeInstancePayload(compareSummary)
		result.CompareInstance = &comparePayload
		if evidence, err := s.buildExperimentReplayEvidence(comparePayload.ID); err == nil {
			result.CompareEvidence = evidence
		}
	}

	if result.WorldName == "" {
		if result.CurrentInstance != nil {
			result.WorldName = strings.TrimSpace(result.CurrentInstance.WorldName)
		} else if report.Compare != nil {
			result.WorldName = strings.TrimSpace(report.Compare.WorldName)
		}
	}
	return result, nil
}

func normalizeExperimentReportWorldName(report core.ExperimentReport) string {
	report = normalizeExperimentReportCompatibility(report)
	candidates := []string{
		strings.TrimSpace(report.Current.WorldName),
		strings.TrimSpace(report.Compare.WorldName),
	}
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func filterReplayEligibleExperimentReports(reports []core.ExperimentReport, worldName string, reportNames []string) []core.ExperimentReport {
	targetWorld := strings.TrimSpace(worldName)
	nameSet := map[string]struct{}{}
	for _, name := range reportNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		nameSet[name] = struct{}{}
	}

	filtered := make([]core.ExperimentReport, 0, len(reports))
	for i := range reports {
		report := normalizeExperimentReportCompatibility(reports[i])
		if strings.TrimSpace(report.CurrentCheckpoint) == "" {
			continue
		}
		if targetWorld != "" && normalizeExperimentReportWorldName(report) != targetWorld {
			continue
		}
		if len(nameSet) > 0 {
			if _, ok := nameSet[strings.TrimSpace(report.Name)]; !ok {
				continue
			}
		}
		filtered = append(filtered, report)
	}
	return filtered
}

func filterReplayAdvanceEntries(entries []experimentReplayAdvanceEntry, worldName string, reportNames []string) []experimentReplayAdvanceEntry {
	targetWorld := strings.TrimSpace(worldName)
	nameSet := map[string]struct{}{}
	for _, name := range reportNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		nameSet[name] = struct{}{}
	}
	filtered := make([]experimentReplayAdvanceEntry, 0, len(entries))
	for _, entry := range entries {
		entry.ReportName = strings.TrimSpace(entry.ReportName)
		entry.WorldName = strings.TrimSpace(entry.WorldName)
		entry.CurrentInstanceID = strings.TrimSpace(entry.CurrentInstanceID)
		entry.CompareInstanceID = strings.TrimSpace(entry.CompareInstanceID)
		if entry.ReportName == "" || entry.CurrentInstanceID == "" {
			continue
		}
		if targetWorld != "" && entry.WorldName != targetWorld {
			continue
		}
		if len(nameSet) > 0 {
			if _, ok := nameSet[entry.ReportName]; !ok {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func (s *Server) handleExperimentReportReplayBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		http.Error(w, "instance manager unavailable", http.StatusNotImplemented)
		return
	}
	engine, _, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		WorldName   string   `json:"world_name"`
		ReportNames []string `json:"report_names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	reports, err := engine.ListExperimentReports()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := filterReplayEligibleExperimentReports(reports, req.WorldName, req.ReportNames)
	result := experimentReplayBatchPayload{
		Mode:      "replay",
		WorldName: strings.TrimSpace(req.WorldName),
		Total:     len(filtered),
		CreatedAt: time.Now().UTC(),
	}
	for _, report := range filtered {
		replay, replayErr := s.replayExperimentReport(report)
		item := experimentReplayBatchItem{
			ReportName: report.Name,
			WorldName:  normalizeExperimentReportWorldName(report),
		}
		if replayErr != nil {
			item.Error = replayErr.Error()
			result.Failures = append(result.Failures, item)
			result.Results = append(result.Results, item)
			continue
		}
		item.Replay = &replay
		result.Successes = append(result.Successes, report.Name)
		result.Results = append(result.Results, item)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) buildExperimentReplayPayloadFromEntry(entry experimentReplayAdvanceEntry) (*experimentReplayPayload, error) {
	if s.resolver == nil {
		return nil, fmt.Errorf("instance manager unavailable")
	}
	entry.ReportName = strings.TrimSpace(entry.ReportName)
	entry.WorldName = strings.TrimSpace(entry.WorldName)
	entry.CurrentInstanceID = strings.TrimSpace(entry.CurrentInstanceID)
	entry.CompareInstanceID = strings.TrimSpace(entry.CompareInstanceID)
	if entry.ReportName == "" || entry.CurrentInstanceID == "" {
		return nil, fmt.Errorf("report_name and current_instance_id are required")
	}
	result := &experimentReplayPayload{
		ReportName: entry.ReportName,
		WorldName:  entry.WorldName,
		CreatedAt:  time.Now().UTC(),
	}
	currentSummary, err := s.resolver.InstanceStatus(entry.CurrentInstanceID)
	if err != nil {
		return nil, err
	}
	currentPayload := toRuntimeInstancePayload(currentSummary)
	result.CurrentInstance = &currentPayload
	if evidence, err := s.buildExperimentReplayEvidence(entry.CurrentInstanceID); err == nil {
		result.CurrentEvidence = evidence
	}
	if result.WorldName == "" {
		result.WorldName = strings.TrimSpace(currentPayload.WorldName)
	}
	if entry.CompareInstanceID != "" {
		compareSummary, err := s.resolver.InstanceStatus(entry.CompareInstanceID)
		if err != nil {
			return nil, err
		}
		comparePayload := toRuntimeInstancePayload(compareSummary)
		result.CompareInstance = &comparePayload
		if evidence, err := s.buildExperimentReplayEvidence(entry.CompareInstanceID); err == nil {
			result.CompareEvidence = evidence
		}
		if result.WorldName == "" {
			result.WorldName = strings.TrimSpace(comparePayload.WorldName)
		}
	}
	return result, nil
}

func (s *Server) buildExperimentReplayEvidence(instanceID string) (*experimentReplayEvidencePayload, error) {
	if s.resolver == nil {
		return nil, fmt.Errorf("instance manager unavailable")
	}
	engine, err := s.resolver.ResolveInstance(strings.TrimSpace(instanceID))
	if err != nil {
		return nil, err
	}
	evidence := &experimentReplayEvidencePayload{
		SimStatus: engine.TickStatus(),
	}
	if trace, ok := engine.GetLatestTrace(); ok {
		trace = normalizeTurnTraceCompatibility(trace)
		evidence.LatestTrace = &trace
	}
	if population, err := engine.GetPopulationInsights(); err == nil {
		evidence.Population = &population
	}
	snapshot := core.RuntimeAuditSnapshot{
		InstanceID:         strings.TrimSpace(instanceID),
		Instance:           engine.InstanceSummary(),
		State:              engine.GetState(),
		PlayerRole:         engine.GetPlayerRole(),
		FocusCharacter:     engine.GetFocusCharacter(),
		Participants:       engine.GetSceneParticipants(),
		ParticipantDetails: engine.GetSceneParticipantDetails(),
		SimStatus:          evidence.SimStatus,
		DirectorConfig:     engine.GetDirectorConfig(),
		DirectorPlan:       engine.GetDirectorPlan(),
		LatestTrace:        evidence.LatestTrace,
	}
	if evidence.Population != nil {
		snapshot.Population = *evidence.Population
	}
	evidence.AuditSummary = buildRuntimeAuditSummary(snapshot)
	return evidence, nil
}

func (s *Server) advanceReplayInstance(instanceID string, count int) error {
	if s.resolver == nil {
		return fmt.Errorf("instance manager unavailable")
	}
	engine, err := s.resolver.ResolveInstance(strings.TrimSpace(instanceID))
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		engine.ManualTick()
	}
	return nil
}

func (s *Server) advanceExperimentReplay(entry experimentReplayAdvanceEntry, count int) (experimentReplayPayload, error) {
	if count <= 0 || count > 200 {
		return experimentReplayPayload{}, fmt.Errorf("count must be between 1 and 200")
	}
	entry.ReportName = strings.TrimSpace(entry.ReportName)
	entry.CurrentInstanceID = strings.TrimSpace(entry.CurrentInstanceID)
	entry.CompareInstanceID = strings.TrimSpace(entry.CompareInstanceID)
	if entry.ReportName == "" || entry.CurrentInstanceID == "" {
		return experimentReplayPayload{}, fmt.Errorf("report_name and current_instance_id are required")
	}
	if err := s.advanceReplayInstance(entry.CurrentInstanceID, count); err != nil {
		return experimentReplayPayload{}, err
	}
	if entry.CompareInstanceID != "" {
		if err := s.advanceReplayInstance(entry.CompareInstanceID, count); err != nil {
			return experimentReplayPayload{}, err
		}
	}
	payload, err := s.buildExperimentReplayPayloadFromEntry(entry)
	if err != nil {
		return experimentReplayPayload{}, err
	}
	return *payload, nil
}

func (s *Server) handleExperimentReportReplayAdvance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.resolver == nil {
		http.Error(w, "instance manager unavailable", http.StatusNotImplemented)
		return
	}
	if _, _, err := s.engineForRequest(r); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		WorldName   string                         `json:"world_name"`
		ReportNames []string                       `json:"report_names"`
		Count       int                            `json:"count"`
		Replays     []experimentReplayAdvanceEntry `json:"replays"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Count <= 0 || req.Count > 200 {
		http.Error(w, "count must be between 1 and 200", http.StatusBadRequest)
		return
	}
	filtered := filterReplayAdvanceEntries(req.Replays, req.WorldName, req.ReportNames)
	result := experimentReplayBatchPayload{
		Mode:      "tick",
		WorldName: strings.TrimSpace(req.WorldName),
		Count:     req.Count,
		Total:     len(filtered),
		CreatedAt: time.Now().UTC(),
	}
	for _, entry := range filtered {
		advanced, advanceErr := s.advanceExperimentReplay(entry, req.Count)
		item := experimentReplayBatchItem{
			ReportName: entry.ReportName,
			WorldName:  entry.WorldName,
		}
		if item.WorldName == "" {
			item.WorldName = advanced.WorldName
		}
		if advanceErr != nil {
			item.Error = advanceErr.Error()
			result.Failures = append(result.Failures, item)
			result.Results = append(result.Results, item)
			continue
		}
		item.Replay = &advanced
		result.Successes = append(result.Successes, entry.ReportName)
		result.Results = append(result.Results, item)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) spawnExperimentReplayInstance(report core.ExperimentReport, compare bool, stamp string) (core.RuntimeInstanceSummary, error) {
	sourceID := strings.TrimSpace(report.SourceInstanceID)
	checkpoint := strings.TrimSpace(report.CurrentCheckpoint)
	labelSuffix := "Current Replay"
	focusCharacter := strings.TrimSpace(report.Current.FocusCharacter)
	roleSuffix := "current"
	if compare {
		if report.Compare == nil {
			return core.RuntimeInstanceSummary{}, fmt.Errorf("compare snapshot unavailable")
		}
		sourceID = strings.TrimSpace(report.CompareInstanceID)
		checkpoint = strings.TrimSpace(report.CompareCheckpoint)
		labelSuffix = "Compare Replay"
		focusCharacter = strings.TrimSpace(report.Compare.FocusCharacter)
		roleSuffix = "compare"
	}
	if sourceID == "" {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("%s replay source instance is required", roleSuffix)
	}
	if checkpoint == "" {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("%s replay checkpoint is required", roleSuffix)
	}
	instanceID := buildExperimentReplayInstanceID(report.Name, roleSuffix, stamp)
	summary, err := s.resolver.CreateInstance(sourceID, instanceID, fmt.Sprintf("%s %s", strings.TrimSpace(report.Name), labelSuffix), focusCharacter)
	if err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	sourceEngine, err := s.resolver.ResolveInstance(sourceID)
	if err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	sourceSlot, err := sourceEngine.LoadCheckpoint(checkpoint)
	if err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	replayEngine, err := s.resolver.ResolveInstance(instanceID)
	if err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	if _, err := replayEngine.CreateCheckpoint(checkpoint, sourceSlot.Branch, sourceSlot.Note); err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	if _, err := replayEngine.LoadCheckpoint(checkpoint); err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	if resolved, err := s.resolver.InstanceStatus(instanceID); err == nil {
		return resolved, nil
	}
	return summary, nil
}

func buildExperimentReplayInstanceID(reportName, side, stamp string) string {
	base := sanitizeExperimentReplaySlug(reportName)
	if base == "" {
		base = "experiment"
	}
	side = sanitizeExperimentReplaySlug(side)
	if side == "" {
		side = "replay"
	}
	return strings.ToLower(strings.Trim(strings.Join([]string{base, side, stamp}, "-"), "-"))
}

func sanitizeExperimentReplaySlug(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func clampPositiveInt(raw string, fallback, max int) int {
	value := fallback
	if strings.TrimSpace(raw) != "" {
		if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
			value = fallback
		}
	}
	if value <= 0 {
		value = fallback
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func trimSaveSlots(slots []core.SaveSlot, limit int) []core.SaveSlot {
	if limit > 0 && len(slots) > limit {
		slots = slots[:limit]
	}
	return sanitizeSaveSlotsForAPI(slots)
}

func trimScenarioPresets(presets []core.ScenarioPreset, limit int) []core.ScenarioPreset {
	if limit > 0 && len(presets) > limit {
		presets = presets[:limit]
	}
	return sanitizeScenarioPresetsForAPI(presets)
}

func sanitizeSaveSlotForAPI(slot core.SaveSlot) core.SaveSlot {
	slot = normalizeSaveSlotCompatibility(slot)
	slot.Character = ""
	return slot
}

func sanitizeSaveSlotsForAPI(slots []core.SaveSlot) []core.SaveSlot {
	out := make([]core.SaveSlot, 0, len(slots))
	for _, slot := range slots {
		out = append(out, sanitizeSaveSlotForAPI(slot))
	}
	return out
}

func sanitizeScenarioPresetForAPI(preset core.ScenarioPreset) core.ScenarioPreset {
	preset = normalizeScenarioPresetCompatibility(preset)
	preset.Character = ""
	return preset
}

func sanitizeScenarioPresetsForAPI(presets []core.ScenarioPreset) []core.ScenarioPreset {
	out := make([]core.ScenarioPreset, 0, len(presets))
	for _, preset := range presets {
		out = append(out, sanitizeScenarioPresetForAPI(preset))
	}
	return out
}

func trimExperimentReports(reports []core.ExperimentReport, limit int) []core.ExperimentReport {
	if limit <= 0 || len(reports) <= limit {
		return append([]core.ExperimentReport(nil), reports...)
	}
	return append([]core.ExperimentReport(nil), reports[:limit]...)
}

func trimPopulationInsights(insights core.PopulationInsights, limit int) core.PopulationInsights {
	trimmed := insights
	if limit > 0 && len(trimmed.Promoted) > limit {
		trimmed.Promoted = append([]core.PopulationCharacterInsight(nil), trimmed.Promoted[:limit]...)
	}
	if limit > 0 && len(trimmed.Background) > limit {
		trimmed.Background = append([]core.PopulationCharacterInsight(nil), trimmed.Background[:limit]...)
	}
	return trimmed
}

func dominantFloatEntryLabel(values map[string]float64) string {
	bestKey := ""
	bestValue := 0.0
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if bestKey == "" || value > bestValue || (value == bestValue && key < bestKey) {
			bestKey = key
			bestValue = value
		}
	}
	if bestKey == "" {
		return ""
	}
	return fmt.Sprintf("%s %.2f", bestKey, bestValue)
}

func buildRuntimeAuditSummary(audit core.RuntimeAuditSnapshot) []string {
	lines := []string{
		fmt.Sprintf("world: %s · focus %s · scene %s", strings.TrimSpace(audit.Instance.WorldName), strings.TrimSpace(audit.FocusCharacter), strings.TrimSpace(audit.State.Scene.Location)),
	}
	if len(audit.Participants) > 0 {
		lines = append(lines, fmt.Sprintf("participants: %s", strings.Join(audit.Participants, ", ")))
	}
	if trajectory, ok := audit.SimStatus["trajectory_summary"].([]string); ok && len(trajectory) > 0 {
		lines = append(lines, "trajectory: "+trajectory[0])
	}
	if dominant := dominantFloatEntryLabel(runtimeAuditPressureStates(audit.SimStatus)); dominant != "" {
		lines = append(lines, "pressure: "+dominant)
	}
	if dominant := dominantFloatEntryLabel(runtimeAuditFactionTensions(audit.SimStatus)); dominant != "" {
		lines = append(lines, "faction: "+dominant)
	}
	if len(audit.Population.Promoted) > 0 {
		top := audit.Population.Promoted[0]
		lines = append(lines, fmt.Sprintf("promotion: %s %.1f", top.Name, top.Attention.Score))
	} else if len(audit.Population.Background) > 0 {
		top := audit.Population.Background[0]
		lines = append(lines, fmt.Sprintf("rising background: %s %.1f", top.Name, top.Attention.Score))
	}
	if audit.LatestTrace != nil {
		lines = append(lines, fmt.Sprintf("latest trace: turn %d · %s", audit.LatestTrace.Turn, strings.TrimSpace(audit.LatestTrace.UserInput)))
	}
	if len(audit.DirectorPlan.Selected) > 0 {
		directorLine := "director: " + strings.Join(audit.DirectorPlan.Selected, " -> ")
		if audit.DirectorPlan.Mode != "" {
			directorLine = fmt.Sprintf("director [%s]: %s", audit.DirectorPlan.Mode, strings.Join(audit.DirectorPlan.Selected, " -> "))
		}
		lines = append(lines, directorLine)
		if len(audit.DirectorPlan.CandidateDetails) > 0 {
			for _, c := range audit.DirectorPlan.CandidateDetails {
				if c.Selected {
					factors := ""
					if len(c.DominantFactors) > 0 {
						factors = fmt.Sprintf(" · 主导 %s", strings.Join(c.DominantFactors, "/"))
					}
					lines = append(lines, fmt.Sprintf("  胜出: %s %.1f%s", c.Name, c.Score, factors))
					break
				}
			}
			for _, c := range audit.DirectorPlan.CandidateDetails {
				if !c.Selected {
					lines = append(lines, fmt.Sprintf("  候选: %s %.1f", c.Name, c.Score))
					break
				}
			}
		}
	}
	if len(audit.DirectorPlan.WorldSignals) > 0 {
		lines = append(lines, "director-world: "+strings.Join(audit.DirectorPlan.WorldSignals, " · "))
	}
	lines = append(lines, fmt.Sprintf("audit assets: traces %d · checkpoints %d · presets %d · reports %d", len(audit.RecentTraces), len(audit.Checkpoints), len(audit.Presets), len(audit.ExperimentReports)))
	return lines
}

func runtimeAuditPressureStates(simStatus map[string]interface{}) map[string]float64 {
	if pressureStates, ok := simStatus["pressure_states"].(map[string]float64); ok {
		return pressureStates
	}
	if raw, ok := simStatus["pressure_states"].(map[string]interface{}); ok {
		out := make(map[string]float64, len(raw))
		for key, value := range raw {
			if number, ok := value.(float64); ok {
				out[key] = number
			}
		}
		return out
	}
	return nil
}

func runtimeAuditFactionTensions(simStatus map[string]interface{}) map[string]float64 {
	if tensions, ok := simStatus["faction_tensions"].(map[string]float64); ok {
		return tensions
	}
	if raw, ok := simStatus["faction_tensions"].(map[string]interface{}); ok {
		out := make(map[string]float64, len(raw))
		for key, value := range raw {
			if number, ok := value.(float64); ok {
				out[key] = number
			}
		}
		return out
	}
	return nil
}

func (s *Server) proofAuditRootPath() string {
	if strings.TrimSpace(s.proofAuditRoot) != "" {
		return s.proofAuditRoot
	}
	return filepath.Join("data", "proof-audits")
}

func extractProofAuditOverall(summary string) string {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- Overall:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "- Overall:"))
		}
	}
	return ""
}

func extractProofAuditPreview(summary string) string {
	lines := strings.Split(summary, "\n")
	out := make([]string, 0, 8)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= 8 {
			break
		}
	}
	return strings.Join(out, "\n")
}

func (s *Server) listProofAudits(limit int) ([]proofAuditPayload, error) {
	root := s.proofAuditRootPath()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	dirs := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name() > dirs[j].Name()
	})
	if limit > 0 && len(dirs) > limit {
		dirs = dirs[:limit]
	}
	out := make([]proofAuditPayload, 0, len(dirs))
	for _, entry := range dirs {
		dirPath := filepath.Join(root, entry.Name())
		record := proofAuditPayload{Name: entry.Name()}
		if info, err := entry.Info(); err == nil {
			record.CreatedAt = info.ModTime().UTC()
		}
		summaryPath := filepath.Join(dirPath, "SUMMARY.md")
		if data, err := os.ReadFile(summaryPath); err == nil {
			record.SummaryPath = filepath.ToSlash(summaryPath)
			record.SummaryPreview = extractProofAuditPreview(string(data))
			record.Overall = extractProofAuditOverall(string(data))
		}
		files, err := os.ReadDir(dirPath)
		if err == nil {
			record.Files = make([]proofAuditFilePayload, 0, len(files))
			for _, file := range files {
				info, infoErr := file.Info()
				if infoErr != nil || info.IsDir() {
					continue
				}
				record.Files = append(record.Files, proofAuditFilePayload{
					Name: file.Name(),
					Size: info.Size(),
				})
			}
			sort.Slice(record.Files, func(i, j int) bool {
				return record.Files[i].Name < record.Files[j].Name
			})
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *Server) handleProofAudits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := clampPositiveInt(r.URL.Query().Get("limit"), 5, 20)
	items, err := s.listProofAudits(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"root":         filepath.ToSlash(s.proofAuditRootPath()),
		"count":        len(items),
		"proof_audits": items,
	})
}

func (s *Server) handleRuntimeAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engine, instanceID, err := s.engineForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	traceLimit := clampPositiveInt(r.URL.Query().Get("trace_limit"), 8, 20)
	checkpointLimit := clampPositiveInt(r.URL.Query().Get("checkpoint_limit"), 5, 20)
	presetLimit := clampPositiveInt(r.URL.Query().Get("preset_limit"), 5, 20)
	reportLimit := clampPositiveInt(r.URL.Query().Get("report_limit"), 5, 20)
	populationLimit := clampPositiveInt(r.URL.Query().Get("population_limit"), 5, 20)

	payload := runtimeAuditPayload{
		InstanceID:         instanceID,
		Instance:           toRuntimeInstancePayload(engine.InstanceSummary()),
		State:              engine.GetState(),
		PlayerRole:         engine.GetPlayerRole(),
		FocusCharacter:     engine.GetFocusCharacter(),
		Participants:       engine.GetSceneParticipants(),
		ParticipantDetails: engine.GetSceneParticipantDetails(),
		SimStatus:          engine.TickStatus(),
		DirectorConfig:     engine.GetDirectorConfig(),
		DirectorPlan:       engine.GetDirectorPlan(),
		RecentTraces:       engine.ListTurnTraces(traceLimit),
		CreatedAt:          time.Now().UTC(),
	}
	for i := range payload.RecentTraces {
		payload.RecentTraces[i] = normalizeTurnTraceCompatibility(payload.RecentTraces[i])
	}
	if trace, ok := engine.GetLatestTrace(); ok {
		trace = normalizeTurnTraceCompatibility(trace)
		payload.LatestTrace = &trace
	}
	if insights, err := engine.GetPopulationInsights(); err == nil {
		payload.Population = trimPopulationInsights(insights, populationLimit)
	}
	if checkpoints, err := engine.ListCheckpoints(); err == nil {
		payload.Checkpoints = trimSaveSlots(checkpoints, checkpointLimit)
	}
	if presets, err := engine.ListScenarioPresets(); err == nil {
		payload.Presets = trimScenarioPresets(presets, presetLimit)
	}
	if reports, err := engine.ListExperimentReports(); err == nil {
		payload.ExperimentReports = trimExperimentReports(reports, reportLimit)
	}
	payload.AuditSummary = buildRuntimeAuditSummary(core.RuntimeAuditSnapshot{
		InstanceID:         payload.InstanceID,
		Instance:           engine.InstanceSummary(),
		State:              payload.State,
		PlayerRole:         payload.PlayerRole,
		FocusCharacter:     payload.FocusCharacter,
		Participants:       payload.Participants,
		ParticipantDetails: payload.ParticipantDetails,
		SimStatus:          payload.SimStatus,
		DirectorConfig:     payload.DirectorConfig,
		DirectorPlan:       payload.DirectorPlan,
		LatestTrace:        payload.LatestTrace,
		RecentTraces:       payload.RecentTraces,
		Population:         payload.Population,
		Checkpoints:        payload.Checkpoints,
		Presets:            payload.Presets,
		ExperimentReports:  payload.ExperimentReports,
		CreatedAt:          payload.CreatedAt,
	})
	writeJSONWithETag(w, r, payload)
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
		name = engine.GetFocusCharacter()
	}

	actions := engine.GetNPCActions(name, 0)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"focus_character": name,
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
	character := q.Get("focus_character")
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
	count := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("count")); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &count); err != nil {
			http.Error(w, "invalid count", http.StatusBadRequest)
			return
		}
	} else if r.Body != nil && r.ContentLength != 0 {
		var req struct {
			Count int `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Count > 0 {
			count = req.Count
		}
	}
	if count <= 0 || count > 200 {
		http.Error(w, "count must be between 1 and 200", http.StatusBadRequest)
		return
	}
	for i := 0; i < count; i++ {
		engine.ManualTick()
	}
	writeJSON(w, 200, map[string]interface{}{"ok": true, "count": count, "tick_status": engine.TickStatus()})
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
