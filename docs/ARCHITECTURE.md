# Typhon Launcher — Architecture

Technical map for coding agents. Facts reflect the working tree on 2026-09-11 (branch `dev`, 0.5.2 plus uncommitted work on release identity, Steam/RU metadata). Reading guide and evidence policy: [INDEX.md](INDEX.md). Companion docs: `CODEMAP.md` (per file), `FLOWS.md` (end-to-end), `INVARIANTS.md`, `TEST_MAP.md`, `DEBUGGING.md`.

## 1. Shape of the system

One Go process (module `typhon`, Go 1.25, Wails v3.0.0-beta.17) hosts ~30 services and a WebView (Svelte 5, `frontend/`). Services are plain structs registered with `application.NewService`; Wails generates TS bindings for their exported methods (`frontend/bindings/`, gitignored, `wails3 task common:generate:bindings`). Go → UI communication is by events (`app.Event.Emit`, per-package `emit()` helper that no-ops when `application.Get()` is nil, i.e. in tests). UI → Go is by bound method calls.

The same binary also runs as helper processes, dispatched at the top of `main.go::main`:

| Argument | Runs | Purpose |
|---|---|---|
| `--install-worker <spec.json>` | `internal/install::RunWorker` | elevated installer worker (Windows UAC; devmock mimics) |
| `--install-broker <dir>` | `internal/install::RunBroker` | pre-elevated rights broker started ahead of a download |
| `--selfupdate-worker <spec.json>` | `internal/selfupdate::RunWorker` | waits for parent exit, replaces the binary/bundle, relaunches |
| `--play <libraryGameID>` | `main.go::playTarget` → `library.Service.PlayGame` | shortcut launch; forwarded to the running instance via Wails SingleInstance (`com.typhon.launcher`) |

Separate Windows-only executables: `cmd/exepicker` (native file picker), `cmd/installguard` (installer window guard). Release tooling: `cmd/signrelease` (signs the self-update manifest in CI), `tools/devrelease` (local fake release server, port 8099), `tools/lintbaseline`, `tools/notices`. `cmd/catalog-migrate` + `internal/catalogmigration` are untracked work in progress (offline catalog migration report/apply).

Build variants (see `INVARIANTS.md` I-16):
- **Windows** (target): `GOOS=windows`, elevation via worker/broker, procs via Win32, credentials in Credential Manager, `.lnk` shortcuts.
- **macOS native** (`darwin && !devmock`): games run through CrossOver via `internal/wine`; Keychain via `/usr/bin/security`; `.app` shortcuts; LaunchAgent autostart.
- **devmock** (`devmock && !windows`): Windows-only subsystems mocked (`internal/devmock` fake process registry). `internal/devmock/forbid_devmock.go` fails the build for `GOOS=windows` or tag `production`; CI asserts this.

## 2. Layers

```
frontend/src (Svelte 5) ── stores subscribe to events, services call bindings
        │ @wailsio/runtime (Events.On / bound methods)   media via GET /media/<rel> (metadata.Middleware)
main.go ── constructs services, wires callbacks (SetOn*, Set*Recorder, Subscribe, AddApplier)
internal/* services ── one package per subsystem, mutex-guarded state, per-package emit()
internal/storage (WriteAtomic/Load/Save envelope), internal/settings (ConfigDir, paths), internal/platform (OS shims)
OS: filesystem, processes (procs/wine/devmock), Credential Manager/Keychain, LaunchAgent/registry, Discord IPC
Network: api.typhon-launcher.com (/v1), feed URLs, BitTorrent (anacrolix/torrent), LAN multicast 239.255.71.84:42816
```

Dependency direction (enforced by import graph, no cycles): `storage`/`uierr`/`version`/`titles`/`redact` ← `settings`/`platform` ← domain services ← `main.go`. Cross-service coupling is done by callbacks set in `main.go`, not by imports, except: `install` imports `download`, `library`, `wine`; `updates` imports `library`, `sources`, `download`, `install`; `relocate` imports all four; `discovery` imports `library`, `catalog`, `install` (executable detection; metadata is passed in as an interface); `sources` imports `catalog`; `library` imports `procs`, `shortcut`, `wine`, `usagestats`, `history`; `wine` imports nothing from the domain.

## 3. Packages

| Package | Responsibility | Depends on (internal) | Key types | State (under ConfigDir unless noted) |
|---|---|---|---|---|
| `app` | logging (`typhon.log`, 10 MiB × 5), AppInfo/system info, folder/exe pickers, log export | settings, platform, redact | `Service`, `AppInfo`, `LogBundle` | `typhon.log`, `typhon.log.N` |
| `settings` | `Settings` struct, defaults, sanitize, `ConfigDir()`, library root (`<parent>/TyphonLibrary/{Games,Downloads,Screenshots}`), consent, Subscribe/AddApplier | storage, uierr | `Settings`, `Service`, `Portable` | `settings.json` (raw JSON, no envelope) |
| `storage` | `WriteAtomic` (CreateTemp → write → chmod → fsync → close → rename → dir fsync), `Save`/`Load` with `{version,data}` envelope + migrations | — | `Migration` | — |
| `platform` | per-OS shims: paths (`Inside`, `Normalize`, `PathKey`, `SamePath`), open folder, storage info, sysinfo, save roots, wine status, exe picker (darwin) | wine (darwin) | `SystemInfo`, `StorageInfo`, `WineStatus`, `SaveRoot` | — |
| `library` | installed games, sessions (launch/detect/stop), playtime, favorites/status, saves, shortcuts, install marker, account-sync snapshot | storage, procs, shortcut, wine, platform, history, usagestats, uierr | `Game`, `InstalledGame`, `InstalledUpdate`, `Marker`, `SessionEvent`, `SessionWatcher` | `library.json` (array incl. archived), `library-excluded.json`; `<InstallDir>/.typhon-install.json` |
| `download` | BitTorrent manager (anacrolix v1.61.0), queue, speed limits, verification, in-place reuse, repoint | settings, storage, hashdir, platform, redact, history, usagestats, uierr | `Download`, `Origin`, `RemovedEvent`, `Manager` | `downloads.json`, `torrents/<infohash>.torrent`, `meta/piece-completion.json`, `meta/` payloads of metadata-only adds |
| `install` | classify downloaded content, plan/execute install (portable copy, exe/msi via elevated worker, archives zip/rar/7z), exe detection, uninstall records, removal, shortcuts | download, library, settings, storage, platform, wine, hashdir, titles, installguard, history, usagestats, uierr | `Installation`, `Plan`, `Candidate`, `RemovedEvent` | `installations.json`, `removals.json`, `installer-<id>.log`, `worker-<id>-{spec.json,state.json,discover.ini,cancel}`, `worker-<id>.log` (devmock), `broker-<downloadID>/{pin.json,spec.json,alive,abort}` |
| `updates` | detect newer releases for installed games, plan (full/patch chain/torrent reuse), apply with journal/rollback, verify/repair, save backup | library, sources, catalog, download, install, settings, storage, version, hashdir, titles, platform, history, usagestats | `Update`, `UpdateAvailability`, `VerifyState` | `updates.json`, `update_history.json`, `rollbacks.json`, `journal.json`, `verify.json`, `manifests/<gameID>.json`, `saves/<gameID>` |
| `relocate` | move a game or the whole library to another disk with journal and startup recovery | library, download, install, updates, settings, storage, hashdir | `Job` | `moves.json`, `moves/<jobID>.json` |
| `hashdir` | directory manifest (per-file SHA-256), `Build`/`Verify`/`Scan` with ctx | — | `Manifest`, `Result` | — |
| `sources` | feed sources (URL/file), scheduled refresh, release diff/merge, matching to catalog, ignore/confirm | catalog, settings, storage, titles, version | `Source`, `Release`, `ReleaseBatch`, `SourceError`, `DownloadRequest` | `sources.json`, `releases/<sourceID>.json` |
| `sources/feed` | feed JSON parser + limits, SSRF-guarded fetch (`guard.go`), file feeds | — | `Feed`, `Entry`, `Result` | — |
| `catalog` | local game catalog, title index, matching (`resolve`), overrides, browse via remote (`RemoteCatalog`), compat badges | storage, titles | `Game`, `Match`, `Query`, `GameQuery`, `RemoteCatalog` | `catalog.json`, `match_overrides.json`, `catalog-redirects.json`, `catalog-pages/<sha256>.json` |
| `metadata` | covers/screenshots/text via backend provider, request budget/backoff, asset cache, media HTTP middleware, language | catalog, storage, account (typhonapi) | `Provider`, `BatchProvider`, `View`, `Art`, `MediaAsset` | `media_assets.json`, `metadata_attempts.json`, `media/` |
| `metadata/typhonapi` | HTTP client for `/v1/metadata/*` (also implements `catalog.RemoteCatalog`) | account, app | `Client` | — |
| `search` | unified search over library, catalog, releases | library, catalog, sources, titles | `Result`, `GameHit`, `ReleaseHit` | — |
| `titles`, `titlesdict` | title normalization/parsing dictionary (embedded `titles-dict.json`, remote refresh 24 h) | storage, account | `Dict`, `Parsed` | `titles-dict.json`, `titles-dict-remote.json` |
| `discovery` | scan `GamesPath` for installed games not in library, adopt markers, detect exe, provision catalog/metadata | library, catalog, metadata, install, titles, platform, settings | `Result`, `Progress` | — |
| `procs` | process listing per OS (Win32 / `ps` + wine translation / devmock registry) | wine (darwin), devmock | `Process` | — |
| `wine` | CrossOver runtime: detect, bottle per game, shared Steam bottle, run/start/boot, processes/kill, registry, path translation | storage | `Runtime`, `Manager`, `Bottle`, `Cmd`, `Process` | `~/Library/Application Support/CrossOver/Bottles/<name>/typhon-bottle.json` |
| `selfupdate` | manifest fetch+ed25519 verify, resumable artifact download, apply via worker (installer on Windows, bundle swap on macOS), release notes | account, app, settings, storage, uierr | `Manifest`, `Artifact`, `Status`, `Progress`, `Outcome` | `selfupdate/state.json`, `selfupdate/release-notes.json`, `selfupdate/<version>/<name>[.partial]`, `selfupdate/worker/{update-spec.json,last-update.json,typhon-update-<pid>.exe,progress.json,worker.log}` |
| `account` | login/register/logout, session token in OS credential store, guest state, profile cache, avatar, `BaseURL`/`TYPHON_API_URL` validation, `CheckRedirect` | settings, storage, uierr, app | `Service`, `Client`, `CredentialStore`, `State`, `CurrentUser` | `account.json`, `profile.json`, `devmock-credential.json`; Credential Manager / Keychain |
| `accountsync` | 5-min sync of portable settings + library flags (playtime, favorite, status, owned) with `/me/sync` | settings, storage, library (via ports) | `Game`, `LibraryPort`, `CatalogPort`, `MetadataPort`, `SettingsPort` | `sync.json` |
| `social`, `online`, `profile`, `presence`, `discord` | friends/feed/reactions polling (60 s); presence heartbeat to `/me/presence` (30 s, away after 10 min via `idle`); profile aggregation; Discord Rich Presence IPC | account, library, settings, playlog | `FriendsPage`, `RequestsSignal`, `PublicProfile`… | — |
| `heartbeat`, `usagestats`, `diagnostics`, `telemetrylog`, `clientid`, `redact` | anonymous presence heartbeat; usage events batch (25 s); error/panic reports + log upload with redaction (20 s flush, 5 min dedup); in-memory "what was sent" log; installation id | settings, app, redact | `Event`, `Report`, `Identity` | `installation.json`, `compat-client.json`, `diagnostics/pending/<ts>.json`, `diagnostics/pending/rejected/` |
| `compat` | per-game launch outcome journal; shared stats download; consented reports | account, storage | `Record`, `Status`, `Stats`, `Sharer`, `Report` | `compat.json`, `compat-stats.json` |
| `lan` | share installed games over LAN (multicast announce 239.255.71.84:42816, torrent listen 42817), receive into `GamesPath`, register in library | library, settings, storage, platform, history, download/classicio (blank import) | `Share`, `Peer`, `Offer`, `Transfer`, `Stats` | `lanshares.json`, `lan/` (torrents, data) |
| `history`, `playlog` | user-visible operation journal (install/update/remove/move/lan); play sessions (90-day retention) | storage | `Record`, `Kind`; `Session` | `history.json`, `playlog.json` |
| `theme`, `legal`, `tray`, `autostart`, `shortcut` | user themes + revert; embedded legal docs; tray controller; autostart (registry / LaunchAgent plist); desktop shortcuts (`.lnk` COM / `.app` bundle) | storage, settings | `Theme`, `Document`, `Controller`, `Link` | `themes.json`; `~/Library/LaunchAgents/<label>.plist` |
| `devmock` | build flag `Enabled`, fake process registry | storage | `Entry`, `Process` | `devmock-processes.json` (or `TYPHON_DEVMOCK_STATE_DIR`) |
| `installguard`, `exepicker` | Windows helpers used by install (installer window guard) and app (native picker) | — | — | — |
| `uierr`, `version`, `idle` | error codes for UI (`typhon:<code>` parsed by `frontend/src/lib/i18n/errors.ts`); version parsing/compare (`Parse`, `Key`, `Newer`); idle time | — | `Error`; `Version`, `Kind` | — |

## 4. Wiring in `main.go`

Order of construction matters only where a constructor takes another service; callbacks are set after all constructors. Fatal at startup (`main.go::fatal` → exit 1): any `NewService` error, `titles.Ready`, `settings.ConfigDir`, and any `ServiceStartup` error (Wails returns it from `Run`). Logged and continued: `app.InitLogging` failure, `clientid.Load` failure (then heartbeat/usagestats/diagnostics are not started), compat sharer failure, autostart/tray apply failure, `--play` parse failure.

| Producer → Consumer | Mechanism |
|---|---|
| download completed/started/gone → install | `Manager.SetOnCompleted(install.HandleDownloadCompleted)`, `SetOnStarted(HandleDownloadStarted)`, `SetOnGone(DropBroker)` |
| install finished → updates | `install.SetOnFinished(updates.HandleInstallFinished)`; `install.SetBusyCheck(updates.Busy)` |
| sources changed → updates | `sources.SetOnChanged(updates.HandleSourcesRefreshed)` |
| game session ended → updates | `library.SetOnSessionEnded(updates.HandleSessionEnded)` |
| session start/stop → presence, online, heartbeat | `library.AddSessionWatcher(...)` (`SessionWatcher` interface, called under library lock — keep implementations non-blocking) |
| session outcome → compat, playlog | `SetOutcomeRecorder`, `SetLaunchFailureRecorder`, `SetPlayRecorder` |
| everything → history / usagestats | `Set{History,Usage}Recorder` on library, download, install, updates, lan, relocate |
| titles for installs | `install.SetTitleResolver`, `install.SetRepackerResolver`, `library.SyncTitles` (catalog title else release title) |
| catalog ↔ compat | `catalog.SetCompatLookup(compatStats.Game)` — called under catalog mutex, must not call back into catalog |
| catalog ↔ metadata provider | `provider.(catalog.RemoteCatalog)` → `catalog.SetRemoteCatalog` (uncommitted) |
| settings → autostart/tray | `settings.AddApplier` (runs before persist, error aborts save); `settings.Subscribe` for presence/usagestats/diagnostics/online (after persist) |
| account token → accountsync, social, online, typhonapi, titlesdict | `accountService.SessionToken` passed as `func() (string, error)`; `account.BaseURL()` everywhere |
| adapters | `sync_adapters.go`: `syncSettings`, `socialSettings`, `syncLibrary`, `syncCatalog`, `syncMetadata` |

## 5. Events (Go → UI)

Registered in `main.go::init` (typed) — families and their frontend consumers (`frontend/src/lib/stores/*.ts` unless noted):

| Family | Events | Consumer |
|---|---|---|
| settings | `settings:updated` | `settings.ts` |
| library | `library:updated`, `game:started`, `game:stopped` | `library.ts` |
| download | `download:{added,updated,progress,completed,failed,removed,degraded}` (`progress` is a slim `ProgressUpdate` every 250 ms) | `downloads.ts`, `degraded.ts` |
| install | `install:{started,updated,completed,failed,cancelled,removed}` | `install.ts` |
| sources | `source:{updated,error,degraded}`, `release:{added,removed,matched,needs-review}` | `sources.ts` |
| metadata | `metadata:updated` | `metadata.ts`, `routes/game/GameDetails.svelte` |
| discovery | `discovery:{started,progress,completed}` | `discovery.ts` |
| updates | `update:{available,started,updated,completed,failed,rollback,degraded}`, `verify:*`, `repair:*` | `updates.ts` |
| relocate | `move:{started,progress,completed,failed,cancelled,degraded}` | `relocate.ts` |
| lan | `lan:{peers,shares,hashing,transfer,stats}` | `lan.ts` |
| history/playlog | `history:{recorded,updated,degraded}`, `playlog:{recorded,degraded}` | `history.ts`, `degraded.ts` |
| theme | `theme:{updated,list,reverted}` | `theme.ts` |
| selfupdate | `launcher:update_status`, `launcher:update_progress`, `launcher:release_notes` | `selfupdate.ts` |
| social/online | `social:friends`, `social:requests` (consts `social.EventFriends/EventRequests`), `presence:status` | `social.ts`, `presence.ts` |
| diagnostics | log upload status (`diagnostics/logsupload.go::emitLogUploadStatus`) | `services/logsUpload.ts` |

`*:degraded` carries `{degraded, message}`: a store failed to persist, memory was rolled back, the service keeps running read-only-ish; the UI toasts once per subsystem (`degraded.ts`).

## 6. Persistent state

The default state root is `ConfigDir` = `os.UserConfigDir()/Typhon` (`%AppData%\Typhon`, `~/Library/Application Support/Typhon`; legacy `Aurora` dir is renamed once by `settings.ConfigDir`). Game data lives under the user-chosen library root: `<parent>/TyphonLibrary/{Games,Downloads,Screenshots}` (`settings.derivePaths`). A devmock build accepts an absolute `TYPHON_TEST_DATA_DIR` override; without it the default root is shared with native builds (`settings/configdir_devmock.go`). See DEBUGGING for isolation limits. Two write styles:

- **Envelope** `{"version":N,"data":...}` via `storage.Save/Load` with migrations: updates, relocate, sources, catalog, metadata, selfupdate, history, playlog, theme, accountsync, account guest/profile, lan shares, library marker, diagnostics pending.
- **Raw JSON** via `storage.WriteAtomic`: settings, library, library-excluded, downloads, installations, removals, compat, clientid, wine bottle marker, media files.

Required semantics for authoritative state (I-2): `fs.ErrNotExist` → start empty; any other error → constructor or `ServiceStartup` fails and the launcher exits (Wails aborts `Run` on a service startup error → `main.go::fatal`). Corrupt torrent piece-completion cache is instead quarantined and rebuilt (`download/completion.go`). Runtime write-failure handling is owner-specific: several services roll memory back and emit `*:degraded`; background completion/failure paths have explicit exceptions (I-3). `os.WriteFile` in production code outside `internal/storage` is forbidden by lint; tests have an explicit exemption.

Full table of files with owners: `DEBUGGING.md` §State on disk. Format details per store: `CODEMAP.md`.

## 7. Background work

| Owner | Loop | Cadence / trigger | Stop |
|---|---|---|---|
| `library` (`watch.go`) | process detection tick | 1 s (`defaultWatchInterval`), only if `procs.Supported()` | `ServiceShutdown` cancels ctx, waits `wg`; game `cmd.Wait` goroutines (`sessionWG`) are deliberately not waited — games outlive the launcher |
| `download` | engine tick, per-torrent watchers | 250 ms progress | `ServiceShutdown` (see CODEMAP) |
| `install` | per-job goroutine; worker poll | per job | `Cancel`/`ServiceShutdown` |
| `updates` | per-update job; re-check on sources change / session end | event-driven | `ServiceShutdown` |
| `relocate` | per-job copy goroutine | per job | `Cancel`/`ServiceShutdown` |
| `sources` | `scheduleLoop` | tick 1 min, refresh per `SourceRefreshInterval` (manual/1h/6h/12h/24h), backoff up to 1 h after failures | `ServiceShutdown` |
| `metadata` | art batch workers, attempts flush | on demand, budget-paced | `ServiceShutdown` |
| `selfupdate` | `periodicCheck` | 15 min (`checkInterval`); after a failed check backoff from 30 s (`checkRetryBase`) doubling up to the interval | `ServiceShutdown` |
| `accountsync` | `schedule` | 5 min, nudge 3 s after library change, min gap 30 s | `ServiceShutdown` |
| `social` | poll friends/requests | 60 s | `ServiceShutdown` |
| `online`, `heartbeat` | presence report | 30 s; away after 10 min idle (`TYPHON_DEVMOCK_AWAY_SECONDS` in devmock) | `ServiceShutdown` sends disconnect (2 s budget) |
| `usagestats`, `diagnostics` | flush queues | 25 s / 20 s, 3 s timeout | `ServiceShutdown` flushes |
| `compat.Sharer` | report / stats | first after 2 min then 10 min; stats 6 h | `ServiceShutdown` |
| `lan` | announce / receive loops, transfer watchers | announce 10 s, peer TTL 35 s | disable on setting off, `ServiceShutdown` |
| `titlesdict` | remote dictionary refresh | 24 h | `ServiceShutdown` |
| `discord` | IPC connection | on enable | `ServiceShutdown` |

Rules: every goroutine is in an owner `wg` and observes `ctx`; `context.Background()` only in `main` and tests (forbidigo); blocking functions take `ctx` first.

## 8. External integrations

| Integration | Where | Notes |
|---|---|---|
| Backend API: `account.BaseURL()` = `https://api.typhon-launcher.com` (`TYPHON_API_URL` override, https unless loopback); clients prepend `account.APIPrefix` = `/v1` | `account` (`/auth/{register,login,logout}`, `/me`, `/me/avatar`), `accountsync` (`/me/sync`), `social` (`/me/friends*`, `/me/blocks*`, `/me/friend-code*`, `/me/feed`, `/users/*`, `/games/<igdb>/friends`, `/activity/<id>/{reactions,note}`), `online` (`/me/presence`), `heartbeat` (`/presence/heartbeat`, `/presence/disconnect`), `usagestats` (`/telemetry/events`), `diagnostics` (`/diagnostics/errors`, `/diagnostics/logs`), `compat` (`/compat/reports`, `/compat/stats`), `metadata/typhonapi` (`/metadata/search`, `/metadata/games/<id>`, `/metadata/resolve`, `/catalog/games` for remote browse), `titlesdict` (`/v1/titles-dict` literal). Exception: `selfupdate` fetches `<BaseURL>/launcher/manifest` without the prefix (`selfupdate/model.go::ManifestPath`) | every client: own `http.Client` with timeout + `account.CheckRedirect` (re-validates scheme/host, strips `Authorization` on host change); bodies via `io.LimitReader`; `User-Agent: Typhon/<ver> (<os>/<arch>)`, `X-Typhon-Version` |
| Release feeds (user-added URLs / files) | `sources/feed` | JSON only, `limits.go` (64 MiB, 100k entries, 60 s, 5 redirects); `guard.go` blocks loopback/private/link-local hosts incl. after DNS and on redirect; Cloudflare challenge detected as distinct error |
| BitTorrent | `download` (anacrolix/torrent, listen port 42815), `lan` (42817) | DHT/trackers per anacrolix defaults; metainfo size-limited |
| CrossOver (macOS) | `wine` (`cxstart`, `cxbottle`, `wineserver`, `ps`) | bottles dir `~/Library/Application Support/CrossOver/Bottles`; template `win10_64`; shared bottle `Steam` (`TYPHON_STEAM_BOTTLE`) |
| OS credential stores | `account/credential_{windows,darwin,devmock}.go` | Windows Credential Manager; macOS Keychain via `/usr/bin/security` (secret over stdin, base64); devmock file; `TYPHON_API_TOKEN` env store for dev |
| Discord | `discord` (IPC) | client id in `main.go::discordClientID` |
| Autostart | `autostart_darwin.go` LaunchAgent plist in `~/Library/LaunchAgents`; Windows via Wails autostart fallback (`ForPlatform`) | |
| Shortcuts | `shortcut_windows.go` (IShellLink COM), `shortcut_darwin.go` (`.app` bundle with launcher script → `typhon --play <id>`) | |

## 9. Trust boundaries

- **Feed content** (titles, versions, URIs, sizes) is untrusted: parsed with limits, sanitized before UI (`catalog/sanitize.go`, `feed` limits), private-network fetches refused.
- **Torrent/archive contents** are untrusted: path traversal checks in `install` archive readers and in `selfupdate/bundle_darwin.go::safeJoin`; metainfo parsing size-limited.
- **Self-update manifest** must verify ed25519 signature against the embedded key (`selfupdate/sign.go`, `key_default.go`; `TYPHON_DEVMOCK_RELEASE_PUBKEY` only in devmock) and the artifact SHA-256 before apply (`VerifyFile`, re-verified at startup).
- **Elevation boundary** (Windows): the unprivileged launcher and the elevated worker talk through files (`worker-<id>-*` spec/state/cancel, broker `pin.json`). `AUDIT_FABLE_5_1.md` (untracked, 2026-09-10) notes the broker executes any spec placed by the same user (TYPHON-AUDIT-010).
- **Outbound data**: gated by consent switches — `UsageStatsAllowed`, `DiagnosticsAllowed` (only after `TelemetryConsentVersion == CurrentTelemetryConsent`), `CompatReportsAllowed`, `AccountSync`. Diagnostics and log uploads pass `redact` (hostname/username from `redact.SetLocal`, paths, URLs). Bearer transport requires https, with the explicit loopback development exception in `account.ValidateBaseURL`.
- **Destructive filesystem ops** are limited to: install destinations owned by Typhon (`Owned` / `InstallType`), download data roots computed from real layout, `.previous/.staging` update dirs, relocate sources after verification, bottles Typhon created (marker inside the bottle). Library `RemoveGame` never deletes files; it archives the entry and adds the dir to `library-excluded.json`.

## 10. Frontend in one paragraph

`frontend/src/main.ts` mounts `App.svelte`, which calls every store's `init*()` (each subscribes to its event family and loads initial lists via bindings) and routes by `stores/router.ts` (in-memory stack, no URL routing). Services in `lib/services/*.ts` wrap bindings and short-circuit when `inWails` is false (browser preview). Errors from Go arrive as `typhon:<code>` strings (`uierr`), mapped to messages by `lib/i18n/errors.ts` + per-domain `*Errors.ts` tables (`install/installErrors.ts`, `sources/sourceErrors.ts`, …); Go tests (`TestErrorCodesMatchTheFrontendTable` in `sources`, `metadata`) keep the tables in sync. All confirmations go through `components/ConfirmModal.svelte` + `lib/confirm/prompts.ts` (native `window.confirm` is broken in the macOS WebView). i18n catalogs: `lib/i18n/catalog/{ru,en}/*.ts`, key type `MessageKey` derived from `ru`.
