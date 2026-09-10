-- +goose Up
-- Replace the age integer with a date of birth.
-- Migration 00016 shipped an age INTEGER column, but a stored age
-- goes stale (it never ticks over on birthdays) and cannot be
-- backfilled into an exact birth date — age "30" spans a 12-month
-- window. date_of_birth is TEXT in YYYY-MM-DD (matching
-- health_snapshots.snapshot_date); NULL means unset. Age is derived
-- server-side via models.AgeAt wherever it is displayed or sent to
-- the Coach prompt, so the LLM only ever sees "age=N".
-- Existing age values are dropped: approximating a DOB from them
-- (e.g. Jan 1 of birth year) would store a date the user never gave
-- us, which is worse than asking again.
ALTER TABLE users ADD COLUMN date_of_birth TEXT;
ALTER TABLE users DROP COLUMN age;

-- +goose Down
ALTER TABLE users ADD COLUMN age INTEGER;
ALTER TABLE users DROP COLUMN date_of_birth;
