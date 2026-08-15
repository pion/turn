# The fork is an owned library, not a tracked fork

`the-sarge/turn` exists to be wiremux's TURN client library; after the 2026-08-14 fork pivot we decided it is fully owned — free to break API, delete surface, and diverge from `pion/turn` without limit — rather than a trimmed fork that preserves upstream shape for cheap patch flow. Wiremux pins exact `-gs` pre-release versions, so divergence costs the one consumer nothing, and the alternative (upstream compatibility) would have blocked the entire molding program's purpose.

## Consequences

- Upstream is a read-only reference. Security posture is deliberately narrow: we watch upstream releases for `internal/proto` (wire-parsing) changes only; client-side upstream fixes are ignored because the client had already diverged (pion/turn#585 plus the `4b818fc` transaction-wait abort live only here). Dependency CVEs still arrive through normal dependency updates.
- Versioning: minors bump per milestone (`v5.1.0-gs.1`, `v5.2.0-gs.1`, …) and the `-gs` suffix is permanent — Go never auto-selects pre-releases, so every consumer bump is deliberate.
- The upstream `pion/turn/v5` test-only dependency (integration-test server fixture, possible only because the module rename removed the import-path collision) is a transitional seam, not a compatibility statement; it exits when fork-owned receipt coverage replaces it.
