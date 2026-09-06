# Changelog

The English mirror of `CHANGELOG.md`. The Russian file is the one that feeds
the signed update manifest and the "What's new" screen inside the launcher;
this file carries the same entries for the GitHub releases. It starts at
0.4.0, the release that introduced the English interface — for anything older
see `CHANGELOG.md`. Sections: "Added", "Changed", "Fixed", "Removed".

## 0.4.1 — 2026-09-06
Update and download reliability: a snapshot of saves before an update, a way back after a patch chain, and state that used to be lost without a word.

### Added
- A snapshot of saves before an update: the "Save backup" switch finally does what it says — the copy is taken before the first write, when the saves folder is known, and it lives in the launcher's data folder rather than next to the installation the update overwrites
- A rollback to the pre-update version after a patch chain: until now a rollback was only offered after a full reinstall

### Changed
- The update card no longer always reads "Save backup: unavailable" — the line now reflects whether a snapshot will actually be taken
- Required disk space accounts for the backup copy the update makes before writing
- An update that could not take the saves snapshot stops before its first write instead of going ahead without one

### Fixed
- Two downloads could share one torrent: the busy check and the add were not under one lock, so a live download of another task was cut off, pause quietly stopped the wrong one, and files landed in the wrong folder
- A failed state write is no longer lost silently: the download queue, update state, sources, play log and move journal roll back to what actually reached the disk, and the subsystem reports itself as degraded in the interface
- Background work that writes state or deletes files can no longer run after the application exits
- Uploading continued after the switch was turned off: failed and paused downloads ignored it
- Installation no longer fails because of the moment the installer swaps its own state file — the read is retried instead of aborting the install
- An interrupted patch chain now names the patch it stopped on
- Files extracted from archives are flushed to disk like every other write; the install check no longer reports an unreadable folder as an empty installation
- Folders open through an absolute path to the system program instead of going through PATH

## 0.4.0 — 2026-09-05
Accounts, friends and the activity feed, an English interface and a rebuilt UI.

### Added
- Player profile: playtime, a monthly summary, a showcase of favourite games and privacy settings
- Five completion statuses instead of a single "completed" mark, each with the date it was set
- A local session log: playtime is counted on the device and feeds the profile
- Friends: friend codes, search by name, requests and profile visibility settings
- Online status of friends and what they are playing, behind its own visibility switch
- An activity feed with reactions and notes under your own events
- Games popular with your friends and a "your week" summary on the activity page
- Sync between devices: settings, the catalog game list, last played date, playtime, favourites and completion statuses
- English interface and English versions of the legal documents

### Changed
- Rebuilt interface: library, catalog, installed games, downloads, friends, feed, profile, game page and settings
- Refreshed colours, spacing and typography
- The library syncs right after it changes instead of waiting for the timer
- Sync no longer sends an empty request when nothing has changed
- The friend code is shown on the friends page only, not in the profile header

### Fixed
- A source behind a Cloudflare challenge now says so instead of returning an opaque 403
- The profile showcase shows the real game cover
- The profile page uses the full width of wide windows and no longer keeps an empty column
- Buttons with an icon no longer sit lower than plain ones
- Presence stops polling a server that has no such API and backs off when rate limited
- Backend errors are translated by their code rather than by the text the server returns
