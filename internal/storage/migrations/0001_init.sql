-- 0001_init: local-tracker v1 baseline schema (6 domain tables + migration ledger).
-- G1: items.owner_user_id replaces the old is_personal boolean; NULL means shared.

CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE users (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL UNIQUE,
  pin_hash   BLOB    NOT NULL,
  pin_salt   BLOB    NOT NULL,
  pin_iter   INTEGER NOT NULL,
  created_at TEXT    NOT NULL
);

CREATE TABLE sessions (
  token_hash BLOB    PRIMARY KEY,                 -- SHA-256(raw token)
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT    NOT NULL,
  expires_at TEXT    NOT NULL
);
CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE items (
  id            INTEGER PRIMARY KEY,
  title         TEXT    NOT NULL,
  kind          TEXT    NOT NULL CHECK (kind IN ('series','movie','book','course')),
  year          INTEGER,
  external_id   TEXT,                              -- Top 100 key; SQLite UNIQUE allows many NULLs
  cover_path    TEXT,                              -- filename only, relative to data/uploads/
  owner_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,  -- G1: NULL = shared
  created_at    TEXT    NOT NULL,
  updated_at    TEXT    NOT NULL,
  UNIQUE (external_id)
);
CREATE INDEX idx_items_owner ON items(owner_user_id);

CREATE TABLE goals (
  id            INTEGER PRIMARY KEY,
  title         TEXT    NOT NULL,
  owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,   -- NULL = couple
  target        INTEGER CHECK (target IS NULL OR target > 0),
  visibility    TEXT    NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','shared')),
  external_key  TEXT    UNIQUE,                    -- 'top100' seed idempotency; NULL for user goals
  created_at    TEXT    NOT NULL,
  updated_at    TEXT    NOT NULL
);
CREATE INDEX idx_goals_owner ON goals(owner_user_id);

CREATE TABLE goal_items (
  goal_id  INTEGER NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
  item_id  INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  added_at TEXT    NOT NULL,
  PRIMARY KEY (goal_id, item_id)
);
CREATE INDEX idx_goal_items_item ON goal_items(item_id);

CREATE TABLE progress (
  id            INTEGER PRIMARY KEY,
  item_id       INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,   -- NULL = couple
  done          INTEGER NOT NULL DEFAULT 0 CHECK (done IN (0,1)),
  start_date    TEXT,                              -- 'YYYY-MM-DD'
  end_date      TEXT,
  updated_at    TEXT    NOT NULL,
  CHECK (end_date IS NULL OR start_date IS NULL OR end_date >= start_date)
);
-- NULL-safe uniqueness: one shared (NULL) row and one row per personal owner.
CREATE UNIQUE INDEX ux_progress_item_owner ON progress(item_id, COALESCE(owner_user_id, 0));
