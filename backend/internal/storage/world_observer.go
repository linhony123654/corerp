package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"corerp.local/backend/internal/core"
)

func authorizeWorldObserver(ctx context.Context, tx *sql.Tx, principal, instance, branch string) (string, error) {
	var role string
	err := tx.QueryRowContext(ctx, `SELECT p.principal_type FROM principals p JOIN capability_grants g ON g.principal_id=p.principal_id
 WHERE p.principal_id=? AND g.instance_id=? AND g.branch_id=? AND g.subject_id IN (?, '*') AND `+studioGrantPredicate+` LIMIT 1`, principal, instance, branch, branch).Scan(&role)
	if err == nil {
		return role, nil
	}
	err = tx.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, principal).Scan(&role)
	if err == nil {
		if role == "creator" || role == "operator" {
			return role, nil
		}
		return "observer", nil
	}
	return "", core.NewError(core.CodeUnauthorized, "active principal required for observer mode")
}

func makeInspectorLink(instance, branch, eventID string) string {
	return fmt.Sprintf("/studio/inspect?instance_id=%s&branch_id=%s&event_id=%s", instance, branch, eventID)
}

func categorizeMacroEvent(eventType string) (category, headline string) {
	switch {
	case strings.HasPrefix(eventType, "RPLaw"):
		return "law", "Law or Legal Policy Enacted"
	case strings.HasPrefix(eventType, "Cohort") || strings.HasPrefix(eventType, "Population"):
		return "population", "Population Demographic Shift"
	case strings.HasPrefix(eventType, "RPHousehold"):
		return "household", "Household Transition"
	case strings.HasPrefix(eventType, "Organization") || strings.HasPrefix(eventType, "RPOrganization"):
		return "organization", "Organization Governance Review"
	case strings.HasPrefix(eventType, "M2Bankruptcy") || strings.HasPrefix(eventType, "M2Arrears"):
		return "economy", "Economic Restructuring or Insolvency"
	case strings.HasPrefix(eventType, "StudioWorld"):
		return "lifecycle", "World Lifecycle Milestone"
	case strings.HasPrefix(eventType, "RPSharedRoundActionsF7"):
		return "communication", "Public or Institutional Notice Published"
	default:
		return "macro", "World Historical Event"
	}
}

func (s *Store) ReadWorldObserver(ctx context.Context, r core.WorldObserverRequest) (core.WorldObserverReport, error) {
	var out core.WorldObserverReport
	if err := r.Validate(); err != nil {
		return out, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, core.WrapError(core.CodeStorageFailure, "begin observer read transaction", err)
	}
	defer tx.Rollback()

	role, err := authorizeWorldObserver(ctx, tx, r.PrincipalID, r.InstanceID, r.BranchID)
	if err != nil {
		return out, err
	}

	var headSeq int64
	var worldTime sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT b.head_sequence, c.current_world_time FROM branches b
 LEFT JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
 WHERE b.instance_id=? AND b.branch_id=?`, r.InstanceID, r.BranchID).Scan(&headSeq, &worldTime); err != nil {
		return out, classifyMissing(err, "observer branch")
	}

	wt := ""
	if worldTime.Valid {
		wt = worldTime.String
	}

	perspective := r.Perspective
	if perspective == "" {
		perspective = core.PerspectiveMacro
	}

	limit := r.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	out = core.WorldObserverReport{
		InstanceID:   r.InstanceID,
		BranchID:     r.BranchID,
		AccessLevel:  role,
		HeadSequence: headSeq,
		WorldTime:    wt,
		Perspective:  perspective,
	}

	switch perspective {
	case core.PerspectiveMacro:
		rows, err := tx.QueryContext(ctx, `SELECT event_id, event_sequence, world_time, event_type, payload
 FROM events WHERE instance_id=? AND branch_id=?
 ORDER BY event_sequence DESC LIMIT ?`, r.InstanceID, r.BranchID, limit)
		if err != nil {
			return out, err
		}
		defer rows.Close()

		out.MacroEvents = []core.ObserverMacroEvent{}
		for rows.Next() {
			var eid, et, wt, raw string
			var seq int64
			if err := rows.Scan(&eid, &seq, &wt, &et, &raw); err != nil {
				return out, err
			}
			cat, headline := categorizeMacroEvent(et)
			desc := et
			if role == "creator" && len(raw) > 0 {
				if len(raw) > 200 {
					desc = raw[:200] + "..."
				} else {
					desc = raw
				}
			}
			out.MacroEvents = append(out.MacroEvents, core.ObserverMacroEvent{
				EventID:       eid,
				Sequence:      seq,
				WorldTime:     wt,
				EventType:     et,
				Category:      cat,
				Headline:      headline,
				Description:   desc,
				InspectorLink: makeInspectorLink(r.InstanceID, r.BranchID, eid),
			})
		}

	case core.PerspectiveEntity:
		var name, cohortID string
		err := tx.QueryRowContext(ctx, `SELECT m.display_name, m.source_cohort_id FROM materialized_entities m
 JOIN cohorts c ON c.cohort_id=m.source_cohort_id
 WHERE c.instance_id=? AND c.branch_id=? AND m.entity_id=?`, r.InstanceID, r.BranchID, r.TargetEntityID).Scan(&name, &cohortID)
		if err == sql.ErrNoRows {
			// Try agent_profiles if not in materialized_entities
			err = tx.QueryRowContext(ctx, `SELECT p.agent_id, 'cohort_resident' FROM agent_profiles p
 WHERE p.instance_id=? AND p.branch_id=? AND p.agent_id=?`, r.InstanceID, r.BranchID, r.TargetEntityID).Scan(&name, &cohortID)
		}
		if err != nil {
			return out, classifyMissing(err, "target entity")
		}

		life := &core.ObserverEntityLife{
			EntityID:        r.TargetEntityID,
			DisplayName:     name,
			CohortID:        cohortID,
			RecentDecisions: []core.ObserverEntityDecision{},
			InspectorLink:   makeInspectorLink(r.InstanceID, r.BranchID, r.TargetEntityID),
		}

		// Employment
		var employerID, posID, defEventID string
		var wage int64
		err = tx.QueryRowContext(ctx, `SELECT c.employer_entity_id, c.position_id, c.gross_wage_minor, c.definition_event_id
 FROM employment_contracts c
 JOIN events e ON e.event_id=c.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=? AND c.employee_entity_id=? AND c.status='active'
 LIMIT 1`, r.InstanceID, r.BranchID, r.TargetEntityID).Scan(&employerID, &posID, &wage, &defEventID)
		if err == nil {
			life.EmployerID = employerID
			life.PositionTitle = posID
			if role != "observer" {
				life.WageMinor = wage
			}
			life.InspectorLink = makeInspectorLink(r.InstanceID, r.BranchID, defEventID)
		}

		// Household
		var hhid, hhname, resPlace string
		err = tx.QueryRowContext(ctx, `SELECT h.household_id, h.display_name, h.residence_place_id
 FROM rp_household_memberships m
 JOIN rp_households h ON h.household_id=m.household_id
 WHERE h.instance_id=? AND h.branch_id=? AND m.member_entity_id=? AND h.status='active' AND m.ended_event_id IS NULL
 LIMIT 1`, r.InstanceID, r.BranchID, r.TargetEntityID).Scan(&hhid, &hhname, &resPlace)
		if err == nil {
			life.HouseholdID = hhid
			life.HouseholdName = hhname
			life.ResidencePlaceID = resPlace
		}

		// Relationships count
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_identity_familiarity
 WHERE instance_id=? AND branch_id=? AND (observer_agent_id=? OR subject_agent_id=?)`,
			r.InstanceID, r.BranchID, r.TargetEntityID, r.TargetEntityID).Scan(&life.RelationshipsCount)

		// Recent decisions
		decRows, err := tx.QueryContext(ctx, `SELECT d.event_id, e.event_sequence, e.world_time, d.action, d.proposal_json
 FROM rp_npc_decisions d
 JOIN events e ON e.event_id=d.event_id
 WHERE e.instance_id=? AND e.branch_id=? AND d.npc_entity_id=?
 ORDER BY e.event_sequence DESC LIMIT 10`, r.InstanceID, r.BranchID, r.TargetEntityID)
		if err == nil {
			for decRows.Next() {
				var deid, dwt, dact, propRaw string
				var dseq int64
				if err := decRows.Scan(&deid, &dseq, &dwt, &dact, &propRaw); err == nil {
					reason := ""
					if role != "observer" {
						var prop struct {
							Reason string `json:"reason"`
						}
						if err := json.Unmarshal([]byte(propRaw), &prop); err == nil {
							reason = prop.Reason
						}
					}
					life.RecentDecisions = append(life.RecentDecisions, core.ObserverEntityDecision{
						EventID:       deid,
						Sequence:      dseq,
						WorldTime:     dwt,
						Action:        dact,
						ReasonCode:    reason,
						InspectorLink: makeInspectorLink(r.InstanceID, r.BranchID, deid),
					})
				}
			}
			decRows.Close()
		}

		out.EntityLife = life

	case core.PerspectiveOrganization:
		var orgName string
		err := tx.QueryRowContext(ctx, `SELECT display_name FROM economic_entities WHERE entity_id=?`, r.TargetOrgID).Scan(&orgName)
		if err != nil {
			orgName = r.TargetOrgID
		}

		org := &core.ObserverOrganization{
			OrganizationID: r.TargetOrgID,
			DisplayName:    orgName,
			RecentReviews:  []core.ObserverOrgReview{},
			InspectorLink:  makeInspectorLink(r.InstanceID, r.BranchID, r.TargetOrgID),
		}

		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts c
 JOIN events e ON e.event_id=c.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=? AND c.employer_entity_id=? AND c.status='active'`,
			r.InstanceID, r.BranchID, r.TargetOrgID).Scan(&org.ActiveEmployees)

		if role == "creator" {
			var bal int64
			err := tx.QueryRowContext(ctx, `SELECT b.balance_minor FROM account_balances b
 JOIN accounts a ON a.account_id=b.account_id
 JOIN economic_entities ee ON ee.account_id=a.account_id
 WHERE ee.entity_id=?`, r.TargetOrgID).Scan(&bal)
			if err == nil {
				org.BalanceMinor = &bal
			}
		}

		// Count active postings
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e
 WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded'
 AND json_extract(e.payload,'$.kind')='posting' AND json_extract(e.payload,'$.organization_id')=?`,
			r.InstanceID, r.BranchID, r.TargetOrgID).Scan(&org.ActivePostings)

		// Recent reviews
		revRows, err := tx.QueryContext(ctx, `SELECT review_id, reviewed_world_time, decision_kind, event_id
 FROM organization_reviews
 WHERE instance_id=? AND branch_id=? AND organization_id=?
 ORDER BY reviewed_world_time DESC LIMIT 10`, r.InstanceID, r.BranchID, r.TargetOrgID)
		if err == nil {
			for revRows.Next() {
				var rid, rwt, rdk, reid string
				if err := revRows.Scan(&rid, &rwt, &rdk, &reid); err == nil {
					org.RecentReviews = append(org.RecentReviews, core.ObserverOrgReview{
						ReviewID:      rid,
						WorldTime:     rwt,
						DecisionKind:  rdk,
						EventID:       reid,
						InspectorLink: makeInspectorLink(r.InstanceID, r.BranchID, reid),
					})
				}
			}
			revRows.Close()
		}

		out.Organization = org

	case core.PerspectiveRelationship:
		rows, err := tx.QueryContext(ctx, `SELECT observer_agent_id, subject_agent_id, learned_world_time, origin_kind, source_event_id
 FROM rp_identity_familiarity
 WHERE instance_id=? AND branch_id=?
 ORDER BY learned_world_time DESC LIMIT ?`, r.InstanceID, r.BranchID, limit)
		if err != nil {
			return out, err
		}
		defer rows.Close()

		out.RelationshipChanges = []core.ObserverRelationshipChange{}
		for rows.Next() {
			var oaid, said, lwt, okind, seid string
			if err := rows.Scan(&oaid, &said, &lwt, &okind, &seid); err == nil {
				out.RelationshipChanges = append(out.RelationshipChanges, core.ObserverRelationshipChange{
					ObserverEntityID: oaid,
					SubjectEntityID:  said,
					WorldTime:        lwt,
					OriginKind:       okind,
					SourceEventID:    seid,
					InspectorLink:    makeInspectorLink(r.InstanceID, r.BranchID, seid),
				})
			}
		}

	case core.PerspectiveDigest:
		digest := &core.ObserverDigest{
			WindowStart:     r.WindowStart,
			WindowEnd:       r.WindowEnd,
			MacroHighlights: []string{},
			KeyEvents:       []core.ObserverMacroEvent{},
		}

		timeFilter := ""
		args := []any{r.InstanceID, r.BranchID}
		if r.WindowStart != "" {
			timeFilter += " AND world_time >= ?"
			args = append(args, r.WindowStart)
		}
		if r.WindowEnd != "" {
			timeFilter += " AND world_time <= ?"
			args = append(args, r.WindowEnd)
		}

		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=?`+timeFilter, args...).Scan(&digest.TotalEvents)

		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cohort_materializations m
 JOIN cohorts c ON c.cohort_id=m.source_cohort_id
 JOIN events e ON e.event_id=m.materialize_event_id
 WHERE c.instance_id=? AND c.branch_id=?`+timeFilter, args...).Scan(&digest.NewEntitiesCount)

		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_households h
 JOIN events e ON e.event_id=h.source_event_id
 WHERE h.instance_id=? AND h.branch_id=?`+timeFilter, args...).Scan(&digest.HouseholdsFormed)

		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts c
 JOIN events e ON e.event_id=c.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=?`+timeFilter, args...).Scan(&digest.ContractsFormed)

		_ = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_paid_minor), 0) FROM wage_obligations w
 JOIN employment_contracts c ON c.contract_id=w.contract_id
 JOIN events e ON e.event_id=c.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=?`+timeFilter, args...).Scan(&digest.DisbursedWages)

		digest.MacroHighlights = append(digest.MacroHighlights,
			fmt.Sprintf("World Events Recorded: %d", digest.TotalEvents),
			fmt.Sprintf("New Materialized Entities: %d", digest.NewEntitiesCount),
			fmt.Sprintf("Active Households Formed: %d", digest.HouseholdsFormed),
			fmt.Sprintf("Employment Contracts Executed: %d", digest.ContractsFormed),
			fmt.Sprintf("Total Wages Disbursed: %d minor units", digest.DisbursedWages),
		)

		keyRows, err := tx.QueryContext(ctx, `SELECT event_id, event_sequence, world_time, event_type
 FROM events WHERE instance_id=? AND branch_id=?`+timeFilter+` ORDER BY event_sequence DESC LIMIT 5`, args...)
		if err == nil {
			for keyRows.Next() {
				var eid, wt, et string
				var seq int64
				if err := keyRows.Scan(&eid, &seq, &wt, &et); err == nil {
					cat, headline := categorizeMacroEvent(et)
					digest.KeyEvents = append(digest.KeyEvents, core.ObserverMacroEvent{
						EventID:       eid,
						Sequence:      seq,
						WorldTime:     wt,
						EventType:     et,
						Category:      cat,
						Headline:      headline,
						Description:   et,
						InspectorLink: makeInspectorLink(r.InstanceID, r.BranchID, eid),
					})
				}
			}
			keyRows.Close()
		}

		out.Digest = digest
	}

	return out, nil
}
