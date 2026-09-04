# Typhon

*Read this in [Russian / по-русски](README.ru.md).*

A desktop game launcher for Windows: it keeps a library of installed games, downloads
releases from the feeds you add, installs and updates them, tracks how long you play,
and — when you sign in — syncs that library across your machines and shows what your
friends are playing.

Version 0.3.1. Go + [Wails 3](https://v3.wails.io) on the backend, Svelte 5 on the front.

## What it does

**Library and catalog.** A canonical game catalog (IGDB-linked) with cover art and
metadata, search across the library, the catalog and every source at once, and a disk
scan that finds games already installed on the machine and adopts them.

**Sources and downloads.** Release feeds — a JSON list of titles with their magnet or
URI links — added by URL or from a file on disk and matched against the catalog. Downloads run through a bundled BitTorrent
engine with per-piece progress kept on disk, so a restart resumes instead of
rehashing everything.

**Install and update.** Silent installers driven through an elevated worker, archive
extraction, desktop shortcuts, uninstall. Updates go through a version graph with
patch chains, every intermediate version registered before the next patch starts, and
a rollback that finishes on the next launch if the machine died mid-update.

**Account and social.** Sign in, or stay a guest — the launcher works either way, and
losing the network drops you to a cached profile instead of signing you out. With an
account: profile with a showcase and play time, friends by username or friend code,
presence, an activity feed, and cross-device sync of settings, library and favorites.

**The rest.** Discord Rich Presence, LAN sharing between machines on the same network,
moving the library between drives with hash verification, play history, themes with
custom CSS, opt-in diagnostics and usage stats, and a signed self-update that resumes
a broken download.

**Two languages.** The whole interface is Russian and English; the language follows
the system by default and is stored in settings, not in the browser.

## Requirements

| | |
|---|---|
| Runtime | Windows 10 version 1809 or newer, 64-bit, with the Microsoft Edge WebView2 Runtime |
| Go | 1.25.0 |
| Node | 22 |
| Wails CLI | `v3.0.0-beta.10` — `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.10` |

## Build and run

```
wails3 task build          # bin/typhon.exe
wails3 task run
wails3 task dev            # hot reload, vite on port 9245
wails3 task package        # production package
```

Cross-compiling from macOS or Linux with `CGO_ENABLED=1` switches to a Docker builder
(`wails3 task setup:docker` prepares the image).

### Working on macOS or Linux

The launcher ships for Windows only, but it is developed elsewhere: the `devmock`
build tag swaps the Windows-only subsystems — installer runner, game process
execution and detection, credential store, desktop shortcuts — for mocks, so the whole
flow can be exercised on a Mac.

```
wails3 task dev:devmock
wails3 task run:devmock
wails3 task test:devmock
```

The tag can never reach a shipped binary: `internal/devmock/forbid_devmock.go` fails
the build outright when `devmock` is combined with `GOOS=windows` or with the
`production` tag, CI asserts both failures, and the release workflow greps the built
`.exe` for the `TYPHON_DEVMOCK_ENABLED` marker.

Mock behaviour is tuned with `TYPHON_DEVMOCK_ELEVATE` (`1` — go through the elevated
worker protocol, `0` — install directly), `TYPHON_DEVMOCK_INSTALL_SECONDS` (default
`2`), `TYPHON_DEVMOCK_GAME_SECONDS` (default `60`), and, for the local self-update
server started by `wails3 task devrelease VERSION=x.y.z`,
`TYPHON_DEVMOCK_MANIFEST_URL` and `TYPHON_DEVMOCK_RELEASE_PUBKEY`.

## Configuration

| Variable | Meaning |
|---|---|
| `TYPHON_API_URL` | Backend base URL. Default `https://api.typhon-launcher.com`. Must be `https`; plain `http` is accepted only for `127.0.0.1` and `localhost`, and the bearer token is never sent over an unencrypted connection. |
| `TYPHON_API_TOKEN` | A session token to use instead of the OS credential store. For development. |

On disk:

| | |
|---|---|
| Config directory | `%AppData%\Typhon` (`os.UserConfigDir()/Typhon`) |
| Settings | `<config>/settings.json` |
| Account state | `<config>/account.json` |
| Log | `<config>/typhon.log`, rotated at 10 MiB, five backups |
| Library | `<folder you pick>/TyphonLibrary`, with `Games`, `Downloads` and `Screenshots` inside |

There is no default library path: nothing is written until you pick a parent folder in
the launcher.

## Tests and checks

The full set CI runs — everything here has to pass before a change is finished:

```
gofmt -l .                                   # must print nothing
go vet . ./internal/...
go build . ./internal/...
GOOS=windows CGO_ENABLED=0 go build . ./internal/...
go test ./internal/...
CGO_ENABLED=1 go test -race ./internal/...
CGO_ENABLED=1 go test -race -tags devmock ./internal/...
golangci-lint run --new-from-rev=origin/dev --whole-files=false ./...
go run ./tools/lintbaseline                  # and: -tags devmock
```

New and changed code must be clean; the whole module is measured against the per-OS
baseline in `.github/lint-baseline.txt` (keyed by `GOOS`, or `GOOS+tags`), and that
number may only go down. `golangci-lint` is pinned to v2.13.1 —
`wails3 task lint:install` puts it in place.

Frontend, from `frontend/`:

```
npm ci
npm run check       # svelte-check
npm run test        # vitest
npm run build
```

Bindings between Go and the frontend are generated:
`wails3 task common:generate:bindings`.

## Layout

```
main.go            wiring: every service is constructed and bound here
internal/          the launcher itself, one package per subsystem
frontend/          Svelte 5 + Vite; src/routes is one directory per screen
frontend/src/lib/i18n    the locale layer and the ru/en message catalogs
build/             per-OS Taskfiles, icons, packaging config
tools/             devrelease, lintbaseline, third-party notices generator
cmd/signrelease    signs an update manifest with the release key
```

`internal/` by subsystem:

| Area | Packages |
|---|---|
| Games | `catalog`, `sources`, `download`, `install`, `library`, `discovery`, `updates`, `metadata`, `relocate`, `lan`, `search`, `hashdir`, `titles`, `shortcut`, `playlog`, `history` |
| Account | `account`, `accountsync`, `social`, `online`, `presence`, `discord`, `heartbeat`, `profile` |
| Updating itself | `selfupdate` |
| Windows | `platform`, `autostart`, `tray`, `procs`, `devmock` |
| Infrastructure | `settings`, `storage`, `redact`, `uierr`, `diagnostics`, `usagestats`, `telemetrylog`, `theme`, `clientid`, `legal`, `version` |

Two of those are load-bearing rules rather than features. `storage` holds the single
atomic-write primitive the whole repository uses — no package writes state files by
itself. `uierr` attaches a stable code to every error that reaches the interface, so
the frontend maps codes to translated text instead of matching Russian substrings; a
test in each package fails if a code exists on one side and not the other.

## The rest of the project

- [`typhon-backend`](https://github.com/10kkyvl/typhon-backend) — accounts, catalog, sync, social and the update feed
- [`typhon-site`](https://github.com/10kkyvl/typhon-site) — the website and the download page

## Legal

`TERMS.md`, `PRIVACY.md` and `COPYRIGHT.md` (with `.en.md` translations) ship with the
launcher and are shown inside it; `THIRD_PARTY_NOTICES.md` is generated from
`tools/notices`. The Russian text prevails where a translation disagrees.
`wails3 task legal:check` verifies a release carries all of it.
