package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

func organizationReviewQueues(ctx context.Context, q replayQuerier, instance, branch string, head int64) (map[string]SchedulerItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind') IN ('agency_policy','organization_review') ORDER BY event_sequence`, instance, branch, head)
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
	items := map[string]SchedulerItem{}
	for _, id := range ids {
		policy, at, err := organizationScheduleSource(ctx, q, instance, branch, id)
		if err != nil {
			return nil, err
		}
		if !policy.AutomaticReview || policy.Status != "active" {
			continue
		}
		item, _, err := organizationReviewItem(id, at, policy)
		if err != nil {
			return nil, err
		}
		var kind, raw, terminalAt string
		err = q.QueryRowContext(ctx, `SELECT event_type,payload,world_time FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_sequence<=?`, "event_"+item.SchedulerItemID, instance, branch, head).Scan(&kind, &raw, &terminalAt)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			valid := false
			if kind == "RPCareerFactRecorded" {
				var f CareerFact
				if json.Unmarshal([]byte(raw), &f) == nil && f.OrganizationReview != nil {
					valid = f.OrganizationReview.ScheduleSourceEventID == id
				}
			}
			if kind == "OrganizationReviewSkipped" {
				var skipped organizationReviewSkipped
				if json.Unmarshal([]byte(raw), &skipped) == nil {
					valid = skipped.SourceEventID == id && (skipped.Reason == "superseded_source" || skipped.Reason == "manager_authority_unavailable")
				}
			}
			if !valid || terminalAt != item.WorldTime {
				return nil, core.NewError(core.CodeProjectionDiverged, "organization schedule terminal source differs")
			}
			item.Status = "completed"
		}
		items[item.SchedulerItemID] = item
	}
	return items, nil
}

func organizationReviewQueueDifferences(ctx context.Context, q replayQuerier, instance, branch string, head int64) ([]ProjectionDifference, error) {
	want, err := organizationReviewQueues(ctx, q, instance, branch, head)
	if err != nil {
		return nil, err
	}
	got := map[string]SchedulerItem{}
	rows, err := q.QueryContext(ctx, `SELECT scheduler_item_id,world_time,phase_id,declared_priority,status,payload FROM scheduler_items WHERE instance_id=? AND branch_id=? AND phase_id=?`, instance, branch, organizationReviewPhase)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item SchedulerItem
		if err := rows.Scan(&item.SchedulerItemID, &item.WorldTime, &item.PhaseID, &item.DeclaredPriority, &item.Status, &item.Payload); err != nil {
			rows.Close()
			return nil, err
		}
		got[item.SchedulerItemID] = item
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for key := range want {
		keys[key] = true
	}
	for key := range got {
		keys[key] = true
	}
	var ordered []string
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var diffs []ProjectionDifference
	for _, key := range ordered {
		w, wok := want[key]
		g, gok := got[key]
		if wok && gok && w == g {
			continue
		}
		wb, err := core.CanonicalJSON(w)
		if err != nil {
			return nil, err
		}
		gb, err := core.CanonicalJSON(g)
		if err != nil {
			return nil, err
		}
		ws, gs := string(wb), string(gb)
		if !wok {
			ws = "absent"
		}
		if !gok {
			gs = "missing"
		}
		diffs = append(diffs, ProjectionDifference{Projection: "organization_review_queue", Key: key, ExpectedText: ws, ActualText: gs})
	}
	return diffs, nil
}

func repairOrganizationReviewQueues(ctx context.Context, conn *sql.Conn, instance, branch string, head int64) error {
	items, err := organizationReviewQueues(ctx, conn, instance, branch, head)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM scheduler_items WHERE instance_id=? AND branch_id=? AND phase_id=?`, instance, branch, organizationReviewPhase); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,?,?)`, item.SchedulerItemID, instance, branch, item.WorldTime, item.PhaseID, item.DeclaredPriority, item.Status, item.Payload); err != nil {
			return err
		}
	}
	return nil
}
