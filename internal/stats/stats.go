// Package stats owns match-end persistence (FinalizeMatch + IP/geo
// helpers) and in-memory stat derivations used by the monitoring
// dashboard. It must not import ws — the WS package imports stats for
// metadata helpers, and any reverse edge would create a cycle. The
// dashboard-facing GetServerStats lives in the monitoring package.
package stats
