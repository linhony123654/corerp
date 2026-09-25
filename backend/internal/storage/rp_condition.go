package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// Operator-sourced condition truth is distinct from an individual's symptoms,
// a third party's observation and any formal diagnosis.
type RPConditionOnsetRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	ConditionKey string             `json:"condition_key"`
	EntityID     string             `json:"entity_id"`
	Kind         string             `json:"kind"`
	Severity     int                `json:"severity"`
}

type RPConditionChangeRequest struct {
	Binding          core.CareerBinding `json:"binding"`
	ConditionID      string             `json:"condition_id"`
	Status           string             `json:"status"`
	Severity         int                `json:"severity"`
	TreatmentEventID string             `json:"treatment_event_id,omitempty"`
	Reason           string             `json:"reason"`
}

type RPConditionFact struct {
	Version          string `json:"version"`
	ConditionID      string `json:"condition_id"`
	EntityID         string `json:"entity_id"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	Severity         int    `json:"severity"`
	SymptomCode      string `json:"symptom_code"`
	FunctionalImpact string `json:"functional_impact"`
	OnsetEventID     string `json:"onset_event_id"`
	OnsetWorldTime   string `json:"onset_world_time"`
	PreviousEventID  string `json:"previous_event_id,omitempty"`
	TreatmentEventID string `json:"treatment_event_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type RPConditionRecord = privateFactRecord[RPConditionFact]

func rpConditionSymptom(kind string) (string, string) {
	switch kind {
	case "minor_illness":
		return "malaise", "reduced_stamina"
	case "minor_injury":
		return "pain", "limited_mobility"
	default:
		return "", ""
	}
}

func authorizeRPLocalHealthOperator(ctx context.Context, conn *sql.Conn, principal string) error {
	var role string
	if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, principal).Scan(&role); err != nil || role != "operator" {
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		return core.NewError(core.CodeUnauthorized, "condition truth requires the local operator")
	}
	return nil
}

func (s *Store) StartRPConditionLocal(ctx context.Context, r RPConditionOnsetRequest) (RPConditionRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPConditionRecord{}, err
	}
	if !studioID(r.ConditionKey) || !studioID(r.EntityID) || r.Severity < 1 || r.Severity > 3 {
		return RPConditionRecord{}, core.NewError(core.CodeInvalidArgument, "bounded condition key, entity and severity required")
	}
	symptom, impact := rpConditionSymptom(r.Kind)
	if symptom == "" {
		return RPConditionRecord{}, core.NewError(core.CodeInvalidArgument, "unsupported minor condition kind")
	}
	idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.ConditionKey})
	if err != nil {
		return RPConditionRecord{}, err
	}
	conditionID := "condition_" + idHash[7:31]
	return executePrivateFactCommand(s, ctx, r.Binding, "StartRPConditionLocal", r,
		privateFactDomain{"rp_condition_onset", "RPConditionStarted", `{"authorization":"local-operator-health-truth-v1"}`},
		func(conn *sql.Conn) error { return authorizeRPLocalHealthOperator(ctx, conn, r.Binding.PrincipalID) },
		func(conn *sql.Conn, c privateFactContext) (RPConditionFact, func() error, error) {
			var fact RPConditionFact
			if err := requireSoloRPHealthActionWindow(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID); err != nil {
				return fact, nil, err
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPConditionStarted'
				AND json_extract(payload,'$.condition_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, conditionID).Scan(&existing); err != nil {
				return fact, nil, err
			}
			if existing != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "condition key already sourced")
			}
			active, err := readRPActiveConditions(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			for _, condition := range active {
				if condition.Kind == r.Kind {
					return fact, nil, core.NewError(core.CodeBranchConflict, "same minor condition kind already active")
				}
			}
			fact = RPConditionFact{Version: "corerp.condition.v1", ConditionID: conditionID, EntityID: r.EntityID,
				Kind: r.Kind, Status: "active", Severity: r.Severity, SymptomCode: symptom,
				FunctionalImpact: impact, OnsetEventID: c.EventID, OnsetWorldTime: c.WorldTime}
			return fact, nil, nil
		})
}

func (s *Store) ChangeRPConditionLocal(ctx context.Context, r RPConditionChangeRequest) (RPConditionRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPConditionRecord{}, err
	}
	if !studioID(r.ConditionID) || (r.Status != "active" && r.Status != "recovering" && r.Status != "resolved") ||
		(r.Status == "resolved" && r.Severity != 0) || (r.Status != "resolved" && (r.Severity < 1 || r.Severity > 3)) ||
		len(r.Reason) < 1 || len(r.Reason) > 256 || strings.TrimSpace(r.Reason) != r.Reason ||
		(r.TreatmentEventID != "" && !studioID(r.TreatmentEventID)) {
		return RPConditionRecord{}, core.NewError(core.CodeInvalidArgument, "bounded condition transition required")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "ChangeRPConditionLocal", r,
		privateFactDomain{"rp_condition_change", "RPConditionChanged", `{"authorization":"local-operator-health-transition-v1"}`},
		func(conn *sql.Conn) error { return authorizeRPLocalHealthOperator(ctx, conn, r.Binding.PrincipalID) },
		func(conn *sql.Conn, c privateFactContext) (RPConditionFact, func() error, error) {
			var fact RPConditionFact
			if err := requireSoloRPHealthActionWindow(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			previous, previousID, err := readRPConditionLatest(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.ConditionID, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			if previous.Status == "resolved" || (previous.Status == r.Status && previous.Severity == r.Severity) ||
				(r.Status == "recovering" && r.Severity > previous.Severity) {
				return fact, nil, core.NewError(core.CodeBranchConflict, "condition progression or recovery differs from source")
			}
			if r.TreatmentEventID != "" {
				var count int
				if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPHealthTreatmentRecorded'
					AND json_extract(payload,'$.entity_id')=?`, r.TreatmentEventID, r.Binding.InstanceID, r.Binding.BranchID, previous.EntityID).Scan(&count); err != nil {
					return fact, nil, err
				}
				if count != 1 {
					return fact, nil, core.NewError(core.CodeNotFound, "scoped treatment source not found")
				}
			}
			fact = previous
			fact.Status, fact.Severity, fact.PreviousEventID = r.Status, r.Severity, previousID
			fact.TreatmentEventID, fact.Reason = r.TreatmentEventID, r.Reason
			if r.Status == "resolved" {
				fact.SymptomCode, fact.FunctionalImpact = "", ""
			}
			return fact, nil, nil
		})
}

func readRPConditionLatest(ctx context.Context, conn *sql.Conn, instance, branch, conditionID, through string) (RPConditionFact, string, error) {
	var fact RPConditionFact
	var eventID, eventType, raw, eventTime string
	var sequence int64
	err := conn.QueryRowContext(ctx, `SELECT event_id,event_type,event_sequence,payload,world_time FROM events WHERE instance_id=? AND branch_id=?
		AND event_type IN ('RPConditionStarted','RPConditionChanged') AND json_extract(payload,'$.condition_id')=? AND world_time<=?
		ORDER BY event_sequence DESC LIMIT 1`, instance, branch, conditionID, through).Scan(&eventID, &eventType, &sequence, &raw, &eventTime)
	if err != nil {
		return fact, "", classifyMissing(err, "sourced condition")
	}
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return RPConditionFact{}, "", err
	}
	if fact.Version != "corerp.condition.v1" || fact.ConditionID != conditionID || fact.EntityID == "" ||
		(fact.Status != "active" && fact.Status != "recovering" && fact.Status != "resolved") ||
		(fact.Status == "resolved" && fact.Severity != 0) || (fact.Status != "resolved" && (fact.Severity < 1 || fact.Severity > 3)) {
		return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "invalid sourced condition state")
	}
	symptom, impact := rpConditionSymptom(fact.Kind)
	if symptom == "" || (fact.Status == "resolved" && (fact.SymptomCode != "" || fact.FunctionalImpact != "")) ||
		(fact.Status != "resolved" && (fact.SymptomCode != symptom || fact.FunctionalImpact != impact)) {
		return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "condition symptoms differ from sourced kind/status")
	}
	if _, err := time.Parse(time.RFC3339Nano, eventTime); err != nil {
		return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "invalid sourced condition time")
	}
	var onsetCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=?
		AND event_type='RPConditionStarted' AND event_sequence<=? AND world_time=? AND json_extract(payload,'$.condition_id')=?
		AND json_extract(payload,'$.entity_id')=? AND json_extract(payload,'$.kind')=?`, fact.OnsetEventID, instance, branch,
		sequence, fact.OnsetWorldTime, conditionID, fact.EntityID, fact.Kind).Scan(&onsetCount); err != nil {
		return RPConditionFact{}, "", err
	}
	if onsetCount != 1 {
		return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "condition lacks matching onset source")
	}
	if eventType == "RPConditionStarted" {
		if fact.OnsetEventID != eventID || fact.OnsetWorldTime != eventTime || fact.PreviousEventID != "" ||
			fact.Status != "active" || fact.TreatmentEventID != "" {
			return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "invalid condition onset lineage")
		}
	} else {
		var previousRaw string
		if fact.PreviousEventID == "" || fact.PreviousEventID == eventID {
			return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "condition transition has no previous source")
		}
		if err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=?
			AND event_type IN ('RPConditionStarted','RPConditionChanged') AND event_sequence<?`, fact.PreviousEventID,
			instance, branch, sequence).Scan(&previousRaw); err != nil {
			if err == sql.ErrNoRows {
				return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "condition transition has no previous source")
			}
			return RPConditionFact{}, "", err
		}
		var previous RPConditionFact
		if err := json.Unmarshal([]byte(previousRaw), &previous); err != nil {
			return RPConditionFact{}, "", err
		}
		if previous.ConditionID != conditionID || previous.EntityID != fact.EntityID || previous.Kind != fact.Kind ||
			previous.OnsetEventID != fact.OnsetEventID || previous.OnsetWorldTime != fact.OnsetWorldTime || previous.Status == "resolved" {
			return RPConditionFact{}, "", core.NewError(core.CodeProjectionDiverged, "condition transition lineage differs")
		}
	}
	return fact, eventID, nil
}

func readRPActiveConditions(ctx context.Context, conn *sql.Conn, instance, branch, entity, through string) ([]RPConditionFact, error) {
	rows, err := conn.QueryContext(ctx, `SELECT DISTINCT json_extract(payload,'$.condition_id') FROM events WHERE instance_id=? AND branch_id=?
		AND event_type='RPConditionStarted' AND json_extract(payload,'$.entity_id')=? AND world_time<=? ORDER BY 1`,
		instance, branch, entity, through)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var active []RPConditionFact
	for _, id := range ids {
		fact, _, err := readRPConditionLatest(ctx, conn, instance, branch, id, through)
		if err != nil {
			return nil, err
		}
		if fact.EntityID != entity {
			return nil, core.NewError(core.CodeProjectionDiverged, "condition entity lineage differs")
		}
		if fact.Status != "resolved" {
			active = append(active, fact)
		}
	}
	return active, nil
}

func readRPOwnConditionSymptoms(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]string, string, error) {
	active, err := readRPActiveConditions(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, input.WorldTime)
	if err != nil {
		return nil, "", err
	}
	var symptoms []string
	impact := ""
	for _, condition := range active {
		if condition.SymptomCode != "" {
			symptoms = append(symptoms, condition.SymptomCode)
		}
		if condition.Kind == "minor_injury" {
			impact = "avoid_strenuous_activity"
		} else if impact == "" {
			impact = "consider_rest_or_leave"
		}
	}
	return symptoms, impact, nil
}
