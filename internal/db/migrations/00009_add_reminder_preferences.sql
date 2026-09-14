-- +goose Up
-- Per-user weight reminder preferences. Each user picks their own
-- frequency, day-of-week, time-of-day, and channels (push + email).
-- The cron package reads reminder_next_fire_at to decide who is
-- due; the periodic tick (every hour) advances it after firing.
--
-- reminder_day_of_week uses Go's time.Weekday convention: 0=Sunday,
-- 1=Monday, ..., 6=Saturday. Nullable because off / daily do not
-- need it.
--
-- reminder_time is stored as "HH:00" UTC. The picker on /profile is
-- hour-only by design (minute precision was explicitly out of scope).
--
-- Existing users are opted in to the previous global behavior: weekly,
-- Sunday, 09:00, both channels. The next fire is the next Sunday
-- 09:00 UTC at or after the migration run. The literal is filled in
-- by the Go migration helper at apply time so the migration file
-- stays portable.
ALTER TABLE users ADD COLUMN reminder_enabled        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN reminder_frequency      TEXT    NOT NULL DEFAULT 'weekly';
ALTER TABLE users ADD COLUMN reminder_day_of_week    INTEGER;
ALTER TABLE users ADD COLUMN reminder_time          TEXT    NOT NULL DEFAULT '09:00';
ALTER TABLE users ADD COLUMN reminder_email_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN reminder_push_enabled  INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN reminder_next_fire_at  DATETIME;
ALTER TABLE users ADD COLUMN reminder_last_fired_at DATETIME;

-- +goose StatementBegin
-- Pre-compute the next Sunday 09:00 UTC so the existing users row is
-- back-filled with a value the hourly tick will pick up on its first
-- run.
--
-- SQLite has no native "next Sunday 09:00" function, so we use a
-- portable trick: pick the first date >= today whose
-- strftime('%w', date) = '0' (Sunday) and concatenate with
-- ' 09:00:00'. The base value is the string literal 'now', which
-- SQLite treats as the current UTC timestamp (matching the
-- CURRENT_TIMESTAMP behavior the rest of the app relies on).
UPDATE users
SET    reminder_enabled     = 1,
       reminder_frequency   = 'weekly',
       reminder_day_of_week = 0,
       reminder_time        = '09:00',
       reminder_next_fire_at = (
           SELECT date('now', '+' || (((7 - cast(strftime('%w', 'now') AS INTEGER)) % 7)) || ' days') || ' 09:00:00'
       )
WHERE  reminder_next_fire_at IS NULL;
-- +goose StatementEnd

-- The hourly tick's "who is due" query is the only path that reads
-- reminder_next_fire_at, so the index is on (next_fire_at) — a
-- partial index gated by enabled=1 would be ideal but the tick is
-- cheap enough (one query per hour) that a plain index is fine.
CREATE INDEX idx_users_reminder_due ON users(reminder_next_fire_at);

-- +goose Down
DROP INDEX IF EXISTS idx_users_reminder_due;
ALTER TABLE users DROP COLUMN reminder_last_fired_at;
ALTER TABLE users DROP COLUMN reminder_next_fire_at;
ALTER TABLE users DROP COLUMN reminder_push_enabled;
ALTER TABLE users DROP COLUMN reminder_email_enabled;
ALTER TABLE users DROP COLUMN reminder_time;
ALTER TABLE users DROP COLUMN reminder_day_of_week;
ALTER TABLE users DROP COLUMN reminder_frequency;
ALTER TABLE users DROP COLUMN reminder_enabled;
