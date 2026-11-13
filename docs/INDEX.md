# Launcher knowledge base — reading guide

Local reference for coding agents. Start with the task row below, then read the relevant sections and actual implementation; do not load all six documents for every task. For a small isolated edit, the owning file and its tests may be enough.

## What lives where

| Document | Owns |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Process boundaries, service wiring, events, integrations and background work |
| [CODEMAP.md](CODEMAP.md) | File and symbol lookup; package-relative paths are relative to the section's package |
| [FLOWS.md](FLOWS.md) | Ordered calls, side effects and recovery for a user operation |
| [INVARIANTS.md](INVARIANTS.md) | Required properties, implementation pointers, exceptions and regression patterns |
| [TEST_MAP.md](TEST_MAP.md) | Test selection, fixtures, platform gates and coverage limits |
| [DEBUGGING.md](DEBUGGING.md) | Commands, environment, logs, authoritative state/marker inventory and symptoms |

## Reading by task

All document names in this table are relative to `docs/`; Go package paths without `internal/` are shorthand for `internal/`. Read only the relevant invariant entries and test cases.

| Task class | Read |
|---|---|
| Update detection / "no update offered" | `FLOWS.md` §9; `INVARIANTS.md` I-8; `CODEMAP.md` → `internal/updates/resolver.go`, `order.go`, `compat.go`, `service.go::check`, `internal/sources/import.go`, `internal/library/library.go::Game`; tests `updates/resolver_test.go`, `provenance_test.go` |
| Update installation / rollback / verify / repair | `FLOWS.md` §10, §14; `INVARIANTS.md` I-4, I-5, I-10, I-14; `internal/updates/{plan,apply,verify,saves}.go`, `internal/install/fsops.go`; tests `apply_recovery_test.go`, `backup_test.go`, `degraded_test.go` |
| Downloads | `FLOWS.md` §4; `internal/download/{manager,torrent,tasks,reuse,completion,verify}.go`; `INVARIANTS.md` I-3, I-6, I-11; tests `manager_test.go`, `toctou_test.go`, `goroutine_tracking_test.go` |
| Installation / executable detection / uninstall | `FLOWS.md` §5–7, §12; `INVARIANTS.md` I-5, I-10, I-13; `internal/install/{service,flow,inspect,engine,executable,removal,elevated,worker*,broker*,runner_*}.go`; `installErrors.ts`; tests `service_test.go`, `startup_test.go`, `elevated_test.go`, `removal_test.go` |
| Moving games / library between disks | `FLOWS.md` §11; `INVARIANTS.md` I-4, I-5; `internal/relocate/{service,recover}.go`, `internal/download/repoint.go`, `internal/library/relocate.go`; tests `relocate_test.go`, `recover_test.go` |
| Game launch / sessions / playtime | `FLOWS.md` §8; `internal/library/{sessions,watch,process*,runtime_darwin}.go`, `internal/cloudsaves/launch.go`, `internal/savebackup/restore.go`, `main.go::addLaunchHooks`, `internal/procs`, `internal/wine`; tests `library/*_test.go`, `cloudsaves/launch*_test.go` |
| Sources / feeds / catalog matching / search | `FLOWS.md` §2–3; `INVARIANTS.md` I-7; `internal/sources/{service,import,releases}.go`, `sources/feed`, `internal/catalog/{match,index,service}.go`, `internal/search` |
| Metadata / covers / hero art | `FLOWS.md` §16; `internal/metadata/{service,budget,attempts,media,handler}.go`, `typhonapi/client.go`, `frontend/src/lib/stores/metadata.ts` |
| Persistence / startup crash | `FLOWS.md` §1, §14; `INVARIANTS.md` I-1–I-3; `internal/storage/doc.go`; owner's `load`/`persist`; `store_integrity_test.go` files |
| Self-update | `FLOWS.md` §13; `internal/selfupdate/{service,client,download,apply_*,worker*}.go`; `tools/devrelease` |
| Account / sync / social / telemetry | `ARCHITECTURE.md` §8–9; `internal/{account,accountsync,social,online,heartbeat,usagestats,diagnostics}`; consent rules in `internal/settings/settings.go` |
| Frontend behaviour | `ARCHITECTURE.md` §10; the store in `frontend/src/lib/stores/`, the service in `lib/services/`, the route in `src/routes/`; i18n catalogs `lib/i18n/catalog/{ru,en}` |
| Tests, CI, lint | `TEST_MAP.md`; `.github/workflows/ci.yml`; `.golangci.yml`; `.github/lint-baseline.txt` |
| Debugging a report | `DEBUGGING.md` §7 first |


## Evidence and maintenance

Initial map: 2026-09-11, `dev`, HEAD `bafa595` plus uncommitted launcher changes. This describes a working-tree snapshot, not a released version. Check current status, symbols and tests before relying on it; “untracked”, counts, timings and audit status can age independently of HEAD.

- Code and observed behaviour establish current facts; the task and applicable AGENTS instructions establish scope and policy. An invariant states a requirement, not proof that every implementation already meets it.
- `path::Symbol` is a search hint, sometimes package-relative or a method name. A matching symbol proves navigation only, not the described semantics or test coverage.
- Change the owning reference when a flow, event, schema or test gate changes. Keep other documents as summaries with pointers; do not copy the full state table or command catalogue into AGENTS.
- Preserve I-1… I-17 identifiers. When changing section numbers, update referring task rows and flow references. Prefer headings or links for new cross-references.
- Do not record routine green test runs, temporary branch status or changing test counts as permanent rules. Record durable lessons with a date and evidence in the workspace `.agents/` memory.
- These documents are tracked; change them in their own `docs:` commits. `REFACTOR_ROADMAP.md` and hand-off notes are local working files, ignored by git.

Review 2026-09-11: navigation and workflow policy revised; commands, config isolation and selected factual claims checked against source. This was a documentation review, not a complete code audit or a live platform verification.
