# Typhon Launcher — Debugging

Where to look when something misbehaves. Navigation and evidence policy: [INDEX.md](INDEX.md). Paths use `<ConfigDir>` = `os.UserConfigDir()/Typhon` (`%AppData%\Typhon` on Windows, `~/Library/Application Support/Typhon` on macOS) and `<Library>` = the user-chosen `<parent>/TyphonLibrary` with `Games/`, `Downloads/`, `Screenshots/`.

In a `devmock` build, an absolute `TYPHON_TEST_DATA_DIR` overrides `<ConfigDir>` directly (`internal/settings/configdir_devmock.go`). If unset, devmock uses the same default location as the native build. The override is ignored by non-devmock builds. It does not isolate the library root, CrossOver bottles, credentials supplied through env, or backend traffic; choose fixture paths and an explicit test backend for the intended scenario.

## 1. Logs

| Log | Path | Producer | Notes |
|---|---|---|---|
| Main log | `<ConfigDir>/typhon.log`, rotated to `typhon.log.1..5` at 10 MiB (`internal/app/logging.go::InitLogging`, `newRotatingWriter`) | `slog` text handler, level Info; also mirrored to stderr | Wrapped by `diagnostics.NewLogHandler` so Error records are queued for upload when consented. Startup line: `typhon starting version=…`; devmock adds `devmock build: Windows-only subsystems are mocked`. |
| Installer log | `<ConfigDir>/installer-<installID>.log` (Inno `/LOG=`, MSI `/L*v`; UTF-16LE for Inno) | the installer | Deleted after a successful chain (`internal/install/flow.go::dropInstallerLog`); read by `installer_phase.go::installerLogVerifying` to detect the "verifying" phase and by `runSilentChain` to forgive GOG crashes after "Installation process succeeded." |
| CrossOver log | `<ConfigDir>/installer-<id>.log.wine.log` (darwin) | cxstart | not cleaned by the launcher |
| Elevated worker log | `<ConfigDir>/worker-<installID>.log` — **devmock only** (`internal/install/elevate_devmock.go`) | worker stdout/stderr | Windows worker writes only `worker-<id>-state.json` |
| Self-update worker | `<ConfigDir>/selfupdate/worker/worker.log` + `progress.json` (devmock), `last-update.json` (outcome, all platforms) | `internal/selfupdate/worker_run.go` | Outcome is shown in "About" on the next start (`selfupdate.Service.GetOutcome`) |
| Installguard trace | stderr of the bridge / worker when `TYPHON_INSTALLGUARD_TRACE=1` | `internal/installguard`, `internal/wine/run.go` | per-window and per-process decisions |
| Log export | "Settings → export logs" → `app.Service.ExportLogs` builds a zip (`LogBundle`) next to the logs with `scrubLines` redaction; "send logs" → `diagnostics.Service.SendLogs` posts to `/v1/diagnostics/logs` | | |

Test-only helper output: `go test` in `internal/download` runs a package-wide goroutine leak check (`leakcheck_test.go::TestMain`); a leaked goroutine fails the whole package, not the test that leaked.

## 2. Environment variables and flags

Runtime (production binary):

| Variable | Effect | Read in |
|---|---|---|
| `TYPHON_API_URL` | backend base URL; must be `https` unless host is loopback | `internal/account/config.go::BaseURL`, `ValidateBaseURL` |
| `TYPHON_API_TOKEN` | session token instead of the OS credential store (dev) | `internal/account/credential.go::NewCredentialStore` (`envCredentialStore`) |
| `TYPHON_STEAM_BOTTLE` | name of the shared CrossOver bottle (default `Steam`) | `internal/wine/shared.go::SharedBottleName` |
| `TYPHON_INSTALLGUARD_TRACE` | verbose installer-guard tracing | `internal/installguard/*_windows.go`, `internal/wine/run.go` |
| `TYPHON_RELEASE_PRIVATE_KEY` | CI only: ed25519 key for `cmd/signrelease` | `cmd/signrelease/main.go` |
| `TORRENT_STORAGE_DEFAULT_FILE_IO` | forced to `classic` by `internal/download/classicio` unless set | anacrolix storage |

devmock build only (`-tags devmock`, never compiled into Windows):

| Variable | Default | Effect |
|---|---|---|
| `TYPHON_TEST_DATA_DIR` | unset | absolute config directory override; no automatic isolation when unset (`internal/settings/configdir_devmock.go`) |
| `TYPHON_DEVMOCK_ELEVATE` | `1` | `0` = fake install directly, otherwise go through the worker spec/state/cancel protocol (`internal/install/runner_devmock.go`) |
| `TYPHON_DEVMOCK_INSTALL_SECONDS` | `2` | duration of the fake install (`0` allowed) |
| `TYPHON_DEVMOCK_GAME_SECONDS` | `60` | lifetime of a fake game process (`internal/devmock/registry.go`) |
| `TYPHON_DEVMOCK_STATE_DIR` | `<ConfigDir>` | where `devmock-processes.json` and the fake `system32/` live |
| `TYPHON_DEVMOCK_AWAY_SECONDS` | 600 | idle threshold for presence "away" (`internal/online/online.go::awayAfterFrom`) |
| `TYPHON_DEVMOCK_MANIFEST_URL`, `TYPHON_DEVMOCK_RELEASE_PUBKEY` | — | point self-update at `tools/devrelease` (`internal/selfupdate/url_devmock.go`, `key_devmock.go`) |
| `TYPHON_DEVMOCK_COMPAT_STATS` | — | path to a ready compat-stats snapshot (`main.go::compatStatsPath`) |

Test gates (all skip by default): `TYPHON_WINE_LIVE=1` (+ `TYPHON_WINE_LIVE_INSTALLER`, `TYPHON_GUARD_FIXTURE_EXE`, `TYPHON_GUARD_X86_FIXTURE_EXE`, `TYPHON_INNO_COMPILER_INSTALLER`, `TYPHON_GUARD_REAL_INSTALLER`, `TYPHON_GUARD_FULL_INSTALL`, `TYPHON_GUARD_VISIBLE_INSTALL`), `TYPHON_KEYCHAIN_LIVE=1`, `TYPHON_PE_LIVE`, `TYPHON_LIVE_PICKER_DIR`/`_FILE`, `TYPHON_METADATA_API`/`TYPHON_METADATA_TOKEN` (`internal/metadata/live_test.go`), `TYPHON_STEAM_E2E_URL` (`typhonapi/steam_e2e_test.go`). See `TEST_MAP.md`.

CLI arguments of the launcher binary: `--play <libraryGameID>`, `--install-worker <spec.json>`, `--install-broker <dir>`, `--selfupdate-worker <spec.json>` (`main.go::main`). Wails: `--disable-gpu` is passed to WebView2 when `HardwareAcceleration` is off (`main.go::browserArgs`).

## 3. Commands

```bash
# build / run (Windows target on Windows; native on macOS)
wails3 task build            # bin/typhon(.exe)
wails3 task run
wails3 task dev              # hot reload, vite on 127.0.0.1:9245
wails3 task package          # NSIS installer / .app; darwin:package:zip for the update bundle

# macOS/Linux development with Windows-only subsystems mocked
wails3 task build:devmock
wails3 task run:devmock
wails3 task dev:devmock
wails3 task test:devmock
wails3 task devrelease VERSION=0.9.9      # fake signed self-update server on 127.0.0.1:8099; prints the two TYPHON_DEVMOCK_* vars

# Go checks: select those relevant to the change; this is not a mandatory sequence.
# Main/internal scope avoids unrelated generated build/ios packages.
gofmt -l . && go vet . ./internal/... && go build . ./internal/... && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build . ./internal/...
go test ./internal/... && CGO_ENABLED=1 go test -race ./internal/... && CGO_ENABLED=1 go test -race -tags devmock ./internal/...
golangci-lint run --new-from-rev=origin/dev --whole-files=false ./...    # pinned v2.13.1, `wails3 task lint:install`
go run ./tools/lintbaseline
go run ./tools/lintbaseline -tags devmock
# Baseline changes: use the typhon-lint-baseline skill; -update writes the file.

# frontend (from frontend/)
npm run check && npm run test && npm run build          # bindings: wails3 task common:generate:bindings
npx vitest run --project dom                            # only the *.dom.test.ts behaviour tests (jsdom); --project node for the rest
```

Live-window verification on macOS (screenshots, clicks, single instance): [typhon-live-check](../../.agents/skills/typhon-live-check/SKILL.md). Never use the user's real `<ConfigDir>` as a test fixture; tests use `t.TempDir()` and `NewServiceAt(...)` constructors.

The build/run examples above are separate commands. Before running the app, set an isolated absolute `TYPHON_TEST_DATA_DIR` for devmock and configure fixture library paths and the intended `TYPHON_API_URL`. Do not copy the whole command block as a workflow.

The Wails development build (`common:build:frontend DEV=true`, also `build:devmock`) runs
`vite build --mode development`, so `import.meta.env.DEV` is false there; dev-only code is gated on
`import.meta.env.MODE === 'development'` (`lib/navigation/showcase.ts`, guard `devGate.test.ts`). The
"Component showcase" sidebar entry exists only in such builds; a release build folds it out.

## 4. Reproducing the main flows (devmock on macOS unless stated)

| Flow | How | Watch |
|---|---|---|
| Startup | `wails3 task run:devmock`; window title `Typhon [devmock]` | `typhon.log` startup lines, every `ServiceStartup` logs its recovery decisions |
| Source refresh | Sources → add a URL or a local feed file (`sources.Service.AddSourceFile`; feed JSON format in `internal/sources/feed/feed.go::Entry`, limits in `limits.go`); manual `RefreshSource` | `source refresh started`, `source:updated`, `release:*` events; `<ConfigDir>/releases/<sourceID>.json` |
| Search | search bar → `search.Service.Search` (library + catalog + releases) | pure in-memory, no state |
| Download | release → download → `download.Manager.FetchMetadata` → `StartDownloadFrom` | `download started/completed`, `<ConfigDir>/downloads.json`, `<Library>/Downloads/<name>` |
| Install | `AutoInstall` setting or Downloads → install → `install.Service.InspectDownload` + `Start`; devmock runs the fake installer (`TYPHON_DEVMOCK_INSTALL_SECONDS`) | `install started/completed`, `installations.json`, `<Library>/Games/<name>[.partial]`, `worker-<id>-*` files when `TYPHON_DEVMOCK_ELEVATE≠0` |
| Executable detection | after install with several `.exe` the record goes `waiting_for_user`; `install executable chosen` / `install waiting for user` log lines list scored candidates | `internal/install/executable.go::FindExecutables`, `HighConfidence` |
| Launch | Library → Play → `library.Service.PlayGame`; devmock spawns a fake process registered in `devmock-processes.json` for `TYPHON_DEVMOCK_GAME_SECONDS` | `game started/stopped`, `game:started/stopped` events, playtime in `library.json` |
| Update detection | edit the feed file: same `distributionId`, higher `version`, refresh the source | `update available`, `<ConfigDir>/updates.json`; rules in `internal/updates/resolver.go::ResolveUpdate` |
| Update install | game page → update → `updates.Service.PreparePlan` → `StartUpdate` | `<Library>/Games/.staging/<gameID>`, `<installDir>.previous`, `journal.json`, `update:*` events |
| Self-update | `wails3 task devrelease VERSION=x.y.z`, export the printed vars, start devmock; About → check | `selfupdate/state.json`, `selfupdate/<version>/<artifact>[.partial]`, `selfupdate/worker/*`, `launcher:update_status` |
| Move between disks | game page → move → `relocate.Service.MoveGame` (or Settings → library folder → `MoveLibrary`) | `moves.json`, `<target>.staging`, `move:*` events |
| Uninstall / delete | game page → remove → `install.Service.InspectRemoval` + `RemoveGame` | `<installDir>.removing[-N]`, `removals.json`, `install:removed`, `library-excluded.json` |
| Crash recovery | kill the process (`kill -9`) mid-flow, restart | `ServiceStartup` log lines listed in §7 |

Windows-only paths (UAC worker, `.lnk`, registry uninstall, Credential Manager) cannot be reproduced on macOS; the devmock protocol tests are the closest stand-in, and `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build . ./internal/...` proves compilation only.

## 5. State on disk

Everything under `<ConfigDir>` unless noted. Format: **E** = `storage.Save` envelope `{"version":N,"data":…}`, **R** = raw JSON via `storage.WriteAtomic`. Corrupt (non-`ErrNotExist`) load → the owning constructor or `ServiceStartup` fails and the launcher exits (`main.go::fatal`), except where noted.

| File | Owner | Format | Contents |
|---|---|---|---|
| `settings.json` | `internal/settings` | R | `Settings` struct; consent keys re-read via `consentProbe` |
| `library.json`, `library-excluded.json` | `internal/library` | R | games incl. archived (`Archived: true`); dirs the user removed (discovery ignores them) |
| `<installDir>/.typhon-install.json` | `internal/library/marker.go` | E | install marker (gameId, release ids, executable rel path) — lets discovery re-adopt a folder after a move |
| `installations.json`, `removals.json` | `internal/install` | R | `[]Installation`; staged `.removing` paths |
| `installer-<id>.log`, `worker-<id>-{spec.json,state.json,discover.ini,cancel}`, `broker-<downloadID>/{pin.json,spec.json,alive,abort}` | `internal/install` | R / markers | worker files are never deleted; `alive` mtime is the broker heartbeat |
| `downloads.json`, `torrents/<infohash>.torrent`, `meta/piece-completion.json` (+`.broken-<ts>`), `meta/` payloads | `internal/download` | R | records (no progress), cached metainfo, piece bitsets |
| `updates.json`, `update_history.json`, `rollbacks.json`, `journal.json`, `verify.json`, `manifests/<gameID>.json`, `saves/<gameID>[.partial\|.replaced]` | `internal/updates` | E | offers/plans, history (200), `.previous` entries, in-flight swap/patch/inplace journals, verify verdicts, SHA-256 manifests, saves snapshots |
| `moves.json`, `moves/<jobID>.json` | `internal/relocate` | E | move jobs (failed/cancelled stay for `List`), per-job manifest |
| `sources.json`, `releases/<sourceID>.json` | `internal/sources` | E | sources; releases per source (a parser change is invisible until the source is refreshed) |
| `catalog.json`, `match_overrides.json`, `catalog-redirects.json`, `catalog-pages/<sha256>.json` | `internal/catalog` | E | games, learned matches, id redirects, cached remote browse pages |
| `media_assets.json`, `metadata_attempts.json`, `media/` | `internal/metadata` | E / files | asset index, retry backoff per game, cover/screenshot files served at `/media/<rel>` |
| `history.json`, `playlog.json`, `compat.json`, `compat-stats.json`, `compat-client.json`, `installation.json` | history, playlog, compat, clientid | E / R | operation journal; 90-day sessions; launch outcomes; shared stats; two anonymous ids |
| `account.json`, `profile.json`, `devmock-credential.json`, `sync.json` | account, accountsync | E / R | guest flag, cached profile, devmock token, sync state; real token in Credential Manager / Keychain |
| `lanshares.json`, `lan/` | `internal/lan` | E / files | shares and their torrents |
| `themes.json`, `titles-dict.json`, `titles-dict-remote.json` | theme, titlesdict | E / R | user themes (each carries `"contract": 2` once written by 2.0; a theme without it is converted in memory on read and reaches the file on the next write, the envelope version stays 1); title dictionary |
| `selfupdate/state.json`, `selfupdate/release-notes.json`, `selfupdate/<version>/<artifact>[.partial]`, `selfupdate/worker/{update-spec.json,last-update.json,typhon-update-<pid>.exe}` | `internal/selfupdate` | E / R | status, notes, resumable artifact, worker hand-off |
| `diagnostics/pending/<unix>.json`, `diagnostics/pending/rejected/` | `internal/diagnostics` | E | unsent error batches; batches the server rejected |
| `devmock-processes.json` | `internal/devmock` | R | fake process registry (devmock) |
| `~/Library/Application Support/CrossOver/Bottles/<name>/typhon-bottle.json` | `internal/wine/bottle.go` | R | marker that Typhon created this bottle (the shared Steam bottle has none) |
| `~/Library/LaunchAgents/<label>.plist` | `internal/autostart/autostart_darwin.go` | plist | autostart |

A `ServiceStartup` error is also fatal: Wails returns `error starting service '<name>': …` from `Run` and `main.go::fatal("run application")` exits. A documented exception is `download`, which quarantines a corrupt `meta/piece-completion.json` (renamed `.broken-<unix>`) and starts empty instead of failing.

## 6. Temporary directories and markers

| Marker | Owner | Meaning / who cleans |
|---|---|---|
| `.tmp-*` next to any state file | `storage.WriteAtomic` | leftover temp of an interrupted atomic write; harmless |
| `<Library>/Games/<name>.partial` | `internal/install` | portable/archive staging; `sweepPartial` at startup (move-mode partials are *recovered*, not deleted) |
| `<installDir>.removing[-N]` | `internal/install/removal.go` | renamed before `RemoveAll`; `sweepRemovals` at startup |
| `<Library>/Games/.staging/<gameID>` | `internal/updates` | new version before the swap; removed at startup for games left Failed |
| `<installDir>.previous`, `.previous.repair`, `.replaced`, `.patch-backup` | `internal/updates` | rollback copy (kept per `KeepPreviousVersion`), repair backup, parked bad install, per-patch merge backup |
| `<file>.typhon-tmp`, `<backup>/added.list` | `internal/install/fsops.go::MergeDirWithBackup` | patch merge staging |
| `<target>.staging` | `internal/relocate` | verified copy before commit |
| `<newRoot>.repoint-staging` | `internal/download/repoint.go` | cross-volume downloads move |
| `<file>.part` | anacrolix | incomplete torrent file (disabled for in-place downloads) |
| `<bottle>/drive_c/typhon-installguard-*/` | `internal/installguard/asset` | extracted bridge; kept on purpose when the installer tree could not be confirmed stopped |
| `catalog-migration-backups/` | `internal/catalogmigration` (untracked WIP) | offline migration backups |

## 7. Failure signatures

Symptom → first suspects. Log lines are `slog` messages to grep in `typhon.log`.

| Symptom | Look at | Log anchors |
|---|---|---|
| Launcher exits at start | `main.go::fatal("<stage>")`; the named constructor's load. Usually a corrupt state file — do **not** delete it blindly, read it | `startup failed stage=… err=…`, `read <file>` / `parse <file>` |
| A subsystem keeps working but shows a red banner | `*:degraded` — persist failed, memory rolled back (`download/manager.go::persistLocked`, `updates/service.go::markDegradedLocked`, `relocate/service.go::transition`, `sources`, `history`, `playlog`) | `persist installations`, `save sources`, `persist moves journal` |
| Download stuck in `verifying` / never completes | `download/manager.go::sample` (needs `selectedHashed`), `verify.go::verifyFilesOnDisk` (`.part` left ⇒ `download.file_incomplete`), AV on Windows (`download.blocked_by_av`) | `torrent write error`, `metadata timeout`, `torrent already attached elsewhere` |
| Two downloads of the same torrent / "hash busy" | `download/reuse.go::reserveHash` family; a torrent reused by verify/repair/update holds the hash | `download.hash_busy` |
| Install fails with no user text | error lacks a `uierr` code → check `frontend/src/lib/install/installErrors.ts` table and `internal/install/uicodes_test.go` | `install failed` |
| Install `interrupted` after restart | `install/service.go::ServiceStartup`: worker alive/done → resume; `crashFinalizable` → finalize; else interrupted | `installation worker still running, resuming`, `installation committed before crash, finalizing`, `installation interrupted` |
| Install dir kept after cancel | by design when the installer tree was not confirmed stopped (`install.installer_not_confirmed_stopped`) | `keep install dir, installer not confirmed stopped`, `job object kill-on-close unavailable, falling back` |
| Wrong `.exe` chosen / game does not launch | `install/executable.go::scoreExe`; `library.PlayGame` errors `library.executable_missing`, `library.runtime_failed`, `library.launch_failed`; compat journal `compat.json` | `install executable chosen offered=…`, `launch game`, `prepare game runtime` |
| Session not detected / playtime not counted | `library/watch.go::detectTick` (1 s), `procs.List` per OS (darwin: wine `ps` parsing, path translation), `PathUnknown` processes | `scan running processes` warnings |
| No update offered although the feed has a newer version | `updates/resolver.go`: `sameDistribution` (same `SourceID` + `DistributionID`, legacy: exact `ReleaseID`), `usable` (available, URIs, canonical game), `Compatible` (edition), confidence < 0.6 ⇒ `new_release`; `UpdateCheckAutomatically` off ⇒ only manual `CheckUpdates` | `update available`, `bind legacy installation to distribution` |
| Update failed "прервано" after restart | `updates/apply.go::recoverJournals` per journal kind; a journal that cannot be recovered stays in `journal.json` | `recover swap journal`, `restore previous version`, `unknown swap journal kind` |
| `.previous` folders piling up | `KeepPreviousVersion` (`off`/`first_launch`/`day`), `HandleSessionEnded` (session ≥ 60 s), `sweepPrevious` every 15 min | `previous version removed after a successful launch`, `previous version expired` |
| Move left `<target>.staging` | `relocate/recover.go` by stage; `ErrAmbiguousRecovery` when both source and target exist at commit | `remove move staging`, `mark move failed on recovery`, `unknown move stage on recovery` |
| Source refresh silently does nothing | conditional GET returned 304 (`result.NotModified`), source disabled, backoff after failures (`retryDelay`, up to 1 h), `SourceRefreshInterval=manual` | `source refresh started`, `source feed served from the preview cache` |
| Feed rejected | `sources/feed/guard.go` (loopback/private host, redirect to private), Cloudflare challenge, `limits.go` caps, content-type not JSON | `source:error` payload (URL hidden) |
| Covers missing | `metadata/service.go` budget/backoff (`metadata_attempts.json`), rate limit `RateLimitError`, provider nil when `typhonapi.New` failed (`metadata provider disabled`), media served from `/media/<rel>` | `metadata provider disabled` |
| Self-update never applies | `selfupdate/service.go::ApplyUpdate` needs `StateReady`; signature/hash (`VerifyFile`) re-checked at startup; worker outcome in `selfupdate/worker/last-update.json` | `selfupdate: …`, worker `progress.json` stages |
| macOS: game will not start, CrossOver installed | `wine.Detect` roots, bottle `Ensure`/`SharedBottle`, `RequiresSteam` tri-state (`nil`/`true` ⇒ shared Steam bottle), drive letter re-pinning (`wine/drive.go`) | `installing into shared bottle`, `shared bottle covers install`, `prepare game runtime` |
| Tray icon missing on macOS | check via Accessibility (`osascript … count of menu bars` = 2), not `CGWindowListCopyWindowInfo`; macOS 26 "Allow in menu bar" setting | `apply tray` |
| Telemetry/social requests absent | consent: `UsageStatsAllowed`/`DiagnosticsAllowed` require `TelemetryConsentVersion == 2`; `AccountSync`; `clientid.Load` failure disables heartbeat/usagestats/diagnostics entirely | `load client identity` |
| Custom theme changed size or lost a value after an upgrade, or its import is refused | `internal/theme/contract.go::toCurrentContract`: a theme without `contract` is read as 1.x (values equal to the released preset follow the current preset, the user's rem lengths are multiplied by 1.1, a user `--accent` also fills the new `--accent-bright`; rem inside custom CSS is not touched, but `data-part` selectors on a renamed part are rewritten through `renamedParts`, unless the CSS is refused by the current rules); a stored theme whose id equals a built-in id is renamed at load (`service.go::unshadowStored`); `contract` above 2 ⇒ `theme.unsupported_version` (`service.go::decodeTheme`); a 1.x launcher importing a 2.0 file ⇒ `theme.invalid_format`; after a downgrade a 1.x write drops the marker, see `lostContractMarker` | the conversion is not logged; `stored theme css refused by current rules`, `theme source reload failed`, `theme source is missing` |
| Test hangs ~10 min | forged `worker-<id>-state.json` without the `Run` token (`install/elevated.go::runElevated`), `t.Skip`/`t.Fatal` while holding a mutex whose cleanup needs it, real CrossOver bottle created because `prepareRuntime` was not stubbed | — |

Error text reaching the UI is `typhon:<code>: <detail>` (`internal/uierr`); the code is what the frontend translates (`frontend/src/lib/i18n/errors.ts::errorCode`). An error without a code renders as generic text — that is a bug in the Go side, not in i18n.

## 8. Where to look first, by area

- **Persistence / "my data disappeared"**: `internal/storage/doc.go`, the owner's `load`/`persist` pair, then `INVARIANTS.md` I-1–I-3. Check for `.tmp-*` leftovers and `*.broken-*` quarantines.
- **Anything with two renames** (update swap, rollback, move, repoint, self-update bundle swap): `FLOWS.md` §14, the journal file, the `ServiceStartup` recovery function of the owner.
- **Process/child ownership**: `internal/install/elevated.go`, `runner_*.go`, `internal/library/sessions.go` (`sessionWG` is not waited on shutdown by design — games outlive the launcher), `internal/wine/run.go`.
- **Frontend shows stale state**: the store's `init*()` in `frontend/src/lib/stores/*.ts` and whether the Go side emitted under the lock (it must not) — `emit` sites are listed per package in `CODEMAP.md`.
- **Windows-only behaviour on a Mac**: cannot be run; read `*_windows.go`, cross-compile with `GOOS=windows`, and check the corresponding `*_windows_test.go` exists (`TEST_MAP.md`).

Historical findings from 2026-09-10 are catalogued in the untracked `AUDIT_FABLE_5_1.md` (TYPHON-AUDIT-001…016: cloud `Owned` flag, sync removals never applied, provenance inheritance on reinstall, `MoveGame` into an existing empty dir on Windows, unjournaled `Rollback` renames, broker spec trust, and more). Treat them as suspects when a symptom matches; re-check their current status before calling them open or fixed.
