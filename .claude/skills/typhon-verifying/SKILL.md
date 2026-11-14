---
name: typhon-verifying
description: Use before claiming any Typhon Launcher work is done, fixed or ready — before the final report, the commit message that says "fixes", or a push request.
---

# Checking before saying "done"

Green tests and subagent reports are not requirement fulfilment. Fulfilment is each item of the
request matching observable behaviour, with the evidence next to it.

## Checklist

1. Return to the request or plan. For each item: done / not done / how verified.
2. Every new test has a shown RED without the change and GREEN with it. For an implementer's branch
   the orchestrator reruns the package tests in the main tree after the merge; the agent's output is
   not the proof.
3. Impact map: what was touched (`grep` usages) → how checked → result. Regression of old behaviour
   gets its own line.
4. Go: `go test ./internal/<pkg>/ -count=1`, `-race` for concurrent code,
   `GOOS=windows GOARCH=amd64 go vet ./...` if platform files changed; before push `wails3 task check:local`.
   A long check is green only by its exit code as the last line (`RC=$?; …; echo "CHECK_LOCAL_EXIT=$RC"`):
   "N files passed" is not enough, since vitest also exits 1 on an unhandled error outside any test; a
   `tail` can cut the code off.
5. Frontend: `npm run check`, affected `vitest` files; for screen behaviour, a live run with
   before/after screenshots (desktop and Big Picture). Bindings regenerated if exports changed.
6. Live scenario "from click to result" for user-facing changes: status, modal, error, retry. Choose
   the build and instance per `../.agents/skills/typhon-live-check` (devmock shares state with the
   native build without `TYPHON_TEST_DATA_DIR`; never kill someone else's launcher by name).
7. Destructive paths: a "killed between X and Y" test and a manual look at the on-disk state after
   recovery.
8. Anything that cannot be checked here (Windows-only, a real game, CrossOver) is named, not hidden.

## Report

Short: result → impact map as a table → unverified → next step. No apologies or self-assessment. Do not
ask the user to check by hand what you could have checked yourself.
