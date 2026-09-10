-- name: CreateAIReport :one
-- Insert a Coach report. The caller enforces idempotency on
-- (user_id, type, period_start): re-runs for the same week
-- read the existing row via GetAIReport instead of inserting.
INSERT INTO ai_reports (id, user_id, type, period_start, period_end, prompt_version, model, payload_json, tokens_in, tokens_out)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAIReport :one
-- Fetch a single report for idempotency checks ((user_id, type,
-- period_start) unique key) before deciding to call the LLM.
SELECT * FROM ai_reports
WHERE user_id = ? AND type = ? AND period_start = ?;

-- name: GetAIReportByID :one
-- Fetch a single report by id scoped to the user so a request
-- cannot read or mutate another user's report.
SELECT * FROM ai_reports
WHERE id = ? AND user_id = ?;

-- name: GetLatestAIReport :one
-- The iOS Coach screen's primary read: newest report of a type.
SELECT * FROM ai_reports
WHERE user_id = ? AND type = ?
ORDER BY period_start DESC
LIMIT 1;

-- name: ListAIReports :many
-- Report history for the iOS Coach screen, newest first.
SELECT * FROM ai_reports
WHERE user_id = ? AND type = ?
ORDER BY period_start DESC
LIMIT ?;

-- name: MarkAIReportDismissed :exec
-- Stamp dismissed_at. Idempotent: re-dismissals overwrite.
UPDATE ai_reports
SET dismissed_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: ReopenAIReport :exec
-- Clear dismissed_at, returning the report to the card list.
-- Idempotent: reopening a non-dismissed row is a no-op that
-- still matches (so the route can call it without checking).
UPDATE ai_reports
SET dismissed_at = NULL
WHERE id = ? AND user_id = ?;
