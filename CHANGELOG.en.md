# Changelog

The English mirror of `CHANGELOG.md`. The Russian file is the one that feeds
the signed update manifest and the "What's new" screen inside the launcher;
this file carries the same entries for the GitHub releases. It starts at
0.4.0, the release that introduced the English interface — for anything older
see `CHANGELOG.md`. Sections: "Added", "Changed", "Fixed", "Removed".

## 0.5.0 — 2026-09-08
The launcher runs on macOS, downloading is told apart from installing, and games stop splitting across the catalogue or offering each other the wrong update.

### Added
- Typhon runs on macOS (Apple Silicon Macs): Windows games are installed and launched through CrossOver, and the launcher builds a separate environment for each game. CrossOver itself is installed separately — About shows its version, or warns that it is missing
- On macOS a game can go to the desktop as a shortcut, and the launcher can start at login
- A compatibility journal keeps itself: after a game fails to start twice in a row it is marked "does not start" in Installed, and a minute of play clears the mark
- On macOS the catalogue shows how many people the game starts for, and a "Only ones that start" filter. The number appears once at least five different machines have reported on the game
- A compatibility report is sent by consent only: the game, the build version and repacker, the macOS version, the CrossOver version and the chip family — no free-form text, no release names, no paths. Consent is asked again, because the previous prompt promised nothing about hardware would be collected
- A "Download" button separate from "Install": with nothing on disk the game page offers to download, and install appears once there is nothing left to fetch
- An "Install after download" checkbox in the download window: it starts from the setting but applies to that one download
- Administrator rights can be confirmed up front, before the download starts, when the torrent carries an installer: the install then runs overnight on its own instead of waiting for a Windows prompt
- The source preview says how many games a feed describes, how many of them the catalogue already has, and how many are new
- A source served over plain http is flagged with a shield in the list and a warning in the preview: its contents can be swapped on the way
- File verification shows up in the activity dock — its progress used to live on the game page only, so leaving the page hid a scan that keeps running
- The release list carries the build form — "Portable", "Repack", "Archive" — and the repacker is written next to the source the release came from
- A "Send to us" button in About: the log archive goes to support and is kept there for three days, and the launcher shows the ticket number. Before sending, the window says what is inside — your system user name, folder paths, and the names of games and torrents
- Unfriending, blocking and discarding a downloaded file ask for confirmation: they used to fire straight from the context menu

### Changed
- The interface is a tenth larger: what the 110% scale used to draw is now what 100% draws, and the other steps moved with it
- Built-in theme names follow the interface language instead of staying Russian in English
- The repacker and edition lists moved into a dictionary that refreshes from the server once a day: a new repacker is recognised without a new launcher version, and your own list can go into a file in the config folder
- Sources refresh faster: releases the feed did not touch are not matched again, and saving a source no longer downloads the feed a second time after the preview
- A running game is noticed, and its session closed, within a second — it used to take up to ten
- The avatar goes to the server as the file you picked, and the square is cut there: the picture is no longer resampled twice on the way
- The activity feed and the profile show a game as a wide shot instead of a cropped portrait cover
- The launcher uses less memory on large catalogues and sources: the game list is not held twice, and releases that vanished from a feed no longer pile up without a ceiling

### Fixed
- A game removed from the library on one device came back from another on the next sync: the removal now travels to the server and reaches the other devices, even if there was no network at the moment it was removed
- One release could split across the catalogue into eight different games: in "Game v.1.0.29315 [Папка игры]" the version and the brackets stayed in the name, so every entry counted as its own game
- A 1.8 GB repack was offered as an update to a 9.8 GB installation when both were published on the same day and neither version could be read
- An install started from a release row lost its version, and updates for that game were never found afterwards
- The release list labelled "Update" even a release the launcher does not treat as one: those read "New release"
- A torrent is no longer matched automatically to a DLC instead of the game itself; picking a DLC by hand still works
- Confirmations for deleting a theme, clearing history and cancelling a download never appeared at all on macOS, and the action silently did nothing
- Closing the add-download window while it says "Fetching details" no longer holds the torrent busy: it can be added again right away instead of a minute and a half later
- Online status disappeared until the launcher was restarted if the server stopped answering the presence request for a minute
- Errors are shown in the interface language: a technical server reply could turn up where readable text belongs — in search, in the library folder setup, and when updating or removing a game
- Percentages disagreed: 4% on the game card, 3% in the activity dock. One rounding rule for the whole interface, rounding down — 100% is not shown until the work is done
- Auto-install started even when the disk had no free space left at all
- The local network transfer list never cleared finished transfers, and a failing offer poll repeated the same error message every ten seconds

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
