# Cut and Stabilize (M0) Implementation Plan

**Date:** 2026-08-14
**Status:** Accepted; not yet implemented
**Track:** 1 of 3 in the 2026-08-14 TURN fork molding program
**Depends on:** nothing — safe to start first
**Related:** [Molding program scope](2026-08-14-molding-program-scope.md) (grilled decisions D1–D5), [Owned-library ADR](2026-08-14-owned-library-fork.md), wiremux fork-pivot amendment (GridSwarm/wiremux#1161)
**Normative scope:** Current outcome, boundaries, invariants, acceptance evidence, blockers, and stop conditions
**Audit history:** Grill record in the scope doc; pre-pivot upstream history in pion/turn#585 and GridSwarm/wiremux#1054

## Goal

Reduce `the-sarge/turn` to the kept surface wiremux consumes — root `Client` UDP allocation plus its internal support — under a fork-owned CI gate, tagged `v5.1.0-gs.1`. The module becomes a single-purpose TURN client library whose entire tree is surface one consumer actually exercises.

## Current Shape (verified 2026-08-14 at `44cecae`)

- Root package mixes client and server: `client.go` (UDP client, `PrepareUDPPeer` at `client.go:593`) plus server surface — `server.go`, `server_config.go`, three `relay_address_generator_*.go` files, `lt_cred.go`, `stun_conn.go`.
- Client TCP allocation surface: `client.go:81` (`tcpAllocation` field), `client.go:117`, `client.go:528-568` (`AllocateTCP`), `client.go:578`, and `internal/client/tcp_alloc.go`, `internal/client/tcp_conn.go`.
- `lt_cred.go` has no non-example consumer; root `stun_conn.go` (`NewSTUNConn`) is consumed only by `server.go`, `internal/server/turn.go`, and TCP/TLS client examples.
- Only root `client_test.go` (8 `NewServer` uses) and `e2e/` (coturn Docker fixture, `e2e/Dockerfile:6`) depend on a TURN server; all `internal/client` tests and `close_latency_test.go` are mock-based.
- CI is ten auto-synced pion configs (`.github/workflows/*.yaml`, marked "DO NOT EDIT … copied from pion/.goassets") calling `pion/.goassets` reusable workflows: api, codeql, e2e, fuzz, lint, release, renovate-go-sum-fix, reuse, test, tidy-check.
- Fuzz targets exist in kept surface: `internal/proto/fuzz_test.go:34,94,127` (`FuzzSetters`, `FuzzChannelData`, `FuzzIsChannelData`).
- Direct dependencies: `pion/logging`, `pion/randutil`, `pion/stun/v3`, `pion/transport/v4`, `testify`, `x/sys`, `x/time`. Non-example consumers of `randutil` are all cut surface (`relay_address_generator_range.go`, `internal/server/turn.go`, `internal/allocation/allocation_manager.go`); `x/time` and direct `x/sys` have example-only consumers.
- Known pre-existing macOS-local failures: `TestCreateTCPConnectionInvalid` (`internal/allocation`), `TestConnectRequest` (`internal/server`) — both in cut surface; both pass in upstream Linux CI at base `b374908`.

## Decision

Grilled and settled in the [scope doc](2026-08-14-molding-program-scope.md); binding facts for this track:

- The fork is an owned library (ADR); API breakage is intended and versioned as `v5.1.0-gs.1` (permanent `-gs` pre-release suffix keeps consumer bumps deliberate).
- D1: delete the server half; root integration tests keep a real-server fixture by pinning upstream `github.com/pion/turn/v5@v5.0.12` **test-only** (collision-free due to the module rename). Transitional seam; exit = fork-owned receipt coverage (Track 2 era).
- D4: replace all inherited pion CI with the portfolio standard via the `standardize-github-ci` flow, retaining a fuzz job over `internal/proto` (complement of D2's proto-only security watch). This plan records operator approval for that flow's implementation phase, scoped to this repository.
- CI replacement precedes the cut: the inherited `api` (API-compat) and `e2e` workflows turn red under the cut, so cutting first leaves the default branch gated by failing checks.

**Rejected alternative (do not do this):** cutting under inherited CI and patching pion's auto-synced configs in place — they are overwritten by upstream sync tooling and keep the fork coupled to `pion/.goassets`. Also rejected: trimming `internal/proto` in this track (kept wholesale; trimming deferred past M0) and splitting the cut into per-package PRs (deletions are one coherent behavior and fit one context; per-package PRs multiply review cycles with no independent value).

**Non-goals:** API reshaping (Track 2, gated on wiremux Slice 1.3 merge); packet-path optimization (Track 3, gated on production profiles); `internal/proto` trimming; replacing the transitional upstream fixture; any behavior change to kept code.

## Slice Graph

| Slice | Status/disposition | Delivers | Blocked by | Removes temporary seam |
|---|---|---|---|---|
| 1 | new | Fork-owned portfolio CI gate (+ proto fuzz) green on the full pre-cut tree | None | Coupling to pion/.goassets auto-synced CI |
| 2 | new | Kept-surface-only tree, upstream test fixture pinned, deps re-audited, tag `v5.1.0-gs.1` | Slice 1 | n/a (introduces documented transitional fixture seam) |

## Implementation Slices

### Slice 1 — Replace inherited CI with the fork-owned portfolio gate

**What it delivers:** All ten inherited pion workflow configs removed; a fork-owned CI gate per the portfolio standard (`standardize-github-ci` flow, implementation approved by this plan) covering build, format, lint/vet, race tests, and dependency hygiene; plus a fuzz job running the three existing `internal/proto` fuzz targets with a bounded per-target fuzztime. All jobs green on the current full (pre-cut) tree. Dependency-update automation (Renovate) remains functional — D2's security posture depends on that flow.

**Existing-work disposition:** new slice. The inherited configs are auto-synced upstream copies, not fork work; none is retained.

**Blocked by:** none.

**Single owner after merge:** the fork's `.github/workflows` tree, owned by this repository (no external sync source). No durable runtime fact changes ownership.

**Authority completeness:** n/a — no persisted runtime fact becomes authoritative.

**Transitional-seam budget:** none introduced. Removes the pion/.goassets coupling seam.

**Blast radius:** `.github/` only; no Go code changes. Traced effects: branch-protection/required-check names change with the workflow set (update rulesets in the same slice if present); the fuzz schedule consumes Actions minutes (bound it); CodeQL/reuse/release job dispositions follow the portfolio policy — record each kept/dropped job in the PR description. Untraced effects: none identified.

**Artifact classification:** all artifacts are verification aids or process metadata. The CI gate is an approved maintained deliverable: payoff = merge gating for every subsequent slice; domain = this repository's Go tree; owner = fork maintainer; retirement = none while the repo lives. The proto fuzz job is part of that maintained gate (D4 approval in the scope doc).

**Representation contract:** supported domain = this repository's workflow set and Go tree as of the plan commit; owner = the workflow files themselves; guarantee = example-level (observed green runs), which is the correct level for CI plumbing.

**Contract closure:** not triggered — verification-aid replacement with no runtime invariant; single reachable path (Actions trigger).

**Evidence budget:** one green run per job on the slice PR head; one observed red on a deliberately failing check (any single job) to prove the gate actually gates, reverted before merge or shown on a scratch branch; fuzz targets run with bounded fuzztime (minutes, not hours). Terminating rule: all required checks green on the PR head plus the one observed-red demonstration. One review; at most one replacement review.

**TDD and preservation evidence:** characterization first — enumerate the inherited jobs and their dispositions (kept-equivalent/dropped/replaced) in the PR before deleting configs; the observed-red demonstration is the failing-test analogue for a CI gate. Preservation: `task`/test invocations unchanged for developers (no Go changes).

**Dispatch context budget:** this slice contract, the scope doc's D4 paragraph, the `.github/workflows` tree (10 small YAML files), and the `standardize-github-ci` skill flow. No Go source context needed. Implementation plus review fits one fresh context comfortably.

**Slice decision audit:** strongest further split — separate "remove inherited" from "add portfolio gate"; rejected because the intermediate state (no CI at all) gates nothing and both halves are small. Strongest merge — fold into the cut slice; rejected because the cut depends on this gate being green first (see edge evidence) and merging makes the observed-red demonstration entangle with mass deletions. Blocking-edge evidence: none inbound.

**Stop conditions:** the portfolio standard proves inapplicable to a Go library repo in some structural way; required checks cannot be made green on the pre-cut tree without code changes (this slice may not change Go code); Renovate/dependency automation cannot be preserved.

### Slice 2 — Cut to the kept surface and tag v5.1.0-gs.1

**What it delivers:** One PR deleting all cut surface: `server.go`, `server_config.go`, `relay_address_generator_range.go`, `relay_address_generator_static.go`, `relay_address_generator_none.go`, `lt_cred.go`, root `stun_conn.go`, `internal/server`, `internal/allocation`, `internal/auth`, `internal/client/tcp_alloc.go`, `internal/client/tcp_conn.go`, the TCP surface in `client.go` (`:81`, `:117`, `:528-568`, `:578`), `examples/`, `e2e/`. Root `client_test.go` re-pins its server fixture to upstream `github.com/pion/turn/v5@v5.0.12` as a test-only dependency. `go.mod` re-audited: `randutil`, `x/time`, and direct `x/sys` expected removable; `logging`, `stun/v3`, `transport/v4`, `testify` remain; upstream `pion/turn/v5` enters test-only. README rewritten to describe the owned library (one short pass; no docs program). After merge and green default-branch CI: annotated tag `v5.1.0-gs.1`.

**Existing-work disposition:** new slice. The two macOS-failing tests are deleted with their packages (settled D5) — record their deletion in the PR body so the disposition is auditable.

**Blocked by:** Slice 1.

**Single owner after merge:** unchanged — no durable fact or lifecycle transition changes owner; deletions only. The public module API's single owner becomes root `Client` (UDP path) alone.

**Authority completeness:** n/a — no fact becomes newly authoritative.

**Transitional-seam budget:** one seam introduced and documented: upstream `pion/turn/v5` as test-only integration fixture. Coherent because it is a verification aid outside shipped behavior, pinned to an exact upstream tag, and collision-free by module path. Removal owner: the Track 2 "fixture replacement" slice, gated on fork-owned receipt coverage (wiremux Slice 1.3 receipts); this program's overview carries that edge. No other duplicate representation, generic mutation path, or double-open lifetime remains.

**Blast radius:** public API (intended break, versioned `v5.1.0-gs.1`, sole consumer pins deliberately — wiremux currently pins `v5.0.13-gs.1` and is unaffected until it deliberately bumps); `go.mod`/`go.sum` (dependency removals plus the test-only addition — license/vuln posture improves monotonically except the upstream test dep, which is MIT and test-scoped); CI (Slice 1 gate must stay green; fuzz targets are kept surface and unaffected); developer docs (README). Concurrency/ordering, persistence, and failure-mode surfaces: untouched — kept code is byte-identical except the `client.go` TCP-surface deletion. Untraced effects: none identified; the `client.go` edit is the only kept-file modification and its TCP members have no UDP-path callers (verified by the consumer greps in Current Shape).

**Artifact classification:** deletions are shipped-behavior removal (intended); the rewired `client_test.go` and the upstream fixture dependency are verification aids (the fixture is the documented transitional seam, maintained until its removal slice); the tag is process metadata; README is process/traceability documentation.

**Representation contract:** supported domain = the kept surface enumerated above at the plan commit; representation owner = the Go compiler and module graph (`go build ./...`, `go vet ./...` prove no dangling references — a finite, mechanical check); guarantee = universal over the enumerated cut/keep lists (finite file sets), example-level for "wiremux needs nothing else" (owned-library ADR accepts this; Track 2 sharpens it with real adoption feedback).

**Contract closure:** not triggered — no invariant with multiple independently reachable paths is created; the material consequence (API break) is a single intended, versioned event with one consumer. Evidence: the criteria below are compiler-verifiable or single-path.

**Evidence budget:** existing test suite green (mock-based `internal/client` suite, `close_latency_test.go`, rewired `client_test.go` against the upstream fixture) with `-race`; `go build ./...` + `go vet ./...` on the kept tree; one negative build check that a representative deleted symbol (`turn.NewServer`) no longer resolves; `go mod tidy` diff reviewed against the expected dependency dispositions. No new tests beyond the fixture rewire. Terminating rule: green CI on PR head plus the criteria checklist. One review; at most one replacement review.

**TDD and preservation evidence:** the fixture rewire lands first within the PR (rewired `client_test.go` green against upstream `v5.0.12` before deletions), so integration coverage never goes dark; deletions follow with the suite kept green at each logical step. Preservation gate: kept-surface tests are unmodified except the fixture import — any other kept-test edit is a stop signal.

**Dispatch context budget:** this slice contract, the cut/keep lists (which duplicate the scope doc's — this plan is normative), `client.go` (~900 lines), `client_test.go`, `go.mod`, and the fixture-rewire pattern. The deletions themselves need no reading beyond confirming the file lists. Fits one fresh context; the only judgment call is the `client.go` TCP-surface excision, which is small and anchored.

**Slice decision audit:** strongest further split — separate "fixture rewire" PR before a "deletions" PR; rejected because the rewire alone has no independent value (the in-repo server still exists) and the two-PR version doubles review for a single coherent behavior. Strongest merge — fold Slice 1 in; rejected per Slice 1's audit. Blocking-edge evidence: under inherited CI the cut turns `api` (API-compat) and `e2e` workflows red, so the edge to Slice 1 is genuine, not convenience.

**Stop conditions:** any kept-surface test requires a semantic change to stay green (indicates a hidden kept→cut dependency); the upstream `v5.0.12` fixture cannot express what the in-repo server fixture provided for an existing root test (indicates D1's fixture decision needs re-grilling); `go mod tidy` retains a cut-only dependency (untraced consumer); the `client.go` TCP excision touches any UDP-path code path.

## Acceptance Criteria

- [ ] `go build ./...` and the full `-race` suite are green on a tree containing only the kept surface (enumerated file list above — finite domain, compiler-owned, universal).
- [ ] `turn.NewServer`, `turn.ServerConfig`, and `Client.AllocateTCP` do not resolve (negative criterion; compiler-owned).
- [ ] Root integration tests run against upstream `pion/turn/v5@v5.0.12` test-only; the dependency does not appear in any non-test import (finite import scan).
- [ ] CI is fork-owned: no workflow references `pion/.goassets`; proto fuzz job present and green (finite workflow-file scan).
- [ ] Tag `v5.1.0-gs.1` exists on the merged default-branch head and `go get github.com/the-sarge/turn/v5@v5.1.0-gs.1` resolves.
- [ ] Wiremux's pinned `v5.0.13-gs.1` build is unaffected (no retro-tag mutation; old tag untouched).

## Validation Gates

Slice 1: all portfolio-gate jobs green on PR head; one observed-red demonstration. Slice 2: `go build ./...`, `go vet ./...`, full suite with `-race`, `go mod tidy` clean, fork CI green on PR head; tag resolution check post-merge. No macOS gate (known-local failures are cut surface; CI is Linux per portfolio standard).

## Operating Discipline

Follow the shared review-loop and contract-closure baselines supplied by `$implement-architecture-slice` for every slice/PR; this repository has no overlay docs, so the shared baselines govern directly. Track-specific stops: do not modify kept-surface behavior; do not trim `internal/proto`; do not touch tag `v5.0.13-gs.1`; vocabulary per `notes/grill-allocation-lifecycle-CONTEXT.md` and `notes/grill-request-processing-CONTEXT.md` (allocation, five-tuple, permission, channel binding — never "session"/"connection ID").
