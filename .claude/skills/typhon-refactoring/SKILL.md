---
name: typhon-refactoring
description: Use when doing a mini-refactor in Typhon Launcher — splitting a file, extracting a shared helper, narrowing an interface, moving I/O out of a lock — or when picking the next item from the refactor roadmap.
---

# Mini-refactors

The queue is `docs/REFACTOR_ROADMAP.md`, a local file ignored by git (codes d1…f10 by phase; mark
finished items with the commit). When it is missing in a checkout, ask the user for it.
Before starting, re-read the item and check that the code has not drifted from the snapshot the roadmap
names.

## Rules

1. Pure move first, behaviour second. Moving a file or function is a commit with no semantic change
   made before the change that needs it; proof is green package tests and a `git diff --stat` that
   shows only relocation. The behaviour change is the next commit with its own RED test.
2. One helper = one commit that converts **all** copies; a half-migration is worse than the duplication.
3. A shared helper does not change external contracts: event names, the JSON shape of `*:degraded`,
   `uierr` codes, state file formats. Frontend contract tests and `store_integrity_test` are the safety
   net.
4. Narrow interface instead of a concrete type: define the interface from the methods actually used
   (`grep` the calls), swap it in the constructor, then use a fake in tests.
5. Moving I/O out from under a mutex only after the package's mutations go through a shared helper;
   re-read I-1…I-3, I-11, I-12; keep the order "reserve under the lock → I/O → record under the lock".
6. Destructive paths (updates, install, relocate): never without a "killed between steps" test; do not
   weaken the existing window tests (`apply_recovery_test.go` A–D, `recover_test.go`).
7. Do not start: `internal/wire` as a package, `ArchivedGame` as a type, merging the two torrent
   clients, replacing the anacrolix fork before an upstream tag contains the patches (see the roadmap).

## Accepting a stage

Package tests, `-race` for concurrent code, `wails3 task check:local` before push, a live launcher start
for `main.go` changes. The report names the roadmap code, what moved, what changed in behaviour
(expected: nothing) and how that was proven.
