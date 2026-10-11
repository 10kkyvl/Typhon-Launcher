# Contributing to Typhon

Bug reports, fixes and translations are welcome. Before larger changes, open an issue
first so the approach can be agreed on before you spend time on it.

## Ground rules

- **Typhon ships no content and no sources.** Pull requests that add built-in feeds,
  trackers, mirrors, "recommended" or "popular" source lists, or that send source
  addresses, links or file names to the Typhon services will not be accepted. The same
  goes for text that explains how to get around a site's protection, an antivirus or
  SmartScreen.
- User-facing text stays neutral: "build", "release", "build installer" rather than names
  of release groups or packagers.
- By contributing you agree that your work is released under the
  [GNU GPL version 3](LICENSE), the licence of the project. Sign off every commit
  (`git commit -s`) to certify the
  [Developer Certificate of Origin](https://developercertificate.org/).

## Branches and commits

`main` is the stable branch, `dev` is where work lands. Branch off `dev` and open the pull
request against `dev`.

Everything that goes to git and GitHub is written in English: commit messages, branch
names, pull requests, review comments, issues. Russian stays in the interface and in
user-facing error text.

Commit subject: `type: subject` — lower case, imperative, no trailing period, for example
`fix(install): keep the backup until the copy is verified`. The body wraps at 72 columns
and explains why, not what the diff already shows.

## Code

The launcher holds user data — libraries, downloads, installed games — so the rules are
mostly about not losing it.

- **An error never turns into a default value.** No `return nil` after logging, no
  `if err == nil { x = v }` that leaves a zero value behind, no `_ = f.Close()` without a
  one-line reason. Return the error, or return a result with an explicit `Unknown` flag.
- **State files are written only through `storage.WriteAtomic`.** Loading distinguishes
  "file does not exist" from every other error; after a failed load the service refuses
  to save instead of overwriting the file with an empty state.
- **In-memory and on-disk state change together.** Change memory, persist, and roll memory
  back if persisting fails. Persist errors reach the interface.
- **Moving data must survive a crash.** Any operation built from more than one rename
  needs recovery at service startup, and a source is deleted only after the copy is
  synced and verified by hash.
- **Blocking functions take `ctx` first,** every goroutine is tracked and stops on `ctx`,
  and `context.Background()` appears only in `main` and tests.
- **Everything from outside is validated before use:** feed entries, archive entry names,
  manifest paths, values from the UI. HTTP clients have a timeout and re-check every
  redirect.
- No comments except a short "why" for a non-obvious workaround or OS-specific behaviour.
  No `TODO`, commented-out code or new dependencies without discussion.

User-visible strings go through the locale layer (`frontend/src/lib/i18n`) in both
languages. An error that reaches the interface gets a `uierr` code and an entry in both
catalogs.

`CHANGELOG.md` is Russian on purpose: its text goes into the signed update manifest and
is shown in the launcher under "What's new". `CHANGELOG.en.md` mirrors it.

## Tests and checks

Every fixed bug comes with a test that fails before the fix — table-driven, using
`t.TempDir()`, without `time.Sleep`. Cover the negative cases: missing file, no
permission, corrupt JSON, cancelled context, empty input.

Run before opening a pull request:

```sh
gofmt -l .
go vet ./...
go build . ./internal/...
GOOS=windows go build . ./internal/...
go test ./internal/...
CGO_ENABLED=1 go test -race ./internal/...
golangci-lint run --new-from-rev=origin/dev ./...
```

and, from `frontend/`, `npm run check`, `npm test` and `npm run build`. `-race` needs cgo
and gcc (MinGW-w64 on Windows). New and changed code must be lint-clean, and
`.github/lint-baseline.txt` may only go down. Do not add `//nolint`, `t.Skip` or relaxed
linter settings to get a green build.

## Reporting a security issue

Do not open a public issue for vulnerabilities — see [SECURITY.md](SECURITY.md).
