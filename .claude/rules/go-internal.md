---
paths:
  - "internal/**/*.go"
  - "*.go"
  - "cmd/**/*.go"
---

# Go: what to check by type of change

Invariants are in `docs/INVARIANTS.md`. The table says which ones a change touches and how to prove
it. "Proof" = a named test (existing, or new and failing without the change) or a live run.

| Change | Invariants | Proof |
|---|---|---|
| State field, `load`/`persist`, new file under ConfigDir | I-1, I-2, I-3 | `store_integrity_test`: corrupt file → startup fails, bytes untouched; persist failure → memory rolled back, `*:degraded` emitted |
| Any `os.Rename`/`RemoveAll` on user data | I-4, I-5, I-10 | On-disk state test after each step ("killed between X and Y" → startup recovery); delete only after hash verification; path through `removableDir`/journal |
| Goroutine, timer, polling loop | I-6 | `wg.Add` under the same mutex as `closing`; ctx derived from the service; `leakcheck`/shutdown test |
| Check + act (busy, hash, dir) | I-11, I-12 | Reserve under the lock, I/O outside, record the result under the lock; no `WalkDir`/`Stat`/`emit` under `mu` |
| `download.Manager` fields or locks | I-11, I-12 | Lock order: `netMu` before `mu`, `saveMu` before `passMu`, never the reverse. Do not add mutexes or fields to `Manager`; a new subsystem gets its own struct with its own lock. Test with a timeout that fails on deadlock. |
| External input: feed, torrent, archive, UI path, URL | I-7 | `validateInfo`/`safeJoin`/`feed/guard`; a new `http.Client` only with `account.CheckRedirect` and a body limit |
| Any outgoing connection to an address taken from external data (feed, torrent tracker, webseed, metadata URL, peer-supplied host) — not only `http.Client` | I-7 | Resolved address is checked against private/loopback/link-local ranges at dial time (reuse the guard from `sources/feed/guard.go`). BitTorrent peers are exempt (LAN transfer needs them). Test: a target on `127.0.0.1` is refused, a public one is not. |
| Error, new `uierr` code | I-15 | Code as a constant, wrapped with `%w`, entry in the TS table; the package `uicodes_test` sees the file |
| Go → UI event | — | Constant in the owning package, `RegisterEvent` in `main.go` from it, TS subscription with a payload type |
| Exported service method | — | `wails3 task common:generate:bindings`; `bindingsContract.test.ts` |
| Build-tagged file | I-16 | Canonical tag form (copy a neighbour); `GOOS=windows go vet`, `-tags devmock` |
| Import of another domain package | ARCHITECTURE §2 | Coupling via a callback in `main.go`; `download` does not import `install` (and the `library` import stays confined to `playthrottle.go`) |
| New package under `internal/`, new feature or new user-facing screen | — | Not without explicit approval from the user in the current task. Propose first, build after. |

Forbidden without an explicit user decision:
- A new test seam in a production struct (a function field that only tests set).
- `time.Sleep` in production or tests; `context.Background()` outside `main` and tests;
  `os.WriteFile` outside `internal/storage`.
- Russian prose inside a coded error (the frontend translates codes, not sentences).
- Splitting a file or function inside a fix or feature commit. A split is a separate, behaviour-free
  commit made before the change that needs it.
- Tests that lock a struct's mutex or write its private fields. Tests use the public API and the
  package's test helpers; if a helper is missing, add it to the helpers file.
