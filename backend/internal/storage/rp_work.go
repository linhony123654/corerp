package storage

import (
	"context"

	"corerp.local/backend/internal/core"
)

type RPWorkJob struct {
	ContractID     string `json:"contract_id"`
	EmployerName   string `json:"employer_name"`
	Status         string `json:"status"`
	WorkplaceName  string `json:"workplace_name"`
	WageMinor      int64  `json:"wage_minor,string"`
	PayPeriodDays  int    `json:"pay_period_days"`
	CurrencyID     string `json:"currency_id"`
	CurrencyScale  int    `json:"currency_scale"`
	CurrencySymbol string `json:"currency_symbol"`
}

type RPWorkAppointment struct {
	ScheduleID        string `json:"schedule_id"`
	WorldTime         string `json:"world_time"`
	OriginalWorldTime string `json:"original_world_time,omitempty"`
	PlaceName         string `json:"place_name"`
	Activity          string `json:"activity"`
}

type RPWork struct {
	Jobs              []RPWorkJob         `json:"jobs"`
	Appointments      []RPWorkAppointment `json:"appointments"`
	MoreAppointments  bool                `json:"more_appointments"`
	WorldTime         string              `json:"world_time"`
	ObservationCursor int64               `json:"observation_cursor"`
}

func (s *Store) ReadRPWork(ctx context.Context, r core.RPSessionReadRequest) (RPWork, error) {
	var result RPWork
	if err := r.Validate(); err != nil {
		return result, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return result, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if session.Status != "active" {
		return result, core.NewError(core.CodeBranchConflict, "work view requires an active session")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.current_world_time,b.head_sequence FROM world_clocks c JOIN branches b ON b.instance_id=c.instance_id AND b.branch_id=c.branch_id WHERE b.instance_id=? AND b.branch_id=?`, session.InstanceID, session.BranchID).Scan(&result.WorldTime, &result.ObservationCursor); err != nil {
		return result, err
	}
	jobs, err := readRPOwnEmployment(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, result.WorldTime)
	if err != nil {
		return result, err
	}
	result.Jobs = []RPWorkJob{}
	for _, own := range jobs {
		job := RPWorkJob{ContractID: own.ContractID, Status: own.Status, WageMinor: own.WageMinor}
		err := tx.conn.QueryRowContext(ctx, `SELECT c.currency_id,c.scale,c.symbol,
		 COALESCE((SELECT display_name FROM economic_entities WHERE entity_id=?),''),
		 COALESCE((SELECT display_name FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=?),''),
		 COALESCE((SELECT pay_period_days FROM employment_contracts WHERE contract_id=? AND employee_entity_id=?),0)
		 FROM currencies c WHERE c.currency_id=COALESCE(
		 (SELECT currency_id FROM employment_contracts WHERE contract_id=? AND employee_entity_id=?),
		 (SELECT currency_id FROM m2_cohort_contracts WHERE contract_id=?))`,
			own.OrganizationID, own.WorkplaceID, session.InstanceID, session.BranchID, own.ContractID, session.ControlledEntityID,
			own.ContractID, session.ControlledEntityID, own.ContractID).Scan(&job.CurrencyID, &job.CurrencyScale, &job.CurrencySymbol, &job.EmployerName, &job.WorkplaceName, &job.PayPeriodDays)
		if err != nil {
			return result, classifyMissing(err, "own work contract metadata")
		}
		if own.PayPeriodDays > 0 {
			job.PayPeriodDays = own.PayPeriodDays
		}
		if job.Status == "" {
			job.Status = "contract_active"
		}
		result.Jobs = append(result.Jobs, job)
	}
	// Query only this actor's real pending schedule. All rows close before delay
	// evidence lookups: Store deliberately uses a single SQLite connection.
	rows, err := tx.conn.QueryContext(ctx, `SELECT s.schedule_id,q.world_time,s.world_time,p.display_name,s.activity_code
	 FROM agent_schedule_entries s JOIN scheduler_items q ON q.scheduler_item_id=s.scheduler_item_id
	 JOIN agent_places p ON p.place_id=s.place_id AND p.instance_id=? AND p.branch_id=?
	 WHERE s.agent_id=? AND s.status='active' AND q.status='pending' AND q.world_time>=?
	 ORDER BY q.world_time,s.declared_priority,s.schedule_id LIMIT 21`, session.InstanceID, session.BranchID, session.ControlledEntityID, result.WorldTime)
	if err != nil {
		return result, err
	}
	result.Appointments = []RPWorkAppointment{}
	for rows.Next() {
		var entry RPWorkAppointment
		if err := rows.Scan(&entry.ScheduleID, &entry.WorldTime, &entry.OriginalWorldTime, &entry.PlaceName, &entry.Activity); err != nil {
			rows.Close()
			return result, err
		}
		result.Appointments = append(result.Appointments, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Appointments) > 20 {
		result.MoreAppointments = true
		result.Appointments = result.Appointments[:20]
	}
	for i := range result.Appointments {
		entry := &result.Appointments[i]
		if entry.OriginalWorldTime == entry.WorldTime {
			entry.OriginalWorldTime = ""
			continue
		}
		delay, err := readLatestRPTransitDelay(ctx, tx.conn, session.InstanceID, session.BranchID, entry.ScheduleID)
		if err != nil {
			return result, err
		}
		if delay == nil || delay.AgentID != session.ControlledEntityID || delay.OriginalWorldTime != entry.OriginalWorldTime || delay.Retry.WorldTime != entry.WorldTime {
			return result, core.NewError(core.CodeProjectionDiverged, "own schedule delay lacks matching evidence")
		}
	}
	return result, nil
}
