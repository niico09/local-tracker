-- 0002_reviews: per-profile ratings and one shared note per catalog item.
-- Ratings are personal, one score per profile per item (checked 1..5); the
-- note is the couple's shared free text. Both cascade with the item.

CREATE TABLE item_ratings (
  item_id    INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  score      INTEGER NOT NULL CHECK (score BETWEEN 1 AND 5),
  updated_at TEXT    NOT NULL,
  PRIMARY KEY (item_id, user_id)
);

CREATE TABLE item_notes (
  item_id    INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
  body       TEXT    NOT NULL,
  updated_at TEXT    NOT NULL
);
