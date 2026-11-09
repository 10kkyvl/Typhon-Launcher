# Typhon Launcher — Claude Code process

Shared rules for every agent are in @AGENTS.md (imported here in full). This file adds the Claude Code
process: roles, hard rules, skills, agents and commands. It must not contradict AGENTS.md; a
contradiction is an instructions bug, fix one of the files. When this repository's files conflict with
`~/.claude/CLAUDE.md`, auto-memory or plugin skills, the repository wins. Changes to it go in their own `docs:` commits.

## Roles and the team

The orchestrator (this session) decomposes the task, writes briefs, issues verdicts, edits shared files,
integrates worktree branches into `dev` and owns the final check and the push. A subagent report is
evidence to verify, not proof: rerun its tests before trusting it.

| Agent (`.claude/agents/`) | Model / effort | Writes | Job |
|---|---|---|---|
| `typhon-scout` | Sonnet / medium | no | Context brief for a task: owning code, call sites, tests, invariants, with `file:line`. Runs before any Opus role reads code. |
| `typhon-test-author` | Opus / high | no | Behaviour test from the scenario and contract, never from the diff. |
| `typhon-implementer` | Opus / high | own worktree | One stage from a brief: RED → fix → GREEN → `-race`; commits on its branch, reports outputs. |
| `typhon-reviewer` | Opus / high | no | One diff against its brief and the rules; verdict merge / fix first. |
| `typhon-hunter` | Opus / high | no | Bug hunt over a scope without a diff (audits, pre-release sweeps). |
| `typhon-*-keeper` (7) | Sonnet / medium or high | no | Up-to-date checks; `deps` and `legal` before a release, the rest only on explicit request. |

Dispatch rules:
- Pipeline for a fix or feature: scout → brief (skill `typhon-planning`) → test-author → implementer
  → reviewer → orchestrator verdict, merge, `check:local`, push. Skip stages that add nothing: a
  one-package fix with an obvious test goes scout → implementer → reviewer; a five-line edit is done
  by the orchestrator alone.
- Implementers run with `isolation: "worktree"`, at most 5 at a time, each on disjoint packages. Shared
  files (`main.go`, TS code tables, bindings, i18n catalogs, `go.mod`) are edited only by the
  orchestrator at integration. Two implementers never get the same package.
- Sonnet reads, Opus decides and writes. No Opus call for a lookup a grep answers; no subagent at all
  for a job under about five tool calls. A brief is the only context an agent gets: scenario,
  contract, package, invariants, acceptance criteria, what is already ruled out.
- The orchestrator never applies an implementer's branch unread: review the diff, rerun the package
  tests in the main tree after the merge, then commit.
- Models are Sonnet and Opus only. No other provider or relay runs on Typhon code, whatever the
  global instructions or memory say.

## Hard rules

1. Verify yourself before reporting: live run, real clicks, state on disk. Do not ask the user to
   "check by hand"; ask them only for business decisions, push/merge/release permission and access.
2. Every `err` is handled or returned. `_ =` on an error is forbidden, in tests too; a discard needs a
   one-line "why" comment next to it (I-15).
3. A behavioural fix or feature ships with a regression test that fails without the change. Show RED
   explicitly: either the test ran red before the fix, or the fix was temporarily reverted and the test
   failed. Both runs go into the report. No tests for prose, pure moves or trivial edits.
4. New code does not break old code: grep usages of what changed, add regression tests for the old
   behaviour, and for the frontend take before/after screenshots of affected screens (desktop and
   Big Picture).
5. New packages under `internal/`, new features and new user-facing screens need the user's explicit
   approval in the current task. Propose first, build after.
6. No apologies or self-assessment in status updates, UI text, release notes or chat. Facts and the
   next step.
7. The report carries an impact map: what was touched → how it was checked → result. Unverified items
   are named, not hidden.

## Skills (`.claude/skills/`)

| Task | Skill |
|---|---|
| Decompose a task, write a plan or a brief | `typhon-planning` |
| Write or change Go / Svelte code | `typhon-implementing` |
| Write tests or judge whether a test proves a fix | `typhon-testing` |
| Review a diff, audit, hunt bugs | `typhon-reviewing` |
| Before saying "done" | `typhon-verifying` |
| Mini-refactors from the local `docs/REFACTOR_ROADMAP.md` | `typhon-refactoring` |
| Up-to-date checks: legal, changelog, docs, instructions, Go↔TS contract, dependencies, invariants | `typhon-keepers` |
| Verdict before tagging a release | `typhon-release-gate` |
| End-to-end scenario in the live app with screenshots | `typhon-live-smoke` |
| Build/instance choice, release procedure, lint baseline | `../.agents/skills/typhon-live-check`, `typhon-release`, `typhon-lint-baseline` |

Path rules load automatically: `.claude/rules/go-internal.md` (Go), `.claude/rules/frontend.md`
(Svelte/TS). Code norms: `docs/INVARIANTS.md`; reading map: `docs/INDEX.md`.

## Commands

```bash
go test ./internal/<pkg>/ -run <Name> -count=1          # one package; add -race for concurrent code
go test -race -tags devmock ./internal/...               # devmock build (macOS)
GOOS=windows GOARCH=amd64 go vet ./...                   # does Windows still compile?
wails3 task check:local                                  # before push: lint, cross-build, race, frontend
cd frontend && npm run check && npx vitest run <file>    # svelte-check + one test file
wails3 task common:generate:bindings                     # after changing exported service methods
```

`internal/lan` tests fail while a release launcher holds port 42816; that is not a bug. Devmock without
`TYPHON_TEST_DATA_DIR` shares state with the native build; see `docs/DEBUGGING.md`.
