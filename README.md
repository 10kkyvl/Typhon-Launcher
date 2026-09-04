# Typhon

*Read this in [Russian / по-русски](README.ru.md).*

A game launcher for Windows. Typhon finds the games already on your drives, downloads
and installs new ones, shows their versions and launches everything from a single
library.

**[Download](https://typhon-launcher.com/download/)** · [typhon-launcher.com](https://typhon-launcher.com)
· Windows 10 1809 or newer, 64-bit, with the Microsoft Edge WebView2 Runtime

It is a portable application: the downloaded file runs directly, no installer, and it
keeps its own state in `%AppData%\Typhon`. Verify the SHA-256 printed next to the
download link before running it.

## What Typhon is not

Typhon ships with no games, carries no preinstalled sources and offers no list of them.
It does not host or supply game files. Sources are added by the user, at the user's own
responsibility.

There is one build, for Windows. There are no macOS or Linux versions, and none will
appear until a working build does — the launcher is *developed* on macOS (see
[below](#developing-on-macos-or-linux)), but that is not a build anyone can use.

## What it does

**The library builds itself.** Typhon scans your drives and finds installed games;
anything installed through it lands in the library straight away. Every game shows its
version, size, install date and playtime. You can remove a game from the library
without deleting its files.

**Downloads.** A built-in BitTorrent client with a queue, a speed limit and file
verification. Progress is kept on disk per piece, so closing the launcher mid-download
resumes where it stopped instead of rehashing everything.

**Install and updates.** It unpacks archives, runs installers and remembers which files
it put where. When a source publishes a release newer than the one you have, the game
is flagged in the list; from there you can update, launch or remove it. Updates apply
patch chains one step at a time with a backup of what they replace, and an update
interrupted by a power cut is finished or rolled back on the next launch rather than
leaving a half-written game folder.

**Metadata.** Typhon looks the title up and fills in the cover, description, release
date, genres, developer, publisher and screenshots. Everything it downloads stays in a
local cache and opens without a connection. Game data provided by IGDB; Typhon is not
affiliated with or endorsed by IGDB or Twitch.

**An account, if you want one.** The launcher works fully as a guest. Sign in and you
get a profile with a showcase and playtime, friends by username or friend code, online
status, an activity feed, and your library, marks and settings synced across machines.
Losing the network drops you to a cached profile instead of signing you out.

**The rest.** Discord Rich Presence, sharing an installed game with another machine on
the same network, moving the library to another drive with hash verification, a history
of what the launcher did, themes down to custom CSS, and a signed self-update that
resumes a broken download.

**Russian and English.** The whole interface, both languages, following the system by
default.

## Your data

Your library, sources, downloads and installations are stored on your computer. Magnet
links, infohashes, file lists and download history are never sent to Typhon services.

What leaves the machine: the game title goes to the Typhon metadata service, which looks
the entry up in IGDB. A source feed is fetched directly from the host you gave it,
bypassing Typhon's own servers. While downloading over BitTorrent, your IP address is
visible to other peers in the swarm. Diagnostics and usage statistics are opt-in, and
what was actually sent is visible in the launcher.

Full text: [`PRIVACY.md`](PRIVACY.md), [`TERMS.md`](TERMS.md), [`COPYRIGHT.md`](COPYRIGHT.md)
(each with an `.en.md` translation). The Russian text prevails where a translation
disagrees.

---

# Building it yourself

Everything below is for working on the launcher, not for using it.

## Requirements

| | |
|---|---|
| Go | 1.25.0 |
| Node | 22 |
| Wails CLI | `v3.0.0-beta.10` — `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.10` |

Go + [Wails 3](https://v3.wails.io) on the backend, Svelte 5 on the front, current
version 0.3.1.

```
wails3 task build          # bin/typhon.exe
wails3 task run
wails3 task dev            # hot reload, vite on port 9245
wails3 task package        # production package
```

Cross-compiling from macOS or Linux with `CGO_ENABLED=1` switches to a Docker builder
(`wails3 task setup:docker` prepares the image).

## Developing on macOS or Linux

The `devmock` build tag swaps the Windows-only subsystems — installer runner, game
process execution and detection, credential store, desktop shortcuts — for mocks, so the
whole flow can be exercised off Windows.

```
wails3 task dev:devmock
wails3 task run:devmock
wails3 task test:devmock
```

The tag can never reach a shipped binary: `internal/devmock/forbid_devmock.go` fails the
build outright when `devmock` is combined with `GOOS=windows` or with the `production`
tag, CI asserts both failures, and the release workflow greps the built `.exe` for the
`TYPHON_DEVMOCK_ENABLED` marker.

Mock behaviour is tuned with `TYPHON_DEVMOCK_ELEVATE` (`1` — go through the elevated
worker protocol, `0` — install directly), `TYPHON_DEVMOCK_INSTALL_SECONDS` (default `2`),
`TYPHON_DEVMOCK_GAME_SECONDS` (default `60`), and, for the local self-update server
started by `wails3 task devrelease VERSION=x.y.z`, `TYPHON_DEVMOCK_MANIFEST_URL` and
`TYPHON_DEVMOCK_RELEASE_PUBKEY`.

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
itself. `uierr` attaches a stable code to every error that reaches the interface, so the
frontend maps codes to translated text instead of matching Russian substrings; a test in
each package fails if a code exists on one side and not the other.

## Contributing

`main` is the stable branch, `dev` is where work lands; features branch off `dev` and
reach `main` through a merge.

- **Everything that goes to git and GitHub is written in English** — commit messages,
  branch names, pull request titles and bodies, review comments, issues, tags, release
  notes. Russian stays in the interface, in user-facing error text, and in conversation.
- Commit subject: `type: subject`, lower case, imperative, no trailing period. The body
  wraps at 72 columns and explains *why*, rather than restating the diff.
- The checklist above is the gate. A change is not finished because it looks right — run
  the commands and show what they printed. New code with a `-race` failure, a raised
  lint baseline, or a test marked `t.Skip` to get past it, is not finished either.
- User-visible strings go through the locale layer in both languages, never hardcoded.
  An error that reaches the interface gets a `uierr` code and an entry in both catalogs.
- `CHANGELOG.md` is Russian on purpose: its text goes into the signed update manifest and
  is shown in the launcher under "What's new". The top entry must match `VERSION`.
- The invariants the code is held to — atomic writes, errors that never become default
  values, crash-safe data moves — are written out in `CLAUDE.md`. It is worth reading
  before a first change.

Bugs and questions: [GitHub issues](https://github.com/10kkyvl/Typhon-Launcher/issues).
A launcher log from `%AppData%\Typhon\typhon.log` helps; it is redacted of paths and
account details by design.

## Licence

There is no licence file in this repository yet, so default copyright applies: the source
is here to be read and built, not licensed for reuse or redistribution. Ask before
building on it.

The licences of the components Typhon bundles are in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md), generated from `tools/notices`.
`wails3 task legal:check` verifies a release carries all of the legal documents.

## The rest of the project

- [`typhon-backend`](https://github.com/10kkyvl/typhon-backend) — accounts, catalog, sync, social and the update feed
- [`typhon-site`](https://github.com/10kkyvl/typhon-site) — the website and the download page
