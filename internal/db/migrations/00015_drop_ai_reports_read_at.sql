-- +goose Up
-- Drop the read_at triage column. Dismiss is the only triage
-- action: a report is either current, dismissed, or replaced by
-- a newer week. Keeping a second boolean-ish stamp added UI
-- without meaning, so it goes entirely (column, queries, API).
ALTER TABLE ai_reports DROP COLUMN read_at;

-- +goose Down
ALTER TABLE ai_reports ADD COLUMN read_at DATETIME;
