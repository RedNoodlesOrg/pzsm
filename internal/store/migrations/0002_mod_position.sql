ALTER TABLE mods ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

-- Backfill positions in insertion order so existing DBs keep a deterministic
-- starting ordering rather than ties on 0.
UPDATE mods SET position = rowid;

CREATE INDEX idx_mods_position ON mods(position);
