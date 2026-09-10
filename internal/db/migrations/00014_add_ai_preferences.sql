-- +goose Up
-- Coach (user-facing name) opt-in and free-text training aim.
-- ai_opt_in is the server-side gate: no workout data leaves the
-- server for LLM processing unless this is 1. It composes with
-- the iOS BetaFeature master switch (which only controls UI
-- visibility) — both must be on for Coach to work.
-- ai_goal_text is what the user is trying to achieve, in their
-- own words (max 1000 chars enforced app-side). It is injected
-- verbatim into the weekly prompt as {{USER_AIM}}; empty means
-- "no stated aim" and the report falls back to general
-- progression guidance. The existing goals table is untouched.
ALTER TABLE users ADD COLUMN ai_opt_in INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN ai_goal_text TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN ai_goal_text;
ALTER TABLE users DROP COLUMN ai_opt_in;
