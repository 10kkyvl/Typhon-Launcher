# Typhon Privacy Policy

Revision of 16 October 2026.

This document describes what data Typhon keeps on the user's device and what it sends outside. It describes the actual behavior of the current version of the app, not intentions.

Typhon is developed and maintained by an independent developer (the "Typhon project operator"). The operator is responsible for data processing in the Typhon server services: the account service, the metadata service, the support desk, sending email, and the distribution of app updates.

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
- **The activity log** `typhon.log` and its previous copies `typhon.log.1` … `typhon.log.5` — these stay on the device and are never sent anywhere automatically in full: the user can export a scrubbed log archive themselves, with a button (sections 2.10 and 2.11). The log is size-limited: 10 MB per file and no more than six files, with the oldest overwritten.
- **The play session log** — which game was launched and finished and when, for the last 90 days, and a per-day summary for statistics: how long each game was played that day, the longest session and the number of launches, for the last 400 days — is kept on this computer only and is not sent to the server. The "favorite" mark and the completion status are kept on this computer and are sent to the service only when sync is enabled (section 2.3).
- **The compatibility log** — which games start on this computer and which do not: the number of attempts, the number of failures, the longest session, and the code and text of the last error. It is kept always and is never sent anywhere on its own; with usage statistics enabled, an anonymized record derived from it is sent, as described in section 8.5. The error text is never sent.
- **The watchlist** — the catalog games the user follows, with the releases already known for each, and up to 200 most recent alerts about new releases: the game title, the release name and version, the source and the time. New releases are looked up in the feeds already downloaded to the device; no separate requests go out for this.
- **Screenshots** — pressing the screenshot key during play saves an image of that game's window as a PNG in the `Screenshots` folder inside the library directory, one subfolder per game. A screenshot is taken only on that key press and only if the window belongs to a running game from the library; the app registers a single key combination with the system and receives no other key presses. Screenshots are not sent anywhere, and capturing can be turned off in the settings.

These lists, paths and contents do not leave the device. If the user has enabled anonymous usage statistics (section 8.2), only anonymized information about the outcome of an operation is sent — for example, that an installation succeeded and how many seconds it took — but nothing of the above. If the user has enabled anonymous diagnostics (section 8.3), an anonymized error report is sent — but not the log itself and not fragments of it.

Local state is stored in the `%APPDATA%\Typhon` directory, in the files `settings.json`, `account.json`, `profile.json`, `sync.json`, `library.json`, `playlog.json`, `sources.json`, `downloads.json`, `installations.json`, `removals.json`, `catalog.json`, `match_overrides.json`, `installation.json`, `compat.json`, `compat-client.json`, `compat-stats.json`, `watchlist.json`, and in the `releases/` and `manifests/` subdirectories and the metadata cache.

`installation.json` holds only the installation identifier — a random UUIDv4 created the first time the app is started. It is not derived from any device characteristics and is not personal data in itself; where it is sent is described in section 8.

`compat-client.json` holds a separate identifier — also a random UUIDv4, and deliberately not the same one. A compatibility report carries the OS version, the processor family and the list of games there is a verdict for; sharing an identifier with ordinary usage statistics would allow the two sets to be joined into one profile considerably more detailed than either on its own. Separate identifiers do not allow that.

`compat-stats.json` holds the downloaded shared statistics — the same numbers everyone else sees. It is incoming data and says nothing about the owner of the device.

Deleting that directory deletes all of the local data listed above except screenshots: they live in the `Screenshots` folder of the library directory and are deleted separately.

## 2. Data sent to the Typhon services

The service address is set when the app is built. The access token is only ever sent over HTTPS; the sole exception is the local address used during development.

### 2.1. Account

The app can run without an account, in guest mode. In that mode no requests are made to the account service, and only the guest-mode flag is stored locally.

When an account is used, the following is sent to the service:

- on registration — email address, username, display name, password and the revision of the Terms of Use the user accepted; an email asking to confirm the address is sent to that address (section 2.12);
- when a new revision of the Terms is accepted — its date-version;
- on sign-in — username or email address, and password;
- on password recovery — the email address, then the token from the email that was sent and the new password (section 2.12);
- when working with the profile — requests to read and change the profile, and to upload and delete the avatar.

Stored on the server are: the account identifier, username, display name, email address, the time the email address was confirmed, a link to the avatar file, and the record's creation and modification dates. The password is not stored in its original form, but as a hash. For each session, a hash of the token, the creation time, the expiry, the length the session was issued for and a revocation flag are stored. An uploaded avatar is stored as a file in object storage and is available at a direct link to anyone who knows that link. The account also stores profile settings: which profile blocks to show to other users, which showcases are selected, the profile layout (which blocks, in what order and width, and which games are picked in them — by the game's catalog identifier) and the styling (theme, colours, avatar frame, name style).

Acceptance of the Terms of Use is stored as a record: the revision, the time of acceptance and where it was made — on the website or in the app. Records are only added: accepting a new revision adds a new one, earlier ones remain. The IP address and browser or device details do not go into this record.

The session token is stored on the device in the Windows credential store (Credential Manager), not in the settings file.

The same account works on the `typhon-launcher.com` website: there you can register, sign in, write to support, and confirm an address or change a password through a link from an email. A website session is the same kind of session record as an app session (token hash, creation time, expiry, issued length, revocation flag), but the token itself lives not in the Windows credential store but in a browser cookie (section 8.4). When registering on the website, the checkbox accepts the Terms of Use and confirms that the user has read the Privacy Policy: without it no account is created, and the accepted revision and time are recorded on the server. In the app, the accepted revision is recorded in the settings on the device and, if the user is signed in, sent to the server — on registration, when a new revision is accepted, and on sign-in and launch if the server does not yet hold a record of the accepted current revision; a failed send is retried until the record appears. Accounts created before these records existed get one at the next acceptance in the app.

### 2.2. Game metadata

To show cover art, descriptions and release years, the app queries the metadata service:

- search by game title;
- resolution of a list of titles into game identifiers;
- retrieval of a game card by identifier.

Metadata is available without an account too: in guest mode these requests are made in the same way, simply without a token.

Points worth understanding:

- **The game title is sent to the service.** When already-installed games are detected automatically, the title is derived from **the folder name on disk**. If the folder has an arbitrary name, that arbitrary name is what gets sent as the search query.
- **Metadata requests made from an account carry its token.** This means the service is technically able to link an account with which game titles were requested. Requests made in guest mode carry no token.
- **Game titles from source feeds are sent to the service too** — for the same cover and description matching. From a feed entry's name the app extracts the game title (without the packager's name, version or other tags) and the year, and sends only those. The server matches them against the catalog and stores nothing from that request.
- The metadata service is **not** sent: source addresses, links from feeds, magnet links, infohashes, download contents, download history or installation paths.

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

The app checks for a new version by itself: it requests the update manifest from the Typhon service at startup and roughly every six hours thereafter. The request is made without an account and without a token. Like every request the app makes to the service (section 2.5), it carries the installed version in its headers: the service uses it to pick a manifest in the format that version can read, and the decision to update is made on the device. If an update is accepted, the new version's file is downloaded from the same service.

As with any network request, the server can see the device's IP address and the fact that a request was made.

### 2.5. Logs and rate limiting

The server records in its log the method, request path, response code and processing time. Search queries, request bodies and IP addresses do not go into that log. Every request the app makes to the service carries the app version, the operating system and the processor architecture in its headers; they do not go into that log either.

A reverse proxy sits in front of the service and the website and keeps its own access log: the time, the IP address, the method, the request address together with its query string (which can contain a search query), the response code, the processing time and the request headers. The account token and cookies do not go into that log: the proxy replaces them with a marker. The proxy log is used to investigate failures and to protect against abuse; it is kept for no longer than 7 days and then deleted.

The IP address is used as a temporary rate-limiting key for requests made without an account: it is held in the service's memory, is not written to the database and is not linked to an account. For requests made with an account, the account identifier serves as that key instead of the address. The same kind of short-lived IP key is used for session state (section 8.1), so that one address cannot count as a thousand running launchers; it lives in the in-memory store and disappears together with the state.

Catalog search strings — what the user types into the game search — are kept by the server in a results cache for one day, lower-cased and not linked to an account, installation identifier or address: the cache exists so the same query is not sent to IGDB again. After a day the entry is deleted.

### 2.6. Session state, anonymous usage statistics and anonymous diagnostics

Beyond the above, the app may send the Typhon service the state of the current session — while the "Launcher online status" toggle is on — as well as anonymous events about operation outcomes and anonymized error reports, if the user has enabled the corresponding toggles. These are three different mechanisms with different data and different retention periods; they are described in section 8.

There are three toggles and they are independent. "Launcher online status" is on by default: in its basic form it carries nothing that points to a device or a person (section 8.1), and it can be turned off in the settings, under "Privacy". Anonymous usage statistics and anonymous diagnostics are off by default: on first launch the app asks about them directly, neither option is pre-checked in that prompt, and until it is answered no events and no error reports are sent. The answer — including a refusal — is saved, and the question is not asked again.

The prompt is versioned. When the set of collected data grows, the version is raised and the question is asked again, with the previous answer pre-checked. Until a new answer is given the new data is not collected, while the previous answer stays in force for what it was given for. That is what happened with game compatibility statistics (section 8.5) and with the installation identifier and the game identifier in session state (section 8.1): they fall under the anonymous usage statistics toggle, but for anyone who answered the previous version of the prompt they are not collected until that person answers the current one.

### 2.7. What the Typhon services do not receive

The addresses of sources added by the user, the contents of source feeds, magnet links, torrent infohashes, tracker and peer addresses, release download addresses, the contents of downloaded files, file names, local installation paths and the local download history are **not** sent to the Typhon services. This holds with cross-device sync enabled as well (section 2.3). The one exception is cloud saves (section 2.13), if the user has turned them on: a game's saves archive contains the relative names and the contents of the files in its saves folder; disk paths and the files of downloads and installations do not go into it.

It also holds with anonymous usage statistics enabled. Its events carry only the outcome of an operation, the game identifier from the catalog and anonymized numeric characteristics — duration, size, average speed — with a normalized error code. In other words, the service may learn that a download failed after so many seconds with such-and-such an error, but not where it was downloading from, what exactly was being downloaded, or where on disk it was being written (section 8.2).

It also holds for compatibility reports (section 8.5). They carry the repacker's nickname — one word from a closed list built into the app — and the build version number, but not the raw release name, not the address of the source it came from, and not the torrent infohash.

### 2.8. Friends and profile

Friends features work only with cross-device sync enabled (section 2.3): while it is off the app makes no requests to this part of the service — not in the background, not when the "Friends" section is opened, and not when a profile is opened. Instead of a friends list, the section offers to enable sync and, when pressed, shows a separate consent screen listing what leaves the device; sync is enabled only after confirmation on that screen. Until it is confirmed, a friend request cannot be sent and the user's own friend code cannot be seen.

The service stores and processes:

- **the friends list and friend requests** — incoming and outgoing, with timestamps;
- **the list of blocked users**;
- **the friend code** — eight random characters tied to the account, used to add friends without searching by name; the code is created on the server the first time it is requested — when a user with sync enabled opens their profile or presses "My code" in the "Friends" section; the user can generate a new one at any time, after which the previous one stops working;
- **the profile bio** — text of up to 150 characters entered by the user;
- **the status** — one emoji and text of up to 60 characters entered by the user;
- **profile block texts** — what the user writes in the profile layout themselves: titles and bodies of text blocks (up to 40 and up to 1000 characters, no more than four blocks), the caption on the pinned game (up to 140 characters), names of their own game collections (up to 40 characters).

What other users see of a profile is determined by the visibility level ("Who sees the profile" in the settings: "Everyone", "Friends" or "Nobody", "Friends" by default) and by individual toggles on top of it: online status, current game, playtime, library (game list, games in common, "friends played" on the game page), recent activity and stats (games, hours, completed). Each toggle is turned off independently and hides only its own item — for example, turning off playtime leaves the game list visible but hides the hours. To a user outside the visibility level this information is not served at all: the server leaves it out of the response rather than hiding it on screen. Layout blocks follow the same toggles: the pinned game, collections and recent games follow the library toggle, genres and the play fingerprint follow stats and playtime (genre shares are computed from hours), recent activity follows activity; the server leaves a hidden block out of the response. The status and block texts are visible to the same people who see the bio. In the friends, requests and mutual friends lists and in the feed a player card is shown next to the name: the styling (colour, avatar frame, name style) is visible to everyone who sees the user in that list, while the status, cover and pinned game are visible only to those who can see the profile.

A blocked user cannot find the profile through search or by friend code and does not see it on direct access — the server answers as it would for a profile that does not exist. Blocking ends any existing friendship and deletes all requests between the two users, incoming and outgoing, regardless of who blocked whom.

Detaching yourself from a particular person needs no request to the operator: remove them from friends, block them or generate a new friend code — previous requests and connections disappear in the process. Data about friends, requests, blocks, the friend code and the bio is deleted together with the account (section 5).

While cross-device sync is enabled (section 2.3) and the user is signed in, the app sends the service an online status — every 30 seconds, and additionally right when a game starts or ends. The status carries: the status chosen by the user ("Online", "Away", "Do not disturb" or "Invisible"), the identifier of the running game in the IGDB catalog, and the app version. The service keeps this in a short-lived in-memory store — as it does session state (section 8.1): roughly 90 seconds without a new signal and the record is gone; no historical "who was online and when" table is kept. Separately from it, the service stores a "last seen" mark: it is updated at most once every 5 minutes and is not updated at all while the "Invisible" status is selected. Apart from that mark, nothing about online status is stored beyond the 90-second window. While sync is off or the user is not signed in, no status is sent at all — not in the background, not when a game starts, and not when the app exits.

Who can see the status is determined the same way as the other profile fields (above): by the visibility level and the "Online status" and "What I'm playing" toggles — a toggle that is off hides the status or the game respectively. The "Invisible" status is the exception: it is shown to everyone as offline, with no last-seen time, regardless of the visibility level and toggles.

Separately from online status, the service maintains a friends activity feed. An entry appears in it only as a result of cross-device sync (section 2.3) and only for a game already listed in the account's library on the server — there are three kinds of event: a game's status changing to "completed", the first playtime on a game (before the sync no time had been accumulated for it anywhere, after it there is), and being added to favorites. A game that has just been added to the server has no previous state to compare against, so importing a library — the first sync of each new game — creates no events: they arise only on subsequent syncs, once there is something to compare a game against. An event of the same kind for the same game is not created more than once every 30 days: the same transition firing again within that window adds no new record. Events are stored for 90 days and are then deleted along with the reactions to them (below).

The activity feed is visible only to friends, and only while the author of the event has the "Show activity" toggle enabled — the same toggle that hides the "Recent activity" widget on the profile page (above). The "Nobody" visibility level hides events entirely, including from friends; the "Everyone" and "Friends" levels are equivalent for the feed — it is available only to those who are friends with the author of the event. A blocked user does not see the feed: blocking ends the friendship (above), and with it access to the events.

An event in the feed can be given a reaction — one of eight fixed icons. A reaction is stored as a triple of event, user and icon; it is visible to everyone who can see the event itself, and the user can remove their reaction at any time.

The author of an event can add a caption to it — a short text entered by hand. The caption is stored with the event, is visible to the same people as the event, and is deleted together with it; the author can remove or change it at any time.

### 2.9. Reviews, chat, profile cover

This is what the user writes and uploads themselves. Everything below requires an account and exists only through the user's own action: until they have written a review, sent a message or uploaded a cover, none of it exists.

**Game reviews.** A review consists of a "recommended" or "not recommended" verdict and a text of 20 to 5000 characters without links. A review is public: it is shown on the game page to every user of the app, including those without an account, together with the author's display name, avatar and playtime in that game according to sync. A review can be posted after 30 minutes in the game through Typhon and no sooner than a day after registration. Other users can mark a review as helpful or report it; a report is stored with a reason from a fixed list and the reporter's identifier. A review that gathers five reports is hidden automatically; the operator can also hide or delete a review. The author can edit or delete their review at any time. Reviews are stored until the author deletes them or until the account is deleted.

**Messages between friends.** Messaging is possible only between users who are friends. A message is a text of up to 4000 characters; a reaction can be added to it. Messages are stored on the server for 7 days from sending and are then deleted together with reactions and read marks; the server keeps no archive of conversations. Message contents are not analyzed and are used for nothing but delivery.

**Profile cover.** A JPG, PNG, WebP or GIF image of up to 8 MB that the user uploads to decorate their profile. It is stored as a file in object storage and is available by direct link to anyone who knows the link — the same as the avatar. On replacement and on deletion the previous file is deleted; when the account is deleted, all of its files are deleted. A GIF stays animated only for accounts allowed to use animated images; for everyone else the server keeps the first frame as an ordinary cover.

The operator may remove an avatar, cover, bio, status, profile block texts, event caption, review, messages or support requests in response to a notice of infringement of rights or law — the procedure is described in [COPYRIGHT.en.md](COPYRIGHT.en.md). Every such action is recorded in an internal journal with its reason and basis.

### 2.10. Log upload at the user's request

Under Settings → About, in the "Launcher logs" card, there are the "Download logs" and "Send to us" buttons. The first saves an archive to the "Downloads" folder on the device, the second sends the same archive to the Typhon server. Sending happens only on that press and after confirmation; on its own the app sends the log nowhere (section 8.4). The same archive can be attached to a support request (section 2.11).

**What is in the archive.** The `info.txt` file and the activity log — `typhon.log` and its previous copies, taking no more than the last 16 MB of each file. `info.txt` records the app version, the time the archive was built, the platform and architecture, the data directory (the path is replaced with a placeholder), the name of the operating system, the processor model, the number of threads and the amount of RAM.

**What is scrubbed.** Every line goes through the same replacement rules as error reports (section 8.3) and is replaced with placeholders: local paths — `<path>`; the operating-system user name, which is taken from the name of the home folder, — `<user>`, and the computer name — `<host>`; IP addresses — `<ip>`; email addresses — `<email>`; MAC addresses — `<mac>`; magnet links, infohashes and long hexadecimal strings — `<hash>`; the values of fields holding tokens, passwords and keys — `<token>`. Of a web address, the scheme and the host name stay and the path and parameters are dropped. A user name shorter than three characters and generic names such as `user` or `admin` are not replaced: they would cut out ordinary words.

**What stays.** The names of games and releases as they are written in the source; file names outside paths; the names of the sources the user added; host names in web addresses, including a source's host if the app contacted it and logged an error. The display name and username of the Typhon account have no scrubbing rule of their own and stay in the text if they got into the log. A path written in an unusual form may not be recognised by the rules and then stays. So the scrubbing comes with no guarantees.

Unlike error reports, the archive has no "refuse rather than let through" check (section 8.3): if something that looks like a sensitive value is left in the text after scrubbing, the archive is still built and sent. That is why the app warns about the contents before sending, and why the archive can be downloaded and read first: an archive with the same contents is what gets sent.

The archive is limited to 8 MB compressed; what does not fit (older copies of the log) is dropped, and the app shows which files were left out.

On the server an archive sent on its own is stored for 72 hours and is then deleted. It is not linked to an account: it is described only by the installation identifier, the session identifier, the app version and the operating system. An archive attached to a request is linked to that request and is stored longer (sections 2.11 and 4). The operator uses it only to investigate the reported problem.

### 2.11. Support requests

This is what the user writes themselves: until they send a request, nothing described below exists. From the app a request is created from an account. On the website it can be left both from an account and without signing in — as a guest, with a confirmed email address.

**What is sent and stored.** The subject of the request (up to 120 characters) and the text (up to 5,000 characters per message), all later messages from the user and replies from support, with timestamps; a category from a closed list (installation, game launch, account, launcher, other); the status of the request; the interface language — Russian or English — in which the emails will arrive. From the app, the request also carries the app version, the operating system and the architecture, and, if the user picked a game, its numeric identifier in the IGDB catalog. A request from an account is linked to it. A guest request is linked to the email address the guest gave: it is stored in the request, and next to it its normalized form (lower case, no tag after "+", no dots for Gmail), by which the server limits the number of unconfirmed requests from one address. The key that opens a guest request is stored by the server only as a hash: it cannot issue it again or put it into another email.

**Logs.** A log archive can be attached to a request and to later messages from the app — with a separate checkbox, after confirmation. It is the same scrubbed archive with the same contents as for a manual upload (section 2.10). It is uploaded to the server as a separate file described only by the installation identifier, the session identifier, the app version and the operating system, and once attached to a request it is linked to that request and, through it, to the account. A log cannot be attached from the website or from a guest request. No more than five archives can be attached to one request.

**Who sees it.** The request and the attached archive are visible to the support staff and the project administrator. The account's email address is not shown to an ordinary support agent, only to the administrator; a guest's address is visible to both, because there is no other way to reply. The content of requests is not analysed automatically and is used for nothing except answering the request. Do not put passwords, sign-in or password-reset links from emails, payment details or source addresses into a request: support does not need them, and people read the text.

**Notifications.** When support replies, an email arrives at the email address with the number of the request and a link to the page where it can be read; the text of the reply is not included in the email (section 2.12). Retention periods are in section 4.

### 2.12. Email

The service sends emails only in connection with a user's action and only in four cases: address confirmation after registration (the link is valid for 48 hours); a password reset on request (the link is valid for 2 hours); confirmation of a guest request (the link is the only way to the conversation, and an unconfirmed request is deleted after 48 hours); a notification about a support reply. There are no advertising or informational mailings and no product emails.

An email is first written to a sending queue in the service's database: the recipient's address, the email type, the language and the template data, that is, the link and, if there is one, the username. Once it is sent or has finally failed, the template data is erased at once, and the record stays for the period given in section 4. Address-confirmation and password-reset tokens are stored by the server as a hash together with the purpose, the address and the expiry.

Emails are sent through the Spaceship mail service (Spacemail), from the address `no-reply@typhon-launcher.com`, over an encrypted connection. The provider sees the recipient's address and the text of the email, including the link; the provider's servers are in the United States. Details are in section 7.1.

### 2.13. Cloud saves

Cloud saves are off by default, are turned on manually in the app settings and work only after signing in to an account. While the toggle is off or nobody is signed in, the app does not contact this part of the service at all.

When they are on, for each game that has an identifier in the Typhon catalog and a saves folder that was found, the following is sent to the service and stored on the server:

- **an archive of the game's save files** — a ZIP archive of the contents of the saves folder, which the app found itself or the user pointed to. The archive contains the relative file names inside that folder and their contents; absolute disk paths are not sent. What exactly is written in those files is up to the game: it may be, for example, a character name or a game profile;
- **the game's identifier in the Typhon catalog**;
- **a device identifier** — a random UUIDv4 created the first time cloud saves are turned on on each device. It is not the same as the installation identifier from `installation.json` (section 1) and is not linked to it;
- **the archive size, the checksum of its contents and the time of the last upload**.

The app contacts the service after a play session ends, at startup and then roughly every five minutes while it is running: it asks for the list of stored archives, and sends or downloads an archive only if the saves changed on one of the two sides. If both sides changed, the app overwrites nothing and asks the user which copy to keep. Before the saves on the device are replaced with a downloaded copy, the current files are copied into a backup that stays on this device only.

A game for which the app has not determined a catalog identifier is not synced. A saves folder that contains symbolic links or special-type files is not sent.

The operator does not open the contents of the archives: to the service an archive is an opaque file that it stores and hands back to the owner of the account. The size of a single archive and the total size of a user's archives are limited by service settings. A new upload replaces the game's previous archive; earlier versions are not kept on the server.

## 3. Why data is processed

The Typhon server services process data only for the following:

- creating an account and signing in;
- checking access rights when server-side features are used;
- the profile: username, display name, email address, avatar;
- friends and profile visibility: friend requests, the lists of friends and blocked users, the friend code, showing the profile to other users according to the visibility level (section 2.8);
- serving game metadata at the app's request;
- serving the update manifest and app update files;
- answering support requests (section 2.11);
- sending the emails without which the account and support do not work: address confirmation, password reset, guest request confirmation, reply notification (section 2.12);
- where the user has explicitly turned the toggle on — storing copies of game saves so that they can be restored on another device (section 2.13);
- rate limiting, so that the services stay available;
- aggregated real-time counting: how many launchers are running right now, how many of them are in a game, which games are being played and which app versions are in use (section 8.1);
- where the user has explicitly enabled the toggle — product usage analytics: how often downloads, installations and updates complete successfully, and which operations the app fails on most often (section 8.2).

The data is not used for advertising, marketing or profiling: no advertising or marketing profiles are built from it and it is not passed to third parties. Analytics is limited to what is described in section 8: aggregated session state and — only with the user's explicit consent — the outcomes of app operations. Clicks, screen navigation and other interface behavior are not tracked. There are no mailing lists: the email address is needed to sign in, to confirm the address, to recover access to an account and for notifications about support replies, not to send product mail.

## 4. Retention periods

Account and profile data, including the Terms acceptance records, is stored for as long as the account exists.

Sessions have an expiry: once it passes, or once the user signs out, the session stops working. While a session is in use its expiry is extended: once less than half of it is left, it is moved forward by the length the session was issued for, but never past 180 days after sign-in, after which signing in again is required. Sessions issued for less than seven days are not extended. The session record itself — the token hash and the timestamps — remains in the database: there is currently no automatic deletion of such records.

The reverse proxy access log (section 2.5) is kept for no longer than 7 days.

An avatar is stored in object storage until it is replaced or deleted. On replacement and on deletion the previous file is deleted immediately.

Session state (section 8.1) is held in a short-lived in-memory store: roughly 90 seconds without a new signal and the record is gone. No historical "who was online and when" table is kept, and these records are not written to disk.

Synced settings and the game list (section 2.3) are stored for as long as the account exists, or until the user deletes them.

Cloud saves archives (section 2.13) are stored until the user deletes them — one game or all at once — or turns cloud saves off with deletion of the data, or until the account is deleted, whichever comes first. A new upload replaces the game's previous archive.

Records of friends, friend requests and blocks (section 2.8) are stored for as long as the account exists, or until the user removes them: declines or cancels a request, removes a friend, or unblocks someone.

Anonymous usage statistics events (section 8.2) are stored on the server for 30 days — the period is a service setting — and are then deleted.

Game compatibility reports (section 8.5) are stored on the server for 180 days — the period is a service setting. The period runs from the last update of a record rather than from its creation: the app periodically sends the verdict again, and while the game remains on the device its record is refreshed and not deleted. Once the app stops sending a verdict — because the game was removed, the toggle was turned off, or the device stopped being used — the record lives out its 180 days and is deleted. The period is longer than for usage statistics because a share of successful launches is only meaningful once accumulated: in 30 days most games would not gather enough observations even to show a number.

Anonymous diagnostics reports (section 8.3) are stored on the server for 30 days — the period is a service setting — and are then deleted. Identical errors are stored as a group rather than individually: a group has a fingerprint, a counter, first- and last-seen times and the set of affected app versions.

Manually uploaded log archives (section 2.10) that are not attached to a request are stored for 72 hours. Messages between friends (section 2.9) — 7 days from sending. Activity feed events and their captions (section 2.8) — 90 days. Reviews and the profile cover (section 2.9) — until the user deletes them or until the account is deleted. The catalog search-string cache (section 2.5) — one day.

Support requests (section 2.11) are stored as follows:

- an open request — until the user or support closes it; such a request has no storage period of its own;
- a request that support has answered and the user has not replied to for 14 days is closed automatically;
- a closed request with its whole conversation is stored for 365 days from closing and is then deleted; a reply from the user in a closed request reopens it, and the period runs from the next closing;
- an unconfirmed guest request, which nobody has seen, is deleted 48 hours after it was created; a confirmed guest request has the same periods as the rest;
- a log archive attached to a request is stored while the request is open and for 30 more days after it is closed; if the request is reopened, the archive stays with it. The check runs hourly.

Emails (section 2.12): once an email is sent, or has finally failed, its text is erased from the queue at once, and the record of it — the recipient address, the email type, the language, the time and a service note about the sending error — is stored for 30 days and then deleted. Address-confirmation and password-reset tokens are stored as a hash and are deleted no later than 7 days after use or expiry (the links themselves are valid for 48 hours and 2 hours). Copies of sent emails at the mail service are kept under its own rules (section 7.1).

No separate retention periods are set for infrastructure backups and technical copies, so no specific timeframe for complete deletion is claimed here.

## 5. Deleting the account and the data

Local data is deleted by deleting the `%APPDATA%\Typhon` directory (section 1). The avatar is deleted in the app, in the profile settings.

Cross-device synced data (section 2.3) is deleted from the server in the app, in the sync settings. Turning sync off does not by itself delete it; it is deleted automatically together with the account.

Cloud saves archives (section 2.13) are deleted from the server in the app, in the cloud saves settings: the archive of one game or all of them at once; in addition, when cloud saves are turned off the app offers to delete the data from the cloud. Turning the toggle off without that mark does not delete the archives; they are deleted automatically together with the account.

The account is deleted by the user themselves in the app, in the profile settings, with the "Delete account" button and password confirmation. Deletion is immediate and irreversible: the server removes the account record, the password hash, all sessions, the Terms acceptance records, the profile and bio, the avatar and cover files, synced settings and the game list, cloud saves archives, friends, requests and blocks, feed events with their captions and reactions, chat messages, reviews, votes and review reports, support requests with their whole conversations, address-confirmation and password-reset tokens, and every mail-queue record addressed to the account's email address. Website sessions are the same records as app sessions and are deleted with them. Log archives attached to deleted requests are detached from them and are deleted by the next hourly cleanup, but not earlier than 72 hours after they were received. Entries in the operator's internal action journal, if any exist, keep only the username as of the time of the action (section 2.9). The anonymized data in section 8 is not linked to the account and is not affected by deletion.

Deleting the account does not delete guest requests left from the same email address before registering or without signing in: they are not linked to the account and are stored for the general periods of section 4. A request to delete them sooner can be sent to [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com) from that address; the operator handles it as described in section 6. A user with an account can close their own request, in the app or on the website; an individual request cannot be deleted from the interface. A specific request, or a single message in it, can be deleted before its period ends on request to [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com) from the account's address or the guest request's address: in the admin panel an administrator deletes either the whole request — with the entire conversation and the attached log archives — or one message together with its archive; the archives are deleted at once. The deletion itself is noted in the operator's internal action journal: who deleted it and when, the reason if one was given, the username if the request came from an account, and service marks — the request number, its section and status, whether it was a guest request, the number of messages, the identifiers of the deleted messages and archives, who wrote the deleted message (the user or support), and the basis if the deletion followed an infringement notice. The subject, the text of the request and its messages, and the guest's address do not go into the journal.

If signing in is impossible (lost password or device), a deletion request is accepted at [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com) from the email address linked to the account; the project operator carries it out manually through the same mechanism.

## 6. User rights

Depending on applicable law, a user may have the right to request access to their personal data, its correction, its deletion, or a restriction of its processing.

Username, display name, email address and avatar can be viewed and changed in the app, in the profile settings; the account as a whole is deleted there too (section 5). For other requests: [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com). The operator answers on the merits, within a reasonable time and free of charge.

A request about data outside the Typhon server services — for example about data on the BitTorrent network or at a third-party source — cannot be fulfilled by the operator: that data is not under the operator's control.

## 7. Third parties

Three different cases need to be told apart here: what the Typhon services pass on, what leaves the device directly bypassing Typhon, and what happens on the BitTorrent network.

### 7.1. Who the Typhon services pass data to

- **Hosting and infrastructure.** The services run on a rented server, account data lives in a database and avatar files and cloud saves archives in object storage. The providers of that infrastructure technically have access to the data hosted with them.
- **IGDB (Twitch).** The metadata service keeps no game database of its own: on receiving a request it queries IGDB and returns the answer to the app. What goes there is the search string — that is, the game title or folder name. Account data is not sent to IGDB.
- **Spaceship mail service (Spacemail).** All of the service's emails (section 2.12) are sent through it from the address `no-reply@typhon-launcher.com`. The provider is given the recipient's address and the full text of the email, including the link with a token or key that it contains; it keeps copies of sent emails under its own rules, and the operator has no influence over that. The provider's servers are in the United States, so the email address and the content of the emails are transferred outside the user's country, including outside Ukraine and the European Union. The transfer is part of an action the user requested themselves (registering, recovering a password, writing to support): without it the email cannot be delivered.

There are no other recipients: account data is not passed to advertising networks, analytics services or data brokers, and is not sold.

### 7.2. What leaves the device directly, bypassing the Typhon services

These transfers are made by the device itself. The Typhon services take no part in them and do not see their contents.

- **Source hosts.** When a source is refreshed, the app makes an HTTP request directly to the host specified by the user. The source host sees the user's IP address and the fact of the request. No proxy is used.
- **Metadata images.** Cover art and screenshots are loaded by the app directly from the IGDB content delivery network (`images.igdb.com`), at addresses taken from the metadata service's response. The owner of that network sees the user's IP address and which images were requested.
- **Discord.** Discord status display is **off** by default. If it is on, the app passes data to the local Discord client through an operating system named pipe — nothing is sent over the network by Typhon here. What is passed is the title of the running game, the rounded session duration and, where available, the cover art address. From there the data is handled by the Discord client under Discord's rules.
- **Steam.** Only on the user's "Add to Steam" command, the app writes to the Steam client's files on this computer — the `shortcuts.vdf` shortcut list and the `grid` artwork folder of each Steam user on the device — the game title, the path to its executable, its launch options and its cover art; a copy of the previous shortcut list is kept next to it. Nothing is sent over the network by Typhon here. A game started from Steam is handled by the Steam client under Steam's rules: for example, it may show the game title to friends. The "Remove from Steam" command and removing the game from the library remove the entry and the artwork. While Steam is running its files are not changed: the "Remove from Steam" command asks to close it, and the entry of a removed game is removed later — when Typhon starts or on the next "Add to Steam" or "Remove from Steam" command while Steam is closed.

### 7.3. The BitTorrent network

When downloading over BitTorrent the app takes part in a distributed network: DHT and peer exchange (PEX) are enabled. This means that **trackers, DHT nodes and other peers in the swarm see the user's IP address and the torrent's infohash**. That behavior is built into the BitTorrent protocol.

Passing data to other peers is governed by two separate settings, both off by default:

- "Upload while downloading" — passing on already-received pieces while a download is active;
- "Seed after download" — passing on a completed download after it has finished.

The BitTorrent network operates independently of Typhon. The operator keeps no record of it, cannot see who transfers what on it, and cannot remove data from it on request.

## 8. Session state, usage statistics and error diagnostics

This section describes three mechanisms by which the app reports on its own operation to the Typhon service. Each has its own toggle in the settings, under "Privacy", and the toggles are independent of each other. Session state is sent while "Launcher online status" is on — it is on by default; anonymous usage statistics and anonymous diagnostics are sent only if the user has turned them on. On first launch the app asks about statistics and diagnostics once and, until it is answered, sends neither events nor error reports.

### 8.1. Session state

**On by default, and can be turned off.** The "Launcher online status" toggle is in the settings, under "Privacy", and takes effect without restarting the app: the next signal already follows the new position of the toggles (a request that was already being sent at the moment of the switch is not recalled). What is sent depends on it and on the anonymous usage statistics toggle (section 8.2):

- **"Launcher online status" off** — nothing is sent: neither the signal every 30 seconds nor the end-of-session message when the app exits. The statistics toggle makes no difference here;
- **on, usage statistics off** (or the current version of the prompt about it not answered yet, section 2.6) — every 30 seconds, and right when a game starts or stops, the basic signal is sent: the session identifier, the state (`idle` — the launcher is running, `playing` — a game is running), the app version, the operating system and the architecture. It carries neither the installation identifier nor the game identifier. When the app exits, only the session identifier is sent;
- **on, usage statistics on** — the installation identifier and, if a game is running, the catalog identifier of the game are added to the basic signal. When the app exits, the installation identifier is sent along with the session identifier.

There is nothing else in these requests.

The session identifier is a random UUIDv4 that the launcher creates at each launch. It exists only in memory and will be different at the next launch. The installation identifier is a random UUIDv4 created at first launch and saved on the device in `%APPDATA%\Typhon\installation.json`. It is persistent, which is why it goes into session state only with consent to usage statistics.

Both identifiers are simply random numbers. **Neither is derived from device characteristics**: not from the MAC address, not from the Windows security identifier (SID), not from a disk serial number, not from a processor identifier, and not from account data.

The game identifier here is the numeric identifier of the game's card in the catalog, not a file name, a folder name or a path on disk.

**The account token is never sent in these requests** — neither in guest mode nor when signed in. The service cannot link session state to an account.

On the server such records are held in a short-lived in-memory store: roughly 90 seconds without a new signal and the record is gone. No historical "who was online and when" table is kept, and nothing is written to disk.

This data is needed only for aggregated counting: how many launchers are running right now, how many of them are in a game, which app versions are in use and — from the signals that carry a game identifier — which games are being played.

If sending fails, the app carries on as usual and shows the user nothing.

This is not a usage history: user events and actions are not accumulated here — only the current state is sent, which the server holds for tens of seconds and then loses. The anonymous diagnostics toggle (section 8.3) has no effect on session state, and the usage statistics toggle only decides whether the installation identifier and the game identifier are added.

Online status for friends (section 2.8) is a separate mechanism tied to the account: the "Launcher online status" toggle has no effect on it.

### 8.2. Anonymous usage statistics

**Off by default.** It is not pre-checked in the first-launch prompt, and it can only be enabled by a separate action — there, or later in the app settings, under "Privacy".

Besides the events described below, this toggle adds the installation identifier and the catalog game identifier to session state (section 8.1), if "Launcher online status" is on as well.

While the toggle is off, nothing is sent: events are neither collected nor accumulated.

When the toggle is on, the app sends events about operation outcomes in batches, roughly every 25 seconds: launcher started and stopped; game started and stopped; download started, finished, failed, cancelled; installation started, finished, failed; update, verification and repair started, finished, failed.

An event contains only: the type, a timestamp, the installation identifier, the session identifier, the app version, the operating system, the architecture, and a limited set of properties — the game identifier from the catalog, duration in seconds, size in bytes, average speed, installer type and a **normalized error code** (for example `timeout`, `permission_denied`, `disk_full`, `unknown`). The text of the error message is not sent.

Events do **not** carry: magnet links, infohashes, source and tracker addresses, release download addresses, original release names, file names, local paths or peer IP addresses. The structure of an event makes it technically impossible for them to get in. The same holds for compatibility reports (section 8.5), which this same toggle enables.

The event queue exists only in memory and is bounded in size; it is not written to disk. If the user turns the toggle off, the accumulated queue is cleared and not sent.

On the server these events are stored for 30 days — the period is a service setting — and are then deleted.

This same toggle enables game compatibility reports; they are described separately, in section 8.5, because the data they carry is different.

Calling it anything else would be wrong: this is product usage analytics. It is needed to see how often downloads, installations and updates run to completion, and which operations the app fails on most often.

### 8.3. Anonymous error diagnostics

**Enabled only by the user's answer.** On first launch the app asks whether to send anonymous error reports. Diagnostics is not pre-checked in that prompt, and until the user answers, nothing goes out. A refusal is saved just as consent is, and the question is not repeated. The toggle stays in the app settings, under "Privacy", and the decision can be changed at any time. The anonymous usage statistics toggle (section 8.2) has no effect on diagnostics, and vice versa.

On devices where the app was installed before this prompt existed, nothing changes by itself: the user's saved choice stands, and updating the launcher does not enable diagnostics on their behalf.

While the toggle is off, nothing is sent out: error reports are neither collected nor accumulated. The activity log on the device is still written as usual — the local log and remote diagnostics are independent of each other, and diagnostics being off has no effect on the local log.

When the toggle is on, the app sends an anonymized report when an error or crash occurs. The report contains only: the report identifier, a timestamp, the app version, the operating system, the architecture, the subsystem and operation in which the failure occurred, a normalized error code, an anonymized message text, an anonymized call stack, and a flag for whether the failure was a crash.

Before sending, every text field is scrubbed: local paths are removed in full — written with backslashes and with forward slashes alike, including paths with spaces in folder names and game installation paths — as are the device name and the operating system account name, IP addresses (IPv4 and IPv6) and MAC addresses, magnet links, infohashes, source, tracker and release addresses, and access and session tokens. The scrubbing is built to refuse rather than to let something through: if a sensitive value is still recognizable in the text after scrubbing, the report is not sent at all. The server scrubs the received fields again and does not rely on the app having already done it. Of an address, only the scheme and host name remain: the path and query string are dropped wholesale rather than by a list of known parameter names, so a signature or temporary credentials in an address cannot survive because of an unfamiliar parameter. What is removed is replaced with a marker (`<path>`, `<host>`, `<ip>`) rather than silently cut out, so that the stack stays readable. The "refuse rather than let through" rule applies to error reports; the log archive the user exports themselves (section 2.10) has no such check.

**Only the structured report is sent.** The app does not send the log in full, does not send "the last N lines of the log", does not attach crash dumps and does not transmit interface state.

The message and stack lengths are bounded, the number of reports per unit of time is limited, and identical errors are collapsed by fingerprint — one failing client cannot send a thousand identical reports. The report queue exists only in memory and is not written to disk. If the user turns the toggle off, the accumulated queue is cleared and not sent.

This can be checked in the app itself. In the settings, under "Privacy", the "Show sent data" button opens the most recent batches sent — both usage events and error reports — exactly as they went to the server, after scrubbing. The list is kept only in the memory of the current run, is limited to the last hundred records and is not sent anywhere; it disappears when the app is closed. Any record can be copied.

An error fingerprint is computed from the normalized error code, the subsystem and the top stack frames. Local paths, line numbers and any user-specific values are not part of the fingerprint.

On the server, reports are stored for 30 days (section 4), grouped by fingerprint. The account is not part of a report.

Why this is needed: to see that a new version of the app is breaking for users, and on which operation exactly, without asking the user to send a log by hand.

### 8.4. What the app does not collect

Neither session state, nor usage statistics, nor error diagnostics contains — or will come to contain — the following (the persistent installation identifier goes into session state only with consent to usage statistics, section 8.1):

- **crash dumps** — process memory dumps are not collected and not sent; the diagnostics of section 8.3 is limited to a structured report with an anonymized call stack;
- **logs** — `typhon.log` stays on the device and is never sent anywhere automatically, whole or in fragments (section 1). The user can export a scrubbed archive manually from the settings — download it or press "Send to us" — or attach it to a support request (sections 2.10 and 2.11); that is a separate action requiring an explicit decision;
- **performance telemetry** — frame rate, response times, processor load and similar measurements are not collected;
- **hardware information** — the exact processor model, the graphics card, the amount of memory and device serial numbers are not collected. Of the device's properties, the operating system name and the architecture are sent, and in a compatibility report on macOS also the OS version to two components (`15.6`, without the build number), the CrossOver version and the processor **family** as a single word from a closed list: `apple_m1` … `apple_m5`, `intel` or `unknown`. The processor brand string the system reports never leaves the device: on Intel machines it names a specific model. Why those three values are needed is explained in section 8.5. The exception is the log archive the user exports themselves: its `info.txt` holds the name of the operating system, the processor model, the number of threads and the amount of RAM (section 2.10);
- **interface behavior** — clicks, screen navigation and time spent on a screen are not tracked;
- **advertising analytics** — the app contains no third-party counters, advertising pixels or analytics kits.

The `typhon-launcher.com` website uses no counters, advertising pixels or third-party fonts. It stores exactly two things in the browser. The first is a service session cookie, `__Host-typhon_session`: it appears only after signing in to or registering an account on the website, contains the session token, is not readable by page scripts (`HttpOnly`), is sent only over HTTPS and only to the site (`Secure`, `SameSite=Lax`), lasts until the session ends (the period is a service setting, currently 30 days) and is removed by signing out. It is needed for sign-in to work, so consent is not asked for it; there are no advertising or analytics cookies. The second is the chosen language (`ru` or `en`) in the browser's local storage under the key `typhon_lang`: it is written only when the user switches the language themselves. A guest request (section 2.11) sets no cookie: access to it is given by the key in the link from the email.

### 8.5. Game compatibility statistics

**Enabled by the same toggle as anonymous usage statistics (section 8.2), and off by default.** It cannot be enabled on its own, nor disabled on its own: it is part of the same statistics, just with different data, which is why it is described separately.

An answer given to a previous version of the data-collection prompt does not extend to these reports. The previous version promised that hardware information was not collected, and a compatibility report carries the processor family and the OS version. So for anyone who has already answered the prompt, reports are not collected until the app asks again — with their previous answer pre-checked — and until they answer. A refusal is saved the same way and is not asked again either.

**Why this exists.** On macOS, Windows games run through CrossOver, and not all of them run. With no data, every user finds this out alone: they download tens of gigabytes, install, and only then learn the game does not start. Shared statistics make it possible to show this in the catalog before downloading.

**The local log is kept always and regardless of the toggle.** The app notices by itself whether a game started, and shows that in the list of installed games. That log does not leave the device: the toggle being off means nothing is sent, not that the app stops remembering which games do not run on this computer.

When the toggle is on, roughly every ten minutes — and only if the verdict has changed since last time — the app sends a snapshot of that log. The snapshot contains only:

- **a device identifier** — a separate random UUIDv4, not the one used for the rest of the statistics (section 1);
- **the app version**;
- **the macOS version** to two components (`15.6`), **the CrossOver version** and **the processor family** as a single word from a closed list;
- **the list of games with a verdict**, each carrying: the numeric game identifier in the external IGDB catalog, the repacker's nickname as a single word from a closed list, the build version number, the verdict `works` or `broken` and, for failures, a normalized reason code (`launch_failed`, `runtime_failed`, `executable_missing`, `self_exit`, `unknown`).

That is all. Free-form fields do not exist in the report at all: every value is either a word from a list built into the app or a number that passed a pattern. The error text, local paths, the device name, the OS account name, the raw release name, the source address, a magnet link and an infohash cannot get in — not because they are scrubbed out, but because there is nowhere for them to go. The server checks the report it receives against the same rules again and does not rely on the app having done so: a value outside the list causes the whole report to be refused, not quietly trimmed. The headers of the request itself (app version, operating system and architecture, section 2.5) are not written into the report. A large snapshot is sent as several requests, each within the service's size limit.

Games without a verdict are not included. A game that was installed but never launched says nothing about compatibility — while saying that the person has it.

**What happens to a report on the server.** Reports are reduced to one number per game: the share of devices on which the game starts. It is counted per device, not per launch — each device has exactly one vote per game, and someone who launched a game five hundred times does not outweigh ten other people. Within one device, success cancels earlier failures: a game that did run in the end counts as working. A snapshot sent again overwrites the previous one rather than adding to it.

Until at least five different devices have spoken about a game, the number is not shown and is not served to apps at all. Not "zero percent", but nothing: a share computed from one or two devices is one stranger's bad luck presented as a general rule.

Only that aggregate comes back to the app — the same numbers for everyone who asks for them. Downloading the aggregate is not governed by the toggle and requires no consent: it is incoming data, and it contains nothing about who requested it.

Sent reports can be inspected in the same place as the rest: in the settings, under "Privacy", via the "Show sent data" button — exactly as they went to the server.

On the server, reports are stored for 180 days from their last update (section 4). The account is not part of a report and a report is not linked to it in any way.

## 9. Changes

This policy may change along with the app. The revision date is stated at the top of the document; the current revision ships with the app and is published on the website.

## 10. Contacting the operator

For questions about personal data, and for other legal questions: [abuse@typhon-launcher.com](mailto:abuse@typhon-launcher.com).

## 11. Related documents

- [TERMS.md](TERMS.md) — terms of use.
- [COPYRIGHT.md](COPYRIGHT.md) — copyright infringement notices.
- [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) — third-party component licenses.
