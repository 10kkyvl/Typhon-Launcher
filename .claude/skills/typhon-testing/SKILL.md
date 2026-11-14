---
name: typhon-testing
description: Use when writing, changing or judging tests in Typhon Launcher (Go `_test.go` or vitest), including deciding whether an existing test proves a fix.
---

# Tests in Typhon: from behaviour, not from code

A test states what the user or a neighbouring component must observe and fails when it is absent. A
test that mirrors the implementation passes with any implementation and protects nothing.

## The RED rule

A new or changed test must **fail without the code change**. Two equal routes: the test is written
before the fix and run (RED), then the fix lands (GREEN); or the fix already exists, so revert it
temporarily (`git stash` or undo the line), run, see a failure with a meaningful message, restore, run
again. Both runs, with commands and output, go into the report. A test that never failed is deleted
or rewritten.

## Where a test comes from

1. The task scenario: "the user did X in state Y → sees Z". For a bug fix, the reported scenario
   including the component boundary (Go ↔ binding ↔ store ↔ screen), not a similar function.
2. An invariant (`docs/INVARIANTS.md`): for each touched I-number, its canonical test — corrupt file →
   startup fails; persist failure → memory rolled back; "process killed between steps X and Y" →
   recovery at startup; cancellation → status `cancelled`, data intact.
3. Failure paths: missing file, no permission, corrupt JSON, truncated write, symlink, empty input,
   cancelled ctx, repeated and concurrent calls. Pick the applicable ones, not all of them.

## How to write

- Assert observable things: return value, `uierr` code, event and payload, bytes of the state file,
  state after restarting the service on the same directory (`NewServiceAt(dir)`).
- Do not assert: private fields, order of internal calls, message text instead of a code, mock call
  counts where the result is what matters. Do not lock a struct's mutex or write its private fields;
  use the public API and the package test helpers, and add a helper to the helpers file if one is
  missing.
- Do not mock the thing under test. Stubs only for OS boundaries (processes, CrossOver, network, time
  via the injectable `clock`/`newTicker`) and existing seams. A new production seam is a separate
  decision.
- Errors in tests are handled (`t.Fatal`/`require`); `_ =` is forbidden. `time.Sleep` is forbidden:
  synchronize with channels, `waitFor` with a deadline, injected time.
- Isolation: `t.TempDir()`, stubbed `prepareRuntime`/`releaseRuntime` (otherwise real bottles),
  test-only LAN ports. Never real user data.
- Frontend: stores and services are tested with vitest by behaviour (event arrived → state changed);
  interactions go in a `*.dom.test.ts` (see `.claude/rules/frontend.md`); an SSR snapshot proves
  markup only.
- Vitest traps: `beforeEach(() => mock.mockReset())` returns the mock, which vitest runs as a teardown
  (a rejecting mock then fails the test after its body): use a block body.
  `vi.mock('…/services/library', importOriginal)` leaves `stores/library` on the real service, because
  `services/library → stores/cloudsaves → stores/library` is an import cycle: import `stores/library`
  first.

## When not to write one

Pure code moves, prose or comment edits, configuration without behaviour. The proof there is green
existing tests and a `git diff` with no semantic change.

## Independent author

Orchestrator rule (not for the executing subagent): when you wrote the fix yourself or the scenario is
large, order the test from `typhon-test-author` (read-only). It gets the scenario, the public
contract and the package path, never the implementation diff, and returns the test text; the
orchestrator writes the file and does RED/GREEN. If the agent is not available, write the test yourself
by this skill.
