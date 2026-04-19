-- Users: mirror of Clerk users who have authenticated at least once.
-- Clerk remains the source of truth for auth/email; we store only what
-- we need for in-app display and foreign keys.
CREATE TABLE IF NOT EXISTS users (
  id            TEXT PRIMARY KEY,            -- Clerk user_id (e.g. "user_2abc...")
  display_name  TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per finished match. TTL-able (drop rows older than N days).
CREATE TABLE IF NOT EXISTS match_summaries (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mode              TEXT NOT NULL CHECK (mode IN ('solo','coop')),
  level_name        TEXT,
  wave_reached      INT NOT NULL,
  duration_seconds  INT NOT NULL,
  game_over         BOOLEAN NOT NULL,
  ended_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS match_summaries_ended_idx ON match_summaries(ended_at);

-- Participants: authenticated players only. Guests do not write rows.
CREATE TABLE IF NOT EXISTS match_participants (
  match_id      UUID NOT NULL REFERENCES match_summaries(id) ON DELETE CASCADE,
  user_id       TEXT NOT NULL REFERENCES users(id),
  display_name  TEXT NOT NULL,
  points        INT NOT NULL,
  kills         INT NOT NULL,
  best_streak   INT NOT NULL,
  PRIMARY KEY (match_id, user_id)
);
CREATE INDEX IF NOT EXISTS match_participants_user_idx   ON match_participants(user_id);
CREATE INDEX IF NOT EXISTS match_participants_points_idx ON match_participants(points DESC);

-- Permanent aggregates per user (survives TTL of match_summaries).
CREATE TABLE IF NOT EXISTS player_stats (
  user_id                   TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  games_played              INT NOT NULL DEFAULT 0,
  total_kills               INT NOT NULL DEFAULT 0,
  total_points              INT NOT NULL DEFAULT 0,
  best_streak               INT NOT NULL DEFAULT 0,
  best_wave                 INT NOT NULL DEFAULT 0,
  best_single_match_points  INT NOT NULL DEFAULT 0,
  updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS player_stats_best_points_idx ON player_stats(best_single_match_points DESC);
CREATE INDEX IF NOT EXISTS player_stats_best_wave_idx   ON player_stats(best_wave DESC);
