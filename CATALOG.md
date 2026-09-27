# Provider catalog and source releases

The Catalog route calls `catalog.Service.BrowseGames`, which pages the Typhon
backend's Steam/IGDB index. It does not query feed-derived local membership.
Visited official records and pages are cached locally; unavailable cached pages
are labelled offline, and uncached failures expose retry. Details and their
media are loaded on demand through the existing metadata service.

A release retains its raw title, source ID, release ID, distribution ID, version,
repacker, languages and parsed service tags. Importing an unknown release never
calls `Provision`. Unresolved releases remain in source management. A name match
against the incomplete visited-page cache is a review candidate, not a confirmed
identity. Manual confirmation in the server catalog mode applies to the chosen
release only; it does not attach every homonymous release. The match dialog
searches the server catalog, including games without releases.

Official titles and source titles use separate processing. Source markers such
as archive/folder, repack, spaced version prefixes and Windows 7 Fix are parsed
without deleting arbitrary parentheses. Release rows display the raw name so
variants remain distinguishable. Official descriptions, covers and developer
names cannot come from a release.

Provider links and redirect sidecars resolve catalog aliases without changing
installation IDs or paths. Library membership and favorites recognize aliases.
Update selection still requires the saved source/release/distribution line and
revision timestamp. The release service presents confirmed aliases to the
update resolver in the installation's ID space without rewriting the stored
release. A more recent foreign distribution is not an update.

## Offline migration

Use only an explicit directory, with the launcher stopped. Save the report
outside that directory:

```sh
go run ./cmd/catalog-migrate --dir /path/to/offline-copy > /tmp/catalog-report.json
go run ./cmd/catalog-migrate --dir /path/to/offline-copy --apply /tmp/catalog-report.json
go run ./cmd/catalog-migrate --dir /path/to/offline-copy --restore /path/to/offline-copy/catalog-migration-backups/BACKUP_ID
```

The first command reads data without changing it. The report lists confirmed
provider groups, ambiguous names/claims, providerless entries and affected JSON
references. Apply verifies hashes and current provider evidence, creates an
atomic backup, and writes only `catalog-redirects.json`. It does not rewrite the
catalog, library, installation markers, history, downloads, update plans,
sources or favorites. Repeating apply is safe. Restore refuses to overwrite
later redirect changes. Unknown installed games remain in the private library.

The backend's `CATALOG.md` describes provider priority, provider merge correction,
sync checkpoints, completion indicators and the revision pagination contract.
