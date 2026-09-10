-- +goose Up
-- Coach (user-facing name) weekly reports and future insight kinds.
-- The table is explicitly named ai_reports so the codebase always
-- reads as AI-generated content, never as a workout or goal.
-- kind is stored in the "type" column: 'weekly' now;
-- 'insight' (daily store/dismiss cards) and 'monthly' (story)
-- reuse this table later with no new migrations for storage.
-- payload_json is the validated report JSON produced from the
-- versioned prompt in internal/ai/prompts/weekly_review.md;
-- prompt_version records which prompt produced the row so reports
-- stay comparable as the prompt is refined over time.
CREATE TABLE ai_reports (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    type       TEXT NOT NULL DEFAULT 'weekly' CHECK(type IN ('weekly', 'insight', 'monthly')),
    period_start DATE NOT NULL,
    period_end   DATE NOT NULL,
    prompt_version TEXT NOT NULL DEFAULT '',
    model        TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    tokens_in    INTEGER NOT NULL DEFAULT 0,
    tokens_out   INTEGER NOT NULL DEFAULT 0,
    read_at      DATETIME,
    dismissed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_ai_reports_user_type_period ON ai_reports(user_id, type, period_start);
CREATE INDEX idx_ai_reports_user ON ai_reports(user_id, type, period_start DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_reports_user;
DROP INDEX IF EXISTS idx_ai_reports_user_type_period;
DROP TABLE IF EXISTS ai_reports;
