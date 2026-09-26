package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

func authorizeWorldQA(ctx context.Context, tx *sql.Tx, principal, instance, branch string) (string, error) {
	var role string
	err := tx.QueryRowContext(ctx, `SELECT p.principal_type FROM principals p JOIN capability_grants g ON g.principal_id=p.principal_id
 WHERE p.principal_id=? AND g.instance_id=? AND g.branch_id=? AND g.subject_id IN (?, '*') AND `+studioGrantPredicate+` LIMIT 1`, principal, instance, branch, branch).Scan(&role)
	if err == nil {
		return role, nil
	}
	err = tx.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active' AND principal_type IN ('creator','operator')`, principal).Scan(&role)
	if err == nil {
		return role, nil
	}
	return "", core.NewError(core.CodeUnauthorized, "active creator or operator required for world QA diagnostics")
}

func (s *Store) ReadWorldQA(ctx context.Context, r core.WorldQARequest) (core.WorldQAReport, error) {
	var report core.WorldQAReport
	if err := r.Validate(); err != nil {
		return report, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return report, core.WrapError(core.CodeStorageFailure, "begin world qa read transaction", err)
	}
	defer tx.Rollback()

	role, err := authorizeWorldQA(ctx, tx, r.PrincipalID, r.InstanceID, r.BranchID)
	if err != nil {
		return report, err
	}

	var headSeq int64
	var worldTime sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT b.head_sequence, c.current_world_time FROM branches b
 LEFT JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
 WHERE b.instance_id=? AND b.branch_id=?`, r.InstanceID, r.BranchID).Scan(&headSeq, &worldTime); err != nil {
		return report, classifyMissing(err, "world qa branch")
	}

	wt := ""
	if worldTime.Valid {
		wt = worldTime.String
	}

	thresholds := core.DefaultWorldQAThresholds()
	if r.Thresholds != nil {
		thresholds = *r.Thresholds
	}

	report = core.WorldQAReport{
		InstanceID:   r.InstanceID,
		BranchID:     r.BranchID,
		AccessLevel:  role,
		HeadSequence: headSeq,
		WorldTime:    wt,
		Status:       core.StatusHealthy,
		Anomalies:    []core.WorldQAAnomaly{},
		Population: core.PopulationQADimension{
			Cohorts: make(map[string]int64),
		},
		Finances: core.FinancesQADimension{
			BalancesByCurrency: make(map[string]int64),
		},
		Events: core.EventQADimension{
			EventsByType:  make(map[string]int64),
			TopEventTypes: []core.EventCount{},
		},
		Decisions: core.DecisionQADimension{
			DecisionsByAction: make(map[string]int64),
			DecisionsByActor:  make(map[string]int64),
		},
		Knowledge: core.KnowledgeQADimension{
			ChannelsBreakdown: make(map[string]int64),
		},
		Organization: core.OrganizationQADimension{
			ReviewsByDecision: make(map[string]int64),
		},
		Spatial: core.SpatialQADimension{
			UnreachablePlaces: []string{},
		},
	}

	// 1. Population & Cohort Materialization
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM materialized_entities m
 JOIN cohorts c ON c.cohort_id=m.source_cohort_id
 WHERE c.instance_id=? AND c.branch_id=?`, r.InstanceID, r.BranchID).Scan(&report.Population.TotalMaterialized)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles WHERE instance_id=? AND branch_id=? AND status='active'`,
		r.InstanceID, r.BranchID).Scan(&report.Population.ActiveAgents)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM population_movements p
 JOIN events e ON e.event_id=p.event_id
 WHERE e.instance_id=? AND e.branch_id=?`, r.InstanceID, r.BranchID).Scan(&report.Population.TotalMovements)

	cohortRows, err := tx.QueryContext(ctx, `SELECT m.source_cohort_id, COUNT(*) FROM materialized_entities m
 JOIN cohorts c ON c.cohort_id=m.source_cohort_id
 WHERE c.instance_id=? AND c.branch_id=?
 GROUP BY m.source_cohort_id`, r.InstanceID, r.BranchID)
	if err == nil {
		for cohortRows.Next() {
			var cid string
			var count int64
			if err := cohortRows.Scan(&cid, &count); err == nil {
				report.Population.Cohorts[cid] = count
			}
		}
		cohortRows.Close()
	}

	// 2. Employment & Vacancies
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts c
 JOIN events e ON e.event_id=c.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=? AND c.status='active'`, r.InstanceID, r.BranchID).Scan(&report.Employment.ActiveContracts)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts c
 JOIN events e ON e.event_id=c.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=? AND c.status='ended'`, r.InstanceID, r.BranchID).Scan(&report.Employment.EndedContracts)

	// Postings and positions from events
	postingRows, err := tx.QueryContext(ctx, `SELECT e.payload FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded'
 AND json_extract(e.payload,'$.kind') IN ('posting','organization_review') AND json_extract(e.payload,'$.posting.position_id') IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM events n WHERE n.instance_id=e.instance_id AND n.branch_id=e.branch_id AND n.event_type='RPCareerFactRecorded'
 AND json_extract(n.payload,'$.kind') IN ('posting','organization_review') AND json_extract(n.payload,'$.posting.position_id')=json_extract(e.payload,'$.posting.position_id') AND n.event_sequence>e.event_sequence)`, r.InstanceID, r.BranchID)
	if err == nil {
		type postingPayload struct {
			Posting *struct {
				PositionID string `json:"position_id"`
				Capacity   int    `json:"capacity"`
				Status     string `json:"status"`
			} `json:"posting"`
		}
		for postingRows.Next() {
			var raw string
			if err := postingRows.Scan(&raw); err == nil {
				var p postingPayload
				if err := json.Unmarshal([]byte(raw), &p); err == nil && p.Posting != nil {
					report.Employment.TotalPositions++
					if p.Posting.Status != "frozen" {
						report.Employment.UnfilledPostings++
					}
				}
			}
		}
		postingRows.Close()
	}

	if report.Population.TotalMaterialized > report.Employment.ActiveContracts {
		report.Employment.UnemployedCount = report.Population.TotalMaterialized - report.Employment.ActiveContracts
	}
	if report.Population.TotalMaterialized > 0 {
		report.Employment.UnemploymentRatio = float64(report.Employment.UnemployedCount) / float64(report.Population.TotalMaterialized)
	}

	// 3. Finances & Arrears
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts a
 JOIN events e ON e.event_id=a.opened_by_event_id
 WHERE e.instance_id=? AND e.branch_id=?`, r.InstanceID, r.BranchID).Scan(&report.Finances.TotalAccounts)

	balRows, err := tx.QueryContext(ctx, `SELECT a.currency_id, COALESCE(SUM(b.balance_minor), 0)
 FROM account_balances b
 JOIN accounts a ON a.account_id=b.account_id
 JOIN events e ON e.event_id=a.opened_by_event_id
 WHERE e.instance_id=? AND e.branch_id=?
 GROUP BY a.currency_id`, r.InstanceID, r.BranchID)
	if err == nil {
		for balRows.Next() {
			var cur string
			var sum int64
			if err := balRows.Scan(&cur, &sum); err == nil {
				report.Finances.BalancesByCurrency[cur] = sum
			}
		}
		balRows.Close()
	}

	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(w.amount_due_minor), 0), COALESCE(SUM(w.amount_paid_minor), 0),
 COALESCE(SUM(CASE WHEN w.status='arrears' THEN w.amount_due_minor - w.amount_paid_minor ELSE 0 END), 0)
 FROM wage_obligations w
 JOIN employment_contracts c ON c.contract_id=w.contract_id
 JOIN events e ON e.event_id=c.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=?`, r.InstanceID, r.BranchID).Scan(
		&report.Finances.WageAccruedMinor, &report.Finances.WagePaidMinor, &report.Finances.WageArrearsMinor)

	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(ro.amount_due_minor), 0), COALESCE(SUM(ro.amount_paid_minor), 0),
 COALESCE(SUM(CASE WHEN ro.status='past_due' THEN ro.amount_due_minor - ro.amount_paid_minor ELSE 0 END), 0)
 FROM rent_obligations ro
 JOIN rent_contracts rc ON rc.contract_id=ro.contract_id
 JOIN events e ON e.event_id=rc.definition_event_id
 WHERE e.instance_id=? AND e.branch_id=?`, r.InstanceID, r.BranchID).Scan(
		&report.Finances.RentDueMinor, &report.Finances.RentPaidMinor, &report.Finances.RentPastDueMinor)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_arrears_cases WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Finances.ArrearsCasesCount)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Finances.BankruptcyCount)

	// 4. Household Rent Pressure
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_households WHERE instance_id=? AND branch_id=? AND status='active'`,
		r.InstanceID, r.BranchID).Scan(&report.Household.ActiveHouseholds)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_household_memberships m
 JOIN rp_households h ON h.household_id=m.household_id
 WHERE h.instance_id=? AND h.branch_id=? AND h.status='active' AND m.ended_event_id IS NULL`,
		r.InstanceID, r.BranchID).Scan(&report.Household.TotalMembers)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_household_memberships m
 JOIN rp_households h ON h.household_id=m.household_id
 WHERE h.instance_id=? AND h.branch_id=? AND h.status='active' AND m.ended_event_id IS NULL AND m.member_role='dependent'`,
		r.InstanceID, r.BranchID).Scan(&report.Household.DependentsCount)

	// Categorize household rent pressure
	hhRows, err := tx.QueryContext(ctx, `SELECT h.household_id, h.rent_account_id, COALESCE(c.rent_minor, 0), COALESCE(b.balance_minor, 0),
 COALESCE((SELECT SUM(ro.amount_due_minor - ro.amount_paid_minor) FROM rent_obligations ro WHERE ro.contract_id=c.contract_id AND ro.amount_due_minor > ro.amount_paid_minor), 0)
 FROM rp_households h
 LEFT JOIN rp_household_rent_agreements a ON a.household_id=h.household_id AND a.status='active'
 LEFT JOIN rent_contracts c ON c.contract_id=a.contract_id
 LEFT JOIN account_balances b ON b.account_id=h.rent_account_id
 WHERE h.instance_id=? AND h.branch_id=? AND h.status='active'`, r.InstanceID, r.BranchID)
	if err == nil {
		for hhRows.Next() {
			var hhid, accid string
			var rentMinor, balanceMinor, pastDueMinor int64
			if err := hhRows.Scan(&hhid, &accid, &rentMinor, &balanceMinor, &pastDueMinor); err == nil {
				if pastDueMinor > 0 {
					report.Household.CriticalHouseholds++
				} else if balanceMinor < rentMinor {
					report.Household.StrainedHouseholds++
				} else {
					report.Household.CoveredHouseholds++
				}
			}
		}
		hhRows.Close()
	}

	// 5. Housing Coverage
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT residence_place_id) FROM rp_households WHERE instance_id=? AND branch_id=? AND status='active'`,
		r.InstanceID, r.BranchID).Scan(&report.Housing.TotalResidences)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT m.member_entity_id) FROM rp_household_memberships m
 JOIN rp_households h ON h.household_id=m.household_id
 WHERE h.instance_id=? AND h.branch_id=? AND h.status='active' AND m.ended_event_id IS NULL`,
		r.InstanceID, r.BranchID).Scan(&report.Housing.HousedPopulation)

	if report.Population.TotalMaterialized > report.Housing.HousedPopulation {
		report.Housing.UnhousedPopulation = report.Population.TotalMaterialized - report.Housing.HousedPopulation
	}
	if report.Population.TotalMaterialized > 0 {
		report.Housing.HousingCoverageRatio = float64(report.Housing.HousedPopulation) / float64(report.Population.TotalMaterialized)
	} else {
		report.Housing.HousingCoverageRatio = 1.0
	}

	// 6. Commute & Transit Topology
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_timed_edges WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Commute.TotalEdges)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_journeys WHERE instance_id=? AND branch_id=? AND status='active'`,
		r.InstanceID, r.BranchID).Scan(&report.Commute.ActiveJourneys)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_journeys WHERE instance_id=? AND branch_id=? AND status='arrived'`,
		r.InstanceID, r.BranchID).Scan(&report.Commute.ArrivedJourneys)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_journeys WHERE instance_id=? AND branch_id=? AND status='cancelled'`,
		r.InstanceID, r.BranchID).Scan(&report.Commute.CancelledJourneys)

	if wt != "" {
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_journeys WHERE instance_id=? AND branch_id=? AND status='active' AND scheduled_arrival_at<?`,
			r.InstanceID, r.BranchID, wt).Scan(&report.Commute.DelayedJourneys)
	}

	// 7. Relationship Network
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Relationship.TotalFamiliarityTies)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT entity_id) FROM (
 SELECT observer_agent_id AS entity_id FROM rp_identity_familiarity WHERE instance_id=? AND branch_id=?
 UNION
 SELECT subject_agent_id AS entity_id FROM rp_identity_familiarity WHERE instance_id=? AND branch_id=?
)`, r.InstanceID, r.BranchID, r.InstanceID, r.BranchID).Scan(&report.Relationship.EntitiesWithTies)

	if report.Population.TotalMaterialized > report.Relationship.EntitiesWithTies {
		report.Relationship.IsolatedEntities = report.Population.TotalMaterialized - report.Relationship.EntitiesWithTies
	}
	if report.Population.TotalMaterialized > 0 {
		report.Relationship.IsolatedRatio = float64(report.Relationship.IsolatedEntities) / float64(report.Population.TotalMaterialized)
	}
	if report.Population.TotalMaterialized > 1 {
		maxPossibleTies := report.Population.TotalMaterialized * (report.Population.TotalMaterialized - 1)
		report.Relationship.GraphDensity = float64(report.Relationship.TotalFamiliarityTies) / float64(maxPossibleTies)
	}

	// 8. Event Frequency & Repetition
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Events.TotalEvents)

	evRows, err := tx.QueryContext(ctx, `SELECT event_type, COUNT(*) FROM events WHERE instance_id=? AND branch_id=? GROUP BY event_type ORDER BY COUNT(*) DESC`,
		r.InstanceID, r.BranchID)
	if err == nil {
		for evRows.Next() {
			var et string
			var count int64
			if err := evRows.Scan(&et, &count); err == nil {
				report.Events.EventsByType[et] = count
				report.Events.TopEventTypes = append(report.Events.TopEventTypes, core.EventCount{EventType: et, Count: count})
			}
		}
		evRows.Close()
	}

	var curDay int
	_ = tx.QueryRowContext(ctx, `SELECT current_day FROM world_clocks WHERE instance_id=? AND branch_id=?`, r.InstanceID, r.BranchID).Scan(&curDay)
	if curDay < 1 {
		curDay = 1
	}
	report.Events.EventsPerDay = float64(report.Events.TotalEvents) / float64(curDay)

	// Check repetitive events
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM (
 SELECT actor_id, event_type, COUNT(*) as cnt FROM events
 WHERE instance_id=? AND branch_id=? GROUP BY actor_id, event_type, substr(world_time, 1, 16) HAVING cnt >= 10
)`, r.InstanceID, r.BranchID).Scan(&report.Events.RepetitionAlerts)

	// 9. Agent Decision Distribution
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_npc_decisions d
 JOIN events e ON e.event_id=d.event_id WHERE e.instance_id=? AND e.branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Decisions.TotalDecisions)

	decActRows, err := tx.QueryContext(ctx, `SELECT d.action, COUNT(*) FROM rp_npc_decisions d
 JOIN events e ON e.event_id=d.event_id WHERE e.instance_id=? AND e.branch_id=? GROUP BY d.action`,
		r.InstanceID, r.BranchID)
	if err == nil {
		for decActRows.Next() {
			var act string
			var count int64
			if err := decActRows.Scan(&act, &count); err == nil {
				report.Decisions.DecisionsByAction[act] = count
			}
		}
		decActRows.Close()
	}

	decActorRows, err := tx.QueryContext(ctx, `SELECT d.npc_entity_id, COUNT(*) FROM rp_npc_decisions d
 JOIN events e ON e.event_id=d.event_id WHERE e.instance_id=? AND e.branch_id=? GROUP BY d.npc_entity_id`,
		r.InstanceID, r.BranchID)
	if err == nil {
		for decActorRows.Next() {
			var aid string
			var count int64
			if err := decActorRows.Scan(&aid, &count); err == nil {
				report.Decisions.DecisionsByActor[aid] = count
			}
		}
		decActorRows.Close()
	}

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=? AND status='active'`,
		r.InstanceID, r.BranchID).Scan(&report.Decisions.ExternalControllers)

	// 10. Knowledge Containment
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM observation_records_f7 o
 JOIN events e ON e.event_id=o.source_event_id WHERE e.instance_id=? AND e.branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Knowledge.TotalObservations)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_knowledge_f7 k
 JOIN events e ON e.event_id=k.source_event_id WHERE e.instance_id=? AND e.branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Knowledge.TotalKnowledge)

	chanRows, err := tx.QueryContext(ctx, `SELECT o.channel, COUNT(*) FROM observation_records_f7 o
 JOIN events e ON e.event_id=o.source_event_id WHERE e.instance_id=? AND e.branch_id=? GROUP BY o.channel`,
		r.InstanceID, r.BranchID)
	if err == nil {
		for chanRows.Next() {
			var ch string
			var count int64
			if err := chanRows.Scan(&ch, &count); err == nil {
				report.Knowledge.ChannelsBreakdown[ch] = count
			}
		}
		chanRows.Close()
	}

	// Potential leak indicators: observations where observer=subject or invalid place/source
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM observation_records_f7 o
 JOIN events e ON e.event_id=o.source_event_id WHERE e.instance_id=? AND e.branch_id=?
 AND o.observer_agent_id = o.subject_agent_id`, r.InstanceID, r.BranchID).Scan(&report.Knowledge.LeakageIndicators)

	// 11. Organization Decisions
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization_agency_policies WHERE instance_id=? AND branch_id=? AND status='active'`,
		r.InstanceID, r.BranchID).Scan(&report.Organization.ActivePolicies)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization_reviews WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Organization.TotalReviews)

	orgRevRows, err := tx.QueryContext(ctx, `SELECT decision_kind, COUNT(*) FROM organization_reviews WHERE instance_id=? AND branch_id=? GROUP BY decision_kind`,
		r.InstanceID, r.BranchID)
	if err == nil {
		for orgRevRows.Next() {
			var dk string
			var count int64
			if err := orgRevRows.Scan(&dk, &count); err == nil {
				report.Organization.ReviewsByDecision[dk] = count
			}
		}
		orgRevRows.Close()
	}

	// 12. Failed / Rejected Actions
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND status='rejected'`,
		r.InstanceID, r.BranchID).Scan(&report.FailedActions.RejectedCommands)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_journeys WHERE instance_id=? AND branch_id=? AND status='cancelled'`,
		r.InstanceID, r.BranchID).Scan(&report.FailedActions.CancelledJourneys)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_default_reviews WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.FailedActions.DefaultReviews)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_insolvency_reviews WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.FailedActions.BankruptcyReviews)

	// 13. Spatial Reachability & Orphans
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_location_nodes WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Spatial.TotalNodes)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE instance_id=? AND branch_id=?`,
		r.InstanceID, r.BranchID).Scan(&report.Spatial.RootPlaces)

	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_location_nodes n
 WHERE n.instance_id=? AND n.branch_id=? AND n.parent_location_id IS NOT NULL
 AND NOT EXISTS (
  SELECT 1 FROM rp_location_nodes p WHERE p.instance_id=n.instance_id AND p.branch_id=n.branch_id AND p.location_id=n.parent_location_id
 )`, r.InstanceID, r.BranchID).Scan(&report.Spatial.OrphanNodes)

	if report.Spatial.RootPlaces > 1 {
		unreachRows, err := tx.QueryContext(ctx, `SELECT p.place_id FROM agent_places p
 WHERE p.instance_id=? AND p.branch_id=?
 AND NOT EXISTS (SELECT 1 FROM rp_place_links l WHERE l.instance_id=p.instance_id AND l.branch_id=p.branch_id AND (l.from_place_id=p.place_id OR l.to_place_id=p.place_id))
 AND NOT EXISTS (SELECT 1 FROM rp_timed_edges t WHERE t.instance_id=p.instance_id AND t.branch_id=p.branch_id AND (t.from_place_id=p.place_id OR t.to_place_id=p.place_id OR t.segment_place_id=p.place_id))`,
			r.InstanceID, r.BranchID)
		if err == nil {
			for unreachRows.Next() {
				var pid string
				if err := unreachRows.Scan(&pid); err == nil {
					report.Spatial.UnreachablePlaces = append(report.Spatial.UnreachablePlaces, pid)
				}
			}
			unreachRows.Close()
		}
	}

	// 14. Economic Conservation
	var zeroSum int64
	var balancedEntries int64
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.amount_minor), 0), COUNT(DISTINCT p.entry_id)
 FROM postings p
 JOIN journal_entries j ON j.entry_id=p.entry_id
 JOIN events e ON e.event_id=j.event_id
 WHERE e.instance_id=? AND e.branch_id=? AND j.status='posted'`, r.InstanceID, r.BranchID).Scan(&zeroSum, &balancedEntries)

	report.Conservation.DoubleEntryBalanceZeroSum = zeroSum
	report.Conservation.BalancedPostingsCount = balancedEntries
	report.Conservation.IssuanceConserved = (zeroSum == 0)

	// Anomaly Evaluation
	if report.Conservation.DoubleEntryBalanceZeroSum != 0 {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "conservation",
			Severity:    core.SeverityCritical,
			Code:        "CONSERVATION_ZERO_SUM_VIOLATION",
			Message:     "Double-entry bookkeeping violates zero-sum conservation",
			MetricValue: float64(report.Conservation.DoubleEntryBalanceZeroSum),
		})
	}

	if report.Spatial.OrphanNodes > 0 {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "spatial",
			Severity:    core.SeverityCritical,
			Code:        "SPATIAL_ORPHAN_NODES",
			Message:     "Disconnected location nodes detected without valid parent",
			MetricValue: float64(report.Spatial.OrphanNodes),
		})
	}

	if len(report.Spatial.UnreachablePlaces) > 0 {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "spatial",
			Severity:    core.SeverityWarning,
			Code:        "SPATIAL_UNREACHABLE_PLACES",
			Message:     "Spatial places detected with zero inbound or outbound transit links",
			MetricValue: float64(len(report.Spatial.UnreachablePlaces)),
		})
	}

	if report.Knowledge.LeakageIndicators > 0 {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "knowledge",
			Severity:    core.SeverityCritical,
			Code:        "KNOWLEDGE_LEAKAGE_DETECTED",
			Message:     "Information containment violation detected in observation records",
			MetricValue: float64(report.Knowledge.LeakageIndicators),
		})
	}

	if report.Commute.DelayedJourneys > 0 {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "commute",
			Severity:    core.SeverityWarning,
			Code:        "COMMUTE_DELAYS_ACTIVE",
			Message:     "Active journeys detected past scheduled arrival time",
			MetricValue: float64(report.Commute.DelayedJourneys),
		})
	}

	if report.Population.TotalMaterialized >= 3 && report.Relationship.IsolatedRatio > thresholds.MaxIsolatedEntityRatio {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "relationship",
			Severity:    core.SeverityWarning,
			Code:        "RELATIONSHIP_ISOLATION_HIGH",
			Message:     "High ratio of isolated individuals with zero social familiarity ties",
			MetricValue: report.Relationship.IsolatedRatio,
			Threshold:   thresholds.MaxIsolatedEntityRatio,
		})
	}

	if report.Population.TotalMaterialized >= 2 && report.Housing.HousingCoverageRatio < thresholds.MinHousingCoverageRatio {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "housing",
			Severity:    core.SeverityWarning,
			Code:        "HOUSING_COVERAGE_LOW",
			Message:     "Low housing coverage ratio; unhoused population exceeds threshold",
			MetricValue: report.Housing.HousingCoverageRatio,
			Threshold:   thresholds.MinHousingCoverageRatio,
		})
	}

	if report.Finances.RentPastDueMinor > 0 && report.Finances.RentDueMinor > 0 {
		ratio := float64(report.Finances.RentPastDueMinor) / float64(report.Finances.RentDueMinor)
		if ratio > thresholds.MaxPastDueRentRatio {
			report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
				Dimension:   "finances",
				Severity:    core.SeverityWarning,
				Code:        "FINANCIAL_RENT_ARREARS_HIGH",
				Message:     "Past-due rent ratio exceeds allowable risk threshold",
				MetricValue: ratio,
				Threshold:   thresholds.MaxPastDueRentRatio,
			})
		}
	}

	if report.Employment.UnfilledPostings > thresholds.MaxStagnantVacancies {
		report.Anomalies = append(report.Anomalies, core.WorldQAAnomaly{
			Dimension:   "employment",
			Severity:    core.SeverityWarning,
			Code:        "EMPLOYMENT_VACANCIES_STAGNANT",
			Message:     "Unfilled postings backlog exceeds threshold",
			MetricValue: float64(report.Employment.UnfilledPostings),
			Threshold:   float64(thresholds.MaxStagnantVacancies),
		})
	}

	// Status resolution
	hasCritical := false
	hasWarning := false
	for _, a := range report.Anomalies {
		if a.Severity == core.SeverityCritical {
			hasCritical = true
		} else if a.Severity == core.SeverityWarning {
			hasWarning = true
		}
	}

	if hasCritical {
		report.Status = core.StatusCritical
	} else if hasWarning {
		report.Status = core.StatusWarning
	} else {
		report.Status = core.StatusHealthy
	}

	// Sort top event types descending
	sort.Slice(report.Events.TopEventTypes, func(i, j int) bool {
		return report.Events.TopEventTypes[i].Count > report.Events.TopEventTypes[j].Count
	})

	return report, nil
}
