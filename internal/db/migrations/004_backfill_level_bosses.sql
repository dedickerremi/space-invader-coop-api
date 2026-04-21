-- Levels 2-5 were seeded into the DB before the `boss` field existed in
-- the embedded JSON. Seed uses ON CONFLICT DO NOTHING so later deploys
-- never refreshed those rows, leaving prod without boss fights.
--
-- This migration merges the `boss` field into existing rows that are
-- missing it. It preserves any admin edits to `waves` — only patches the
-- missing boss key.

DO $$
DECLARE
  r RECORD;
  bosses JSONB := '{
    "level2.json": {"kind": "sentinel"},
    "level3.json": {"kind": "warden"},
    "level4.json": {"kind": "citadel"},
    "level5.json": {"kind": "nexus"}
  }'::jsonb;
  lvl TEXT;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.tables WHERE table_name = 'levels'
  ) THEN
    RETURN;
  END IF;

  FOR lvl IN SELECT jsonb_object_keys(bosses) LOOP
    UPDATE levels
       SET definition = jsonb_set(definition, '{boss}', bosses -> lvl, true),
           updated_at = now()
     WHERE name = lvl
       AND NOT (definition ? 'boss');
  END LOOP;
END $$;
