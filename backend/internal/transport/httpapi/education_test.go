package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestEducationHTTPBoundedTrainingUnlocksCareerApplication(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "education-http.db"))
	defer s.Close()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := setup.Routine.EventSequence
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	org := core.CareerOrganizationRequest{Binding: binding("edu-http-org"), Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op", ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}}
	response := performJSON(t, handler, "/api/v1/career/organizations/define", creatorToken, org)
	assertStatus(t, response, http.StatusOK)
	head = decodeData[storage.CareerRecord](t, response).EventSequence
	posting := core.CareerPostingRequest{Binding: binding("edu-http-post"), Posting: core.CareerPostingDefinition{PositionID: "edu_http_position", OrganizationID: org.Organization.OrganizationID, Title: "Safety assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12, RequiredQualifications: []string{}, RequiredCredentials: []core.CredentialRequirement{{Code: "safety_training", IssuerID: storage.M2AgentBoPrincipal}}}}
	response = performJSON(t, handler, "/api/v1/career/positions/post", boAgentToken, posting)
	assertStatus(t, response, http.StatusOK)
	head = decodeData[storage.CareerRecord](t, response).EventSequence
	application := core.CareerApplicationRequest{Binding: binding("edu-http-apply"), ApplicationID: "edu_http_application", PositionID: posting.Posting.PositionID, CandidateID: storage.M2AgentAdaID, Statement: "I can describe the safety checks."}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/applications/submit", adaAgentToken, application), http.StatusBadRequest, core.CodeInvalidArgument)
	program := storage.EducationProgramRequest{Binding: binding("edu-http-program"), ProgramID: "edu_http_safety", Code: "safety_training", IssuerID: storage.M2AgentBoPrincipal, MinimumMinutes: 1, ExercisePrompt: "Describe the safety checks."}
	assertAPIError(t, performJSON(t, handler, "/api/v1/education/programs/define-local", boAgentToken, program), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/education/programs/define-local", operatorToken, program)
	assertStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), "event_education") {
		t.Fatal("program receipt exposed raw Event ID")
	}
	head = decodeData[educationReceipt](t, response).EventSequence
	programQuery := educationProgramRead{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, LearnerID: storage.M2AgentAdaID, Code: program.Code, IssuerID: program.IssuerID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/education/programs/query", boAgentToken, programQuery), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/education/programs/query", adaAgentToken, programQuery)
	assertStatus(t, response, http.StatusOK)
	market := decodeData[storage.EducationProgramMarket](t, response)
	if len(market.Programs) != 1 || market.Programs[0].ProgramID != program.ProgramID || market.Programs[0].ExercisePrompt != program.ExercisePrompt || strings.Contains(response.Body.String(), "event_education") {
		t.Fatalf("bounded program discovery: %+v", market)
	}
	programQuery.AfterSequence = market.NextCursor
	response = performJSON(t, handler, "/api/v1/education/programs/query", adaAgentToken, programQuery)
	assertStatus(t, response, http.StatusOK)
	if len(decodeData[storage.EducationProgramMarket](t, response).Programs) != 0 {
		t.Fatal("program pagination repeated source")
	}
	enroll := storage.EducationEnrollmentRequest{Binding: binding("edu-http-enroll"), EnrollmentID: "edu_http_enrollment", ProgramID: program.ProgramID, LearnerID: storage.M2AgentAdaID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/education/enrollments/start", boAgentToken, enroll), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/education/enrollments/start", adaAgentToken, enroll)
	assertStatus(t, response, http.StatusOK)
	enrolled := decodeData[educationReceipt](t, response)
	head = enrolled.EventSequence
	start, err := time.Parse(time.RFC3339Nano, enrolled.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	target := start.Add(2 * time.Minute).UTC().Format(time.RFC3339)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: storage.M2RPPlayerPrincipal, InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "edu-http-wait-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: storage.M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: target, Budget: 100, IdempotencyKey: "edu-http-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("training clock: %+v %v", wait, err)
	}
	head = wait.EventSequence
	exercise := storage.EducationExerciseRequest{Binding: binding("edu-http-exercise"), EnrollmentID: enroll.EnrollmentID, Answer: "Check exits, equipment and hazards; report any unsafe condition."}
	response = performJSON(t, handler, "/api/v1/education/exercises/submit", adaAgentToken, exercise)
	assertStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), exercise.Answer) || strings.Contains(response.Body.String(), "event_education") {
		t.Fatal("exercise receipt exposed private answer/source")
	}
	head = decodeData[educationReceipt](t, response).EventSequence
	complete := storage.EducationCompletionRequest{Binding: binding("edu-http-complete"), EnrollmentID: enroll.EnrollmentID, Proficiency: "competent", Reason: "Observed a complete safety procedure."}
	assertAPIError(t, performJSON(t, handler, "/api/v1/education/training/complete", adaAgentToken, complete), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/education/training/complete", boAgentToken, complete)
	assertStatus(t, response, http.StatusOK)
	head = decodeData[educationReceipt](t, response).EventSequence
	issue := storage.EducationCredentialRequest{Binding: binding("edu-http-issue"), CredentialID: "edu_http_credential", EnrollmentID: enroll.EnrollmentID, ExpiresAt: start.Add(24 * time.Hour).UTC().Format(time.RFC3339)}
	response = performJSON(t, handler, "/api/v1/education/credentials/issue", boAgentToken, issue)
	assertStatus(t, response, http.StatusOK)
	head = decodeData[educationReceipt](t, response).EventSequence
	qualification := educationQualificationRead{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, LearnerID: storage.M2AgentAdaID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/education/qualifications/own", boAgentToken, qualification), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/education/qualifications/own", adaAgentToken, qualification)
	assertStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), exercise.Answer) || strings.Contains(response.Body.String(), "event_education") {
		t.Fatal("qualification query exposed raw learner evidence")
	}
	got := decodeData[storage.EducationQualificationView](t, response)
	if len(got.Credentials) != 1 || got.Credentials[0].Status != "active" {
		t.Fatalf("HTTP qualification: %+v", got)
	}
	application.Binding = binding("edu-http-apply")
	response = performJSON(t, handler, "/api/v1/career/applications/submit", adaAgentToken, application)
	assertStatus(t, response, http.StatusOK)
	if decodeData[storage.CareerRecord](t, response).Fact.Application.Status != "submitted" {
		t.Fatal("HTTP application did not enter candidate flow")
	}
}
