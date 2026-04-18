-- Levels: admin-created game levels. Definition stores the wave JSON.
CREATE TABLE IF NOT EXISTS levels (
  name        TEXT PRIMARY KEY,
  definition  JSONB NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
