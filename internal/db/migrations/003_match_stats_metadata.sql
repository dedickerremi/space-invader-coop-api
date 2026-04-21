-- Extend match_summaries with client metadata + richer outcome tracking.
-- Pre-existing rows are none (nothing wrote to this table before), so
-- backfills are defensive.

ALTER TABLE match_summaries
  ADD COLUMN IF NOT EXISTS started_at    TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS outcome       TEXT,
  ADD COLUMN IF NOT EXISTS bosses_killed TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS user_agent    TEXT,
  ADD COLUMN IF NOT EXISTS platform      TEXT,
  ADD COLUMN IF NOT EXISTS locale        TEXT,
  ADD COLUMN IF NOT EXISTS country       CHAR(2),
  ADD COLUMN IF NOT EXISTS ip_hash       CHAR(64);

-- Derive outcome for any row that might exist from before this migration.
UPDATE match_summaries
   SET outcome = CASE WHEN game_over THEN 'defeat' ELSE 'abandoned' END
 WHERE outcome IS NULL;

-- Allow guests in match_participants by making user_id nullable + swapping
-- to a surrogate primary key. Safe: nothing wrote to the table before.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'match_participants'
      AND column_name = 'user_id'
      AND is_nullable = 'NO'
  ) THEN
    ALTER TABLE match_participants DROP CONSTRAINT IF EXISTS match_participants_user_id_fkey;
    ALTER TABLE match_participants DROP CONSTRAINT IF EXISTS match_participants_pkey;
    ALTER TABLE match_participants ALTER COLUMN user_id DROP NOT NULL;
    ALTER TABLE match_participants ADD COLUMN id BIGSERIAL PRIMARY KEY;
    ALTER TABLE match_participants
      ADD CONSTRAINT match_participants_user_id_fkey
      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
  END IF;
END $$;

ALTER TABLE match_participants
  ADD COLUMN IF NOT EXISTS deaths INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS match_participants_match_idx    ON match_participants(match_id);
CREATE INDEX IF NOT EXISTS match_summaries_ended_desc_idx  ON match_summaries(ended_at DESC);
CREATE INDEX IF NOT EXISTS match_summaries_outcome_idx     ON match_summaries(outcome);
