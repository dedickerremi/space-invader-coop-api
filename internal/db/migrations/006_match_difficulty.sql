-- Matches record the difficulty they were played at, so defeats can be read
-- per difficulty. NULL for matches recorded before this migration.
ALTER TABLE match_summaries
  ADD COLUMN IF NOT EXISTS difficulty TEXT;
