# Changelog

The English mirror of `CHANGELOG.md`. The Russian file is the one that feeds
the signed update manifest and the "What's new" screen inside the launcher;
this file carries the same entries for the GitHub releases. It starts at
0.4.0, the release that introduced the English interface — for anything older
see `CHANGELOG.md`. Sections: "Added", "Changed", "Fixed", "Removed".

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
