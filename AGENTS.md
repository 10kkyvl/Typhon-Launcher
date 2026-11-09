# Typhon Launcher — shared agent instructions

Go / Wails v3 desktop launcher with a Svelte 5 / Vite / TypeScript frontend. Windows is the primary
target; native macOS runs games through CrossOver; the `devmock` build tag replaces platform subsystems
for development. `main.go` constructs the services and wires their callbacks; the UI talks to Go through
generated bindings and Wails events. Dependency versions: `go.mod`, `frontend/package.json`.

This file holds what every agent must know regardless of tooling: product shape, where to read,
invariants, verification commands, commit policy. Tool-specific process lives elsewhere: Claude Code in
`CLAUDE.md` and `.claude/`; Codex in `../.agents/CODEX.md`. Neither overrides this file.

## Working agreement

- Follow the workspace [AGENTS.md](../AGENTS.md) and the user's current task. Talk to the user in
  Russian; GitHub texts are English; UI strings go through ru/en i18n.
- Check the branch and `git status` before editing. Preserve unrelated and uncommitted work. Local
  edits, commit, push, release are distinct actions; use only the authorization given for the task.
- No unrelated refactors, no placeholder code. Dependencies only with explicit user authorization.
  Follow nearby conventions.
- Comments are few and earn their place: one explains an intent, trade-off or invariant the code
  cannot show. Do not comment every function, block or test, restate the code, narrate steps, or
  leave change notes ("added", "fixed", "now") in the code; that history belongs in the commit message.
- A file or function split is a separate, behaviour-free commit made before the change that needs it,
  never folded into a fix or feature commit.
- Product bugs are not fixed as part of changing agent instructions. User release data and the dev
  environment stay separate during diagnosis.

## Find the relevant context

[docs/INDEX.md](docs/INDEX.md) maps task classes to documents. Read the relevant flow, invariant and
owning code/tests, not the whole knowledge base. The documents describe a dated snapshot; verify
claims against current code.

- Behaviour across services: [FLOWS](docs/FLOWS.md), then [CODEMAP](docs/CODEMAP.md);
  callbacks/events: [ARCHITECTURE](docs/ARCHITECTURE.md).
- State, filesystem, cancellation, identity or concurrency: the relevant entries in
  [INVARIANTS](docs/INVARIANTS.md) (I-1…I-17 and the list of recurring agent mistakes).
- Test selection: [TEST_MAP](docs/TEST_MAP.md). Logs, state locations, commands: [DEBUGGING](docs/DEBUGGING.md).
- Refactor queue: `docs/REFACTOR_ROADMAP.md` (local working file, ignored by git; ask the user when it is missing).
- Workspace skills in `../.agents/skills/`: `typhon-workflow`, `typhon-live-check`, `typhon-release`,
  `typhon-lint-baseline`, `project-review`. Use the one that matches the task.

## Guardrails

- Persist state through `storage.WriteAtomic`/`Save`. Distinguish missing state from unreadable or
  corrupt state; check both the load and the later save path. A failed persist must not leave a
  memory-only mutation (I-1–I-3; documented exceptions require reading the owner).
- Before touching a destructive operation, identify ownership, the real payload layout, the journal and
  the startup recovery. Verify copied content by hash before deleting its source; an error-path
  rollback alone does not cover a killed process (I-4, I-5, I-10).
- Installed release provenance is `SourceID + DistributionID`; legacy migration needs the exact saved
  release identity. A similar title, repacker or version is not the same distribution (I-8).
- Keep check-and-act under one lock; do expensive work outside it. Track goroutines, cancellation and
  shutdown ownership. Game processes outlive the launcher; installer directories stay if the process
  tree is not confirmed stopped (I-6, I-11–I-13).
- Preserve error causes and carry user-facing codes, model fields and events through
  Go → bindings → TS store → UI. Validate external paths and network input at the trust boundary (I-7, I-15).
- Cross-service coupling goes through callbacks wired in `main.go`, not imports (ARCHITECTURE §2):
  do not add a `download` → `install` import and do not widen the existing `download` → `library` edge
  beyond `playthrottle.go`. `devmock` must not compile for Windows or with `production` (I-16).

## Verification

- Before pushing launcher code: `wails3 task check:local` on macOS (lint, Windows cross-build, race
  tests, frontend, packaging). Hosted CI runs Windows execution and frontend checks; full remote checks
  are available through the CI workflow's `full_checks` input. Linux lint needs a real Linux environment.
- A behavioural fix comes with a regression test that fails without the fix and covers the failure path
  and the affected component boundary. No tests for prose, pure moves or trivial reversible edits.
- UI behaviour is checked as the user journey in the running app when feasible: status, modal, loading,
  error, retry. Browser mocks and green unit tests do not prove Wails integration. State unavailable
  checks explicitly.
- Tests use temporary state and stubbed runtime hooks; native macOS tests otherwise create real
  CrossOver bottles. `devmock` alone does not isolate files or backend traffic (see DEBUGGING). Windows
  cross-builds prove compilation only; set `GOARCH=amd64` when checking the shipped Windows binary from
  Apple Silicon. Run `go generate ./internal/installguard/asset` after changing the embedded helper sources.
- Before reporting completion, compare the result with every requested requirement, inspect the final
  diff, and check the full affected user scenario scaled to the task. Separate established cause,
  assumption and unverified behaviour. Do not use real user data as fixtures.

## Commits

Commit locally after each completed and verified logical part; one commit = one coherent feature, fix or
stage, with its tests and product docs. Split accumulated changes by meaning, by hunks if needed, keeping
intermediate states working. Commit messages follow `CONTRIBUTING.md`: English, `type: subject` or
`type(scope): subject` (types in use: feat, fix, perf, refactor, test, docs, chore, ci, build, legal,
release), lower case, imperative, no trailing period, body wrapped at 72 columns explaining why; sign off
with `git commit -s`. Branches come off `dev`; pull requests target `dev`. Push, merge,
tags, releases and rewriting published history need authorization in the current task. Instructions
and the knowledge base (`CLAUDE.md`, `AGENTS.md`, `.claude/`, `docs/`) are tracked and change in their
own `docs:` commits, never folded into a product commit. `docs/REFACTOR_ROADMAP.md` and hand-off
notes stay local (ignored); the workspace `.agents/` lives outside this repository.
