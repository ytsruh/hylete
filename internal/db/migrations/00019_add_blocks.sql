-- +goose Up
-- Blocks are user-owned planned exercise groups (Beta). A block has a
-- name, optional description, a kind (standard|circuit|amrap|emom) with
-- kind-specific config, and 1-20 planned items in block_items.
--
-- block_items rows are *planned references* to the exercise catalog —
-- they hold only a free-text target (e.g. "3x5 @ 100kg"). They are
-- deliberately NOT rows in exercise_entries: logged sets/sessions live
-- there and feed history, charts, and exports, while a plan has no
-- metrics yet. A future "log from block" flow will copy items into new
-- exercise_entries rows (optionally stamped with the source block).
-- Future workouts (many-to-many over blocks) will add their own
-- workouts + workout_blocks join tables; no stub columns here.
CREATE TABLE blocks (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id),
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    block_type       TEXT NOT NULL DEFAULT 'standard',
    rounds           INTEGER NOT NULL DEFAULT 0,
    rest_seconds     INTEGER NOT NULL DEFAULT 0,
    time_cap_seconds INTEGER NOT NULL DEFAULT 0,
    interval_seconds INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_blocks_user ON blocks(user_id);

CREATE TABLE block_items (
    id          TEXT PRIMARY KEY,
    block_id    TEXT NOT NULL REFERENCES blocks(id) ON DELETE CASCADE,
    exercise_id TEXT NOT NULL REFERENCES exercises(id),
    position    INTEGER NOT NULL,
    target_text TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_block_items_block ON block_items(block_id, position);

-- +goose Down
DROP INDEX IF EXISTS idx_block_items_block;
DROP TABLE IF EXISTS block_items;
DROP INDEX IF EXISTS idx_blocks_user;
DROP TABLE IF EXISTS blocks;
