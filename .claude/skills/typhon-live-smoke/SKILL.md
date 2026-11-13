---
name: typhon-live-smoke
description: Use when a change touches the user journey in Typhon Launcher (sources, download, install, launch, update, remove, settings, Big Picture) and unit tests cannot prove it — to run the scenario in the live app with screenshots.
---

# Live scenario run

Evidence for UI and end-to-end flows: screen, log and on-disk state match the expectation. Choose the
build, instance and isolation per `../.agents/skills/typhon-live-check` (never kill someone else's
launcher by name; devmock without `TYPHON_TEST_DATA_DIR` shares state with the native build).

## Baseline scenario (devmock, about 10 minutes)

1. Start with a clean `TYPHON_TEST_DATA_DIR`; the window opens, `typhon.log` has no `ERROR` at startup.
2. Sources: add a test feed (`tools/devrelease` or a file), wait for releases and matching.
3. Download: add a task, pause/resume, progress event in the UI, `downloads.json` updates.
4. Install: completion → plan → install (devmock worker) → the game in the library with a
   `.typhon-install.json` marker.
5. Launch/stop: `game:started`/`game:stopped`, playtime recorded, no false crash classification.
6. Update and rollback if `updates` changed: the journal appears and disappears, `.previous` handled per
   setting.
7. Remove: files deleted only within their boundaries, the record archived, `library-excluded.json`.
8. Settings: a changed setting survives a restart.
9. Big Picture: the same screens for the changed flow (the controller mode is a separate implementation).

## Recording

Before/after screenshots of each affected screen (desktop and Big Picture) in `output/<task>/screens/`;
log excerpts for the key events; file state (`ls`, size, `jq` on keys). The report is a table step →
expectation → observation → match/mismatch, plus what stayed unverified (Windows-only, a real game,
CrossOver).

A browser run that replaces Go's answers (a state devmock cannot reach) proves the frontend only. For
each replaced call name the Go test that proves the real answer in that state, or list the pair as
unverified. Example: a dialog passed such a run while the real `Status()` did not list the conflict
the launch gate had blocked on.

## Do not

Re-download or re-install for one screenshot when the state already exists; mix the user's release
state with test state; treat an SSR snapshot or a green vitest as a substitute for this run; count a
browser run with replaced Go answers as proof of the real Go behaviour.
