-- +goose Up
-- Optional user profile fields: height, gender & age.
-- All three are nullable/empty = unset so existing rows need no
-- backfill. Height is stored as a plain number in centimetres
-- (no conversion, mirroring how weight is a number labelled by
-- the user's preferred unit). Gender is free-form TEXT gated
-- app-side to a fixed list (male / female / non-binary /
-- prefer-not-to-say); empty means unset. Age is an integer in
-- years (10–120 enforced app-side); NULL means unset. All three
-- are fed into the Coach prompt as a USER_PROFILE block.
ALTER TABLE users ADD COLUMN height_cm REAL;
ALTER TABLE users ADD COLUMN gender TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN age INTEGER;

-- +goose Down
ALTER TABLE users DROP COLUMN age;
ALTER TABLE users DROP COLUMN gender;
ALTER TABLE users DROP COLUMN height_cm;
