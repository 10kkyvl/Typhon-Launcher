# Typhon Privacy Policy

Revision of 4 September 2026.

This document describes what data Typhon keeps on the user's device and what it sends outside. It describes the actual behavior of the current version of the app, not intentions.

Typhon is developed and maintained by an independent developer (the "Typhon project operator"). The operator is responsible for data processing in the Typhon server services: the account service, the metadata service and the distribution of app updates.

The operator does not control the BitTorrent network, DHT nodes, peer exchange (PEX), third-party sources added by the user, or third-party services. What happens to data there is described in section 7, but the operator cannot influence it.

Saying "we collect no data at all" would be untrue, so what follows is a breakdown: what never leaves the device, what goes to the Typhon services, and what goes to third parties.

This is a translation of the Russian original. In case of any discrepancy, the Russian text prevails.

## 1. Data that stays on the device

The following data is stored locally only and is not sent to the Typhon services:

- **Release sources** — the addresses of the sources added and the contents of their feeds. A feed is requested directly from the source host; the connection runs from the user's device with no Typhon servers in between.
- **Downloads** — magnet links, infohashes, torrent file lists, the contents of downloaded files, and download history and state.
- **Installations** — installation paths, the set of installed files, the history of installations and removals, and update manifests.
- **Library** — the list of games, paths to executables, and the times and durations of play sessions. If cross-device sync is enabled (section 2.3), some of this — the game's identifier in the external IGDB catalog, the last launch date, playtime, the "favorite" mark and the completion status — is sent to the service; paths, file names and release names are not.
- **App settings**, including BitTorrent parameters. With sync enabled, some settings are sent to the service (section 2.3); directory paths, speed limits and data-collection consents are not.
- **The activity log** `typhon.log` and its previous copies `typhon.log.1` … `typhon.log.5` — these stay on the device and are never sent anywhere automatically in full. The log is size-limited: 10 MB per file and no more than six files, with the oldest overwritten.
- **The play session log** — which game was launched and finished and when, for the last 90 days — is kept on this computer only and is not sent to the server. The "favorite" mark and the completion status are kept on this computer and are sent to the service only when sync is enabled (section 2.3).

These lists, paths and contents do not leave the device. If the user has enabled anonymous usage statistics (section 8.2), only anonymized information about the outcome of an operation is sent — for example, that an installation succeeded and how many seconds it took — but nothing of the above. If the user has enabled anonymous diagnostics (section 8.3), an anonymized error report is sent — but not the log itself and not fragments of it.

Local state is stored in the `%APPDATA%\Typhon` directory, in the files `settings.json`, `account.json`, `profile.json`, `sync.json`, `library.json`, `playlog.json`, `sources.json`, `downloads.json`, `installations.json`, `removals.json`, `catalog.json`, `match_overrides.json`, `installation.json`, and in the `releases/` and `manifests/` subdirectories and the metadata cache.

`installation.json` holds only the installation identifier — a random UUIDv4 created the first time the app is started. It is not derived from any device characteristics and is not personal data in itself; where it is sent is described in section 8.

Deleting that directory deletes all of the local data listed above.

## 2. Data sent to the Typhon services

The service address is set when the app is built. The access token is only ever sent over HTTPS; the sole exception is the local address used during development.

### 2.1. Account

The app can run without an account, in guest mode. In that mode no requests are made to the account service, and only the guest-mode flag is stored locally.

When an account is used, the following is sent to the service:

- on registration — email address, username, display name and password;
- on sign-in — username or email address, and password;
- when working with the profile — requests to read and change the profile, and to upload and delete the avatar.

Stored on the server are: the account identifier, username, display name, email address, a link to the avatar file, and the record's creation and modification dates. The password is not stored in its original form, but as a hash. For each session, a hash of the token, the creation time, the expiry and a revocation flag are stored. An uploaded avatar is stored as a file in object storage and is available at a direct link to anyone who knows that link. The account also stores profile settings: which profile blocks to show to other users and which showcases are selected.

The session token is stored on the device in the Windows credential store (Credential Manager), not in the settings file.

### 2.2. Game metadata

To show cover art, descriptions and release years, the app queries the metadata service:

- search by game title;
- resolution of a list of titles into game identifiers;
- retrieval of a game card by identifier.

Metadata is available without an account too: in guest mode these requests are made in the same way, simply without a token.

Points worth understanding:

- **The game title is sent to the service.** When already-installed games are detected automatically, the title is derived from **the folder name on disk**. If the folder has an arbitrary name, that arbitrary name is what gets sent as the search query.
- **Metadata requests made from an account carry its token.** This means the service is technically able to link an account with which game titles were requested. Requests made in guest mode carry no token.
- The metadata service is **not** sent: source addresses, feed contents, magnet links, infohashes, download contents, download history or installation paths.

### 2.3. Cross-device sync

Sync is off by default and is enabled manually in the app settings. While it is off, no requests are made to this part of the service at all.

When it is on, the following is sent to the service and stored on the server:

- **some app settings** — theme, interface scale, animations, minimize to tray, Discord Rich Presence, the install, verification and cleanup policies, the source refresh interval, game update parameters, and seeding after download. The list is bounded by the database schema: each value sits in its own field of a predefined type, and enumerated values are constrained to a list of permitted options;
- **the game list** — only the game's numeric identifier in the external IGDB catalog, the flag marking that the app manages the installation, the last launch date, playtime, the "favorite" mark, the completion status and the dates these changed;
- **device identifiers** — a random UUIDv4 created the first time sync is enabled on each device, together with first- and last-seen timestamps. This identifier is not the same as the installation identifier from `installation.json` (section 1) and is not linked to it.

Playtime is stored as a separate counter per device: otherwise two computers would overwrite each other's figures.

Sync does not send source addresses or feed contents, release names, magnet links, infohashes, disk paths, file names, the contents of an installation or download history. Nor does it send the consents to anonymous statistics and diagnostics: those toggles stay local to each device and are not carried between devices.

A game for which the app has not determined an IGDB identifier is not synced at all — there is no fallback that "sends the title instead". On the server the game identifier is a numeric field with a foreign key into the catalog, so an arbitrary string, link or file name cannot be written into it.

### 2.4. Checking for app updates

The app checks for a new version by itself: it requests the update manifest from the Typhon service at startup and roughly every six hours thereafter. The request is made without an account and without a token, and carries no information about the installed version — the manifest is the same for everyone and versions are compared on the device. If an update is accepted, the new version's file is downloaded from the same service.

As with any network request, the server can see the device's IP address and the fact that a request was made.

### 2.5. Logs and rate limiting

The server records in its log the method, request path, response code and processing time. Search queries, request bodies and IP addresses do not go into that log.

The IP address is used as a temporary rate-limiting key for requests made without an account: it is held in the service's memory, is not written to the database and is not linked to an account. For requests made with an account, the account identifier serves as that key instead of the address.

### 2.6. Session state, anonymous usage statistics and anonymous diagnostics

Beyond the above, the app sends the Typhon service the state of the current session — this always happens and cannot be turned off — and, if the user has enabled the corresponding toggles in the settings, anonymous events about operation outcomes and anonymized error reports. These are three different mechanisms with different data and different retention periods; they are described in section 8.

There are two toggles and they are independent: anonymous usage statistics and anonymous diagnostics are turned on and off separately. On first launch the app asks about this directly and sends nothing until the user has answered. In that prompt only error diagnostics is pre-checked; usage statistics is not. The answer — including a refusal — is saved, and the question is not asked again.

### 2.7. What the Typhon services do not receive

The addresses of sources added by the user, the contents of source feeds, magnet links, torrent infohashes, tracker and peer addresses, release download addresses, the contents of downloaded files, file names, local installation paths and the local download history are **not** sent to the Typhon services. This holds with cross-device sync enabled as well (section 2.3).

It also holds with anonymous usage statistics enabled. Its events carry only the outcome of an operation, the game identifier from the catalog and anonymized numeric characteristics — duration, size, average speed — with a normalized error code. In other words, the service may learn that a download failed after so many seconds with such-and-such an error, but not where it was downloading from, what exactly was being downloaded, or where on disk it was being written (section 8.2).

### 2.8. Friends and profile

Friends features work only with cross-device sync enabled (section 2.3): while it is off the app makes no requests to this part of the service — not in the background, not when the "Friends" section is opened, and not when a profile is opened. Instead of a friends list, the section offers to enable sync and, when pressed, shows a separate consent screen listing what leaves the device; sync is enabled only after confirmation on that screen. Until it is confirmed, a friend request cannot be sent and the user's own friend code cannot be seen.

The service stores and processes:

- **the friends list and friend requests** — incoming and outgoing, with timestamps;
- **the list of blocked users**;
- **the friend code** — eight random characters tied to the account, used to add friends without searching by name; the code is created on the server the first time it is requested — when a user with sync enabled opens their profile or presses "My code" in the "Friends" section; the user can generate a new one at any time, after which the previous one stops working;
- **the profile bio** — text of up to 150 characters entered by the user.

What other users see of a profile is determined by the visibility level ("Who sees the profile" in the settings: "Everyone", "Friends" or "Nobody", "Friends" by default) and by individual toggles on top of it: online status, current game, playtime, library (game list, games in common, "friends played" on the game page), recent activity and stats (games, hours, completed). Each toggle is turned off independently and hides only its own item — for example, turning off playtime leaves the game list visible but hides the hours. To a user outside the visibility level this information is not served at all: the server leaves it out of the response rather than hiding it on screen.

A blocked user cannot find the profile through search or by friend code and does not see it on direct access — the server answers as it would for a profile that does not exist. Blocking ends any existing friendship and deletes all requests between the two users, incoming and outgoing, regardless of who blocked whom.

Detaching yourself from a particular person needs no request to the operator: remove them from friends, block them or generate a new friend code — previous requests and connections disappear in the process. Data about friends, requests, blocks, the friend code and the bio is deleted together with the account (section 5).

While cross-device sync is enabled (section 2.3) and the user is signed in, the app sends the service an online status — every 30 seconds, and additionally right when a game starts or ends. The status carries: the status chosen by the user ("Online", "Away", "Do not disturb" or "Invisible"), the identifier of the running game in the IGDB catalog, and the app version. The service keeps this in a short-lived in-memory store — as it does session state (section 8.1): roughly 90 seconds without a new signal and the record is gone; no historical "who was online and when" table is kept. Separately from it, the service stores a "last seen" mark: it is updated at most once every 5 minutes and is not updated at all while the "Invisible" status is selected. Apart from that mark, nothing about online status is stored beyond the 90-second window. While sync is off or the user is not signed in, no status is sent at all — not in the background, not when a game starts, and not when the app exits.

Who can see the status is determined the same way as the other profile fields (above): by the visibility level and the "Online status" and "What I'm playing" toggles — a toggle that is off hides the status or the game respectively. The "Invisible" status is the exception: it is shown to everyone as offline, with no last-seen time, regardless of the visibility level and toggles.

Separately from online status, the service maintains a friends activity feed. An entry appears in it only as a result of cross-device sync (section 2.3) and only for a game already listed in the account's library on the server — there are three kinds of event: a game's status changing to "completed", the first playtime on a game (before the sync no time had been accumulated for it anywhere, after it there is), and being added to favorites. A game that has just been added to the server has no previous state to compare against, so importing a library — the first sync of each new game — creates no events: they arise only on subsequent syncs, once there is something to compare a game against. An event of the same kind for the same game is not created more than once every 30 days: the same transition firing again within that window adds no new record. Events are stored for 90 days and are then deleted along with the reactions to them (below).

The activity feed is visible only to friends, and only while the author of the event has the "Show activity" toggle enabled — the same toggle that hides the "Recent activity" widget on the profile page (above). The "Nobody" visibility level hides events entirely, including from friends; the "Everyone" and "Friends" levels are equivalent for the feed — it is available only to those who are friends with the author of the event. A blocked user does not see the feed: blocking ends the friendship (above), and with it access to the events.

An event in the feed can be given a reaction — one of eight fixed icons. A reaction is stored as a triple of event, user and icon; it is visible to everyone who can see the event itself, and the user can remove their reaction at any time.

## 3. Why data is processed

The Typhon server services process data only for the following:

- creating an account and signing in;
- checking access rights when server-side features are used;
- the profile: username, display name, email address, avatar;
- friends and profile visibility: friend requests, the lists of friends and blocked users, the friend code, showing the profile to other users according to the visibility level (section 2.8);
- serving game metadata at the app's request;
- serving the update manifest and app update files;
- rate limiting, so that the services stay available;
- aggregated real-time counting: how many launchers are running right now, how many of them are in a game, which games are being played and which app versions are in use (section 8.1);
- where the user has explicitly enabled the toggle — product usage analytics: how often downloads, installations and updates complete successfully, and which operations the app fails on most often (section 8.2).

The data is not used for advertising, marketing or profiling: no advertising or marketing profiles are built from it and it is not passed to third parties. Analytics is limited to what is described in section 8: aggregated session state and — only with the user's explicit consent — the outcomes of app operations. Clicks, screen navigation and other interface behavior are not tracked. There are no mailing lists: the email address is needed to sign in and to recover access to an account, not to send product mail.

## 4. Retention periods

Account and profile data is stored for as long as the account exists.

Sessions have an expiry: once it passes, or once the user signs out, the session stops working. The session record itself — the token hash and the timestamps — remains in the database: there is currently no automatic deletion of such records.

An avatar is stored in object storage until it is replaced or deleted. On replacement and on deletion the previous file is deleted immediately.

Session state (section 8.1) is held in a short-lived in-memory store: roughly 90 seconds without a new signal and the record is gone. No historical "who was online and when" table is kept, and these records are not written to disk.

Synced settings and the game list (section 2.3) are stored for as long as the account exists, or until the user deletes them.

Records of friends, friend requests and blocks (section 2.8) are stored for as long as the account exists, or until the user removes them: declines or cancels a request, removes a friend, or unblocks someone.

Anonymous usage statistics events (section 8.2) are stored on the server for 30 days — the period is a service setting — and are then deleted.

Anonymous diagnostics reports (section 8.3) are stored on the server for 30 days — the period is a service setting — and are then deleted. Identical errors are stored as a group rather than individually: a group has a fingerprint, a counter, first- and last-seen times and the set of affected app versions.

No separate retention periods are set for infrastructure backups and technical copies, so no specific timeframe for complete deletion is claimed here.

## 5. Deleting the account and the data

Local data is deleted by deleting the `%APPDATA%\Typhon` directory (section 1). The avatar is deleted in the app, in the profile settings.

Cross-device synced data (section 2.3) is deleted from the server in the app, in the sync settings. Turning sync off does not by itself delete it; it is deleted automatically together with the account.

There is currently no way to delete an account directly from the app or from the website — that capability is not implemented yet, and deletion does not happen automatically. To request deletion of an account and the personal data associated with it, send a request to [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com) from the email address linked to the account. The request is handled by the project operator manually.

## 6. User rights

Depending on applicable law, a user may have the right to request access to their personal data, its correction, its deletion, or a restriction of its processing.

Username, display name, email address and avatar can be viewed and changed in the app, in the profile settings. For other requests: [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com). The operator answers on the merits, within a reasonable time and free of charge.

A request about data outside the Typhon server services — for example about data on the BitTorrent network or at a third-party source — cannot be fulfilled by the operator: that data is not under the operator's control.

## 7. Third parties

Three different cases need to be told apart here: what the Typhon services pass on, what leaves the device directly bypassing Typhon, and what happens on the BitTorrent network.

### 7.1. Who the Typhon services pass data to

- **Hosting and infrastructure.** The services run on a rented server, account data lives in a database and avatar files in object storage. The providers of that infrastructure technically have access to the data hosted with them.
- **IGDB (Twitch).** The metadata service keeps no game database of its own: on receiving a request it queries IGDB and returns the answer to the app. What goes there is the search string — that is, the game title or folder name. Account data is not sent to IGDB.

There are no other recipients: account data is not passed to advertising networks, analytics services or data brokers, and is not sold.

### 7.2. What leaves the device directly, bypassing the Typhon services

These transfers are made by the device itself. The Typhon services take no part in them and do not see their contents.

- **Source hosts.** When a source is refreshed, the app makes an HTTP request directly to the host specified by the user. The source host sees the user's IP address and the fact of the request. No proxy is used.
- **Metadata images.** Cover art and screenshots are loaded by the app directly from the IGDB content delivery network (`images.igdb.com`), at addresses taken from the metadata service's response. The owner of that network sees the user's IP address and which images were requested.
- **Discord.** Discord status display is **off** by default. If it is on, the app passes data to the local Discord client through an operating system named pipe — nothing is sent over the network by Typhon here. What is passed is the title of the running game, the rounded session duration and, where available, the cover art address. From there the data is handled by the Discord client under Discord's rules.

### 7.3. The BitTorrent network

When downloading over BitTorrent the app takes part in a distributed network: DHT and peer exchange (PEX) are enabled. This means that **trackers, DHT nodes and other peers in the swarm see the user's IP address and the torrent's infohash**. That behavior is built into the BitTorrent protocol.

Passing data to other peers is governed by two separate settings, both off by default:

- "Upload while downloading" — passing on already-received pieces while a download is active;
- "Seed after download" — passing on a completed download after it has finished.

The BitTorrent network operates independently of Typhon. The operator keeps no record of it, cannot see who transfers what on it, and cannot remove data from it on request.

## 8. Session state, usage statistics and error diagnostics

This section describes three mechanisms by which the app reports on its own operation to the Typhon service. They are built differently, and the difference between them matters: session state is always sent, while anonymous usage statistics and anonymous diagnostics are sent only if the user has enabled the corresponding toggle. There are two toggles and they are independent of each other. On first launch the app asks about them once and, until it is answered, sends neither events nor error reports.

### 8.1. Session state

**This sending cannot be turned off.** There is no toggle for it in the settings, and the toggles from sections 8.2 and 8.3 have no effect on it.

At each launch the launcher creates a random session identifier (UUIDv4). It exists only in memory and will be different at the next launch. Alongside it, the installation identifier is used — a random UUIDv4 created at first launch and saved on the device in `%APPDATA%\Typhon\installation.json`.

Both identifiers are simply random numbers. **Neither is derived from device characteristics**: not from the MAC address, not from the Windows security identifier (SID), not from a disk serial number, not from a processor identifier, and not from account data.

Every 30 seconds the app sends the Typhon service: the session identifier, the installation identifier, the state (`idle` — the launcher is running, `playing` — a game is running), the catalog identifier of the game if one is running, the app version, the operating system and the architecture. There is nothing else in that request.

The game identifier here is the numeric identifier of the game's card in the catalog, not a file name, a folder name or a path on disk.

**The account token is never sent in these requests** — neither in guest mode nor when signed in. The service cannot link session state to an account.

On the server such records are held in a short-lived in-memory store: roughly 90 seconds without a new signal and the record is gone. No historical "who was online and when" table is kept, and nothing is written to disk.

This data is needed only for aggregated counting: how many launchers are running right now, how many of them are in a game, which games are being played and which app versions are in use.

If sending fails, the app carries on as usual and shows the user nothing.

This is not opt-in telemetry and not a usage history: user events and actions are not accumulated here — only the current state is sent, which the server holds for tens of seconds and then loses. The mechanisms in sections 8.2 and 8.3 are built differently, and their toggles do not extend to session state.

Session state is needed only for the service's aggregated real-time statistics and is kept to a minimum: how many launchers are running right now, how many of them are in a game, what is being played and which app versions are in use.

### 8.2. Anonymous usage statistics

**Off by default.** It is not pre-checked in the first-launch prompt, and it can only be enabled by a separate action — there, or later in the app settings, under "Privacy".

While the toggle is off, nothing is sent: events are neither collected nor accumulated.

When the toggle is on, the app sends events about operation outcomes in batches, roughly every 25 seconds: launcher started and stopped; game started and stopped; download started, finished, failed, cancelled; installation started, finished, failed; update, verification and repair started, finished, failed.

An event contains only: the type, a timestamp, the installation identifier, the session identifier, the app version, the operating system, the architecture, and a limited set of properties — the game identifier from the catalog, duration in seconds, size in bytes, average speed, installer type and a **normalized error code** (for example `timeout`, `permission_denied`, `disk_full`, `unknown`). The text of the error message is not sent.

Events do **not** carry: magnet links, infohashes, source and tracker addresses, release download addresses, original release names, file names, local paths or peer IP addresses. The structure of an event makes it technically impossible for them to get in.

The event queue exists only in memory and is bounded in size; it is not written to disk. If the user turns the toggle off, the accumulated queue is cleared and not sent.

On the server these events are stored for 30 days — the period is a service setting — and are then deleted.

Calling it anything else would be wrong: this is product usage analytics. It is needed to see how often downloads, installations and updates run to completion, and which operations the app fails on most often.

### 8.3. Anonymous error diagnostics

**Enabled only by the user's answer.** On first launch the app asks whether to send anonymous error reports, and in that prompt diagnostics is pre-checked — but a pre-checked option is not consent: until the user answers, nothing goes out. A refusal is saved just as consent is, and the question is not repeated. The toggle stays in the app settings, under "Privacy", and the decision can be changed at any time. The anonymous usage statistics toggle (section 8.2) has no effect on diagnostics, and vice versa.

On devices where the app was installed before this prompt existed, nothing changes by itself: the user's saved choice stands, and updating the launcher does not enable diagnostics on their behalf.

While the toggle is off, nothing is sent out: error reports are neither collected nor accumulated. The activity log on the device is still written as usual — the local log and remote diagnostics are independent of each other, and diagnostics being off has no effect on the local log.

When the toggle is on, the app sends an anonymized report when an error or crash occurs. The report contains only: the report identifier, a timestamp, the app version, the operating system, the architecture, the subsystem and operation in which the failure occurred, a normalized error code, an anonymized message text, an anonymized call stack, and a flag for whether the failure was a crash.

Before sending, every text field is scrubbed: local paths are removed in full — written with backslashes and with forward slashes alike, including paths with spaces in folder names and game installation paths — as are the device name and the operating system account name, IP addresses (IPv4 and IPv6) and MAC addresses, magnet links, infohashes, source, tracker and release addresses, and access and session tokens. The scrubbing is built to refuse rather than to let something through: if a sensitive value is still recognizable in the text after scrubbing, the report is not sent at all. The server scrubs the received fields again and does not rely on the app having already done it. Of an address, only the scheme and host name remain: the path and query string are dropped wholesale rather than by a list of known parameter names, so a signature or temporary credentials in an address cannot survive because of an unfamiliar parameter. What is removed is replaced with a marker (`<path>`, `<host>`, `<ip>`) rather than silently cut out, so that the stack stays readable.

**Only the structured report is sent.** The app does not send the log in full, does not send "the last N lines of the log", does not attach crash dumps and does not transmit interface state.

The message and stack lengths are bounded, the number of reports per unit of time is limited, and identical errors are collapsed by fingerprint — one failing client cannot send a thousand identical reports. The report queue exists only in memory and is not written to disk. If the user turns the toggle off, the accumulated queue is cleared and not sent.

This can be checked in the app itself. In the settings, under "Privacy", the "Show sent data" button opens the most recent batches sent — both usage events and error reports — exactly as they went to the server, after scrubbing. The list is kept only in the memory of the current run, is limited to the last hundred records and is not sent anywhere; it disappears when the app is closed. Any record can be copied.

An error fingerprint is computed from the normalized error code, the subsystem and the top stack frames. Local paths, line numbers and any user-specific values are not part of the fingerprint.

On the server, reports are stored for 30 days (section 4), grouped by fingerprint. The account is not part of a report.

Why this is needed: to see that a new version of the app is breaking for users, and on which operation exactly, without asking the user to send a log by hand.

### 8.4. What the app does not collect

Neither session state, nor usage statistics, nor error diagnostics contains — or will come to contain — the following:

- **crash dumps** — process memory dumps are not collected and not sent; the diagnostics of section 8.3 is limited to a structured report with an anonymized call stack;
- **logs** — `typhon.log` stays on the device and is never sent anywhere automatically, whole or in fragments (section 1). The user can export it manually from the settings and send it themselves — that is a separate action requiring an explicit decision;
- **performance telemetry** — frame rate, response times, processor load and similar measurements are not collected;
- **hardware information** — processor model, graphics card, amount of memory and device serial numbers are not collected; of the device's properties, only the operating system name and the architecture are sent;
- **interface behavior** — clicks, screen navigation and time spent on a screen are not tracked;
- **advertising analytics** — the app contains no third-party counters, advertising pixels or analytics kits.

The `typhon-launcher.com` website uses no counters, advertising pixels or third-party fonts, sets no cookies and stores nothing in the browser.

## 9. Changes

This policy may change along with the app. The revision date is stated at the top of the document; the current revision ships with the app and is published on the website.

## 10. Contacting the operator

For questions about personal data, and for other legal questions: [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com).

## 11. Related documents

- [TERMS.md](TERMS.md) — terms of use.
- [COPYRIGHT.md](COPYRIGHT.md) — copyright infringement notices.
- [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) — third-party component licenses.
