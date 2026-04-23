-- Stress-test matches written by cmd/loadgen accumulated as "abandoned"
-- rows before the finalizer learned to skip them. This drops those rows
-- on every boot; match_participants cascades via the FK defined in 002.
-- Idempotent: once the finalizer skips loadgen writes, this becomes a
-- no-op on subsequent boots.

DELETE FROM match_summaries WHERE platform = 'loadgen';
