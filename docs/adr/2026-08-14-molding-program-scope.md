# TURN Fork Molding Program — Scope

**Status:** Grilled and resolved 2026-08-14. Decisions D1–D5 are settled below; ready for `architecture-handoff` packaging.
**Date:** 2026-08-14 (drafted and grilled same day)
**Baseline:** `main` @ `cca6b57` (payload = pion/turn#585 head `c7dcea7` + `4b818fc` transaction-wait abort + `/v5` module rename), published as tag `v5.0.13-gs.1` (at `e8db91a`, same payload).
**Context:** `the-sarge/turn` is a permanent fork of `pion/turn`, per the fork-pivot amendment in wiremux's `docs/adr/2026-08-09-turn-carrier-prerequisites-plan.md` (GridSwarm/wiremux#1161). This program molds the fork into exactly what wiremux needs: cut unused surface, modernize, refactor, and optimize for wiremux's consumption alone.

## Identity

The fork is an **owned library** — wiremux's TURN client, full stop. Upstream is a read-only reference, never a compatibility target. See [the owned-library ADR](2026-08-14-owned-library-fork.md) for rationale and consequences (security posture, versioning, transitional fixture seam).

## Consumer contract

Wiremux's sole consumer is the Slice 1.3 allocator/composite: root `Client` performing UDP allocation over a caller-owned `net.PacketConn`, `Client.PrepareUDPPeer` deterministic readiness, the lifetime ChannelData-only write invariant, waiter-local cancellation, and joined Pion-worker allocation close after socket-owner unblock. Wiremux does not consume the TURN server, TCP/TLS transports, ICE, or credential-generation helpers.

## Resolved decisions

- **D1 — Server cut with transitional upstream fixture.** Delete the server half entirely. Verified: `internal/client` tests (all PR-585 work) and `close_latency_test.go` are mock-based; only root `client_test.go` (8 `NewServer` uses) and `e2e/` touch the in-repo server. Root integration tests keep their server fixture by pinning upstream `github.com/pion/turn/v5@v5.0.12` as a **test-only** dependency — no import collision thanks to the module rename. This seam is transitional: exit criterion is fork-owned receipt coverage (wiremux Slice 1.3 receipts plus any minimal fork fixture), after which the upstream test dependency is dropped.
- **D2 — Security posture: watch proto-only.** Watch upstream `pion/turn` releases solely for `internal/proto` (wire-parsing) changes and port those; ignore upstream client-side fixes — the fork's client had already diverged before the pivot. Dependency CVEs (`pion/stun`, `pion/transport`, `x/crypto`) flow through normal dependency updates.
- **D3 — Versioning: `v5.N.0-gs.1` minors.** M0 tags `v5.1.0-gs.1`; each milestone bumps the minor; the `-gs` suffix is permanent so Go's version selection never auto-upgrades the consumer — every wiremux bump is deliberate.
- **D4 — CI: portfolio standard plus proto fuzz.** Replace all ten inherited pion reusable-workflow configs by running the `standardize-github-ci` audit/plan/implement flow against the fork inside M0, with one fork-specific addition: retain a fuzz job over `internal/proto`, the complement of D2's proto-focused security bet.
- **D5 — Pre-existing macOS failures: dissolved by D1.** `TestCreateTCPConnectionInvalid` (`internal/allocation`) and `TestConnectRequest` (`internal/server`) are deleted with their packages.

## Sequencing

Wiremux Slice 1.3 adopts `v5.0.13-gs.1` **now** — its adoption audit covers the pre-cut tree (the post-M0 re-pin is a cheap deletion-heavy re-audit), and its consumer experience is the required input for M1. M0 follows adoption kickoff; M1 does not start until Slice 1.3 merges.

## Cut and keep lists (M0)

**Delete:** `server.go`, `server_config.go`, `relay_address_generator_range.go`, `relay_address_generator_static.go`, `relay_address_generator_none.go`, `lt_cred.go` (no non-example consumer), root `stun_conn.go` (consumed only by server and TCP/TLS examples), `internal/server`, `internal/allocation`, `internal/auth`, `internal/client/tcp_alloc.go`, `internal/client/tcp_conn.go` plus the TCP-allocation entry points in root `client.go`, all of `examples/`, all of `e2e/`, and the ten inherited `.github/workflows` configs (replaced per D4).

**Keep:** root `client.go` (UDP paths) and `errors.go`; `internal/client` UDP path (`allocation.go`, `binding.go`, `client.go`, `errors.go`, `periodic_timer.go`, `permission.go`, `transaction.go`, `trylock.go`, `udp_conn.go`); `internal/proto` wholesale (trimming deferred past M0); `internal/ipnet`. Approximately 6,600 LOC.

**Dependency re-audit at cut time:** expect `golang.org/x/time` and parts of `pion/transport/v4` to become removable; `stretchr/testify` stays; upstream `pion/turn/v5` enters as test-only.

## Program shape

1. **Slice M0 — Cut and stabilize:** execute the cut and keep lists, land portfolio CI with proto fuzz, green gate on the kept surface, tag `v5.1.0-gs.1`. No behavior change to kept code.
2. **Slice M1 — Modernize the kept API** for the one consumer (context-first cancellation is already half-arrived via `PrepareUDPPeer`; extend rather than keep two idioms). Gated on Slice 1.3 merging — its adoption experience defines M1's content.
3. **Slice M2 — Optimize the packet path** (read pump, ChannelData encode/decode in `internal/proto`). Gated on profiles from real wiremux traffic; no speculative optimization.

## Tracking

The program is packaged through `architecture-handoff`: program issues live in `the-sarge/turn` (wiremux issues remain consumer-side only), mirrored to OmniFocus per that workflow.

## Grill record

Grilled 2026-08-14 (fork-identity, D1–D5, sequencing all put to the owner; facts verified against the tree at `cca6b57`). Superseded draft framing: the original draft treated the fixture problem as gating "the size of the entire cut" — verification showed the mock-based test coverage already carries the PR-585 surface, shrinking D1 to the root integration tests and wiremux receipts only.
