package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"corerp.local/backend/internal/core"
)

type RPMessagesReadRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	BeforeSequence int64  `json:"before_sequence,omitempty"`
}

type RPMessage struct {
	MessageID string `json:"message_id"`
	Sequence  int64  `json:"sequence"`
	WorldTime string `json:"world_time"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

type RPMessages struct {
	Messages           []RPMessage `json:"messages"`
	NextBeforeSequence int64       `json:"next_before_sequence,omitempty"`
	WorldTime          string      `json:"world_time"`
	ObservationCursor  int64       `json:"observation_cursor"`
}

// Own transaction notices/receipts, not a fabricated SMS delivery ledger or
// ambient speech history. Explicitly project public fields from allowed facts;
// never serialize CareerFact or its internal evaluation/assessment pointers.
func (s *Store) ReadRPMessages(ctx context.Context, r RPMessagesReadRequest) (RPMessages, error) {
	var result RPMessages
	if err := (core.RPSessionReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID}).Validate(); err != nil {
		return result, err
	}
	if r.BeforeSequence < 0 {
		return result, core.NewError(core.CodeInvalidArgument, "message cursor cannot be negative")
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
		return result, core.NewError(core.CodeBranchConflict, "messages require an active session")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return result, err
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.current_world_time,b.head_sequence FROM world_clocks c JOIN branches b ON b.instance_id=c.instance_id AND b.branch_id=c.branch_id WHERE b.instance_id=? AND b.branch_id=?`, session.InstanceID, session.BranchID).Scan(&result.WorldTime, &result.ObservationCursor); err != nil {
		return result, err
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events
 WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded'
 AND json_extract(payload,'$.candidate_id')=? AND (?=0 OR event_sequence<?)
 AND (
 (json_extract(payload,'$.kind')='interview' AND json_extract(payload,'$.interview.status') IN ('invited','responded')) OR
 (json_extract(payload,'$.kind')='offer' AND json_extract(payload,'$.offer.status') IN ('offered','accepted','declined','expired')) OR
 (json_extract(payload,'$.kind')='employment' AND json_extract(payload,'$.employment.employee_id')=?) OR
 (json_extract(payload,'$.kind')='leave' AND json_extract(payload,'$.leave.status') IN ('approved','rejected')) OR
 (json_extract(payload,'$.kind')='position_change' AND json_extract(payload,'$.position_change.status') IN ('offered','accepted','declined')) OR
 (json_extract(payload,'$.kind')='overtime' AND json_extract(payload,'$.overtime.status') IN ('offered','accepted','declined','cancelled'))
 ) ORDER BY event_sequence DESC LIMIT 51`, session.InstanceID, session.BranchID, session.ControlledEntityID, r.BeforeSequence, r.BeforeSequence, session.ControlledEntityID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Messages = []RPMessage{}
	for rows.Next() {
		var m RPMessage
		var raw string
		if err := rows.Scan(&m.MessageID, &m.Sequence, &m.WorldTime, &raw); err != nil {
			return result, err
		}
		var fact CareerFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return result, err
		}
		if err := describeRPMessage(&m, fact); err != nil {
			return result, err
		}
		result.Messages = append(result.Messages, m)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Messages) > 50 {
		result.Messages = result.Messages[:50]
		result.NextBeforeSequence = result.Messages[49].Sequence
	}
	return result, nil
}

func describeRPMessage(m *RPMessage, f CareerFact) error {
	m.Kind = f.Kind
	switch {
	case f.Kind == "interview" && f.Interview != nil:
		m.Title = "面试邀请"
		m.Body = f.Interview.Question
		if f.Interview.Status == "responded" {
			m.Title = "面试答复回执"
			m.Body = "问题：" + f.Interview.Question + "\n你的答复：" + f.Interview.Answer
		}
	case f.Kind == "offer" && f.Offer != nil:
		m.Title = "录用邀约"
		if f.Offer.Status == "declined" {
			m.Title = "录用邀约拒绝回执"
		}
		if f.Offer.Status == "accepted" {
			m.Title = "录用邀约接受回执"
		}
		if f.Offer.Status == "expired" {
			m.Title = "录用邀约已到期"
		}
		m.Body = fmt.Sprintf("邀约入职日期：%s。工作时段：%02d:00–%02d:00。答复期限：%s。邀约本身不代表已经入职。", careerTime(f.Offer.StartsOnDay, 0, 0)[:10], f.Offer.WorkStartHour, f.Offer.WorkEndHour, f.Offer.ExpiresAt)
		if f.Offer.DeclineReason != "" {
			m.Body += "\n你的答复：" + f.Offer.DeclineReason
		}
	case f.Kind == "employment" && f.Employment != nil:
		m.Title = "入职约定回执"
		m.Body = "已接受工作约定；实际生效和当前条款请查看工作信息。"
		if f.EmploymentChange != nil {
			labels := map[string]string{"regularized": "转正通知", "raise": "调薪通知", "promotion": "晋升约定回执", "transfer": "调岗约定回执", "demotion": "职级调整约定回执", "resignation": "离职回执", "termination": "解约通知", "layoff": "裁员通知"}
			label, ok := labels[f.EmploymentChange.Kind]
			if !ok {
				return core.NewError(core.CodeProjectionDiverged, "unsupported employment notice kind")
			}
			m.Title = label
			m.Body = f.EmploymentChange.Notice + fmt.Sprintf("\n生效日期：%s。通知不代表工资已经到账。", careerTime(f.Employment.EffectiveFromDay, 0, 0)[:10])
		}
	case f.Kind == "leave" && f.Leave != nil:
		m.Title = "请假审批：未批准"
		if f.Leave.Status == "approved" {
			m.Title = "请假审批：已批准"
		}
		m.Body = fmt.Sprintf("请假区间：%s 至 %s（不含结束日）。\n%s", careerTime(f.Leave.StartDay, 0, 0)[:10], careerTime(f.Leave.EndDay, 0, 0)[:10], f.Leave.Notice)
	case f.Kind == "position_change" && f.PositionChange != nil:
		m.Title = "岗位调整邀约"
		if f.PositionChange.Status == "declined" {
			m.Title = "岗位调整拒绝回执"
		}
		if f.PositionChange.Status == "accepted" {
			m.Title = "岗位调整接受回执"
		}
		m.Body = f.PositionChange.Notice + "\n邀约不代表条款已经生效。"
		if f.PositionChange.ResponseReason != "" {
			m.Body += "\n你的答复：" + f.PositionChange.ResponseReason
		}
	case f.Kind == "overtime" && f.Overtime != nil:
		labels := map[string]string{"offered": "加班邀约", "accepted": "加班接受回执", "declined": "加班拒绝回执", "cancelled": "加班取消通知"}
		m.Title = labels[f.Overtime.Status]
		m.Body = fmt.Sprintf("日期：%s，%02d:00–%02d:00。\n%s\n报酬以实际工作记录和结算为准。", careerTime(f.Overtime.Day, 0, 0)[:10], f.Overtime.StartHour, f.Overtime.EndHour, f.Overtime.Reason)
		if f.Overtime.ResponseReason != "" {
			m.Body += "\n答复：" + f.Overtime.ResponseReason
		}
	default:
		return core.NewError(core.CodeProjectionDiverged, "invalid own message source")
	}
	return nil
}
