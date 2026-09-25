package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type AgentKnowledgeFact struct {
	ClaimKey         string `json:"claim_key,omitempty"`
	SubjectAgentID   string `json:"subject_agent_id,omitempty"`
	PlaceID          string `json:"place_id,omitempty"`
	LearnedWorldTime string `json:"learned_world_time,omitempty"`
	SourceEventID    string `json:"source_event_id,omitempty"`
	ObservationID    string `json:"observation_id,omitempty"`
}

type AgentKnowledgeView struct {
	ObserverAgentID string               `json:"observer_agent_id"`
	Facts           []AgentKnowledgeFact `json:"facts"`
}

type EncounterParticipant struct {
	AgentID         string `json:"agent_id"`
	DisplayName     string `json:"display_name"`
	ActivityCode    string `json:"activity_code,omitempty"`
	EvidenceEventID string `json:"evidence_event_id,omitempty"`
}

type EncounterView struct {
	ObserverAgentID  string                 `json:"observer_agent_id"`
	PlaceID          string                 `json:"place_id,omitempty"`
	WorldTime        string                 `json:"world_time,omitempty"`
	ObserverActivity string                 `json:"observer_activity,omitempty"`
	Participants     []EncounterParticipant `json:"participants,omitempty"`
	EvidenceRefs     []string               `json:"evidence_refs,omitempty"`
}

func (s *Store) ReadAgentKnowledge(ctx context.Context, request core.AgentKnowledgeRead) (AgentKnowledgeView, error) {
	if err := request.Validate(); err != nil {
		return AgentKnowledgeView{}, err
	}
	if err := s.authorizeInternalAgentKnowledgePurpose(ctx, request); err != nil {
		return AgentKnowledgeView{}, err
	}
	allowed, err := s.authorizeAgentFields(ctx, request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID, request.ObserverAgentID, request.Fields)
	if err != nil {
		return AgentKnowledgeView{}, err
	}
	if err := s.requireAgentInScope(ctx, request.InstanceID, request.BranchID, request.ObserverAgentID); err != nil {
		return AgentKnowledgeView{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT claim_key, subject_agent_id, place_id, learned_world_time, source_event_id, observation_id
		FROM agent_knowledge WHERE observer_agent_id = ? ORDER BY claim_key`, request.ObserverAgentID)
	if err != nil {
		return AgentKnowledgeView{}, core.WrapError(core.CodeStorageFailure, "read Agent knowledge", err)
	}
	defer rows.Close()
	view := AgentKnowledgeView{ObserverAgentID: request.ObserverAgentID, Facts: make([]AgentKnowledgeFact, 0)}
	for rows.Next() {
		var claimKey, subjectID, placeID, learnedAt, sourceEventID, observationID string
		if err := rows.Scan(&claimKey, &subjectID, &placeID, &learnedAt, &sourceEventID, &observationID); err != nil {
			return AgentKnowledgeView{}, core.WrapError(core.CodeStorageFailure, "scan Agent knowledge", err)
		}
		fact := AgentKnowledgeFact{}
		if allowed["claim_key"] {
			fact.ClaimKey = claimKey
		}
		if allowed["subject_agent_id"] {
			fact.SubjectAgentID = subjectID
		}
		if allowed["place_id"] {
			fact.PlaceID = placeID
		}
		if allowed["learned_world_time"] {
			fact.LearnedWorldTime = learnedAt
		}
		if allowed["source_event_id"] {
			fact.SourceEventID = sourceEventID
		}
		if allowed["observation_id"] {
			fact.ObservationID = observationID
		}
		view.Facts = append(view.Facts, fact)
	}
	if err := rows.Err(); err != nil {
		return AgentKnowledgeView{}, core.WrapError(core.CodeStorageFailure, "iterate Agent knowledge", err)
	}
	return view, nil
}

// The legacy raw-ID read serves internal career referral and creator review,
// not physical perception or external controller model context. The client
// cannot declare a trustworthy purpose in JSON, so bind it to the authenticated
// principal type, its own Agent profile and the specific capability instead.
func (s *Store) authorizeInternalAgentKnowledgePurpose(ctx context.Context, request core.AgentKnowledgeRead) error {
	if request.CapabilityID != "world.agent.knowledge.read" {
		return core.NewError(core.CodeUnauthorized, "Agent knowledge requires the internal read capability")
	}
	var principalType string
	err := s.db.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, request.PrincipalID).Scan(&principalType)
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewError(core.CodeUnauthorized, "internal Agent knowledge purpose is unavailable to this principal")
	}
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read Agent knowledge principal type", err)
	}
	if principalType == "creator" {
		return nil // field/scope grant is checked below; existing creator review stays intact.
	}
	if principalType != "agent" {
		return core.NewError(core.CodeUnauthorized, "raw Agent knowledge is not an external controller view")
	}
	var own int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles WHERE agent_id=? AND principal_id=? AND instance_id=? AND branch_id=? AND status='active'`, request.ObserverAgentID, request.PrincipalID, request.InstanceID, request.BranchID).Scan(&own)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "check internal Agent knowledge observer", err)
	}
	if own != 1 {
		return core.NewError(core.CodeUnauthorized, "internal Agent may read only its own knowledge")
	}
	return nil
}

func (s *Store) ResolveEncounter(ctx context.Context, request core.EncounterRead) (EncounterView, error) {
	if err := request.Validate(); err != nil {
		return EncounterView{}, err
	}
	allowed, err := s.authorizeAgentFields(ctx, request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID, request.ObserverAgentID, request.Fields)
	if err != nil {
		return EncounterView{}, err
	}
	var placeID, observerActivity, worldTime string
	if err := s.db.QueryRowContext(ctx, `
		SELECT p.place_id, p.activity_code, c.current_world_time
		FROM agent_profiles a JOIN agent_positions p ON p.agent_id = a.agent_id
		JOIN world_clocks c ON c.instance_id = a.instance_id AND c.branch_id = a.branch_id
		WHERE a.agent_id = ? AND a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'`,
		request.ObserverAgentID, request.InstanceID, request.BranchID,
	).Scan(&placeID, &observerActivity, &worldTime); err != nil {
		return EncounterView{}, classifyMissing(err, "encounter observer")
	}
	view := EncounterView{ObserverAgentID: request.ObserverAgentID}
	if allowed["place_id"] {
		view.PlaceID = placeID
	}
	if allowed["world_time"] {
		view.WorldTime = worldTime
	}
	if allowed["activity"] {
		view.ObserverActivity = observerActivity
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.agent_id, e.display_name, p.activity_code, m.event_id
		FROM agent_profiles a
		JOIN agent_positions p ON p.agent_id = a.agent_id
		JOIN materialized_entities e ON e.entity_id = a.agent_id
		JOIN agent_movements m ON m.agent_id = a.agent_id
		JOIN events ev ON ev.event_id = m.event_id AND ev.event_sequence = p.last_event_sequence
		WHERE a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'
		  AND p.place_id = ? AND a.agent_id <> ?
		ORDER BY a.agent_id`, request.InstanceID, request.BranchID, placeID, request.ObserverAgentID)
	if err != nil {
		return EncounterView{}, core.WrapError(core.CodeStorageFailure, "resolve co-located encounter", err)
	}
	type encountered struct {
		participant EncounterParticipant
		eventID     string
	}
	physical := make([]encountered, 0)
	for rows.Next() {
		var participant EncounterParticipant
		var evidenceEventID string
		if err := rows.Scan(&participant.AgentID, &participant.DisplayName, &participant.ActivityCode, &evidenceEventID); err != nil {
			rows.Close()
			return EncounterView{}, core.WrapError(core.CodeStorageFailure, "scan encounter participant", err)
		}
		physical = append(physical, encountered{participant, evidenceEventID})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return EncounterView{}, core.WrapError(core.CodeStorageFailure, "iterate encounter participants", err)
	}
	if err := rows.Close(); err != nil {
		return EncounterView{}, core.WrapError(core.CodeStorageFailure, "close encounter participants", err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return EncounterView{}, err
	}
	defer conn.Close()
	participants := make([]EncounterParticipant, 0, len(physical))
	evidence := make([]string, 0, len(physical))
	for _, seen := range physical {
		allowedSight, err := rpCanPerceive(ctx, conn, request.InstanceID, request.BranchID, request.ObserverAgentID, seen.participant.AgentID, "visual", "")
		if err != nil {
			return EncounterView{}, err
		}
		if !allowedSight {
			continue
		}
		participant := seen.participant
		known, err := rpIdentityKnown(ctx, conn, request.InstanceID, request.BranchID, request.ObserverAgentID, participant.AgentID)
		if err != nil {
			return EncounterView{}, err
		}
		if !known {
			participant.DisplayName = "陌生人"
			participant.AgentID, err = rpAnonymousEntityID(ctx, conn, request.InstanceID, request.BranchID, request.ObserverAgentID, participant.AgentID)
			if err != nil {
				return EncounterView{}, err
			}
			seen.eventID, err = rpAnonymousEvidenceID(ctx, conn, request.InstanceID, request.BranchID, request.ObserverAgentID, seen.eventID)
			if err != nil {
				return EncounterView{}, err
			}
		}
		if !allowed["activity"] {
			participant.ActivityCode = ""
		}
		if allowed["evidence"] {
			participant.EvidenceEventID = seen.eventID
			evidence = append(evidence, seen.eventID)
		}
		participants = append(participants, participant)
	}
	if allowed["participants"] {
		view.Participants = participants
	}
	if allowed["evidence"] {
		sort.Strings(evidence)
		view.EvidenceRefs = evidence
	}
	return view, nil
}

func (s *Store) authorizeAgentFields(ctx context.Context, principalID, capabilityID, instanceID, branchID, subjectID string, requested []string) (map[string]bool, error) {
	var fieldScope string
	err := s.db.QueryRowContext(ctx, `
		SELECT g.field_scope FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.status = 'active' AND g.capability_id = ?
		  AND g.instance_id = ? AND g.branch_id = ? AND g.subject_id IN (?, '*') AND g.status = 'active'
		ORDER BY CASE WHEN g.subject_id = ? THEN 0 ELSE 1 END, g.grant_id LIMIT 1`,
		principalID, capabilityID, instanceID, branchID, subjectID, subjectID,
	).Scan(&fieldScope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewError(core.CodeUnauthorized, "principal lacks Agent read scope for this instance, branch, and observer")
	}
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read Agent field grant", err)
	}
	var granted []string
	if err := json.Unmarshal([]byte(fieldScope), &granted); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "decode Agent field scope", err)
	}
	allowed := make(map[string]bool, len(requested))
	grantedSet := make(map[string]bool, len(granted))
	for _, field := range granted {
		grantedSet[field] = true
	}
	for _, field := range requested {
		if !grantedSet[field] {
			return nil, core.NewError(core.CodeUnauthorized, "requested Agent field is outside the grant: "+field)
		}
		allowed[field] = true
	}
	return allowed, nil
}

func (s *Store) requireAgentInScope(ctx context.Context, instanceID, branchID, agentID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles WHERE agent_id = ? AND instance_id = ? AND branch_id = ? AND status = 'active'`, agentID, instanceID, branchID).Scan(&count); err != nil {
		return core.WrapError(core.CodeStorageFailure, "verify Agent scope", err)
	}
	if count != 1 {
		return core.NewError(core.CodeNotFound, "active Agent not found")
	}
	return nil
}
