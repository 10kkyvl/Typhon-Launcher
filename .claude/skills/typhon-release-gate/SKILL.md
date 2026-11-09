---
name: typhon-release-gate
description: Use right before tagging a Typhon Launcher release or merging dev into main for a release — to decide "can tag / cannot tag" with reasons.
---

# Release gate

"Can tag" is given only when every item is green. Any red item → the list of reasons and what to do;
the tag and the push happen only with the user's permission (`../.agents/skills/typhon-release`
describes the release procedure itself).

## Checks

1. **Version**: `VERSION`, `internal/app/app.go`, `build/config.yml`, `build/windows/info.json`,
   `build/darwin/Info.plist` and `Info.dev.plist` — `go test ./internal/app -run TestVersionSourcesMatch`;
   macOS bundle metadata if a Mac artifact ships.
2. **Changelog**: `go test ./cmd/signrelease`; ru/en match in meaning; no dev-only items
   (`typhon-changelog-keeper` when the notes are prepared, per `dispatch.md`).
3. **Legal**: `legal.Version` and the "edition" lines agree; new outgoing data is described; site copies
   updated; the `CurrentTelemetryConsent` decision is made (`typhon-legal-keeper`).
4. **Dependencies and pipeline**: `typhon-deps-keeper` without reds; `release.yml`/`Taskfile` not
   changed without review; supply-chain rules hold (lifecycle scripts off, signer built apart from
   Node, artifact hash checked).
5. **Contract**: bindings regenerated (`wails3 task common:generate:bindings`) and committed where
   they are part of the build; contract tests green.
6. **Build and tests**: `wails3 task check:local` green; `GOOS=windows GOARCH=amd64 go vet ./...`;
   hosted CI green on the last dev commit.
7. **State migrations**: new fields or versions in `storage.Save` have a migration and an "old file →
   new version" test; `settings.sanitize` covers new fields.
8. **Release base**: the tag goes on the dev→main merge; `git log main..dev` is empty after the merge.
9. **Live smoke**: skill `typhon-live-smoke` on the release build or devmock if install, update or
   launch changed.

## Report

Table item → status → evidence (command and result); one-line verdict; what needs the user's decision
(legal, consent, skipping an item).
