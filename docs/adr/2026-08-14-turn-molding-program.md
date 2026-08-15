# TURN Fork Molding Program — 2026-08-14

**Status:** Accepted; Track 1 not yet implemented; Tracks 2–3 gated, plans pending

## What this is

The program that molds `the-sarge/turn` into wiremux's minimal owned TURN client library. Plan docs in `docs/adr/` are the normative source of truth; issues, task-manager mirrors, and this index carry pointers and current state only. Grill and pivot history: [scope doc](2026-08-14-molding-program-scope.md), [owned-library ADR](2026-08-14-owned-library-fork.md), wiremux fork-pivot amendment (GridSwarm/wiremux#1161).

## Outcomes that require no implementation

- Fork identity settled: owned library, upstream read-only ([ADR](2026-08-14-owned-library-fork.md)).
- Security posture settled: proto-only upstream watch; dependency CVEs via normal updates (scope doc D2).
- Versioning settled: `v5.N.0-gs.1` minors, permanent `-gs` suffix (scope doc D3).
- Pre-existing macOS test failures closed with no fix: deleted with cut surface (scope doc D5).
- Upstream PR pion/turn#585 left open passively; its outcome gates nothing.

## Tracks, dependencies, and frontier

| # | Track | Plan | Parent issue | Blocked by | Slices | Status |
|---|---|---|---|---|---|---|
| 1 | Cut and stabilize (M0) | [plan](2026-08-14-cut-and-stabilize-plan.md) | pending | None | 2 | FRONTIER |
| 2 | Modernize the kept API (M1) | plan pending | pending | Track 1; wiremux Slice 1.3 merged (GridSwarm/wiremux#1056) | TBD | GATED — requires a future `$architecture-handoff` run once its gate opens; content defined by 1.3 adoption experience |
| 3 | Optimize the packet path (M2) | plan pending | pending | Track 2; profiles from production wiremux traffic | TBD | GATED — same; no speculative optimization |

Cross-track slice edge: the transitional upstream-fixture seam introduced by Track 1 Slice 2 is removed by a Track 2 fixture-replacement slice, gated on fork-owned receipt coverage from wiremux Slice 1.3. Parallel-safe: Track 1 proceeds independently of wiremux Slice 1.3 adoption (which pins the already-published `v5.0.13-gs.1`). Recommended starter: Track 1 Slice 1.

## Rules that bind every track

- No behavior change to kept code except through an accepted track plan; kept-surface preservation gates per plan.
- Single-owner and authority-complete invariants, transitional-seam budgets, artifact classification, and evidence budgets per the shared contract-closure baseline (no repository overlay exists).
- Shared review-loop baseline governs every PR; post-merge ritual per slice: review loop → merge → append-dev-journal → complete the OmniFocus slice task.
- Versioning and tag hygiene: never mutate published `-gs` tags; new capability ships as the next `v5.N.0-gs.1`.
- Vocabulary: `notes/grill-allocation-lifecycle-CONTEXT.md`, `notes/grill-request-processing-CONTEXT.md`.
