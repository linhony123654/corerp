package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// These self-scoped summaries deliberately omit immutable Event IDs, exercise
// answers, private condition/fatigue causes, and other learners' records.
type EducationTrainingStatus struct {
	ProgramID   string `json:"program_id"`
	Code        string `json:"code"`
	IssuerID    string `json:"issuer_id"`
	Status      string `json:"status"`
	Proficiency string `json:"proficiency,omitempty"`
}

type EducationCredentialStatus struct {
	CredentialID string `json:"credential_id"`
	Code         string `json:"code"`
	IssuerID     string `json:"issuer_id"`
	Proficiency  string `json:"proficiency"`
	ExpiresAt    string `json:"expires_at"`
	Status       string `json:"status"`
}

type EducationExperienceStatus struct {
	TaskCode string `json:"task_code"`
	Day      int    `json:"day"`
	Outcome  string `json:"outcome"`
}

type EducationQualificationView struct {
	Training    []EducationTrainingStatus   `json:"training"`
	Credentials []EducationCredentialStatus `json:"credentials"`
	Experience  []EducationExperienceStatus `json:"experience"`
}

type EducationProgramView struct {
	ProgramID      string                       `json:"program_id"`
	Code           string                       `json:"code"`
	IssuerID       string                       `json:"issuer_id"`
	Prerequisites  []core.CredentialRequirement `json:"prerequisites"`
	MinimumMinutes int                          `json:"minimum_minutes"`
	ExercisePrompt string                       `json:"exercise_prompt"`
}

type EducationProgramMarket struct {
	Programs   []EducationProgramView `json:"programs"`
	NextCursor int64                  `json:"next_cursor"`
}

func (s *Store) DiscoverEducationPrograms(ctx context.Context, principalID, instanceID, branchID, learnerID, code, issuerID string, after int64) (EducationProgramMarket, error) {
	result := EducationProgramMarket{Programs: []EducationProgramView{}}
	if !educationText(principalID, 256) || !educationText(instanceID, 256) || !educationText(branchID, 256) || !educationText(learnerID, 256) || !educationText(code, 256) || !educationText(issuerID, 256) || after < 0 || after >= core.MaxJSONSafeInteger {
		return result, core.NewError(core.CodeInvalidArgument, "bounded education program query required")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	b := core.CareerBinding{PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID}
	if err := authorizeCareerCandidate(ctx, tx.conn, b, learnerID); err != nil {
		return result, err
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT event_sequence,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEducationFactRecorded' AND json_extract(payload,'$.kind')='program' AND json_extract(payload,'$.code')=? AND json_extract(payload,'$.issuer_id')=? AND event_sequence>? ORDER BY event_sequence LIMIT 100`, instanceID, branchID, code, issuerID, after)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var sequence int64
		var raw string
		if err := rows.Scan(&sequence, &raw); err != nil {
			rows.Close()
			return result, err
		}
		var fact EducationFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return result, core.NewError(core.CodeProjectionDiverged, "invalid program source")
		}
		if fact.Version != "corerp.education.v1" || fact.Kind != "program" || fact.RecordID != fact.ProgramID || fact.Code != code || fact.IssuerID != issuerID || fact.MinimumMinutes < 1 || fact.ExercisePrompt == "" {
			rows.Close()
			return result, core.NewError(core.CodeProjectionDiverged, "program terms differ from source")
		}
		result.Programs = append(result.Programs, EducationProgramView{ProgramID: fact.ProgramID, Code: fact.Code, IssuerID: fact.IssuerID, Prerequisites: append([]core.CredentialRequirement{}, fact.Prerequisites...), MinimumMinutes: fact.MinimumMinutes, ExercisePrompt: fact.ExercisePrompt})
		result.NextCursor = sequence
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	return result, nil
}

func (s *Store) ReadEducationQualification(ctx context.Context, principalID, instanceID, branchID, learnerID string) (EducationQualificationView, error) {
	result := EducationQualificationView{Training: []EducationTrainingStatus{}, Credentials: []EducationCredentialStatus{}, Experience: []EducationExperienceStatus{}}
	if !educationText(principalID, 256) || !educationText(instanceID, 256) || !educationText(branchID, 256) || !educationText(learnerID, 256) {
		return result, core.NewError(core.CodeInvalidArgument, "scoped learner query required")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	b := core.CareerBinding{PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID}
	if err := authorizeCareerCandidate(ctx, tx.conn, b, learnerID); err != nil {
		return result, err
	}
	var worldTime string
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, instanceID, branchID).Scan(&worldTime); err != nil {
		return result, err
	}
	now, err := time.Parse(time.RFC3339Nano, worldTime)
	if err != nil {
		return result, err
	}
	enrollments, err := educationLearnerFacts(ctx, tx.conn, b, learnerID, "enrollment")
	if err != nil {
		return result, err
	}
	for _, enrollment := range enrollments {
		status := EducationTrainingStatus{ProgramID: enrollment.Fact.ProgramID, Code: enrollment.Fact.Code, IssuerID: enrollment.Fact.IssuerID, Status: "enrolled"}
		program, err := readEducationFact(ctx, tx.conn, b, "program", enrollment.Fact.ProgramID)
		if err != nil {
			return result, err
		}
		if enrollment.Fact.ProgramEventID != program.EventID || enrollment.Fact.LearnerID != learnerID {
			return result, core.NewError(core.CodeProjectionDiverged, "training enrollment lineage differs")
		}
		if _, err := readEducationFact(ctx, tx.conn, b, "exercise", enrollment.Fact.EnrollmentID); err == nil {
			status.Status = "exercise_submitted"
		} else if !core.HasCode(err, core.CodeNotFound) {
			return result, err
		}
		if completion, err := readEducationFact(ctx, tx.conn, b, "completion", enrollment.Fact.EnrollmentID); err == nil {
			if completion.Fact.EnrollmentEventID != enrollment.EventID || completion.Fact.ExerciseEventID == "" {
				return result, core.NewError(core.CodeProjectionDiverged, "training completion lineage differs")
			}
			status.Status = "completed"
			status.Proficiency = completion.Fact.Proficiency
		} else if !core.HasCode(err, core.CodeNotFound) {
			return result, err
		}
		result.Training = append(result.Training, status)
	}
	credentials, err := educationLearnerFacts(ctx, tx.conn, b, learnerID, "credential")
	if err != nil {
		return result, err
	}
	for _, credential := range credentials {
		status := EducationCredentialStatus{CredentialID: credential.Fact.RecordID, Code: credential.Fact.Code, IssuerID: credential.Fact.IssuerID, Proficiency: credential.Fact.Proficiency, ExpiresAt: credential.Fact.ExpiresAt, Status: "active"}
		expiry, err := validateEducationCredentialSource(ctx, tx.conn, b, credential)
		if err != nil {
			return result, err
		}
		if !expiry.After(now) {
			status.Status = "expired"
		}
		if revoked, err := readEducationFact(ctx, tx.conn, b, "revocation", status.CredentialID); err == nil {
			if revoked.Fact.CredentialEventID != credential.EventID {
				return result, core.NewError(core.CodeProjectionDiverged, "credential revocation lineage differs")
			}
			status.Status = "revoked"
		} else if !core.HasCode(err, core.CodeNotFound) {
			return result, err
		}
		result.Credentials = append(result.Credentials, status)
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWorkTaskAttempted' AND json_extract(payload,'$.entity_id')=? ORDER BY event_sequence DESC LIMIT 100`, instanceID, branchID, learnerID)
	if err != nil {
		return result, err
	}
	var workFacts []RPWorkTaskFact
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return result, err
		}
		var fact RPWorkTaskFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return result, core.NewError(core.CodeProjectionDiverged, "invalid work experience source")
		}
		workFacts = append(workFacts, fact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	for _, fact := range workFacts {
		if fact.Version != "corerp.work_task.v1" || fact.EntityID != learnerID || fact.TaskCode != "routine_check" || (fact.Outcome != "completed" && fact.Outcome != "recheck_required") {
			return result, core.NewError(core.CodeProjectionDiverged, "invalid bounded experience")
		}
		var employee, workEvent string
		err := tx.conn.QueryRowContext(ctx, `SELECT c.employee_entity_id,e.event_id FROM employment_contracts c JOIN events origin ON origin.event_id=c.definition_event_id AND origin.instance_id=? AND origin.branch_id=? JOIN events e ON e.event_id=? AND e.instance_id=? AND e.branch_id=? AND e.actor_id=? WHERE c.contract_id=?`, instanceID, branchID, fact.WorkSourceEventID, instanceID, branchID, learnerID, fact.ContractID).Scan(&employee, &workEvent)
		if err != nil {
			return result, classifyMissing(err, "sourced work experience")
		}
		if employee != learnerID || workEvent != fact.WorkSourceEventID {
			return result, core.NewError(core.CodeProjectionDiverged, "work experience employee differs")
		}
		result.Experience = append(result.Experience, EducationExperienceStatus{TaskCode: fact.TaskCode, Day: fact.Day, Outcome: fact.Outcome})
	}
	return result, nil
}

func educationLearnerFacts(ctx context.Context, conn *sql.Conn, b core.CareerBinding, learnerID, kind string) ([]EducationRecord, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEducationFactRecorded' AND json_extract(payload,'$.kind')=? AND json_extract(payload,'$.learner_id')=? ORDER BY event_sequence DESC LIMIT 100`, b.InstanceID, b.BranchID, kind, learnerID)
	if err != nil {
		return nil, err
	}
	result := []EducationRecord{}
	for rows.Next() {
		var record EducationRecord
		var raw string
		if err := rows.Scan(&record.EventID, &record.EventSequence, &record.WorldTime, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &record.Fact); err != nil {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid private education source")
		}
		if record.Fact.Version != "corerp.education.v1" || record.Fact.Kind != kind || record.Fact.LearnerID != learnerID {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "education learner source differs")
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return result, nil
}
