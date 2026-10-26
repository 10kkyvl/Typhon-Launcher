# Changelog

The English mirror of `CHANGELOG.md`. The Russian file is the one that feeds
the signed update manifest and the "What's new" screen inside the launcher;
this file carries the same entries for the GitHub releases. It starts at
0.4.0, the release that introduced the English interface — for anything older
see `CHANGELOG.md`. Sections: "Added", "Changed", "Fixed", "Removed".

## 1.3.0 — 2026-10-26
The friends feed shows more: achievements, reviews, plans and the week's play time. Old games can run in a compatibility mode, a crashed game comes with the reason, and themes can now change the window's layout too.

### Added
- New events in the friends feed: more than an hour played this week, achievements unlocked, a review written, a game added to "Backlog". The "What goes into the feed" block in "Profile privacy" turns any kind off — others stop seeing it, including what was already there
- So friends can see your new achievements, the launcher sends the server which achievements you unlocked and when — only while you are signed in and sync between devices is on. Without it achievements stay on the computer
- Compatibility mode for old games on Windows: a game's "Properties" show which old graphics API it uses and turn on dgVoodoo2 or DXVK. The wrapper comes from its official page, and the game files it replaced come back when the mode is off
- When a game crashes on Windows, the launcher reads the system log, names the cause and says what to do: install Visual C++ or DirectX, turn on the compatibility mode, or add the folder to the antivirus exclusions if a game file went missing. "Collect report" saves the details
- A copy of the saves before every game launch: if a game breaks a save, its page has "Roll back saves", and each copy shows how long you played after it. On from the start, turned off in Settings on the "Downloads" tab
- A "Completed 100%" status and a completion date: the launcher asks for the date when a game is marked completed and shows it on the game page. The library filters by status and sorts by status or completion date
- After a game's first launch the launcher offers to mark it "Playing", and once every achievement is unlocked, "Completed 100%"
- A "Completed this year" block in the profile lists the games finished this year; whoever can see your library can see it
- A "What to play?" button in the library and in Big Picture picks a random game from installed games, the whole library, the backlog or a collection. You can keep only games you have not launched for 30 days, or one genre
- The tray menu lists the five most recent games: a game starts with its last profile without opening the window, and the running one can be stopped from there. On Windows the same games are in the taskbar icon's menu and in Start
- A game page with no downloads of its own now has "Other versions": the original, a remake, a remaster or an edition of the same game with their downloads. Games that never came out on PC are marked "Never released on PC"
- The same page has "Similar releases" — releases that may belong to this game. "Link to this game" ties a release to the game on this computer
- Personal avatar frames: a frame granted to your account shows wherever your avatar does — profile, friends, feed, chat, reviews and Big Picture. Pick it in the profile style panel
- Launcher updates download only the part of the program that changed, not the whole installer. This starts with the next update after 1.3.0
- If a new launcher version works worse, "Settings → About" has "Go back to version …". The previous version is saved during an update, so the button appears after the next one
- A skill for AI agents such as Claude Code: save it from the appearance settings or from "Support", ask the agent for a theme in any style and import the file it makes
- "Support" links to the Typhon service status page
- A theme can change the window's layout: put the sidebar on the right, reorder and hide its items, centre the search, reorder the top bar buttons and the blocks of the game page. The theme editor has a "Layout" section for it; "Settings" can never be hidden
- A theme can be pasted as text: "Paste text" in Settings → Appearance takes what an AI agent printed and saves the theme under its own name. A custom theme can now be deleted right from its tile
- An imported theme file is followed: edit it in your editor and the change shows up in the launcher at once, without a restart
- The theme editor's "Readability" block shows the contrast of text, buttons and links against the background by WCAG thresholds and warns about pairs that are hard to read, without stopping you from saving
- "Try on the window" shows the draft theme on the whole launcher, with its CSS and layout. Unless you press "Keep while editing", the window goes back by itself after 15 seconds
### Changed
- The profile gradient covers the whole profile instead of a thin strip under the cover. The profile accent can follow "Launcher theme" and then changes with the theme
- An avatar in a frame is centred on the frame's hole, not on the picture, and leaves room around itself for the frame, so the frame no longer covers the name, the status or anything next to it
- A theme's custom CSS can no longer load images or fonts from the internet, including through workaround spellings: a stranger's theme could use that to tell an outside server your address. A theme saved earlier with such CSS opens without it, and the editor says why
- Releases find their game more often: the launcher understands Russian spellings of titles and names like "Portal 1", and a catalog edition shows the downloads of its main game
- The documents are updated to the 25 October 2026 edition: the policy covers the new feed events, sending achievements, the completion date and personal frames. The launcher asks you to accept them again
### Fixed
- A click in the tray menu no longer starts another game or quits the launcher instead of the item you picked. A tray switched on in the settings while the launcher runs opens its menu on a right click straight away, without a restart
- A game shortcut clicked while the launcher is open hands the launch over at once — before, a second copy went through most of its startup, and each click cost a few seconds
- Cloud saves and save copies no longer take a game's cache for its saves: for World of Tanks hundreds of megabytes of cache were archived instead. When a found folder is refused, the launcher says why and asks you to pick the folder yourself
- The Steamworks Spacewar sample no longer lands in the library on a Steam import
- System requirements of an installed game no longer say "not enough space" when the disk filled up after the install
- Big Picture under the light theme no longer shows white text on white panels
- A click in an image opened from the chat no longer closes the launcher
- Checking a game for updates no longer hides a release that just came out when two checks run at once
- The "Reset appearance" button no longer pops up after every start with a custom theme — only after the theme changes
- "Export" in the theme editor no longer writes the previous version to the file: it is unavailable while there are unsaved edits. After editing a built-in theme the launcher switches to the copy it made instead of staying on the built-in one

## 1.2.0 — 2026-10-21
Downloads follow a schedule, the launcher runs from a USB stick, a game's page shows achievements, system requirements and notes, and the computer switches to a game mode while you play.

### Added
- Download schedule: the download settings take windows of weekdays and hours. Outside every window downloads and seeding stand still, and a banner on the Downloads page says when the next window opens
- A single download can be sent "Download now": it runs outside the schedule until it finishes and is marked "Outside schedule"
- Downloads can pause or slow down while a game runs: the mode is picked in the download settings, kicks in with the first running game and lifts when the last one closes. Downloads you paused yourself are left alone
- Portable mode: an empty `portable.txt` next to `typhon.exe` moves everything into a `data` folder beside the program; such a copy lives on a USB stick and runs alongside an installed one. Releases ship a `typhon-windows-portable.zip`, and the copy updates itself from it
- Achievements: a game with a known Steam id gets an "Achievements" card on its page with names and icons. Which ones are unlocked is read from progress files next to the game or in the profile after each play session
- New achievements pop up as a toast; which ones you have unlocked is never sent to the server
- System requirements on the game page and in the install dialog: "Runs well", "Minimum" or "Won't run" from comparing the Steam requirements with your CPU, graphics card, memory, DirectX and disk space
- The requirements estimate goes by model names and may be wrong; hardware details stay on the computer
- Game mode in the settings: while a game runs the "High performance" power plan is on, the launcher hides to the tray and the game's priority rises above normal. Everything is put back when the game exits, even if the launcher crashed meanwhile
- Launch profiles in a game's Properties: up to eight profiles, each with its own executable, arguments and permissions. The Play button starts the last chosen one, and every profile gets its own shortcut and Steam entry
- Notes on a game's page: codes, where you stopped, what to install. Up to 4000 characters, saved as you type, kept on the computer and never sent to the server
- A "Disk usage" screen in the sidebar: how much games, downloads without an install, leftovers, screenshots and save snapshots take, and what can be freed with one button. Games not played for 90 days are marked
- The game page shows Steam details: DLC with links to their pages, controller support, the Metacritic score and the recommendation count, and a "Reference" block links to PCGamingWiki and HowLongToBeat
- A friend's profile has a "You both play" card: shared games with each player's time, a "Both played recently" mark and an "Invite to play" button that drafts a chat message
- On the Downloads page a download that came from the catalog has a "Game page" button, so the description and requirements are one click away

- Several game folders: the Library card in the settings takes folders on other disks. The install dialog offers the library folders to choose from, and Find games and Disk usage look in all of them
- Your own colours in the profile appearance: a custom background colour sits under the theme tiles, the Custom theme adds a second colour and "colours as is" without darkening, and the name and the avatar frame take their own colour instead of the accent. The appearance panel scrolls on its own
- The theme editor in the settings names colours in plain words ("Cards and windows", "Borders", "Text"), says where each one is used and shows a full-width preview above the fields: menu, search, cards, hovered buttons, a focused field, a toast, a dialog and a download progress bar
- A source refresh shows a bar with the stage and a counter ("Parsing entries N of M", "Matching N of M") instead of a spinner, and Refresh all shows the overall percentage
- A chat message can carry up to four images, each up to 8 MB, by the paperclip button or by pasting from the clipboard. They show as previews, open full size, are stored on the server with the message and deleted with it after 7 days
### Changed
- The install dialog measures space for the folder you typed or picked and recalculates on every change. When the chosen folder is not empty it puts the game into a subfolder named after it, and it replaces the leftovers of an earlier failed extraction instead of refusing with "folder is not empty"
- A portable build moves to the same disk with a single rename: no copy, no doubled space, done in seconds. Move is now the default when seeding is off; to another disk the game is still copied with verification
- The bell no longer lists "Downloading N%" for every running download: completion and failures arrive as toasts
- Hidden games are hidden from friends too: the "hidden" mark travels to the account with the rest
- Steam tools and applications such as Proton, Steamworks and Wallpaper Engine no longer land in the library — only games, demos, betas and mods
- Removing a game or download data stops seeding those files on its own instead of sending you to the settings to turn it off. The "folder in use" error names what to close: the game itself and Explorer windows showing that folder
- The documents move to the 21 October 2026 edition: the policy describes portable mode and achievements. The launcher asks you to accept them again

- The catalog loads the next page by itself when you reach the end; the Show more button stays only after a failed load
- Source refreshes are several times faster: the server's answer about a found game is kept for a week instead of a day, and release files are rewritten only when something changed. Each cycle used to ask the server about half of the releases again
- A catalog search puts the game with exactly that title first, then titles that start with the query, and only then the rest by popularity
- The Reset appearance button no longer sits in the corner for good: it shows for a few seconds after a theme change, and Ctrl+Shift+Alt+T still resets at any time. The theme editor opens for the current theme right away instead of only after clicking a tile
- When the Typhon server does not answer, the bar at the top explains that the profile comes from cache, links to the service status page that shows what happened and when it is fixed, and closes with a cross until the next outage
### Fixed
- Downloads no longer fill the log with thousands of "short write" warnings and no longer lose finished pieces when two neighbouring ones finish at once, which used to surface as "error promoting part file" on Windows. A cancelled download no longer leaves a `.part` file behind
- Cancelling a file or folder dialog is no longer shown as an error
- CPU requirements given only as a clock speed and graphics requirements like "GTX 700 series" are now compared instead of staying "could not compare"
- The disk usage screen no longer lists removed games with an unknown size
- Chat reactions are drawn with their own icons: on Windows 10 one of them showed as an empty box. The chat button no longer shifts when a message arrives, and the desktop popup for a new message is redrawn

- The launcher finds WinRAR and 7-Zip installed anywhere, even after it started: the lookup also reads the installation records, and the log says where it looked. The error asks to try again when the archiver was just installed
- A download with a game folder and a small manifests archive next to it is no longer taken for an archive: the type follows the archive's share of the data
- A cancelled archive install removes the folder it created itself; folders that existed before the install are left alone
## 1.1.0 — 2026-10-19
Games from Steam, GOG and Epic now sit in one library with the rest, saves sync through your account, and every game gets its own properties, tags, screenshots and statistics.

### Added
- Cloud saves: turn on "Cloud saves" in the settings; it needs a signed-in account and your consent to upload. Your account keeps one copy of each catalog game's saves, so on a second PC or a Mac the game picks up where you left off
- The cloud copy is checked at startup, every five minutes and after you play. If the saves changed both on the computer and in the cloud, the launcher overwrites nothing on its own and asks which copy to keep, and puts the previous files aside as a local snapshot before replacing them
- The "Find Steam, GOG and Epic games" button in the settings adds games installed through other launchers to the library: no files are copied or changed, Steam and Epic games start through their own launcher, and playtime is counted as usual (Windows only)
- Games found this way stay in step with the other launchers: new installs are added on their own, and a game removed there becomes not installed — it keeps its playtime and catalog link
- Each game card in the library shows where the game comes from: Typhon, Steam, Epic or GOG, and once games come from more than one place, a filter by source appears
- The game's quick menu has "Add to Steam": the game shows up in Steam as a non-Steam game and starts from there with the Steam overlay and Steam Input. While Steam is running, the launcher leaves its files alone and asks you to close Steam (Windows only)
- "Properties" on the game page and in the quick menu: launch options, a different executable, run as administrator, your own saves folder and a "Show in Discord" switch for that game
- Games can carry tags, up to 20 each, and can be hidden. A hidden game leaves the library, "Continue playing", your profile, Discord and the online status, but stays under the "Hidden" filter
- Collections are saved selections by tags, genres and status: each one shows up as its own section in the library and as a shelf in Big Picture
- Take an in-game screenshot with Ctrl+F12; the key can be changed in the settings next to the overlay key. Screenshots go to the library folder, under Screenshots, per game, and the game page has a gallery of them
- The "Statistics" section shows how much you played over the last week, month or year: by day, by game, and your longest session
- A catalog game's page has a "Watch" button: once a release of that game appears in your sources, you get a notification in the bell
- If a game closes right after launch with an error, the launcher names the likely cause and offers to verify the files or install "Runtime libraries" — Visual C++, DirectX and .NET from the game folder. For libraries that are missing it opens the official Microsoft page

### Changed
- When moving a game, you pick the folder it goes into rather than the game folder itself: the game moves to "chosen folder\game folder name", and the final path is shown before the move starts. Before, only an empty folder could be picked
- The catalog opens at once: the first page is drawn from what the launcher received in the last half hour and is quietly refreshed. Paging through it and opening game cards is faster when the catalog had grown to tens of thousands of games — the launcher drops stale entries at startup
- The documents move to the 16 October 2026 edition: the privacy policy covers watched games, per-day statistics, screenshots, Steam shortcuts and cloud saves. The launcher will ask you to accept them again

### Fixed
- Moving a game can no longer hang or leave the game without a working copy: if a crash or a shutdown interrupted the move, the launcher finishes it or puts everything back on the next start
- The launcher deletes the game's old folder only after checking the copy, and if you kept playing from the old folder, it stays where it is. A game cannot be started while it is being moved
- When the launcher is started from a game shortcut, its taskbar window no longer takes that game's icon, and the pinned Typhon icon groups the window under it
- Errors in the game's quick menu show readable text instead of a code like "typhon:library.not_installed"

## 1.0.2 — 2026-10-13
The sign-in screen says what exactly went wrong, and deleting your account no longer throws you out of it when the request never reached the server.

### Changed
- On the sign-in and registration screens an empty field is no longer sent to the server: the launcher highlights what needs filling in and does not spend your attempt budget on it
- While a sign-in or registration is in progress you cannot switch between the two — before, the reply could land on the other screen and an error about a taken address ended up under the wrong field

### Fixed
- If the account is suspended, the sign-in screen says so and points to the Support section — before, it showed a generic server error and people kept trying to sign in
- When registering with an address that cannot receive mail, the hint sits next to the address field instead of the general error
- Deleting the account in the profile settings no longer drops you to the sign-in screen when the request never reached the server — for example, when the edge protection rejected it or a newer launcher is required: the account and the sign-in stay in place
- If the saved sign-in cannot be read from the system credential store, the launcher says so in its own words instead of reporting a server error
- An empty server reply about the profile is no longer taken for a successful sign-in

## 1.0.1 — 2026-10-13
Error reports and compatibility statistics no longer get lost on the way, and a busy sign-in tells you what to do.

### Changed
- When the account sign-in server is overloaded, the sign-in and registration windows ask you to try again in a couple of seconds — before, they showed a generic server error that looked like something had broken

### Fixed
- Error reports sent with "Anonymous diagnostics" turned on in the settings now arrive in full: the server used to refuse some of them — after the computer woke from sleep, for example — and they were lost
- On macOS, game compatibility statistics (sent only with "Anonymous usage statistics" turned on) also get through for people who have launched more than 500 games: the server used to refuse such a report, and the launcher kept resending it every 10 minutes
- The player card that opens when you hover over a friend in the friends list shows the whole name — before, the profile cover hid the top half of the letters

## 1.0.0 — 2026-10-11
A new interface, a profile you build yourself and support right inside the launcher. The launcher's online status can now be turned off, and without consent to usage statistics it no longer tells the server which computer it is or what you are playing.

### Added
- Your profile is built from blocks: a pinned game, a hand-picked or automatic collection, a genre breakdown, a play fingerprint and free text. Blocks are arranged and resized right on the profile page, and one button saves all the edits
- Profile styling without limits: any accent colour, a custom gradient, a theme taken from the art of the game you are playing, avatar frames, name styles and a status with an emoji
- Hovering a friend in the friends list opens their player card in the style they picked
- A Support section: open a ticket, keep the conversation going and close it without leaving the launcher. A badge and a notification tell you about a new reply. You can attach your logs to a ticket, and the launcher says how long the server keeps them before you do
- The sign-in screen has a "Forgot your password?" link and a way to the support page on the website for anyone who cannot sign in
- You can delete your account in the profile settings, under "Danger zone", after entering your password. Games and files on your computer stay where they are
- A game's page has a "Where to buy" block with links to the official Steam, GOG and Epic stores, which open in your browser
- On first launch and whenever the terms of use change, the launcher shows a short summary with an "I Agree" button; the installer shows the same during installation
- If a page has unsaved edits, for example in the profile editor, the launcher asks before you leave it

### Changed
- The whole interface is refreshed: "Continue playing" in the library is a large banner over the game's art, "Play" is the main button on a game's page with the update next to it, the catalog's filter bar sticks to the top, and buttons on covers appear on hover
- The dark theme is now blue, with a brighter accent that is easier to read. Pages animate smoothly, and when the system asks for reduced motion, animations no longer delay elements from appearing
- Downloads line up in even columns and stay still while speeds change; failed downloads get their own section instead of vanishing from the screen. History is grouped by day, and the disk bar under "Installed" matches its labels even when some game sizes are unknown
- The launcher's online status can be turned off in "Settings → General → Privacy". Without consent to usage statistics, the signal carries only a random session number, the launcher version and the system: the installation number and the game you are playing are no longer sent
- Because of this the launcher asks about sending data once more, with your previous answers already selected. In the consent window all switches start off, and diagnostics has its own switch, separate from statistics
- The launcher's source code is open under the GPL-3.0 licence

### Fixed
- Downloads and game sharing over the local network did not start when the usual port was taken on a computer with Hyper-V or Docker: the launcher kept trying ports from one range the system had reserved
- An install waiting for an exe to be picked after its download was removed stayed in the dock as "Action required" forever and could not be cleared. Such an install can be cancelled, and the launcher clears old ones by itself
- A game could not be removed while its install was waiting for an exe to be picked
- When two parts of the launcher saved settings at almost the same moment, one of the settings went back to its old value
- A server failure looked like an empty screen: a game "was missing" from the catalog, a source "had" no releases, a profile stayed blank. The launcher shows the error and a "Retry" button instead
- The log archive for support kept parts of game folder paths and the account's email address
- A built-in theme could not be saved as your own, and an exported theme could not be imported back
- Steam categories such as "Utilities" or "Movie" showed up untranslated in the catalog's genre filter
- A cover art error was repeated as a notification on every progress update of the downloads page
- Double-clicking cancel on an install in the dock showed an error even though the install was cancelled

## 0.9.0 — 2026-10-08
Chat, friends and music right over your game, save backups, and torrents through a VPN or a proxy. Installs and downloads no longer get stuck or lose data.

### Added
- In-game overlay: press Alt+` over a game to open chat, friends, music controls and a browser without leaving the game. Turn it on and set it up in Settings → General → Game overlay; Windows only for now
- The overlay does not open while a game runs in exclusive fullscreen; switch such a game to borderless windowed mode. If another program already uses Alt+`, pick Shift+F1, Shift+F2 or Ctrl+Shift+O instead
- Save backups: right-click a game → Saves… opens a window where you can back up now, restore any earlier copy and delete the ones you no longer need. Before restoring, the launcher makes a safety copy of your current saves
- Saves are backed up automatically after every play session and before a game update. Each game keeps its last 5 automatic copies (change the number in Settings → Downloads → Updates); manual copies are never deleted on their own. Backups stay on your computer
- Torrents can go through a VPN adapter or a SOCKS5 or HTTP proxy: Settings → Downloads → Torrent network. If the VPN disconnects or the proxy goes away, downloads and seeding pause with a banner at the top of the window and resume on their own when the network is back, so no traffic leaks around it
- The proxy password is kept in the system credential store, not in the settings file. DHT and UDP do not work through a proxy, so torrents without trackers will not find peers in that mode. Binding to a VPN adapter works on Windows only
- When an installer refuses to install silently, the install window offers Install manually, which opens the installer's usual wizard
- When the built-in extractor cannot unpack a RAR archive, the launcher tries an installed 7-Zip or WinRAR, and if there is none or it is too old, tells you which version to install
- Top bar search: Enter or the "N more in the catalog" line opens the catalog with your query

### Changed
- Checksum verification built into build installers is now opt-in: Settings → Downloads → Install → Verify build files during install. It is off by default, so an install finishes right away instead of waiting hours for the check
- Installer sound is muted during an install; other programs and system sounds are left alone (Windows)
- The launcher asks for confirmation before removing your avatar or cover, resetting your profile appearance and signing out

### Fixed
- An installer that needed administrator rights failed at once with "Could not complete the operation" when it was not installing silently
- When an installer put the game next to the library folder instead of inside it, the launcher could not find the game afterwards; the folder now comes from the entry the installer registers in Windows
- A fully downloaded game sometimes ended up in Error with a message about an unfinished .part file, usually when seeding or an antivirus held the file at that moment
- Double-clicking Retry started the installer several times
- The launcher did not start when an antivirus held a file in the launcher's own update folder
- When the file listing downloads, installs or game updates got damaged, the launcher wrote an empty list over it on exit and everything disappeared
- Cancelling a game update download could delete a folder named like the release in the source and leave the downloaded data behind
- An interrupted patch update showed "Update available" although some patches were already installed; you can now see which patch it stopped on
- Switching catalog genres quickly could leave a genre other than the last one you clicked selected
- Downloads that ended in an error disappeared from the Downloads screen; they now show under Failed with the reason, Retry and Remove from list
- Games without an executable had a Play button that only led to an error
- The update card said "New release available" when there was no newer version and only a rollback was left
- Shortcuts to the packager's website that installers drop on the desktop stayed after the install
- A build's install bar got stuck at 99% with a size larger than expected; when the final size is unknown, it shows how much has been written
- If the launcher closed in the middle of several installers in a row (a game and its add-ons), after a restart the game counted as installed and the add-ons were never installed
- After the launcher crashed, the installer kept running in the background
- A damaged or incomplete RAR archive is reported as damaged or incomplete, not as "format not supported"

## 0.8.0 — 2026-09-28
Steam-style game reviews and a fullscreen Big Picture mode for playing with a controller. Games started from shortcuts or requiring administrator rights now launch as expected.

### Added
- Game pages now have reviews: recommend a game or not, write a few words, mark other reviews as helpful and report spam. The sidebar shows the overall score, like “Very Positive, 92% of 120 reviews”
- You can write a review once your account is a day old and you have played the game for at least 30 minutes through Typhon. If it is too early, the launcher tells you how long is left and opens the form by itself when the time comes
- To keep reviews free of spam, you can post a new review once every 2 minutes and no more than 5 a day; links and the same text on different games are not accepted. A review reported by five people is hidden until a moderator checks it
- Big Picture mode: the “Big Picture mode” button in the top bar turns the launcher into a fullscreen, controller-friendly interface. It has your library, the catalog, downloads, friends, profile and settings — find, install and launch a game without leaving the couch
- Big Picture works with Xbox and PlayStation controllers: A selects, B goes back, the bumpers switch sections. When you quit a game, the window comes back to where you launched it from
- In chat, older messages show when they were sent next to the time: “Yesterday” or the date

### Fixed
- Games that need administrator rights now launch through the usual Windows prompt — before, such a game just failed to start with an error
- A game started from Typhon launches the same way a double-click in Explorer does — before, it inherited the launcher’s own environment and console, so some games behaved differently than on a normal start
- A game shortcut on the desktop starts the game even when Typhon was closed — before, it only opened the launcher window
- A game started from a shortcut while Typhon sits in the tray opens on top of other windows instead of somewhere behind them
- More releases from sources find their game in the catalog: names with versions, build tags, packager notes and DLC lists are read correctly. Before, some games had no downloads on their page
- Refreshing a large source no longer fails as a whole because of one failed part — matches already found are kept and the source shows a warning

## 0.7.2 — 2026-09-15
More reliable chat, more accurate release matching, and more complete interface translations.

### Changed
- You can keep typing and sending messages while chat reconnects. Pending messages show a “Sending” status
- Native dialogs, the tray menu, and the updater window follow the selected language; the Windows installer supports Russian and English

### Fixed
- Chat reconnects after temporary network and server errors. Fixed delivery interruptions caused by slow session checks and full event queues
- Drafts survive failed sends and are cleared after a successful send or retry if you have not changed the text in the meantime
- The first incoming message immediately appears in the conversation list. Refresh errors preserve loaded conversations; loading older messages and quickly toggling reactions retain their results during concurrent updates
- Release matching separates versions, build dates, and bonus notes from game titles more accurately. Fixed GTA name variants, repeated sequel numbers such as “2 (II)”, and mixed Cyrillic and Latin letters
- Editions such as GTA IV: Complete Edition more accurately select their own catalog entry; alternative numeral spellings are checked while preserving distinct game identities
- Added missing genre and theme translations, statuses, and error messages for sources, updates, and local network transfers. Notifications and ongoing operations refresh their text when the language changes

## 0.7.1 — 2026-09-14
Catalog loading is independent of recommendations, search uses the server catalog, and releases match games more accurately.

### Changed
- The game list loads alongside the discovery shelf. Refreshing the shelf or requesting different picks preserves loaded pages; shelf games do not appear twice in the list
- Reopening the catalog shows cached picks immediately and refreshes them in the background when needed. Network errors preserve recommendations already on screen
- Preparing recommendations requires fewer requests and less repeated library processing

### Fixed
- Global search finds games in the server catalog even if their detail pages have never been opened. The local library and sources add installation and release information to those results
- Updating a source matches release titles against official names and name variants in the server catalog. Previously saved unmatched releases are checked again even when the source has not changed; manual matches are preserved
- Version numbers, packager details, and bonus content notes are separated from game titles more accurately when matching releases

## 0.7.0 — 2026-09-14
Chat with friends, personalized catalog recommendations, and profile customization with your own cover and colors.

### Added
- Direct messages to friends inside the launcher, with conversation history, reactions, read receipts, and unread counts. Chat is available when social features are enabled
- Sound notifications for new messages, with a shortcut to the conversation
- A discovery shelf featuring games outside your library and clear recommendation reasons. Picks use favorites, play history, genres, and themes
- Catalog sorting by popularity, rating, and release date, plus automatic ordering that gradually reflects your interests. “Not interested” hides a game from recommendations; dismissed games can be viewed and restored
- Profile appearance customization with a live preview: upload and crop a 4:1 cover, choose a theme including black, and set custom accent colors. Other users see this appearance when viewing your profile
- Custom HEX color input and copying in the app accent palette

### Changed
- Returning from a game keeps loaded catalog pages without downloading the entire list again. Changing favorites preserves the current pages and scroll position; new picks apply after an explicit refresh or filter change
- Clicking the selected catalog genre or library filter again shows all games
- Catalog grid cards show the developer when that information is available
- Catalog metadata coverage is shown in status details so incomplete data no longer looks like an endless import
- The launcher checks for updates every 15 minutes and retries sooner after network failures
- When automatic error reporting is enabled, reports include sanitized operation details and recent events to help identify failures. Full logs are still sent manually after confirmation

### Fixed
- Genre filters work consistently across Steam and IGDB data, Russian game details, and previously saved catalogs
- Searching for a game to link your own release also finds games previously marked as not interested
- Errors from earlier catalog requests no longer obscure newer results; new sorting options explain their unavailability when the server version is incompatible

## 0.6.1 — 2026-09-12
Fixed Steam and IGDB duplicates, card refreshes, and catalog pagination.

### Changed
- All Games uses the titles and covers supplied by the catalog without requesting full metadata for every visible card. Detailed information loads when opening a game
- Server-side page loading, search and filter counts are faster across the large Steam and IGDB catalog

### Fixed
- A game from Steam and IGDB no longer remains two cards after a confirmed link is found: the launcher accepts new links while the index is incomplete and refreshes the list after viewing details
- Older saved pages use provider links already learned by the launcher; installations, favorites, and personal references to previous cards are preserved
- Developers are shown from available catalog data without opening every game's details
- Background Steam and IGDB linking processes consecutive batches without a minute-long pause between them and continues past missing provider records
- The next-page loading error during background catalog updates. Changes outside the displayed list no longer reset pagination; changes to the displayed list are still checked to prevent skipped or repeated games
- A server-side deadlock between Steam imports and IGDB synchronization that caused catalog updates to fail
- Fixed 1 additional non-critical bug

## 0.6.0 — 2026-09-11
A Steam and IGDB catalog, personal accent colors, screenshot zoom and more reliable game updates.

### Added
- A Steam and IGDB game catalog: search and browsing are no longer limited to connected release sources. Filter by platform and entry type: games, bundles and editions
- Saved catalog pages remain available offline with a stale-data notice. While the server imports the index, the launcher indicates that the catalog is still incomplete
- Steam is an additional source of descriptions, covers and other game information; details and images load as you browse
- Personal accent colors: choose a palette preset or preview a custom shade. The accent applies over the selected theme, including light and custom themes
- Zoom screenshots up to 400%, drag to pan, and adjust zoom with the wheel, slider or “+” and “−” keys; “0” resets the view

### Changed
- Releases are separate from the official catalog: unknown releases stay in source management, and manual matching applies only to the selected release. Game pages explain when no releases are available
- Release lists retain original titles to distinguish editions and variants. Game details come from the catalog rather than release titles
- Updates and patches must belong to the same distribution and source as the installed game. The source must supply a stable distribution identifier; matching titles, packagers or versions are insufficient. New revisions with an unchanged version number are also considered
- Settings use an adaptive layout, grouping theme selection, accents and the appearance editor. Controls and image transitions have been refreshed and respect reduced-motion preferences
- The “What's new” window is wider to make longer changelogs easier to read

### Fixed
- Game removal requires local proof that Typhon installed it: a cloud flag alone cannot authorize deleting an externally installed game folder and its saves
- If contact with an installer is lost, its folder is not cleaned up until the process is confirmed stopped; two installations cannot occupy the same folder concurrently
- Protected recovery after an interrupted game update or failed rollback: new operations cannot overwrite an unfinished recovery journal, and restarting Typhon resumes recovery of both files and the library record
- macOS self-updates now replace the application atomically: a crash between renames can no longer leave the usual application path empty. This protection applies when updating from 0.6.0 or later
- With administrator permission granted in advance on Windows, the installer and job parameters are verified before execution: replacement of the file or job after authorization is rejected
- After navigating between games, a loading failure no longer leaves the previous game's releases on the new page, where they could be downloaded by mistake
- Fixed critical identity preservation issues in the new catalog: partial Steam/IGDB responses do not erase known identifiers or local matches; index refreshes do not revive removed duplicates, and mapping corrections preserve game ownership of provider links
- During preparation of the new catalog, fixed indexing stopping at a malformed record, excessive delays after rate limiting, and a separate cover request for every bulk search result
- Also fixed 39 confirmed non-critical bugs across the launcher, server and website found during verification of this release

## 0.5.2 — 2026-09-10
More reliable game installation through CrossOver, clearer log uploads, and activity history preserved after removing a game.

### Added
- On macOS, the executable for a Windows game can be selected through a CrossOver dialog; its folder opens in the file explorer of its CrossOver environment
- Log uploads show archive preparation, transfer progress, and the wait for the server response
- The website now has getting-started, macOS, and game-transfer guides, a help section, and release history in English and Russian

### Changed
- Installation progress stays below 100% until the installer finishes; QuickSFV runs appear as a separate file-verification stage
- Executable selection after installation shows full paths and ranks servers and editors below the game itself
- When error reporting is enabled, Typhon also sends scrubbed messages and stacks for ordinary internal errors. A full log archive is still sent only manually after confirmation

### Fixed
- Fixed extraction stalls affecting some build installers in CrossOver and waiting for successful QuickSFV completion. Recognized installers have music disabled and optional website-opening actions deselected
- Cancelling an installation in CrossOver waits for the installer and its child processes to stop before cleaning up files
- Retrying an installation into an originally empty folder preserves the ability to uninstall the game from the computer
- Removing a game from the library preserves its local activity history and total playtime; history no longer opens a missing game page
- Fixed premature log-upload timeouts, server-side archive blocking, and copying the support reference in the desktop app
- Invalid older error reports no longer block subsequent deliveries; large queues are split into batches

## 0.5.1 — 2026-09-09
Easier game launching on Mac, clearer friend activity, and more reliable library moves, game updates and account sync.

### Added
- Games on macOS now use a shared Steam environment in CrossOver by default. Choose it in Settings or override it for an individual game on its page; CrossOver is still installed separately
- Away status lets friends see when you have not used your computer for a while

### Changed
- Game pages distinguish friends who have the game in their library from friends playing it right now
- Log archives saved locally or sent to support receive an additional cleanup pass to hide recognized paths, addresses, tokens and other sensitive values. Review the contents before sharing an archive
- Log upload errors explain whether to try again later, reduce the archive size or wait for a rate limit to expire

### Fixed
- Saving settings and installation state on Windows retries brief file locks from another reader, keeping the previous complete copy until replacement succeeds
- Typhon picks a game's launch file more accurately after installation, avoiding helper programs with similar names
- Improved launching and tracking games through CrossOver on macOS, handling the shared Steam environment and loading DLLs from the game folder
- A game added back to the library could disappear again after sync. Also fixed syncing large libraries and carrying pending actions across different accounts
- Library moves and local-network transfers preserve existing destination files; retrying a move no longer attempts to move an already transferred game again
- Deleting downloaded files from a shared folder is limited to that download's files; cancelling a download waits for background work on its files to finish
- Game repair and verification handle the rollback backup more carefully. Rolling back also restores the displayed game version
- Rapid changes to multiple settings no longer overwrite one another; delayed feed and sent-data responses no longer replace newer results
- Disabling error reports prevents old pending reports from reappearing in the queue; a delivered report is no longer repeatedly sent when its local file cannot be deleted
- The version in macOS application metadata now matches the launcher version

## 0.5.0 — 2026-09-08
The launcher runs on macOS, downloading is told apart from installing, and games stop splitting across the catalogue or offering each other the wrong update.

### Added
- Typhon runs on macOS (Apple Silicon Macs): Windows games are installed and launched through CrossOver, and the launcher builds a separate environment for each game. CrossOver itself is installed separately — About shows its version, or warns that it is missing
- On macOS a game can go to the desktop as a shortcut, and the launcher can start at login
- A compatibility journal keeps itself: after a game fails to start twice in a row it is marked "does not start" in Installed, and a minute of play clears the mark
- On macOS the catalogue shows how many people the game starts for, and a "Only ones that start" filter. The number appears once at least five different machines have reported on the game
- A compatibility report is sent by consent only: the game, the release version and packager, the macOS version, the CrossOver version and the chip family — no free-form text, no release names, no paths. Consent is asked again, because the previous prompt promised nothing about hardware would be collected
- A "Download" button separate from "Install": with nothing on disk the game page offers to download, and install appears once there is nothing left to fetch
- An "Install after download" checkbox in the download window: it starts from the setting but applies to that one download
- Administrator rights can be confirmed up front, before the download starts, when the torrent carries an installer: the install then runs overnight on its own instead of waiting for a Windows prompt
- The source preview says how many games a feed describes, how many of them the catalogue already has, and how many are new
- A source served over plain http is flagged with a shield in the list and a warning in the preview: its contents can be swapped on the way
- File verification shows up in the activity dock — its progress used to live on the game page only, so leaving the page hid a scan that keeps running
- The release list carries the build form — "Portable", "Repack", "Archive" — and the packager is written next to the source the release came from
- A "Send to us" button in About: the log archive goes to support and is kept there for three days, and the launcher shows the ticket number. Before sending, the window says what is inside — your system user name, folder paths, and the names of games and torrents
- Unfriending, blocking and discarding a downloaded file ask for confirmation: they used to fire straight from the context menu

### Changed
- The interface is a tenth larger: what the 110% scale used to draw is now what 100% draws, and the other steps moved with it
- Built-in theme names follow the interface language instead of staying Russian in English
- The packager and edition lists moved into a dictionary that refreshes from the server once a day: a new packager is recognised without a new launcher version, and your own list can go into a file in the config folder
- Sources refresh faster: releases the feed did not touch are not matched again, and saving a source no longer downloads the feed a second time after the preview
- A running game is noticed, and its session closed, within a second — it used to take up to ten
- The avatar goes to the server as the file you picked, and the square is cut there: the picture is no longer resampled twice on the way
- The activity feed and the profile show a game as a wide shot instead of a cropped portrait cover
- The launcher uses less memory on large catalogues and sources: the game list is not held twice, and releases that vanished from a feed no longer pile up without a ceiling

### Fixed
- A game removed from the library on one device came back from another on the next sync: the removal now travels to the server and reaches the other devices, even if there was no network at the moment it was removed
- One release could split across the catalogue into eight different games: in "Game v.1.0.29315 [Game folder]" the version and the brackets stayed in the name, so every entry counted as its own game
- A 1.8 GB build was offered as an update to a 9.8 GB installation when both were published on the same day and neither version could be read
- An install started from a release row lost its version, and updates for that game were never found afterwards
- The release list labelled "Update" even a release the launcher does not treat as one: those read "New release"
- A torrent is no longer matched automatically to a DLC instead of the game itself; picking a DLC by hand still works
- Confirmations for deleting a theme, clearing history and cancelling a download never appeared at all on macOS, and the action silently did nothing
- Closing the add-download window while it says "Fetching details" no longer holds the torrent busy: it can be added again right away instead of a minute and a half later
- Online status disappeared until the launcher was restarted if the server stopped answering the presence request for a minute
- Errors are shown in the interface language: a technical server reply could turn up where readable text belongs — in search, in the library folder setup, and when updating or removing a game
- Percentages disagreed: 4% on the game card, 3% in the activity dock. One rounding rule for the whole interface, rounding down — 100% is not shown until the work is done
- Auto-install started even when the disk had no free space left at all
- An install the installer finished while the launcher was closed could be marked interrupted: one failed read of the state file, at the moment the installer was replacing it, was enough to decide
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
