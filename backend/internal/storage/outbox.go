package storage

import (
	"context"
	"time"

	"corerp.local/backend/internal/core"
)

type OutboxMessage struct {
	OutboxID      string `json:"outbox_id"`
	EventID       string `json:"event_id"`
	InstanceID    string `json:"instance_id"`
	BranchID      string `json:"branch_id"`
	EventSequence int64  `json:"event_sequence"`
	Topic         string `json:"topic"`
	AudienceScope string `json:"audience_scope"`
	Payload       string `json:"payload"`
	Attempts      int64  `json:"attempts"`
}

type OutboxPublisher interface {
	Publish(context.Context, OutboxMessage) error
}

type OutboxPublishFunc func(context.Context, OutboxMessage) error

func (function OutboxPublishFunc) Publish(ctx context.Context, message OutboxMessage) error {
	return function(ctx, message)
}

type DispatchResult struct {
	Eligible        int `json:"eligible"`
	Delivered       int `json:"delivered"`
	MarkedPublished int `json:"marked_published"`
	Failed          int `json:"failed"`
}

func (s *Store) DispatchOutbox(ctx context.Context, limit int, publisher OutboxPublisher) (DispatchResult, error) {
	if limit <= 0 || limit > 1000 {
		return DispatchResult{}, core.NewError(core.CodeInvalidArgument, "Outbox dispatch limit must be between 1 and 1000")
	}
	if publisher == nil {
		return DispatchResult{}, core.NewError(core.CodeInvalidArgument, "Outbox publisher is required")
	}
	now := s.now().UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.outbox_id, o.event_id, e.instance_id, e.branch_id, e.event_sequence,
		       o.topic, o.audience_scope, o.payload, o.attempts
		FROM outbox o JOIN events e ON e.event_id = o.event_id
		WHERE o.published_at_utc IS NULL
		  AND (o.next_attempt_at_utc IS NULL OR o.next_attempt_at_utc <= ?)
		ORDER BY e.event_sequence, o.outbox_id
		LIMIT ?`, now.Format(time.RFC3339Nano), limit)
	if err != nil {
		return DispatchResult{}, core.WrapError(core.CodeStorageFailure, "query pending Outbox", err)
	}
	messages := make([]OutboxMessage, 0, limit)
	for rows.Next() {
		var message OutboxMessage
		if err := rows.Scan(
			&message.OutboxID, &message.EventID, &message.InstanceID, &message.BranchID,
			&message.EventSequence, &message.Topic, &message.AudienceScope, &message.Payload, &message.Attempts,
		); err != nil {
			rows.Close()
			return DispatchResult{}, core.WrapError(core.CodeStorageFailure, "scan pending Outbox", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Close(); err != nil {
		return DispatchResult{}, core.WrapError(core.CodeStorageFailure, "close pending Outbox rows", err)
	}
	if err := rows.Err(); err != nil {
		return DispatchResult{}, core.WrapError(core.CodeStorageFailure, "iterate pending Outbox", err)
	}

	result := DispatchResult{Eligible: len(messages)}
	for _, message := range messages {
		if err := publisher.Publish(ctx, message); err != nil {
			result.Failed++
			nextAttempt := now.Add(outboxRetryDelay(message.Attempts + 1)).Format(time.RFC3339Nano)
			if _, updateErr := s.db.ExecContext(ctx, `
				UPDATE outbox
				SET attempts = attempts + 1, next_attempt_at_utc = ?
				WHERE outbox_id = ? AND published_at_utc IS NULL`, nextAttempt, message.OutboxID); updateErr != nil {
				return result, core.WrapError(core.CodeStorageFailure, "record Outbox publish failure", updateErr)
			}
			return result, core.WrapError(core.CodeOutboxDelivery, "publish Outbox "+message.OutboxID, err)
		}
		result.Delivered++
		if s.afterPublish != nil {
			if err := s.afterPublish(message); err != nil {
				return result, err
			}
		}
		update, err := s.db.ExecContext(ctx, `
			UPDATE outbox
			SET attempts = attempts + 1, next_attempt_at_utc = NULL, published_at_utc = ?
			WHERE outbox_id = ? AND published_at_utc IS NULL`, now.Format(time.RFC3339Nano), message.OutboxID)
		if err != nil {
			return result, core.WrapError(core.CodeStorageFailure, "mark Outbox published", err)
		}
		rowsAffected, err := update.RowsAffected()
		if err != nil {
			return result, core.WrapError(core.CodeStorageFailure, "inspect Outbox publish mark", err)
		}
		if rowsAffected == 1 {
			result.MarkedPublished++
		}
	}
	return result, nil
}

func (s *Store) PendingOutboxCount(ctx context.Context) (int64, error) {
	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox WHERE published_at_utc IS NULL`).Scan(&count); err != nil {
		return 0, core.WrapError(core.CodeStorageFailure, "count pending Outbox", err)
	}
	return count, nil
}

func outboxRetryDelay(attempt int64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 7 {
		attempt = 7
	}
	delay := time.Second * time.Duration(1<<(attempt-1))
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
