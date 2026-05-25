package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"corerp/internal/runtime"
)

type Server struct {
	engine *runtime.Engine
}

func NewServer(engine *runtime.Engine) *Server {
	return &Server{engine: engine}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/character", s.handleCharacter)
	mux.HandleFunc("/api/characters", s.handleCharacters)
	mux.HandleFunc("/api/switch", s.handleSwitch)
	mux.HandleFunc("/api/world", s.handleWorld)
	mux.HandleFunc("/api/npc-actions", s.handleNPCActions)
	mux.HandleFunc("/api/causality", s.handleCausality)
	mux.HandleFunc("/api/debug/memory", s.handleDebugMemory)
	mux.HandleFunc("/api/director", s.handleDirector)
	mux.HandleFunc("/", s.handleStatic)
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

	ch, err := s.engine.ProcessTurn(req.Message)
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

	state := s.engine.GetState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func (s *Server) handleCharacter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	char, ok := s.engine.GetCharacter()
	if !ok {
		http.Error(w, "Character not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(char)
}

func (s *Server) handleCharacters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	chars := s.engine.GetLoadedCharacters()
	active := s.engine.GetCharacterName()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active":     active,
		"characters": chars,
	})
}

func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Character string `json:"character"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.engine.SwitchCharacter(req.Character); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Include recent NPC actions for "while you were away" summary
	npcActions := s.engine.GetNPCActions(req.Character, 0)

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"name": s.engine.GetWorldName(),
	})
}

func (s *Server) handleNPCActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("character")
	if name == "" {
		name = s.engine.GetCharacterName()
	}

	actions := s.engine.GetNPCActions(name, 0)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"character": name,
		"actions":   actions,
	})
}

func (s *Server) handleCausality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	chain, err := s.engine.GetCausalityChain(eventID, depth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	summary, _ := s.engine.GetCausalitySummary(eventID, depth)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"event_id": eventID,
		"depth":    depth,
		"chain":    chain,
		"summary":  summary,
	})
}

func (s *Server) handleDebugMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := s.engine.DebugInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *Server) handleDirector(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		s.engine.SetTension(req.Value)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"action":  req.Action,
			"value":   req.Value,
			"state":   s.engine.GetState().Tension,
		})
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
	}
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
	w.Write(data)
}
