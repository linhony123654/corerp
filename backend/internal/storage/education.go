package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// Education records are private, immutable source Events. A program is also a
// local operator's scoped grant to its named issuer, not an organization-wide
// capability or a learner's self-asserted qualification.
type EducationProgramRequest struct {
	Binding        core.CareerBinding           `json:"binding"`
	ProgramID      string                       `json:"program_id"`
	Code           string                       `json:"code"`
	IssuerID       string                       `json:"issuer_id"`
	Prerequisites  []core.CredentialRequirement `json:"prerequisites,omitempty"`
	MinimumMinutes int                          `json:"minimum_minutes"`
	ExercisePrompt string                       `json:"exercise_prompt"`
}

type EducationEnrollmentRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	EnrollmentID string             `json:"enrollment_id"`
	ProgramID    string             `json:"program_id"`
	LearnerID    string             `json:"learner_id"`
}

type EducationExerciseRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	EnrollmentID string             `json:"enrollment_id"`
	Answer       string             `json:"answer"`
}

type EducationCompletionRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	EnrollmentID string             `json:"enrollment_id"`
	Proficiency  string             `json:"proficiency"`
	Reason       string             `json:"reason"`
}

type EducationCredentialRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	CredentialID string             `json:"credential_id"`
	EnrollmentID string             `json:"enrollment_id"`
	ExpiresAt    string             `json:"expires_at"`
}

type EducationRevocationRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	CredentialID string             `json:"credential_id"`
	Reason       string             `json:"reason"`
}

type EducationFact struct {
	Version           string                       `json:"version"`
	Kind              string                       `json:"kind"`
	RecordID          string                       `json:"record_id"`
	ProgramID         string                       `json:"program_id"`
	Code              string                       `json:"code"`
	IssuerID          string                       `json:"issuer_id"`
	LearnerID         string                       `json:"learner_id,omitempty"`
	EnrollmentID      string                       `json:"enrollment_id,omitempty"`
	ProgramEventID    string                       `json:"program_event_id,omitempty"`
	EnrollmentEventID string                       `json:"enrollment_event_id,omitempty"`
	ExerciseEventID   string                       `json:"exercise_event_id,omitempty"`
	CompletionEventID string                       `json:"completion_event_id,omitempty"`
	CredentialID      string                       `json:"credential_id,omitempty"`
	CredentialEventID string                       `json:"credential_event_id,omitempty"`
	Prerequisites     []core.CredentialRequirement `json:"prerequisites,omitempty"`
	MinimumMinutes    int                          `json:"minimum_minutes,omitempty"`
	ExercisePrompt    string                       `json:"exercise_prompt,omitempty"`
	Answer            string                       `json:"answer,omitempty"`
	Proficiency       string                       `json:"proficiency,omitempty"`
	Reason            string                       `json:"reason,omitempty"`
	ExpiresAt         string                       `json:"expires_at,omitempty"`
}

type EducationRecord = privateFactRecord[EducationFact]

func educationText(value string, max int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= max && !strings.ContainsAny(value, "\x00\r\n")
}

func educationPrepare(ctx context.Context, conn *sql.Conn, b core.CareerBinding) error {
	return requireNoActiveRPSharedRound(ctx, conn, b.InstanceID, b.BranchID)
}

func authorizeEducationOperator(ctx context.Context, conn *sql.Conn, principalID string) error {
	var role string
	if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, principalID).Scan(&role); err != nil && err != sql.ErrNoRows {
		return err
	}
	if role != "operator" {
		return core.NewError(core.CodeUnauthorized, "education program requires local operator")
	}
	return nil
}

func validateEducationIssuer(ctx context.Context, conn *sql.Conn, b core.CareerBinding, issuerID string) error {
	var role string
	if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, issuerID).Scan(&role); err != nil {
		return classifyMissing(err, "active education issuer")
	}
	if role == "operator" {
		return nil
	}
	if role != "agent" {
		return core.NewError(core.CodeInvalidArgument, "issuer must be an active scoped agent or operator")
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles WHERE principal_id=? AND instance_id=? AND branch_id=? AND status='active'`, issuerID, b.InstanceID, b.BranchID).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return core.NewError(core.CodeUnauthorized, "education issuer has no active agent in this branch")
	}
	return nil
}

func readEducationFact(ctx context.Context, conn *sql.Conn, b core.CareerBinding, kind, id string) (EducationRecord, error) {
	var record EducationRecord
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEducationFactRecorded' AND json_extract(payload,'$.kind')=? AND json_extract(payload,'$.record_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, kind, id).Scan(&record.EventID, &record.EventSequence, &record.WorldTime, &raw)
	if err != nil {
		return record, classifyMissing(err, "education "+kind)
	}
	if err := json.Unmarshal([]byte(raw), &record.Fact); err != nil || record.Fact.Version != "corerp.education.v1" || record.Fact.Kind != kind || record.Fact.RecordID != id {
		return EducationRecord{}, core.NewError(core.CodeProjectionDiverged, "invalid education source")
	}
	return record, nil
}

func requireNewEducationFact(ctx context.Context, conn *sql.Conn, b core.CareerBinding, kind, id string) error {
	_, err := readEducationFact(ctx, conn, b, kind, id)
	if core.HasCode(err, core.CodeNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return core.NewError(core.CodeBranchConflict, "education record already exists")
}

func (s *Store) DefineEducationProgramLocal(ctx context.Context, r EducationProgramRequest) (EducationRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return EducationRecord{}, err
	}
	if !educationText(r.ProgramID, 256) || !educationText(r.Code, 256) || !educationText(r.IssuerID, 256) || !educationText(r.ExercisePrompt, 1000) || r.MinimumMinutes < 1 || r.MinimumMinutes > 7*24*60 || len(r.Prerequisites) > 16 {
		return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "bounded education program required")
	}
	seen := map[string]bool{}
	for _, p := range r.Prerequisites {
		if !educationText(p.Code, 256) || !educationText(p.IssuerID, 256) || seen[p.Code+"\x00"+p.IssuerID] {
			return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "invalid program prerequisite")
		}
		seen[p.Code+"\x00"+p.IssuerID] = true
	}
	r.Prerequisites = append([]core.CredentialRequirement(nil), r.Prerequisites...)
	b := r.Binding
	return executePrivateFactCommand(s, ctx, b, "DefineEducationProgramLocal", r, privateFactDomain{"education", "RPEducationFactRecorded", `{"authorization":"local-operator-program-v1"}`},
		func(conn *sql.Conn) error { return authorizeEducationOperator(ctx, conn, b.PrincipalID) },
		func(conn *sql.Conn, _ privateFactContext) (EducationFact, func() error, error) {
			if err := educationPrepare(ctx, conn, b); err != nil {
				return EducationFact{}, nil, err
			}
			if err := requireNewEducationFact(ctx, conn, b, "program", r.ProgramID); err != nil {
				return EducationFact{}, nil, err
			}
			if err := validateEducationIssuer(ctx, conn, b, r.IssuerID); err != nil {
				return EducationFact{}, nil, err
			}
			return EducationFact{Version: "corerp.education.v1", Kind: "program", RecordID: r.ProgramID, ProgramID: r.ProgramID, Code: r.Code, IssuerID: r.IssuerID, Prerequisites: r.Prerequisites, MinimumMinutes: r.MinimumMinutes, ExercisePrompt: r.ExercisePrompt}, nil, nil
		})
}

func (s *Store) EnrollEducation(ctx context.Context, r EducationEnrollmentRequest) (EducationRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return EducationRecord{}, err
	}
	if !educationText(r.EnrollmentID, 256) || !educationText(r.ProgramID, 256) || !educationText(r.LearnerID, 256) {
		return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "invalid enrollment identity")
	}
	b := r.Binding
	return executePrivateFactCommand(s, ctx, b, "EnrollEducation", r, privateFactDomain{"education", "RPEducationFactRecorded", `{"authorization":"current-learner-v1"}`},
		func(conn *sql.Conn) error { return authorizeCareerCandidate(ctx, conn, b, r.LearnerID) },
		func(conn *sql.Conn, c privateFactContext) (EducationFact, func() error, error) {
			if err := educationPrepare(ctx, conn, b); err != nil {
				return EducationFact{}, nil, err
			}
			if err := requireNewEducationFact(ctx, conn, b, "enrollment", r.EnrollmentID); err != nil {
				return EducationFact{}, nil, err
			}
			program, err := readEducationFact(ctx, conn, b, "program", r.ProgramID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			if err := requireCareerCredentials(ctx, conn, b, r.LearnerID, program.Fact.Prerequisites, c.WorldTime); err != nil {
				return EducationFact{}, nil, err
			}
			return EducationFact{Version: "corerp.education.v1", Kind: "enrollment", RecordID: r.EnrollmentID, ProgramID: r.ProgramID, ProgramEventID: program.EventID, EnrollmentID: r.EnrollmentID, Code: program.Fact.Code, IssuerID: program.Fact.IssuerID, LearnerID: r.LearnerID}, nil, nil
		})
}

func (s *Store) SubmitEducationExercise(ctx context.Context, r EducationExerciseRequest) (EducationRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return EducationRecord{}, err
	}
	if !educationText(r.EnrollmentID, 256) || !educationText(r.Answer, 2000) {
		return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "bounded exercise answer required")
	}
	b := r.Binding
	return executePrivateFactCommand(s, ctx, b, "SubmitEducationExercise", r, privateFactDomain{"education", "RPEducationFactRecorded", `{"authorization":"current-learner-v1"}`},
		func(conn *sql.Conn) error {
			enrollment, err := readEducationFact(ctx, conn, b, "enrollment", r.EnrollmentID)
			if err != nil {
				return err
			}
			if err := authorizeCareerCandidate(ctx, conn, b, enrollment.Fact.LearnerID); core.HasCode(err, core.CodeUnauthorized) {
				return core.NewError(core.CodeNotFound, "education enrollment not found")
			} else {
				return err
			}
		},
		func(conn *sql.Conn, c privateFactContext) (EducationFact, func() error, error) {
			if err := educationPrepare(ctx, conn, b); err != nil {
				return EducationFact{}, nil, err
			}
			if err := requireNewEducationFact(ctx, conn, b, "exercise", r.EnrollmentID); err != nil {
				return EducationFact{}, nil, err
			}
			enrollment, err := readEducationFact(ctx, conn, b, "enrollment", r.EnrollmentID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			program, err := readEducationFact(ctx, conn, b, "program", enrollment.Fact.ProgramID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			if enrollment.Fact.ProgramEventID != program.EventID {
				return EducationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "enrollment program lineage differs")
			}
			started, err := time.Parse(time.RFC3339Nano, enrollment.WorldTime)
			if err != nil {
				return EducationFact{}, nil, core.NewError(core.CodeProjectionDiverged, "invalid enrollment clock")
			}
			now, err := time.Parse(time.RFC3339Nano, c.WorldTime)
			if err != nil {
				return EducationFact{}, nil, err
			}
			if now.Sub(started) < time.Duration(program.Fact.MinimumMinutes)*time.Minute {
				return EducationFact{}, nil, core.NewError(core.CodeBranchConflict, "minimum training interval not elapsed")
			}
			return EducationFact{Version: "corerp.education.v1", Kind: "exercise", RecordID: r.EnrollmentID, EnrollmentID: r.EnrollmentID, EnrollmentEventID: enrollment.EventID, ProgramID: program.Fact.ProgramID, Code: program.Fact.Code, IssuerID: program.Fact.IssuerID, LearnerID: enrollment.Fact.LearnerID, Answer: r.Answer}, nil, nil
		})
}

func authorizeEducationIssuer(ctx context.Context, conn *sql.Conn, b core.CareerBinding, enrollmentID string) (EducationRecord, EducationRecord, error) {
	enrollment, err := readEducationFact(ctx, conn, b, "enrollment", enrollmentID)
	if err != nil {
		return enrollment, EducationRecord{}, err
	}
	program, err := readEducationFact(ctx, conn, b, "program", enrollment.Fact.ProgramID)
	if err != nil {
		return enrollment, program, err
	}
	if enrollment.Fact.ProgramEventID != program.EventID || enrollment.Fact.IssuerID != program.Fact.IssuerID {
		return enrollment, program, core.NewError(core.CodeProjectionDiverged, "education issuer lineage differs")
	}
	if b.PrincipalID != program.Fact.IssuerID {
		return enrollment, program, core.NewError(core.CodeNotFound, "education enrollment not found")
	}
	if err := validateEducationIssuer(ctx, conn, b, b.PrincipalID); err != nil {
		return enrollment, program, err
	}
	return enrollment, program, nil
}

func (s *Store) CompleteEducationTraining(ctx context.Context, r EducationCompletionRequest) (EducationRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return EducationRecord{}, err
	}
	if !educationText(r.EnrollmentID, 256) || !educationText(r.Reason, 1000) || (r.Proficiency != "competent" && r.Proficiency != "proficient") {
		return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "bounded completion assessment required")
	}
	b := r.Binding
	return executePrivateFactCommand(s, ctx, b, "CompleteEducationTraining", r, privateFactDomain{"education", "RPEducationFactRecorded", `{"authorization":"program-issuer-v1"}`},
		func(conn *sql.Conn) error {
			_, _, err := authorizeEducationIssuer(ctx, conn, b, r.EnrollmentID)
			return err
		},
		func(conn *sql.Conn, c privateFactContext) (EducationFact, func() error, error) {
			if err := educationPrepare(ctx, conn, b); err != nil {
				return EducationFact{}, nil, err
			}
			if err := requireNewEducationFact(ctx, conn, b, "completion", r.EnrollmentID); err != nil {
				return EducationFact{}, nil, err
			}
			enrollment, program, err := authorizeEducationIssuer(ctx, conn, b, r.EnrollmentID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			exercise, err := readEducationFact(ctx, conn, b, "exercise", r.EnrollmentID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			if exercise.Fact.EnrollmentEventID != enrollment.EventID || exercise.Fact.LearnerID != enrollment.Fact.LearnerID {
				return EducationFact{}, nil, core.NewError(core.CodeInvalidArgument, "exercise evidence does not match enrollment")
			}
			if err := requireCareerCredentials(ctx, conn, b, enrollment.Fact.LearnerID, program.Fact.Prerequisites, c.WorldTime); err != nil {
				return EducationFact{}, nil, err
			}
			return EducationFact{Version: "corerp.education.v1", Kind: "completion", RecordID: r.EnrollmentID, EnrollmentID: r.EnrollmentID, EnrollmentEventID: enrollment.EventID, ExerciseEventID: exercise.EventID, ProgramID: program.Fact.ProgramID, Code: program.Fact.Code, IssuerID: program.Fact.IssuerID, LearnerID: enrollment.Fact.LearnerID, Proficiency: r.Proficiency, Reason: r.Reason}, nil, nil
		})
}

func (s *Store) IssueEducationCredential(ctx context.Context, r EducationCredentialRequest) (EducationRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return EducationRecord{}, err
	}
	if !educationText(r.CredentialID, 256) || !educationText(r.EnrollmentID, 256) {
		return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "invalid credential source")
	}
	expires, err := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	if err != nil {
		return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "credential expiry required")
	}
	r.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	b := r.Binding
	return executePrivateFactCommand(s, ctx, b, "IssueEducationCredential", r, privateFactDomain{"education", "RPEducationFactRecorded", `{"authorization":"program-issuer-v1"}`},
		func(conn *sql.Conn) error {
			_, _, err := authorizeEducationIssuer(ctx, conn, b, r.EnrollmentID)
			return err
		},
		func(conn *sql.Conn, c privateFactContext) (EducationFact, func() error, error) {
			if err := educationPrepare(ctx, conn, b); err != nil {
				return EducationFact{}, nil, err
			}
			if err := requireNewEducationFact(ctx, conn, b, "credential", r.CredentialID); err != nil {
				return EducationFact{}, nil, err
			}
			enrollment, program, err := authorizeEducationIssuer(ctx, conn, b, r.EnrollmentID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			completion, err := readEducationFact(ctx, conn, b, "completion", r.EnrollmentID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			if completion.Fact.EnrollmentEventID != enrollment.EventID || completion.Fact.LearnerID != enrollment.Fact.LearnerID || completion.Fact.IssuerID != program.Fact.IssuerID || completion.Fact.ExerciseEventID == "" {
				return EducationFact{}, nil, core.NewError(core.CodeInvalidArgument, "completion evidence does not match issuer or learner")
			}
			if err := requireCareerCredentials(ctx, conn, b, enrollment.Fact.LearnerID, program.Fact.Prerequisites, c.WorldTime); err != nil {
				return EducationFact{}, nil, err
			}
			now, err := time.Parse(time.RFC3339Nano, c.WorldTime)
			if err != nil {
				return EducationFact{}, nil, err
			}
			if !expires.After(now) || expires.After(now.AddDate(10, 0, 0)) {
				return EducationFact{}, nil, core.NewError(core.CodeInvalidArgument, "credential validity must be future and bounded")
			}
			return EducationFact{Version: "corerp.education.v1", Kind: "credential", RecordID: r.CredentialID, EnrollmentID: r.EnrollmentID, EnrollmentEventID: enrollment.EventID, CompletionEventID: completion.EventID, ProgramID: program.Fact.ProgramID, ProgramEventID: program.EventID, Code: program.Fact.Code, IssuerID: program.Fact.IssuerID, LearnerID: enrollment.Fact.LearnerID, Proficiency: completion.Fact.Proficiency, ExpiresAt: r.ExpiresAt}, nil, nil
		})
}

func (s *Store) RevokeEducationCredential(ctx context.Context, r EducationRevocationRequest) (EducationRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return EducationRecord{}, err
	}
	if !educationText(r.CredentialID, 256) || !educationText(r.Reason, 1000) {
		return EducationRecord{}, core.NewError(core.CodeInvalidArgument, "credential and revocation reason required")
	}
	b := r.Binding
	authorize := func(conn *sql.Conn) error {
		credential, err := readEducationFact(ctx, conn, b, "credential", r.CredentialID)
		if err != nil {
			return err
		}
		if credential.Fact.IssuerID != b.PrincipalID {
			return core.NewError(core.CodeNotFound, "education credential not found")
		}
		_, program, err := authorizeEducationIssuer(ctx, conn, b, credential.Fact.EnrollmentID)
		if err != nil {
			return err
		}
		if credential.Fact.ProgramEventID != program.EventID {
			return core.NewError(core.CodeProjectionDiverged, "credential issuer grant differs")
		}
		return nil
	}
	return executePrivateFactCommand(s, ctx, b, "RevokeEducationCredential", r, privateFactDomain{"education", "RPEducationFactRecorded", `{"authorization":"program-issuer-v1"}`}, authorize,
		func(conn *sql.Conn, _ privateFactContext) (EducationFact, func() error, error) {
			if err := educationPrepare(ctx, conn, b); err != nil {
				return EducationFact{}, nil, err
			}
			if err := requireNewEducationFact(ctx, conn, b, "revocation", r.CredentialID); err != nil {
				return EducationFact{}, nil, err
			}
			credential, err := readEducationFact(ctx, conn, b, "credential", r.CredentialID)
			if err != nil {
				return EducationFact{}, nil, err
			}
			return EducationFact{Version: "corerp.education.v1", Kind: "revocation", RecordID: r.CredentialID, CredentialID: r.CredentialID, CredentialEventID: credential.EventID, ProgramID: credential.Fact.ProgramID, Code: credential.Fact.Code, IssuerID: credential.Fact.IssuerID, LearnerID: credential.Fact.LearnerID, Reason: r.Reason}, nil, nil
		})
}

func validateEducationCredentialSource(ctx context.Context, conn *sql.Conn, b core.CareerBinding, credential EducationRecord) (time.Time, error) {
	f := credential.Fact
	if f.Version != "corerp.education.v1" || f.Kind != "credential" || !educationText(f.RecordID, 256) || !educationText(f.EnrollmentID, 256) || !educationText(f.ProgramID, 256) {
		return time.Time{}, core.NewError(core.CodeProjectionDiverged, "invalid credential source")
	}
	expires, err := time.Parse(time.RFC3339Nano, f.ExpiresAt)
	if err != nil {
		return time.Time{}, core.NewError(core.CodeProjectionDiverged, "invalid sourced credential expiry")
	}
	issued, err := time.Parse(time.RFC3339Nano, credential.WorldTime)
	if err != nil || !expires.After(issued) {
		return time.Time{}, core.NewError(core.CodeProjectionDiverged, "credential expiry precedes issuance")
	}
	program, err := readEducationFact(ctx, conn, b, "program", f.ProgramID)
	if err != nil {
		return time.Time{}, err
	}
	enrollment, err := readEducationFact(ctx, conn, b, "enrollment", f.EnrollmentID)
	if err != nil {
		return time.Time{}, err
	}
	exercise, err := readEducationFact(ctx, conn, b, "exercise", f.EnrollmentID)
	if err != nil {
		return time.Time{}, err
	}
	completion, err := readEducationFact(ctx, conn, b, "completion", f.EnrollmentID)
	if err != nil {
		return time.Time{}, err
	}
	if f.ProgramEventID != program.EventID || f.EnrollmentEventID != enrollment.EventID || f.CompletionEventID != completion.EventID || f.Code != program.Fact.Code || f.IssuerID != program.Fact.IssuerID || f.LearnerID != enrollment.Fact.LearnerID || f.Proficiency != completion.Fact.Proficiency ||
		enrollment.Fact.ProgramEventID != program.EventID || enrollment.Fact.ProgramID != program.Fact.ProgramID || enrollment.Fact.Code != program.Fact.Code || enrollment.Fact.IssuerID != program.Fact.IssuerID ||
		exercise.Fact.EnrollmentEventID != enrollment.EventID || exercise.Fact.LearnerID != enrollment.Fact.LearnerID || exercise.Fact.Answer == "" ||
		completion.Fact.EnrollmentEventID != enrollment.EventID || completion.Fact.ExerciseEventID != exercise.EventID || completion.Fact.LearnerID != enrollment.Fact.LearnerID || completion.Fact.IssuerID != program.Fact.IssuerID || (completion.Fact.Proficiency != "competent" && completion.Fact.Proficiency != "proficient") {
		return time.Time{}, core.NewError(core.CodeProjectionDiverged, "credential source chain differs")
	}
	return expires, nil
}

// A credential is valid only if its source chain still resolves in this exact
// branch and its expiry has not passed. No claim in a resume/interview counts.
func requireCareerCredentials(ctx context.Context, conn *sql.Conn, b core.CareerBinding, learnerID string, requirements []core.CredentialRequirement, at string) error {
	if len(requirements) == 0 {
		return nil
	}
	now, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return err
	}
	for _, requirement := range requirements {
		rows, err := conn.QueryContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEducationFactRecorded' AND json_extract(payload,'$.kind')='credential' AND json_extract(payload,'$.code')=? AND json_extract(payload,'$.issuer_id')=? AND json_extract(payload,'$.learner_id')=? ORDER BY event_sequence DESC`, b.InstanceID, b.BranchID, requirement.Code, requirement.IssuerID, learnerID)
		if err != nil {
			return err
		}
		var candidates []EducationRecord
		for rows.Next() {
			var rec EducationRecord
			var raw string
			if err := rows.Scan(&rec.EventID, &rec.EventSequence, &rec.WorldTime, &raw); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(raw), &rec.Fact); err != nil {
				rows.Close()
				return core.NewError(core.CodeProjectionDiverged, "invalid credential source")
			}
			candidates = append(candidates, rec)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		matched := false
		for _, rec := range candidates {
			expires, err := validateEducationCredentialSource(ctx, conn, b, rec)
			if err != nil {
				return err
			}
			if !expires.After(now) {
				continue
			}
			revocation, err := readEducationFact(ctx, conn, b, "revocation", rec.Fact.RecordID)
			if err == nil {
				if revocation.Fact.CredentialEventID != rec.EventID || revocation.Fact.IssuerID != rec.Fact.IssuerID {
					return core.NewError(core.CodeProjectionDiverged, "credential revocation source differs")
				}
				continue
			}
			if !core.HasCode(err, core.CodeNotFound) {
				return err
			}
			matched = true
			break
		}
		if !matched {
			return core.NewError(core.CodeInvalidArgument, "required sourced credential missing or expired")
		}
	}
	return nil
}
