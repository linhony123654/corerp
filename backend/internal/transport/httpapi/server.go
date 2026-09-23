package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

const defaultMaxBodyBytes int64 = 1 << 20

type Service interface {
	Ready(context.Context) error
	Purchase(context.Context, core.PurchaseCommand) (core.PurchaseResult, error)
	IssueCurrency(context.Context, core.IssueCurrencyCommand) (core.IssueCurrencyResult, error)
	MaterializeCohort(context.Context, core.MaterializeCohortCommand) (core.CohortTransitionResult, error)
	DematerializeCohort(context.Context, core.DematerializeCohortCommand) (core.CohortTransitionResult, error)
	RunStrictWorldAuthorized(context.Context, core.StrictSimulationRequest) (storage.StrictRunResult, error)
	RunAgentLifeAuthorized(context.Context, core.AgentLifeRunRequest) (storage.AgentLifeRunResult, error)
	DefineM2AgentRoutine(context.Context, core.AgentRoutineRequest) (storage.AgentRoutineResult, error)
	ReadPrivateEconomy(context.Context, core.PrivateEconomicRead) (storage.PrivateEconomicView, error)
	ReadAgentKnowledge(context.Context, core.AgentKnowledgeRead) (storage.AgentKnowledgeView, error)
	ResolveEncounter(context.Context, core.EncounterRead) (storage.EncounterView, error)
	OpenRPSession(context.Context, core.RPSessionOpenRequest) (storage.RPSession, error)
	ReadRPSession(context.Context, core.RPSessionReadRequest) (storage.RPSession, error)
	ResumeRPSession(context.Context, core.RPSessionReadRequest) (storage.RPSession, error)
	CloseRPSession(context.Context, core.RPSessionReadRequest) (storage.RPSession, error)
	ObserveRPSession(context.Context, core.RPSessionReadRequest) (storage.RPObservation, error)
	ReadDemoStateAuthorized(context.Context, core.StateReadRequest) (storage.State, error)
	ListVisibleEvents(context.Context, core.VisibleEventRequest) (storage.VisibleEventPage, error)
}

type Server struct {
	service           Service
	authenticator     Authenticator
	cursors           *CursorCodec
	maxBodyBytes      int64
	eventPollInterval time.Duration
	heartbeatInterval time.Duration
	requestCount      atomic.Uint64
}

func New(service Service, authenticator Authenticator, cursors *CursorCodec) (*Server, error) {
	if service == nil {
		return nil, core.NewError(core.CodeInvalidArgument, "HTTP API service is required")
	}
	if authenticator == nil {
		return nil, core.NewError(core.CodeInvalidArgument, "HTTP API authenticator is required")
	}
	if cursors == nil {
		return nil, core.NewError(core.CodeInvalidArgument, "HTTP API cursor codec is required")
	}
	return &Server{
		service: service, authenticator: authenticator, cursors: cursors,
		maxBodyBytes: defaultMaxBodyBytes, eventPollInterval: 250 * time.Millisecond,
		heartbeatInterval: 15 * time.Second,
	}, nil
}

func (s *Server) Handler() http.Handler { return s }

func (s *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	requestID := fmt.Sprintf("req-%016x", s.requestCount.Add(1))
	response.Header().Set("X-Request-ID", requestID)
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	defer func() {
		if recover() != nil {
			writeError(response, requestID, core.NewError(core.CodeStorageFailure, "internal server error"))
		}
	}()

	switch request.URL.Path {
	case "/healthz":
		if !requireMethod(response, requestID, request, http.MethodGet) {
			return
		}
		writeData(response, http.StatusOK, map[string]string{"status": "ok"})
		return
	case "/readyz":
		if !requireMethod(response, requestID, request, http.MethodGet) {
			return
		}
		if err := s.service.Ready(request.Context()); err != nil {
			writeErrorStatus(response, requestID, err, http.StatusServiceUnavailable)
			return
		}
		writeData(response, http.StatusOK, map[string]string{"status": "ready"})
		return
	}

	principalID, err := s.authenticator.Authenticate(request)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	switch request.URL.Path {
	case "/api/v1/commands/purchase":
		s.handlePurchase(response, request, requestID, principalID)
	case "/api/v1/commands/issue-currency":
		s.handleIssuance(response, request, requestID, principalID)
	case "/api/v1/commands/materialize-cohort":
		s.handleMaterializeCohort(response, request, requestID, principalID)
	case "/api/v1/commands/dematerialize-cohort":
		s.handleDematerializeCohort(response, request, requestID, principalID)
	case "/api/v1/simulations/strict":
		s.handleSimulation(response, request, requestID, principalID)
	case "/api/v1/simulations/agents":
		s.handleAgentSimulation(response, request, requestID, principalID)
	case "/api/v1/commands/define-agent-routine":
		s.handleAgentRoutine(response, request, requestID, principalID)
	case "/api/v1/private-economy/query":
		s.handlePrivateEconomy(response, request, requestID, principalID)
	case "/api/v1/agent-knowledge/query":
		s.handleAgentKnowledge(response, request, requestID, principalID)
	case "/api/v1/encounters/query":
		s.handleEncounter(response, request, requestID, principalID)
	case "/api/v1/rp/sessions/open":
		s.handleRPSessionOpen(response, request, requestID, principalID)
	case "/api/v1/rp/sessions/read":
		s.handleRPSessionRead(response, request, requestID, principalID)
	case "/api/v1/rp/sessions/resume":
		s.handleRPSessionResume(response, request, requestID, principalID)
	case "/api/v1/rp/sessions/close":
		s.handleRPSessionClose(response, request, requestID, principalID)
	case "/api/v1/rp/observe":
		s.handleRPObserve(response, request, requestID, principalID)
	case "/api/v1/state/query":
		s.handleState(response, request, requestID, principalID)
	case "/api/v1/events":
		s.handleEvents(response, request, requestID, principalID)
	case "/api/v1/events/stream":
		s.handleEventStream(response, request, requestID, principalID)
	default:
		writeErrorStatus(response, requestID, core.NewError(core.CodeNotFound, "route not found"), http.StatusNotFound)
	}
}

type visibleEventResponse struct {
	Events     []storage.VisibleEvent `json:"events"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

func (s *Server) handleEvents(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodGet) {
		return
	}
	input, previousCursor, err := s.visibleEventRequest(request, principalID)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	page, err := s.service.ListVisibleEvents(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	nextCursor := previousCursor
	if len(page.Events) > 0 {
		last := page.Events[len(page.Events)-1]
		nextCursor, err = s.cursors.Encode(principalID, input.InstanceID, input.BranchID, last.Sequence)
		if err != nil {
			writeError(response, requestID, err)
			return
		}
	}
	writeData(response, http.StatusOK, visibleEventResponse{Events: page.Events, NextCursor: nextCursor})
}

func (s *Server) handleEventStream(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodGet) {
		return
	}
	input, _, err := s.visibleEventRequest(request, principalID)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	page, err := s.service.ListVisibleEvents(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	flusher, ok := response.(http.Flusher)
	if !ok {
		writeErrorStatus(response, requestID, core.NewError(core.CodeStorageFailure, "streaming is unavailable"), http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	response.Header().Set("Connection", "keep-alive")
	response.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(response)
	setWriteDeadline := func() error {
		err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if errors.Is(err, http.ErrNotSupported) {
			return nil
		}
		return err
	}
	writePage := func(page storage.VisibleEventPage) error {
		if err := setWriteDeadline(); err != nil {
			return err
		}
		for _, event := range page.Events {
			cursor, err := s.cursors.Encode(principalID, input.InstanceID, input.BranchID, event.Sequence)
			if err != nil {
				return err
			}
			payload, err := json.Marshal(event)
			if err != nil {
				return core.WrapError(core.CodeStorageFailure, "encode visible SSE event", err)
			}
			if _, err := fmt.Fprintf(response, "id: %s\nevent: world_event\ndata: %s\n\n", cursor, payload); err != nil {
				return err
			}
		}
		flusher.Flush()
		return nil
	}
	if err := writePage(page); err != nil {
		return
	}
	internalAfter := page.ScannedThrough
	poll := time.NewTicker(s.eventPollInterval)
	heartbeat := time.NewTicker(s.heartbeatInterval)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-poll.C:
			input.AfterSequence = internalAfter
			page, err := s.service.ListVisibleEvents(request.Context(), input)
			if err != nil {
				return
			}
			internalAfter = page.ScannedThrough
			if len(page.Events) > 0 {
				if err := writePage(page); err != nil {
					return
				}
			}
		case <-heartbeat.C:
			if err := setWriteDeadline(); err != nil {
				return
			}
			if _, err := io.WriteString(response, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) visibleEventRequest(request *http.Request, principalID string) (core.VisibleEventRequest, string, error) {
	query := request.URL.Query()
	limit := 50
	if text := query.Get("limit"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil {
			return core.VisibleEventRequest{}, "", core.NewError(core.CodeInvalidArgument, "event limit must be an integer")
		}
		limit = value
	}
	input := core.VisibleEventRequest{
		PrincipalID: principalID, CapabilityID: query.Get("capability_id"),
		InstanceID: query.Get("instance_id"), BranchID: query.Get("branch_id"),
		SubjectID: query.Get("subject_id"), Limit: limit,
	}
	cursor := query.Get("cursor")
	if cursor == "" {
		cursor = request.Header.Get("Last-Event-ID")
	}
	if cursor != "" {
		sequence, err := s.cursors.Decode(cursor, principalID, input.InstanceID, input.BranchID)
		if err != nil {
			return core.VisibleEventRequest{}, "", err
		}
		input.AfterSequence = sequence
	}
	if err := input.Validate(); err != nil {
		return core.VisibleEventRequest{}, "", err
	}
	return input, cursor, nil
}

func (s *Server) handlePurchase(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var command core.PurchaseCommand
	if err := decodeJSON(response, request, s.maxBodyBytes, &command); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&command.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.Purchase(request.Context(), command)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleIssuance(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var command core.IssueCurrencyCommand
	if err := decodeJSON(response, request, s.maxBodyBytes, &command); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&command.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.IssueCurrency(request.Context(), command)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleMaterializeCohort(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var command core.MaterializeCohortCommand
	if err := decodeJSON(response, request, s.maxBodyBytes, &command); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&command.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.MaterializeCohort(request.Context(), command)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleDematerializeCohort(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var command core.DematerializeCohortCommand
	if err := decodeJSON(response, request, s.maxBodyBytes, &command); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&command.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.DematerializeCohort(request.Context(), command)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleSimulation(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.StrictSimulationRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.RunStrictWorldAuthorized(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handlePrivateEconomy(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.PrivateEconomicRead
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadPrivateEconomy(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleAgentSimulation(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.AgentLifeRunRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.RunAgentLifeAuthorized(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleAgentRoutine(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.AgentRoutineRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.DefineM2AgentRoutine(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleAgentKnowledge(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.AgentKnowledgeRead
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadAgentKnowledge(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleEncounter(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.EncounterRead
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ResolveEncounter(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPSessionOpen(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPSessionOpenRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.OpenRPSession(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPSessionRead(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	input, ok := s.decodeRPSessionRead(response, request, requestID, principalID)
	if !ok {
		return
	}
	result, err := s.service.ReadRPSession(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPSessionResume(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	input, ok := s.decodeRPSessionRead(response, request, requestID, principalID)
	if !ok {
		return
	}
	result, err := s.service.ResumeRPSession(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPSessionClose(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	input, ok := s.decodeRPSessionRead(response, request, requestID, principalID)
	if !ok {
		return
	}
	result, err := s.service.CloseRPSession(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPObserve(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	input, ok := s.decodeRPSessionRead(response, request, requestID, principalID)
	if !ok {
		return
	}
	result, err := s.service.ObserveRPSession(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) decodeRPSessionRead(response http.ResponseWriter, request *http.Request, requestID, principalID string) (core.RPSessionReadRequest, bool) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return core.RPSessionReadRequest{}, false
	}
	var input core.RPSessionReadRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return core.RPSessionReadRequest{}, false
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return core.RPSessionReadRequest{}, false
	}
	return input, true
}

func (s *Server) handleState(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.StateReadRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadDemoStateAuthorized(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func bindPrincipal(bodyPrincipal *string, authenticated string) error {
	if *bodyPrincipal == "" {
		*bodyPrincipal = authenticated
		return nil
	}
	if *bodyPrincipal != authenticated {
		return core.NewError(core.CodeUnauthorized, "request principal does not match the authenticated principal")
	}
	return nil
}

type bodyTooLargeError struct{ cause error }

func (e *bodyTooLargeError) Error() string { return e.cause.Error() }

func decodeJSON(response http.ResponseWriter, request *http.Request, limit int64, destination any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return core.NewError(core.CodeInvalidArgument, "Content-Type must be application/json")
	}
	request.Body = http.MaxBytesReader(response, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return &bodyTooLargeError{cause: err}
		}
		return core.WrapError(core.CodeInvalidArgument, "request body must be one valid JSON object with known fields", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return core.NewError(core.CodeInvalidArgument, "request body must contain exactly one JSON value")
		}
		return core.WrapError(core.CodeInvalidArgument, "request body contains trailing data", err)
	}
	return nil
}

func requireMethod(response http.ResponseWriter, requestID string, request *http.Request, expected string) bool {
	if request.Method == expected {
		return true
	}
	response.Header().Set("Allow", expected)
	writeEnvelope(response, http.StatusMethodNotAllowed, map[string]any{"error": map[string]string{
		"code": "METHOD_NOT_ALLOWED", "message": "method not allowed", "request_id": requestID,
	}})
	return false
}

func writeDecodeError(response http.ResponseWriter, requestID string, err error) {
	var tooLarge *bodyTooLargeError
	if errors.As(err, &tooLarge) {
		writeErrorStatus(response, requestID, core.NewError(core.CodeInvalidArgument, "request body exceeds 1048576 bytes"), http.StatusRequestEntityTooLarge)
		return
	}
	writeError(response, requestID, err)
}

func writeError(response http.ResponseWriter, requestID string, err error) {
	status := http.StatusInternalServerError
	var typed *core.Error
	if errors.As(err, &typed) {
		switch typed.Code {
		case core.CodeUnauthenticated:
			status = http.StatusUnauthorized
		case core.CodeUnauthorized:
			status = http.StatusForbidden
		case core.CodeNotFound:
			status = http.StatusNotFound
		case core.CodeIdempotencyMismatch, core.CodeCommandInProgress, core.CodeBranchConflict, core.CodeMaterializationConflict:
			status = http.StatusConflict
		case core.CodeInsufficientFunds, core.CodeInsufficientStock, core.CodeIssuanceLimit, core.CodeIntegerOverflow, core.CodeConservationFailed:
			status = http.StatusUnprocessableEntity
		case core.CodeInvalidArgument:
			status = http.StatusBadRequest
		}
	}
	writeErrorStatus(response, requestID, err, status)
}

func writeErrorStatus(response http.ResponseWriter, requestID string, err error, status int) {
	code := string(core.CodeStorageFailure)
	message := "internal server error"
	var typed *core.Error
	if errors.As(err, &typed) {
		code = string(typed.Code)
		message = typed.Message
		if message == "" {
			message = code
		}
	}
	writeEnvelope(response, status, map[string]any{"error": map[string]string{
		"code": code, "message": message, "request_id": requestID,
	}})
}

func writeData(response http.ResponseWriter, status int, data any) {
	writeEnvelope(response, status, map[string]any{"data": data})
}

func writeEnvelope(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
