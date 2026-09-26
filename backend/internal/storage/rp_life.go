package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

// One snapshot; numeric state remains in economics, memory in observation
// evidence. No private context is borrowed from other characters or a Cohort.
func buildRPLifeContext(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) (*core.RPLifeContext, error) {
	life := &core.RPLifeContext{Employment: []core.RPOwnEmployment{}, Relationships: []core.RPRelationship{}, SalientMemories: []core.RPLifeMemory{}, RecentWork: []core.RPLifeMemory{}, EconomicSourceEventIDs: []string{}}
	var origin string
	err := conn.QueryRowContext(ctx, `SELECT m.materialize_event_id,a.definition_event_id,r.balance_minor,-l.balance_minor
 FROM materialized_entities n JOIN cohort_materializations m ON m.materialization_id=n.materialization_id
 JOIN agent_profiles a ON a.agent_id=n.entity_id JOIN account_balances r ON r.account_id=n.receivable_account_id
 JOIN account_balances l ON l.account_id=n.liability_account_id
 WHERE n.entity_id=? AND a.instance_id=? AND a.branch_id=?`, input.NPCEntityID, input.InstanceID, input.BranchID).Scan(&origin, &life.RoutineSourceEventID, &life.ReceivableMinor, &life.LiabilityMinor)
	if err != nil {
		return nil, classifyMissing(err, "NPC own life state")
	}
	life.Disposition = core.DeriveRPDisposition(input.NPCEntityID, origin)
	var backgroundJSON string
	err = conn.QueryRowContext(ctx, `SELECT e.payload FROM agent_profiles a JOIN events e ON e.event_id=a.definition_event_id
	 WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND e.event_type='RPBackgroundMaterialized'`, input.NPCEntityID, input.InstanceID, input.BranchID).Scan(&backgroundJSON)
	if err == nil {
		var background core.RPBackground
		if err := json.Unmarshal([]byte(backgroundJSON), &background); err != nil {
			return nil, err
		}
		if background.EntityID != input.NPCEntityID || background.MaterializationEventID != origin {
			return nil, core.NewError(core.CodeProjectionDiverged, "own background lineage differs")
		}
		life.Background = &background
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	// Projection last_event_sequence can change during rebuild without a new
	// economic fact. Trace causal sources through immutable posted entries.
	rows, err := conn.QueryContext(ctx, `WITH own_latest AS (
 SELECT MAX(e.event_sequence) AS sequence FROM materialized_entities n
 JOIN postings p ON p.account_id IN (n.asset_account_id,n.receivable_account_id,n.liability_account_id)
 JOIN journal_entries j ON j.entry_id=p.entry_id AND j.status='posted'
 JOIN events e ON e.event_id=j.event_id AND e.instance_id=? AND e.branch_id=?
 WHERE n.entity_id=? GROUP BY p.account_id)
 SELECT DISTINCT e.event_id FROM own_latest l JOIN events e ON e.event_sequence=l.sequence
 WHERE e.instance_id=? AND e.branch_id=? ORDER BY e.event_sequence,e.event_id`, input.InstanceID, input.BranchID, input.NPCEntityID, input.InstanceID, input.BranchID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		life.EconomicSourceEventIDs = append(life.EconomicSourceEventIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(o.amount_due_minor-o.amount_paid_minor),0)
 FROM rent_contracts c JOIN rent_obligations o ON o.contract_id=c.contract_id
 WHERE c.tenant_entity_id=? AND o.amount_due_minor>o.amount_paid_minor`, input.NPCEntityID).Scan(&life.RentDueMinor); err != nil {
		return nil, err
	}

	life.Employment, err = readRPOwnEmployment(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, input.WorldTime)
	if err != nil {
		return nil, err
	}
	life.HouseholdPressure, err = readRPHouseholdPressure(ctx, conn, input)
	if err != nil {
		return nil, err
	}
	life.Health, err = readRPOwnSleepHealth(ctx, conn, input)
	if err != nil {
		return nil, err
	}
	symptoms, conditionImpact, err := readRPOwnConditionSymptoms(ctx, conn, input)
	if err != nil {
		return nil, err
	}
	if len(symptoms) > 0 {
		if life.Health == nil {
			life.Health = &core.RPHealthSelf{}
		}
		life.Health.Symptoms = append(life.Health.Symptoms, symptoms...)
		life.Health.ConditionImpact = conditionImpact
	}
	life.Information, err = readRPOwnInformation(ctx, conn, input)
	if err != nil {
		return nil, err
	}

	// Repeat contact supports familiarity, not trust. Other dimensions remain
	// neutral until there are explicit sourced interpersonal actions.
	rows, err = conn.QueryContext(ctx, `SELECT subject_agent_id,COUNT(*),MIN(source_event_id),MAX(source_event_id)
 FROM agent_knowledge WHERE observer_agent_id=? AND json_extract(claim_payload,'$.claim_type') IN ('speaker_said','agent_presence')
 AND claim_key NOT LIKE 'information:%' AND source_event_id NOT IN (SELECT event_id FROM events WHERE event_type='RPInformationDelivered')
 GROUP BY subject_agent_id ORDER BY subject_agent_id`, input.NPCEntityID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		relation := core.RPRelationship{Role: "acquaintance"}
		var first, last string
		if err := rows.Scan(&relation.SubjectEntityID, &relation.Familiarity, &first, &last); err != nil {
			rows.Close()
			return nil, err
		}
		if relation.Familiarity > 100 {
			relation.Familiarity = 100
		}
		relation.SourceEventIDs = []string{first}
		if last != first {
			relation.SourceEventIDs = append(relation.SourceEventIDs, last)
		}
		life.Relationships = append(life.Relationships, relation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = conn.QueryContext(ctx, `SELECT subject_agent_id,learned_world_time,source_event_id,claim_payload FROM agent_knowledge
 WHERE observer_agent_id=? AND json_extract(claim_payload,'$.claim_type') IN ('speaker_said','agent_presence')
 AND claim_key NOT LIKE 'information:%' AND source_event_id NOT IN (SELECT event_id FROM events WHERE event_type='RPInformationDelivered')
 ORDER BY last_event_sequence DESC,claim_key LIMIT 8`, input.NPCEntityID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var memory core.RPLifeMemory
		var raw string
		var claim struct {
			Kind string `json:"claim_type"`
			Text string `json:"text"`
		}
		if err := rows.Scan(&memory.SubjectEntityID, &memory.WorldTime, &memory.SourceEventID, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &claim); err != nil {
			rows.Close()
			return nil, err
		}
		memory.Kind = claim.Kind
		memory.Text = claim.Text
		life.SalientMemories = append(life.SalientMemories, memory)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = conn.QueryContext(ctx, `SELECT event_id,world_time,activity_code,'own_work_arrival' AS kind FROM agent_movements
 WHERE agent_id=? AND activity_code='work'
 UNION ALL SELECT event_id,world_time,json_extract(payload,'$.activity_code'),'own_work_start' FROM events
 WHERE actor_id=? AND instance_id=? AND branch_id=? AND event_type='AgentActivityStarted' AND json_extract(payload,'$.activity_code')='work'
 ORDER BY world_time DESC,event_id LIMIT 5`, input.NPCEntityID, input.NPCEntityID, input.InstanceID, input.BranchID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		memory := core.RPLifeMemory{Kind: "own_work_arrival", SubjectEntityID: input.NPCEntityID}
		if err := rows.Scan(&memory.SourceEventID, &memory.WorldTime, &memory.Text, &memory.Kind); err != nil {
			rows.Close()
			return nil, err
		}
		life.RecentWork = append(life.RecentWork, memory)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := appendCareerLifeEvidence(ctx, conn, input, life); err != nil {
		return nil, err
	}
	if err := applyRPSocialLife(ctx, conn, input.NPCEntityID, life); err != nil {
		return nil, err
	}
	life.CultureAffiliations, err = readOwnCultureAffiliations(ctx, conn, input.NPCEntityID)
	if err != nil {
		return nil, err
	}
	life.LawCases, err = readOwnRPLawCases(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return nil, err
	}
	for _, job := range life.Employment {
		life.Relationships = append(life.Relationships, core.RPRelationship{SubjectEntityID: job.OrganizationID, Role: "employee", SourceEventIDs: []string{job.SourceEventID}})
	}
	core.DeriveRPLifeGoals(input, life)
	return life, nil
}

func readRPOwnEmployment(ctx context.Context, conn *sql.Conn, instanceID, branchID, entityID, worldTime string) ([]core.RPOwnEmployment, error) {
	rows, err := conn.QueryContext(ctx, `SELECT c.contract_id,c.actor_id,c.unit_rate_minor,s.split_event_id
 FROM m2_wage_participation_splits s JOIN m2_cohort_contracts c ON c.contract_id=s.contract_id AND c.kind='wage'
 LEFT JOIN m2_wage_participation_returns r ON r.materialization_id=s.materialization_id
 WHERE s.entity_id=? AND s.effective_from<=? AND c.effective_from<=?
 AND (c.effective_until IS NULL OR c.effective_until>?) AND (r.effective_from IS NULL OR r.effective_from>?)
 AND NOT EXISTS (SELECT 1 FROM events ended WHERE ended.instance_id=? AND ended.branch_id=? AND ended.event_type='CareerAggregateExitActivated' AND json_extract(ended.payload,'$.materialization_id')=s.materialization_id AND ended.world_time<=?)
 UNION ALL SELECT contract_id,employer_entity_id,gross_wage_minor,definition_event_id FROM employment_contracts
 WHERE employee_entity_id=? AND status='active' ORDER BY contract_id`, entityID, worldTime, worldTime, worldTime, worldTime, instanceID, branchID, worldTime, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []core.RPOwnEmployment{}
	for rows.Next() {
		var job core.RPOwnEmployment
		if err := rows.Scan(&job.ContractID, &job.OrganizationID, &job.WageMinor, &job.SourceEventID); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := enrichCareerOwnEmployment(ctx, conn, entityID, worldTime, jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func applyRPSocialLife(ctx context.Context, conn *sql.Conn, observer string, life *core.RPLifeContext) error {
	history, err := readOwnCultureHistory(ctx, conn, observer)
	if err != nil {
		return err
	}
	rows, err := conn.QueryContext(ctx, `SELECT k.subject_agent_id,k.source_event_id,k.learned_world_time,k.claim_payload,e.event_sequence FROM agent_knowledge k JOIN events e ON e.event_id=k.source_event_id
	WHERE k.observer_agent_id=? AND json_extract(k.claim_payload,'$.claim_type')='interpersonal_action'
	ORDER BY e.event_sequence,k.claim_key`, observer)
	if err != nil {
		return err
	}
	defer rows.Close()
	index := map[string]int{}
	for i, r := range life.Relationships {
		index[r.SubjectEntityID] = i
	}
	promises := map[string]core.RPSocialEvidence{}
	socialMemories := []core.RPLifeMemory{}
	for rows.Next() {
		var other, eventID, worldTime, raw string
		var e core.RPSocialEvidence
		var sequence int64
		if err := rows.Scan(&other, &eventID, &worldTime, &raw, &sequence); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return err
		}
		if !((e.ActorEntityID == observer && e.TargetEntityID == other) || (e.ActorEntityID == other && e.TargetEntityID == observer)) {
			return core.NewError(core.CodeProjectionDiverged, "interpersonal knowledge subject mismatch")
		}
		i, found := index[other]
		if !found {
			i = len(life.Relationships)
			index[other] = i
			life.Relationships = append(life.Relationships, core.RPRelationship{SubjectEntityID: other, Role: "acquaintance", SourceEventIDs: []string{}})
		}
		var evaluations []core.RPCultureEvaluation
		if e.Action == "gift" && e.TargetEntityID == observer {
			evaluations, err = evaluateOwnCultureAt(history, sequence, observer, e.Action)
			if err != nil {
				return err
			}
			if len(evaluations) > 0 {
				life.CultureExperiences = append(life.CultureExperiences, core.RPCultureExperience{ActionEventID: eventID, ActorEntityID: other, Evaluations: evaluations, Conflicts: core.RPCultureConflicts(evaluations)})
				if len(life.CultureExperiences) > 8 {
					life.CultureExperiences = life.CultureExperiences[1:]
				}
			}
		}
		core.ApplyRPSocialEvidenceWithCulture(&life.Relationships[i], observer, eventID, e, evaluations)
		if e.Action == "promise_meeting" {
			e.PromiseEventID = eventID
			promises[eventID] = e
		}
		if e.Action == "keep_meeting" {
			delete(promises, e.PromiseEventID)
		}
		socialMemories = append(socialMemories, core.RPLifeMemory{Kind: "interpersonal_action", SubjectEntityID: other, SourceEventID: eventID, WorldTime: worldTime, Text: e.Description})
		if len(socialMemories) > 8 {
			socialMemories = socialMemories[1:]
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	sort.Slice(life.Relationships, func(i, j int) bool {
		return life.Relationships[i].SubjectEntityID < life.Relationships[j].SubjectEntityID
	})
	life.Commitments = []core.RPSocialEvidence{}
	keys := make([]string, 0, len(promises))
	for key := range promises {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		life.Commitments = append(life.Commitments, promises[key])
	}
	// Interpersonal experiences are salient, while the original observation
	// remains authoritative. Retain bounded recent ordinary memories as well.
	for i, j := 0, len(socialMemories)-1; i < j; i, j = i+1, j-1 {
		socialMemories[i], socialMemories[j] = socialMemories[j], socialMemories[i]
	}
	life.SalientMemories = append(socialMemories, life.SalientMemories...)
	if len(life.SalientMemories) > 12 {
		life.SalientMemories = life.SalientMemories[:12]
	}
	return nil
}
