-- Preserve existing 040 speech rounds and wait receipts while extending the
-- private proposal table with a typed, selected immediate move.
ALTER TABLE rp_shared_round_actions ADD COLUMN action_kind TEXT NOT NULL DEFAULT 'speech' CHECK (action_kind IN ('speech','move'));
ALTER TABLE rp_shared_rounds ADD COLUMN selected_action_kind TEXT NOT NULL DEFAULT 'speech' CHECK (selected_action_kind IN ('speech','move'));

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-f3-shared-move-rounds-041-2026-09-25','2026-09-25T00:00:00Z');
