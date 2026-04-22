CREATE TABLE mods (
    workshop_id TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    thumbnail   TEXT NOT NULL,
    description TEXT NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE mod_ids (
    workshop_id TEXT NOT NULL REFERENCES mods(workshop_id) ON DELETE CASCADE,
    mod_id      TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (workshop_id, mod_id)
);

CREATE INDEX idx_mod_ids_enabled ON mod_ids(enabled) WHERE enabled = 1;

CREATE TABLE activity_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_email TEXT NOT NULL,
    action     TEXT NOT NULL,
    target     TEXT NOT NULL DEFAULT '',
    details    TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_activity_created_at ON activity_log(created_at DESC);
CREATE INDEX idx_activity_user ON activity_log(user_email, created_at DESC);
