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
	CreateStudioWorld(context.Context, storage.StudioCreateRequest) (storage.StudioCreateResult, error)
	ReadStudioEvent(context.Context, storage.StudioEventRequest) (storage.StudioEventEvidence, error)
	ListStudioScopes(context.Context, storage.StudioScopeRequest) (storage.StudioScopes, error)
	ListStudioEvents(context.Context, storage.StudioTimelineRequest) (storage.StudioTimeline, error)
	ReadStudioExplanation(context.Context, storage.StudioExplanationRequest) (storage.StudioExplanation, error)
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
	MoveRP(context.Context, core.RPMoveRequest) (storage.RPMoveResult, error)
	StartRPJourney(context.Context, core.RPMoveRequest) (storage.RPJourneyResult, error)
	CancelRPJourney(context.Context, core.RPJourneyCancelRequest) (storage.RPJourneyCancelRecord, error)
	SurveyRPMap(context.Context, storage.RPMapSurveyRequest) (storage.RPMapSurveyRecord, error)
	ReadRPMap(context.Context, core.RPSessionReadRequest) ([]storage.RPMapMemory, error)
	MaterializeRPLocation(context.Context, storage.RPLocationMaterializeRequest) (storage.RPLocationRecord, error)
	RenameRPLocation(context.Context, storage.RPLocationRenameRequest) (storage.RPLocationRenameRecord, error)
	DefineRPTimedEdge(context.Context, storage.RPTimedEdgeRequest) (storage.RPTimedEdgeRecord, error)
	DefineRPPerceptionLink(context.Context, storage.RPPerceptionLinkRequest) (storage.RPPerceptionLinkRecord, error)
	PlaceRPActorInZone(context.Context, storage.RPActorZoneRequest) (storage.RPActorZoneRecord, error)
	SocialRP(context.Context, core.RPSocialRequest) (storage.RPSocialResult, error)
	MaterializeRPBackground(context.Context, core.RPBackgroundRequest) (storage.RPBackgroundResult, error)
	DefineCareerOrganization(context.Context, core.CareerOrganizationRequest) (storage.CareerRecord, error)
	DefineCareerGradeScale(context.Context, core.CareerGradeScaleRequest) (storage.CareerRecord, error)
	OfferCareerPositionChange(context.Context, core.CareerPositionOfferRequest) (storage.CareerRecord, error)
	DeclineCareerPositionChange(context.Context, core.CareerPositionDeclineRequest) (storage.CareerRecord, error)
	AcceptCareerPositionChange(context.Context, core.CareerPositionAcceptRequest) (storage.CareerRecord, error)
	SpeakCareerAnnouncement(context.Context, core.CareerAnnouncementRequest) (storage.CareerRecord, error)
	DefineRPCulture(context.Context, storage.CultureDefinitionRequest) (storage.CultureRecord, error)
	DefineRPOpportunityPolicy(context.Context, storage.OpportunityPolicyRequest) (storage.OpportunityPolicyRecord, error)
	DefineRPEnvironmentSource(context.Context, storage.EnvironmentSourceRequest) (storage.EnvironmentSourceRecord, error)
	DefineRPStorefrontSource(context.Context, storage.StorefrontSourceRequest) (storage.StorefrontSourceRecord, error)
	DefineRPTransitWorks(context.Context, storage.TransitWorksRequest) (storage.TransitWorksRecord, error)
	ReadOwnRPLawCases(context.Context, storage.LawCasesRequest) ([]core.RPLawCase, error)
	DefineRPInstitution(context.Context, storage.InstitutionDefinitionRequest) (storage.InstitutionRecord, error)
	ChangeRPInstitutionAuthority(context.Context, storage.InstitutionAuthorityRequest) (storage.InstitutionRecord, error)
	ProposeRPLaw(context.Context, storage.LawProposalRequest) (storage.InstitutionRecord, error)
	EnactRPLaw(context.Context, storage.LawEnactmentRequest) (storage.InstitutionRecord, error)
	AnnounceRPLaw(context.Context, storage.LawAnnouncementRequest) (storage.InstitutionRecord, error)
	RecordRPLawViolation(context.Context, storage.LawViolationRequest) (storage.InstitutionRecord, error)
	EnforceRPLaw(context.Context, storage.LawEnforcementRequest) (storage.InstitutionRecord, error)
	DisputeRPLaw(context.Context, storage.LawDisputeRequest) (storage.InstitutionRecord, error)
	ForwardRPLawDispute(context.Context, storage.LawDisputeForwardRequest) (storage.InstitutionRecord, error)
	ReviewRPLawDispute(context.Context, storage.LawReviewRequest) (storage.InstitutionRecord, error)
	TransmitRPCulture(context.Context, storage.CultureTransmissionRequest) (storage.CultureRecord, error)
	InternalizeRPCulture(context.Context, storage.CultureStanceRequest) (storage.CultureRecord, error)
	AffiliateRPCulture(context.Context, storage.CultureAffiliationRequest) (storage.CultureRecord, error)
	ProposeRPCulturalFamily(context.Context, storage.CultureFamilyProposalRequest) (storage.CultureRecord, error)
	AcceptRPCulturalFamily(context.Context, storage.CultureFamilyAcceptRequest) (storage.CultureRecord, error)
	DefineRPCultureTerritory(context.Context, storage.CultureTerritoryRequest) (storage.CultureRecord, error)
	ChangeRPCultureAuthority(context.Context, storage.CultureAuthorityRequest) (storage.CultureRecord, error)
	EndCareerEmployment(context.Context, core.CareerExitRequest) (storage.CareerRecord, error)
	RequestCareerAggregateExit(context.Context, core.CareerAggregateExitRequest) (storage.CareerRecord, error)
	PostCareerPosition(context.Context, core.CareerPostingRequest) (storage.CareerRecord, error)
	ApplyForCareerPosition(context.Context, core.CareerApplicationRequest) (storage.CareerRecord, error)
	DiscoverCareerPositions(context.Context, string, string, string, string, int64) (storage.CareerMarket, error)
	ReadCareerApplication(context.Context, string, string, string, string) (storage.CareerRecord, error)
	InviteCareerInterview(context.Context, core.CareerInterviewRequest) (storage.CareerRecord, error)
	AnswerCareerInterview(context.Context, core.CareerInterviewAnswerRequest) (storage.CareerRecord, error)
	EvaluateCareerApplication(context.Context, core.CareerEvaluationRequest) (storage.CareerRecord, error)
	OfferCareerEmployment(context.Context, core.CareerOfferRequest) (storage.CareerRecord, error)
	DeclineCareerOffer(context.Context, core.CareerOfferDeclineRequest) (storage.CareerRecord, error)
	AcceptCareerOffer(context.Context, core.CareerOfferAcceptRequest) (storage.CareerRecord, error)
	ReadCareerAttendance(context.Context, string, string, string, string, int) (storage.CareerAttendanceRecord, error)
	RecordCareerPerformance(context.Context, core.CareerPerformanceRequest) (storage.CareerRecord, error)
	RegularizeCareerEmployment(context.Context, core.CareerRegularizationRequest) (storage.CareerRecord, error)
	RequestCareerLeave(context.Context, core.CareerLeaveRequest) (storage.CareerRecord, error)
	ReviewCareerLeave(context.Context, core.CareerLeaveReviewRequest) (storage.CareerRecord, error)
	OfferCareerOvertime(context.Context, core.CareerOvertimeRequest) (storage.CareerRecord, error)
	RespondCareerOvertime(context.Context, core.CareerOvertimeResponseRequest) (storage.CareerRecord, error)
	RaiseCareerWage(context.Context, core.CareerRaiseRequest) (storage.CareerRecord, error)
	ReferCareerCandidate(context.Context, core.CareerReferralRequest) (storage.CareerRecord, error)
	ReadCareerRecruitmentRecord(context.Context, string, string, string, string, string) (storage.CareerRecord, error)
	DefineEducationProgramLocal(context.Context, storage.EducationProgramRequest) (storage.EducationRecord, error)
	EnrollEducation(context.Context, storage.EducationEnrollmentRequest) (storage.EducationRecord, error)
	SubmitEducationExercise(context.Context, storage.EducationExerciseRequest) (storage.EducationRecord, error)
	CompleteEducationTraining(context.Context, storage.EducationCompletionRequest) (storage.EducationRecord, error)
	IssueEducationCredential(context.Context, storage.EducationCredentialRequest) (storage.EducationRecord, error)
	RevokeEducationCredential(context.Context, storage.EducationRevocationRequest) (storage.EducationRecord, error)
	ReadEducationQualification(context.Context, string, string, string, string) (storage.EducationQualificationView, error)
	DiscoverEducationPrograms(context.Context, string, string, string, string, string, string, int64) (storage.EducationProgramMarket, error)
	SetRPStyle(context.Context, storage.RPStyleSetRequest) (storage.RPStyleSetResult, error)
	ReadRPStyle(context.Context, core.RPSessionReadRequest) (storage.RPResolvedStyle, error)
	ReadRPWallet(context.Context, core.RPSessionReadRequest) (storage.RPWallet, error)
	ReadRPContacts(context.Context, storage.RPContactsReadRequest) (storage.RPContacts, error)
	ReadRPContext(context.Context, storage.RPContextReadRequest) (storage.RPClientContext, error)
	RetireRPRequest(context.Context, storage.RPRequestRetireRequest) (storage.RPRequestOutcome, error)
	DiscoverRPBindings(context.Context, storage.RPDiscoverRequest) (storage.RPDiscovery, error)
	ReadRPEvents(context.Context, storage.RPEventsReadRequest) (storage.RPClientEvents, error)
	SendRPInformation(context.Context, storage.RPInformationSendRequest) (storage.RPInformationRecord, error)
	RelayRPInformation(context.Context, storage.RPInformationRelayRequest) (storage.RPInformationRecord, error)
	PublishRPOrganizationNotice(context.Context, storage.RPOrganizationNoticePublishRequest) (storage.RPInformationRecord, error)
	SubmitRPSharedOrganizationNoticePublish(context.Context, storage.RPSharedOrganizationNoticePublishRequest) (storage.RPSharedRound, error)
	AccessRPOrganizationNotice(context.Context, storage.RPOrganizationNoticeAccessRequest) (storage.RPInformationRecord, error)
	ReadRPOrganizationNotices(context.Context, core.RPSessionReadRequest) (storage.RPOrganizationNoticeList, error)
	PublishRPPublicNotice(context.Context, storage.RPPublicNoticePublishRequest) (storage.RPInformationRecord, error)
	ReadRPNoticePublicationSources(context.Context, storage.RPNoticePublicationSourceReadRequest) (storage.RPNoticePublicationSourceList, error)
	SubmitRPSharedPublicNoticePublish(context.Context, storage.RPSharedPublicNoticePublishRequest) (storage.RPSharedRound, error)
	AccessRPPublicNotice(context.Context, storage.RPPublicNoticeAccessRequest) (storage.RPInformationRecord, error)
	ReadRPPublicNotices(context.Context, core.RPSessionReadRequest) (storage.RPPublicNoticeList, error)
	RecordRPInformationStance(context.Context, storage.RPInformationStanceRequest) (storage.RPInformationStanceRecord, error)
	ReadRPMessages(context.Context, storage.RPMessagesReadRequest) (storage.RPMessages, error)
	ReadRPWork(context.Context, core.RPSessionReadRequest) (storage.RPWork, error)
	ReadRPNarrative(context.Context, storage.RPNarrativeReadRequest) (storage.RPNarrativeReadResult, error)
	StreamRPNarrative(context.Context, storage.RPNarrativeReadRequest, func(core.RPNarrativeChunk) error) (storage.RPNarrativeReadResult, error)
	WaitRP(context.Context, core.RPWaitRequest) (storage.RPWaitResult, error)
	OpenRPSharedRoundLocal(context.Context, storage.RPSharedRoundOpenRequest) (storage.RPSharedRound, error)
	ReadRPSharedRound(context.Context, storage.RPSharedRoundReadRequest) (storage.RPSharedRound, error)
	SubmitRPSharedWait(context.Context, storage.RPSharedWaitRequest) (storage.RPSharedRound, error)
	SubmitRPSharedSpeech(context.Context, storage.RPSharedSpeechRequest) (storage.RPSharedRound, error)
	SubmitRPSharedMove(context.Context, storage.RPSharedMoveRequest) (storage.RPSharedRound, error)
	SubmitRPSharedSleep(context.Context, storage.RPSharedSleepRequest) (storage.RPSharedRound, error)
	SubmitRPSharedWorkTask(context.Context, storage.RPSharedWorkTaskRequest) (storage.RPSharedRound, error)
	SubmitRPSharedInformationSend(context.Context, storage.RPSharedInformationSendRequest) (storage.RPSharedRound, error)
	SubmitRPSharedInformationStance(context.Context, storage.RPSharedInformationStanceRequest) (storage.RPSharedRound, error)
	SubmitRPSharedInformationRelay(context.Context, storage.RPSharedInformationRelayRequest) (storage.RPSharedRound, error)
	SubmitRPSharedPublicNoticeAccess(context.Context, storage.RPSharedPublicNoticeAccessRequest) (storage.RPSharedRound, error)
	SubmitRPSharedOrganizationNoticeAccess(context.Context, storage.RPSharedOrganizationNoticeAccessRequest) (storage.RPSharedRound, error)
	AdvanceRPSharedRound(context.Context, storage.RPSharedRoundAdvanceRequest) (storage.RPSharedRound, error)
	SpeakRP(context.Context, core.RPSpeechRequest) (storage.RPSpeechResult, error)
	PlayRPTurn(context.Context, core.RPSpeechRequest) (storage.RPTurnResult, error)
	PlayResumeRPTurn(context.Context, storage.RPTurnResumeRequest) (storage.RPTurnResult, error)
	RunRPInteraction(context.Context, core.RPInteractionRequest) (storage.RPInteractionResult, error)
	ResumeRPInteraction(context.Context, storage.RPInteractionResumeRequest) (storage.RPInteractionResult, error)
	StopRPInteraction(context.Context, storage.RPInteractionResumeRequest) (storage.RPInteractionResult, error)
	ReadRPInteractionMode(context.Context, core.RPSessionReadRequest) (storage.RPInteractionModeView, error)
	SetRPInteractionMode(context.Context, storage.RPInteractionModeSetRequest) (storage.RPInteractionModeView, error)
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
	case "/api/v1/studio/events/read":
		s.handleStudioEvent(response, request, requestID, principalID)
	case "/api/v1/studio/worlds/create":
		s.handleStudioCreate(response, request, requestID, principalID)
	case "/api/v1/studio/scopes/list":
		s.handleStudioScopes(response, request, requestID, principalID)
	case "/api/v1/studio/events/list":
		s.handleStudioTimeline(response, request, requestID, principalID)
	case "/api/v1/studio/events/explain":
		s.handleStudioExplanation(response, request, requestID, principalID)
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
	case "/api/v1/rp/bindings/list":
		s.handleRPDiscovery(response, request, requestID, principalID)
	case "/api/v1/rp/sessions/read":
		s.handleRPSessionRead(response, request, requestID, principalID)
	case "/api/v1/rp/sessions/resume":
		s.handleRPSessionResume(response, request, requestID, principalID)
	case "/api/v1/rp/sessions/close":
		s.handleRPSessionClose(response, request, requestID, principalID)
	case "/api/v1/rp/observe":
		s.handleRPObserve(response, request, requestID, principalID)
	case "/api/v1/rp/context/read":
		s.handleRPContextRead(response, request, requestID, principalID)
	case "/api/v1/rp/information/direct/send":
		s.handleRPInformationSend(response, request, requestID, principalID)
	case "/api/v1/rp/information/rumor/relay":
		s.handleRPInformationRelay(response, request, requestID, principalID)
	case "/api/v1/rp/information/organization/publish":
		s.handleRPOrganizationNoticePublish(response, request, requestID, principalID)
	case "/api/v1/rp/information/organization/list":
		s.handleRPOrganizationNoticeList(response, request, requestID, principalID)
	case "/api/v1/rp/information/organization/access":
		s.handleRPOrganizationNoticeAccess(response, request, requestID, principalID)
	case "/api/v1/rp/information/public/publish":
		s.handleRPPublicNoticePublish(response, request, requestID, principalID)
	case "/api/v1/rp/information/public/list":
		s.handleRPPublicNoticeList(response, request, requestID, principalID)
	case "/api/v1/rp/information/publication/sources":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPNoticePublicationSourceReadRequest) *string { return &r.PrincipalID }, s.service.ReadRPNoticePublicationSources)
	case "/api/v1/rp/information/public/access":
		s.handleRPPublicNoticeAccess(response, request, requestID, principalID)
	case "/api/v1/rp/information/stance/record":
		s.handleRPInformationStance(response, request, requestID, principalID)
	case "/api/v1/rp/requests/retire":
		s.handleRPRequestRetire(response, request, requestID, principalID)
	case "/api/v1/rp/events":
		s.handleRPEvents(response, request, requestID, principalID, false)
	case "/api/v1/rp/events/stream":
		s.handleRPEvents(response, request, requestID, principalID, true)
	case "/api/v1/rp/actions/move":
		s.handleRPMove(response, request, requestID, principalID)
	case "/api/v1/rp/journeys/start":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *core.RPMoveRequest) *string { return &r.PrincipalID }, s.service.StartRPJourney)
	case "/api/v1/rp/journeys/cancel":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *core.RPJourneyCancelRequest) *string { return &r.PrincipalID }, s.service.CancelRPJourney)
	case "/api/v1/rp/map/survey":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPMapSurveyRequest) *string { return &r.PrincipalID }, s.service.SurveyRPMap)
	case "/api/v1/rp/map/read":
		input, ok := s.decodeRPSessionRead(response, request, requestID, principalID)
		if !ok {
			return
		}
		result, err := s.service.ReadRPMap(request.Context(), input)
		if err != nil {
			writeError(response, requestID, err)
			return
		}
		writeData(response, http.StatusOK, result)
	case "/api/v1/rp/locations/materialize":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPLocationMaterializeRequest) *string { return &r.Binding.PrincipalID }, s.service.MaterializeRPLocation)
	case "/api/v1/rp/locations/rename":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPLocationRenameRequest) *string { return &r.Binding.PrincipalID }, s.service.RenameRPLocation)
	case "/api/v1/rp/edges/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPTimedEdgeRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPTimedEdge)
	case "/api/v1/rp/perception/links/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPPerceptionLinkRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPPerceptionLink)
	case "/api/v1/rp/perception/zones/place":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPActorZoneRequest) *string { return &r.Binding.PrincipalID }, s.service.PlaceRPActorInZone)
	case "/api/v1/rp/actions/social":
		s.handleRPSocial(response, request, requestID, principalID)
	case "/api/v1/rp/background/materialize":
		s.handleRPBackground(response, request, requestID, principalID)
	case "/api/v1/career/organizations/define":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerOrganizationRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineCareerOrganization)
	case "/api/v1/culture/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureDefinitionRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPCulture)
	case "/api/v1/opportunities/policy/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.OpportunityPolicyRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPOpportunityPolicy)
	case "/api/v1/opportunities/environment/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.EnvironmentSourceRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPEnvironmentSource)
	case "/api/v1/opportunities/storefront/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.StorefrontSourceRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPStorefrontSource)
	case "/api/v1/opportunities/transit/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.TransitWorksRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPTransitWorks)
	case "/api/v1/laws/cases/own":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawCasesRequest) *string { return &r.PrincipalID }, s.service.ReadOwnRPLawCases)
	case "/api/v1/institutions/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.InstitutionDefinitionRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPInstitution)
	case "/api/v1/institutions/authority":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.InstitutionAuthorityRequest) *string { return &r.Binding.PrincipalID }, s.service.ChangeRPInstitutionAuthority)
	case "/api/v1/laws/propose":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawProposalRequest) *string { return &r.Binding.PrincipalID }, s.service.ProposeRPLaw)
	case "/api/v1/laws/enact":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawEnactmentRequest) *string { return &r.Binding.PrincipalID }, s.service.EnactRPLaw)
	case "/api/v1/laws/announce":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawAnnouncementRequest) *string { return &r.Binding.PrincipalID }, s.service.AnnounceRPLaw)
	case "/api/v1/laws/violations/record":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawViolationRequest) *string { return &r.Binding.PrincipalID }, s.service.RecordRPLawViolation)
	case "/api/v1/laws/enforce":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawEnforcementRequest) *string { return &r.Binding.PrincipalID }, s.service.EnforceRPLaw)
	case "/api/v1/laws/dispute":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawDisputeRequest) *string { return &r.Binding.PrincipalID }, s.service.DisputeRPLaw)
	case "/api/v1/laws/disputes/forward":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawDisputeForwardRequest) *string { return &r.Binding.PrincipalID }, s.service.ForwardRPLawDispute)
	case "/api/v1/laws/review":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.LawReviewRequest) *string { return &r.Binding.PrincipalID }, s.service.ReviewRPLawDispute)
	case "/api/v1/culture/territories/define":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureTerritoryRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineRPCultureTerritory)
	case "/api/v1/culture/territories/authority":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureAuthorityRequest) *string { return &r.Binding.PrincipalID }, s.service.ChangeRPCultureAuthority)
	case "/api/v1/culture/affiliate":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureAffiliationRequest) *string { return &r.Binding.PrincipalID }, s.service.AffiliateRPCulture)
	case "/api/v1/culture/families/propose":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureFamilyProposalRequest) *string { return &r.Binding.PrincipalID }, s.service.ProposeRPCulturalFamily)
	case "/api/v1/culture/families/accept":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureFamilyAcceptRequest) *string { return &r.Binding.PrincipalID }, s.service.AcceptRPCulturalFamily)
	case "/api/v1/culture/transmit":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureTransmissionRequest) *string { return &r.Binding.PrincipalID }, s.service.TransmitRPCulture)
	case "/api/v1/culture/internalize":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.CultureStanceRequest) *string { return &r.Binding.PrincipalID }, s.service.InternalizeRPCulture)
	case "/api/v1/career/positions/post":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerPostingRequest) *string { return &r.Binding.PrincipalID }, s.service.PostCareerPosition)
	case "/api/v1/career/grades/define":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerGradeScaleRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineCareerGradeScale)
	case "/api/v1/career/position-changes/offer":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerPositionOfferRequest) *string { return &r.Binding.PrincipalID }, s.service.OfferCareerPositionChange)
	case "/api/v1/career/position-changes/decline":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerPositionDeclineRequest) *string { return &r.Binding.PrincipalID }, s.service.DeclineCareerPositionChange)
	case "/api/v1/career/position-changes/accept":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerPositionAcceptRequest) *string { return &r.Binding.PrincipalID }, s.service.AcceptCareerPositionChange)
	case "/api/v1/career/announcements/speak":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerAnnouncementRequest) *string { return &r.Binding.PrincipalID }, s.service.SpeakCareerAnnouncement)
	case "/api/v1/career/employment/end":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerExitRequest) *string { return &r.Binding.PrincipalID }, s.service.EndCareerEmployment)
	case "/api/v1/career/aggregate-employment/exit":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerAggregateExitRequest) *string { return &r.Binding.PrincipalID }, s.service.RequestCareerAggregateExit)
	case "/api/v1/career/applications/submit":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerApplicationRequest) *string { return &r.Binding.PrincipalID }, s.service.ApplyForCareerPosition)
	case "/api/v1/career/positions/query":
		s.handleCareerMarket(response, request, requestID, principalID)
	case "/api/v1/career/applications/read":
		s.handleCareerApplicationRead(response, request, requestID, principalID)
	case "/api/v1/career/interviews/invite":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerInterviewRequest) *string { return &r.Binding.PrincipalID }, s.service.InviteCareerInterview)
	case "/api/v1/career/interviews/answer":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerInterviewAnswerRequest) *string { return &r.Binding.PrincipalID }, s.service.AnswerCareerInterview)
	case "/api/v1/career/evaluations/record":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerEvaluationRequest) *string { return &r.Binding.PrincipalID }, s.service.EvaluateCareerApplication)
	case "/api/v1/career/offers/make":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerOfferRequest) *string { return &r.Binding.PrincipalID }, s.service.OfferCareerEmployment)
	case "/api/v1/career/offers/decline":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerOfferDeclineRequest) *string { return &r.Binding.PrincipalID }, s.service.DeclineCareerOffer)
	case "/api/v1/career/offers/accept":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerOfferAcceptRequest) *string { return &r.Binding.PrincipalID }, s.service.AcceptCareerOffer)
	case "/api/v1/career/attendance/read":
		s.handleCareerAttendanceRead(response, request, requestID, principalID)
	case "/api/v1/career/performance/record":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerPerformanceRequest) *string { return &r.Binding.PrincipalID }, s.service.RecordCareerPerformance)
	case "/api/v1/career/employment/regularize":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerRegularizationRequest) *string { return &r.Binding.PrincipalID }, s.service.RegularizeCareerEmployment)
	case "/api/v1/career/leave/request":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerLeaveRequest) *string { return &r.Binding.PrincipalID }, s.service.RequestCareerLeave)
	case "/api/v1/career/leave/review":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerLeaveReviewRequest) *string { return &r.Binding.PrincipalID }, s.service.ReviewCareerLeave)
	case "/api/v1/career/overtime/offer":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerOvertimeRequest) *string { return &r.Binding.PrincipalID }, s.service.OfferCareerOvertime)
	case "/api/v1/career/overtime/respond":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerOvertimeResponseRequest) *string { return &r.Binding.PrincipalID }, s.service.RespondCareerOvertime)
	case "/api/v1/career/employment/raise":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerRaiseRequest) *string { return &r.Binding.PrincipalID }, s.service.RaiseCareerWage)
	case "/api/v1/career/referrals/submit":
		handleCareerCommand(s, response, request, requestID, principalID, func(r *core.CareerReferralRequest) *string { return &r.Binding.PrincipalID }, s.service.ReferCareerCandidate)
	case "/api/v1/career/records/read":
		s.handleCareerRecordRead(response, request, requestID, principalID)
	case "/api/v1/education/programs/define-local":
		handleEducationCommand(s, response, request, requestID, principalID, func(r *storage.EducationProgramRequest) *string { return &r.Binding.PrincipalID }, s.service.DefineEducationProgramLocal)
	case "/api/v1/education/enrollments/start":
		handleEducationCommand(s, response, request, requestID, principalID, func(r *storage.EducationEnrollmentRequest) *string { return &r.Binding.PrincipalID }, s.service.EnrollEducation)
	case "/api/v1/education/exercises/submit":
		handleEducationCommand(s, response, request, requestID, principalID, func(r *storage.EducationExerciseRequest) *string { return &r.Binding.PrincipalID }, s.service.SubmitEducationExercise)
	case "/api/v1/education/training/complete":
		handleEducationCommand(s, response, request, requestID, principalID, func(r *storage.EducationCompletionRequest) *string { return &r.Binding.PrincipalID }, s.service.CompleteEducationTraining)
	case "/api/v1/education/credentials/issue":
		handleEducationCommand(s, response, request, requestID, principalID, func(r *storage.EducationCredentialRequest) *string { return &r.Binding.PrincipalID }, s.service.IssueEducationCredential)
	case "/api/v1/education/credentials/revoke":
		handleEducationCommand(s, response, request, requestID, principalID, func(r *storage.EducationRevocationRequest) *string { return &r.Binding.PrincipalID }, s.service.RevokeEducationCredential)
	case "/api/v1/education/qualifications/own":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *educationQualificationRead) *string { return &r.PrincipalID }, func(ctx context.Context, r educationQualificationRead) (storage.EducationQualificationView, error) {
			return s.service.ReadEducationQualification(ctx, r.PrincipalID, r.InstanceID, r.BranchID, r.LearnerID)
		})
	case "/api/v1/education/programs/query":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *educationProgramRead) *string { return &r.PrincipalID }, func(ctx context.Context, r educationProgramRead) (storage.EducationProgramMarket, error) {
			return s.service.DiscoverEducationPrograms(ctx, r.PrincipalID, r.InstanceID, r.BranchID, r.LearnerID, r.Code, r.IssuerID, r.AfterSequence)
		})
	case "/api/v1/rp/style/set":
		s.handleRPStyleSet(response, request, requestID, principalID)
	case "/api/v1/rp/style/read":
		s.handleRPStyleRead(response, request, requestID, principalID)
	case "/api/v1/rp/wallet/read":
		s.handleRPWalletRead(response, request, requestID, principalID)
	case "/api/v1/rp/contacts/read":
		s.handleRPContactsRead(response, request, requestID, principalID)
	case "/api/v1/rp/messages/read":
		s.handleRPMessagesRead(response, request, requestID, principalID)
	case "/api/v1/rp/work/read":
		s.handleRPWorkRead(response, request, requestID, principalID)
	case "/api/v1/rp/narrative/render":
		s.handleRPNarrativeRead(response, request, requestID, principalID)
	case "/api/v1/rp/narrative/stream":
		s.handleRPNarrativeStream(response, request, requestID, principalID)
	case "/api/v1/rp/actions/wait":
		s.handleRPWait(response, request, requestID, principalID)
	case "/api/v1/rp/rounds/open":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedRoundOpenRequest) *string { return &r.Binding.PrincipalID }, s.service.OpenRPSharedRoundLocal)
	case "/api/v1/rp/rounds/read":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedRoundReadRequest) *string { return &r.PrincipalID }, s.service.ReadRPSharedRound)
	case "/api/v1/rp/rounds/wait":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedWaitRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedWait)
	case "/api/v1/rp/rounds/speech":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedSpeechRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedSpeech)
	case "/api/v1/rp/rounds/move":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedMoveRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedMove)
	case "/api/v1/rp/rounds/sleep":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedSleepRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedSleep)
	case "/api/v1/rp/rounds/work-task":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedWorkTaskRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedWorkTask)
	case "/api/v1/rp/rounds/information-send":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedInformationSendRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedInformationSend)
	case "/api/v1/rp/rounds/information-stance":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedInformationStanceRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedInformationStance)
	case "/api/v1/rp/rounds/information-relay":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedInformationRelayRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedInformationRelay)
	case "/api/v1/rp/rounds/information-public-access":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedPublicNoticeAccessRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedPublicNoticeAccess)
	case "/api/v1/rp/rounds/information-organization-access":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedOrganizationNoticeAccessRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedOrganizationNoticeAccess)
	case "/api/v1/rp/rounds/information-public-publish":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedPublicNoticePublishRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedPublicNoticePublish)
	case "/api/v1/rp/rounds/information-organization-publish":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedOrganizationNoticePublishRequest) *string { return &r.PrincipalID }, s.service.SubmitRPSharedOrganizationNoticePublish)
	case "/api/v1/rp/rounds/advance":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *storage.RPSharedRoundAdvanceRequest) *string { return &r.PrincipalID }, s.service.AdvanceRPSharedRound)
	case "/api/v1/rp/actions/speak":
		s.handleRPSpeak(response, request, requestID, principalID)
	case "/api/v1/rp/turns/run":
		s.handleRPTurnRun(response, request, requestID, principalID)
	case "/api/v1/rp/turns/resume":
		s.handleRPTurnResume(response, request, requestID, principalID)
	case "/api/v1/rp/interactions/run":
		s.handleRPInteractionRun(response, request, requestID, principalID)
	case "/api/v1/rp/interactions/resume":
		s.handleRPInteractionResume(response, request, requestID, principalID)
	case "/api/v1/rp/interactions/stop":
		s.handleRPInteractionStop(response, request, requestID, principalID)
	case "/api/v1/rp/interactions/default/read":
		s.handleRPInteractionModeRead(response, request, requestID, principalID)
	case "/api/v1/rp/interactions/default/set":
		s.handleRPInteractionModeSet(response, request, requestID, principalID)
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

func (s *Server) handleRPMove(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPMoveRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.MoveRP(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPBackground(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPBackgroundRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.MaterializeRPBackground(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPSocial(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPSocialRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.SocialRP(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPWait(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPWaitRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.WaitRP(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPSpeak(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPSpeechRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.SpeakRP(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPTurnRun(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input core.RPSpeechRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.PlayRPTurn(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleRPTurnResume(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input storage.RPTurnResumeRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.PlayResumeRPTurn(request.Context(), input)
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
		case core.CodeIdempotencyMismatch, core.CodeCommandInProgress, core.CodeRequestRetired, core.CodeBranchConflict, core.CodeMaterializationConflict:
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
