---
name: typhon-reviewing
description: Use when reviewing a diff, branch or package in Typhon Launcher, auditing for bugs, or judging whether a fix is real — before writing any finding.
---

# Evidence-based review

A finding = condition → actual behaviour (file:line) → consequence for the user or the data. A
hypothesis without a reachable scenario is not a finding. Style, naming and anything caught by
`golangci-lint`, `go vet` or `svelte-check` are not findings.

## Order

1. Scope from the request: diff, package, flow or whole project. Do not narrow a requested wide scope.
2. Read the scope's files in full, function by function. For each: what happens when each call fails;
   on empty, huge or corrupt input; on ctx cancellation; on repeated and concurrent calls; if the
   process dies between steps; whether the lock, subscription, file and goroutine are released on every
   path; whether I/O or `emit` runs under a mutex. Anchor: the recurring mistakes list at the end of
   `docs/INVARIANTS.md`.
3. Callers and contracts via `grep`; the scenario must be reachable from real code.
4. **Try to refute before writing**: another build (build tags are not duplicates), a check higher up
   the stack, journal/recovery, an existing test, a "why" comment (not proof, but a reason to re-check).
   Refuted → not written.
5. For a diff additionally: is there a test that fails without this change; was a test seam added to
   production; are the TS table, events and bindings updated (mistake 12); which invariants from
   `.claude/rules/go-internal.md` are touched and how that is proven.

## Severity (project scale)

CRIT — user data lost or corrupted, an operation left inconsistent, the launcher does not start, keys,
signing or privilege elevation. HIGH — wrong result or action not performed, data intact.
MEDIUM — edge case with a workaround.

## Report

- Confirmed separate from plausible; duplicates merged by cause; ordered by consequence.
- For the user: plain words, code lines and commands next to the evidence.
- The orchestrator issues the verdict; `typhon-hunter` findings are re-checked by reading the code.
  When only an audit is requested, product code is not changed.
