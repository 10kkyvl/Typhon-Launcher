---
name: typhon-implementing
description: Use when writing or changing Go or Svelte code in Typhon Launcher — features, fixes, refactors — before the first edit and again before the commit.
---

# Implementing in Typhon

## Before the first edit

- `git status` and branch; leave other people's uncommitted changes alone.
- Read the owning file and its test in full, grep usages, open the `docs/INDEX.md` row for the task.
  Invariants: `.claude/rules/go-internal.md` or `frontend.md`.
- A behavioural change starts with the test (skill `typhon-testing`); it must fail. A pure move has no
  test: the proof is "zero behaviour diff" (package tests green).
- A split that the change needs (file, function, component) is its own behaviour-free commit made
  first.

## While editing

- One commit = one coherent part. Pure moves and behaviour changes never share a commit.
- State mutation pattern: snapshot → mutate → persist → on failure roll memory back and return the
  error (background exceptions carry a comment). Disk writes, `emit` and callbacks run outside the
  mutex unless the lock is needed for check-and-act.
- New goroutines only through the package helper (`spawnTrackedLocked` and its equivalents) with the
  service ctx.
- Errors: `uierr` code as a constant plus `%w`; "unknown" is an explicit flag, not a zero value. A new
  code goes into the TS table in the same change.
- New event or export → constant, `RegisterEvent`, bindings, TS type and subscription (frontend rule).
  Check `bindingsContract.test.ts`.
- Nearby conventions beat personal habits. Comments only where the intent is not visible in the code:
  one per non-obvious decision, none per function or block, no change notes.
- No test seam in a production struct for the convenience of a test; if there is no other way, say so
  in the report.

## Before the commit

- `go test ./internal/<pkg>/ -count=1` (plus `-race` for concurrent code); frontend: `npm run check`
  and the affected `vitest` files. Destructive paths get a "killed between X and Y" test.
- Show RED/GREEN for the new test (revert → fails → restore → passes).
- Compare with the plan or request: each item done, how verified. Then `typhon-verifying`.
- Commit message per `CONTRIBUTING.md`: `type(scope): subject`, lower case, imperative, English, body says
  why; `git commit -s`; `Co-Authored-By` per the session rules.
