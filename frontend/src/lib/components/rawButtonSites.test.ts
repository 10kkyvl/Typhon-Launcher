import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

/**
 * The main window draws its buttons with the base: Button, IconButton, Chip, TextLink, the segmented
 * control, tabs, menu items, the switch and the checkbox. A raw <button> elsewhere is either a
 * control of another kind, listed here by file and class with what it is, or a hand-made copy of a
 * base control, which this guard refuses. Big Picture, the overlay and the test harnesses are not on
 * this path. A new raw button is added here with its kind, or drawn with the base.
 *
 * Limits: the markup is read as written; an entry covers every raw button of its class in its file,
 * and what the button looks like is not read: the kind is the reviewer's to check against the site.
 */

type Kind =
  | 'row' // a row or a card that opens what it shows (a game, a profile, a ticket, a dialog)
  | 'nav item' // the sidebar's navigation
  | 'radio tile' // one choice of a radio group, drawn as a tile or a swatch (roving.ts)
  | 'list option' // one pick from a list of candidates in a dialog or a panel
  | 'tile' // a tile that selects what the panel beside it shows
  | 'thumbnail' // a picture that opens the image viewer
  | 'swatch' // a colour that opens its palette, or one colour of a set
  | 'cover control' // drawn over a game's cover, on its scrim
  | 'title bar' // the window's own buttons
  | 'account chip' // the top bar's account: avatar, name and status as one trigger
  | 'drag handle' // grabbed to move a block
  | 'copies itself' // a value that copies its own text when pressed
  | 'badge trigger' // a badge that opens a menu; Chip has no size small enough for the sidebar rail
  | 'panel trigger' // a summary line that opens a popover panel about what it shows (the activity widget's head)
  | 'disclosure'; // the live island's message: the pill that opens into its card, the card's head that closes it

/** The base components that own a raw <button>: the only place a button's look is written. */
const BASE = ['Button', 'Checkbox', 'Chip', 'IconButton', 'MenuList', 'SegmentedControl', 'Select', 'Tabs', 'TextLink', 'Toggle'].map(
  (name) => `lib/components/${name}.svelte`,
);

const NOT_YET = (path: string) =>
  path.startsWith('lib/bigpicture/') || path.startsWith('overlay/') || path.startsWith('lib/testing/') || path === 'lib/components/BigPictureShell.svelte';

/** Raw buttons that are not a base control, by file and class. */
const ALLOWED: Record<string, Record<string, Kind>> = {
  'lib/components/AccountMenu.svelte': { chip: 'account chip' },
  'lib/components/AchievementsCard.svelte': { tile: 'tile' },
  'lib/components/ActivityWidget.svelte': { head: 'panel trigger', row: 'row' },
  'lib/components/BetaBadge.svelte': { badge: 'badge trigger' },
  'lib/components/ChatAttachments.svelte': { tile: 'thumbnail' },
  'lib/components/ChatConversations.svelte': { conversation: 'row' },
  'lib/components/ChatPanel.svelte': { 'toast-open': 'row' },
  'lib/components/FeedRow.svelte': { who: 'row', game: 'row' },
  'lib/components/GameCard.svelte': { cover: 'row', info: 'row', play: 'cover control', 'quick-btn': 'cover control' },
  'lib/components/GameProfiles.svelte': { candidate: 'radio tile' },
  'lib/components/GamePropertiesModal.svelte': { candidate: 'list option' },
  'lib/components/InstallModal.svelte': { cand: 'radio tile' },
  'lib/components/LiveIsland.svelte': { head: 'disclosure', top: 'disclosure' },
  'lib/components/MetadataMatchModal.svelte': { candidate: 'list option' },
  'lib/components/NotificationsPanel.svelte': { notification: 'row' },
  'lib/components/ReleaseMatchModal.svelte': { candidate: 'radio tile' },
  'lib/components/RemoveGameModal.svelte': { mode: 'radio tile' },
  'lib/components/SavesCandidatesModal.svelte': { candidate: 'list option' },
  'lib/components/Sidebar.svelte': { 'nav-item': 'nav item' },
  'lib/components/SidebarStatus.svelte': { item: 'row' },
  'lib/components/SourceDetailsModal.svelte': { row: 'row' },
  'lib/components/TitleBar.svelte': { wb: 'title bar' },
  'lib/components/profile/BlockGrid.svelte': { handle: 'drag handle' },
  'lib/components/profile/BlockPicker.svelte': { preset: 'list option' },
  'lib/components/profile/CollectionBlock.svelte': { tile: 'row' },
  'lib/components/profile/GamePicker.svelte': { row: 'list option' },
  'lib/components/profile/PinnedBlock.svelte': { hero: 'row' },
  'routes/activity/PopularGames.svelte': { row: 'row' },
  'routes/activity/WeekActivity.svelte': { row: 'row' },
  'routes/catalog/Catalog.svelte': { 'list-row': 'row' },
  'routes/friends/AddFriendModal.svelte': { preview: 'row' },
  'routes/friends/FriendCodeCard.svelte': { code: 'copies itself' },
  'routes/friends/FriendRow.svelte': { who: 'row' },
  'routes/game/GameDetails.svelte': { shot: 'thumbnail' },
  'routes/game/GameFriendsPanel.svelte': { who: 'row' },
  'routes/game/GameScreenshots.svelte': { thumb: 'thumbnail' },
  'routes/game/ReportReviewModal.svelte': { reason: 'list option' },
  'routes/game/ReviewCard.svelte': { who: 'row' },
  'routes/game/ReviewComposer.svelte': { toggle: 'radio tile' },
  'routes/game/ReviewSummary.svelte': { score: 'row' },
  'routes/installed/Installed.svelte': { game: 'row', 'card-cover': 'row' },
  'routes/library/Library.svelte': { 'list-row': 'row' },
  'routes/profile/ProfileActivity.svelte': { row: 'row' },
  'routes/profile/ProfileAppearancePanel.svelte': { theme: 'radio tile', 'pick-card': 'radio tile', 'emoji-cell': 'radio tile', color: 'swatch' },
  'routes/profile/ProfilePlaying.svelte': { open: 'row' },
  'routes/settings/AccentPicker.svelte': { swatch: 'radio tile' },
  'routes/settings/ThemeTiles.svelte': { preset: 'radio tile' },
  'routes/sources/Sources.svelte': { source: 'row' },
  'routes/stats/Stats.svelte': { row: 'row' },
  'routes/support/TicketList.svelte': { row: 'row' },
  'routes/user/GameRow.svelte': { row: 'row' },
  'routes/user/UserActivity.svelte': { row: 'row' },
  'routes/user/UserMutual.svelte': { person: 'row' },
  'routes/user/UserRecent.svelte': { capsule: 'row' },
};

/**
 * Hand-made copies of a base control that their package has not moved yet: each with what it should
 * be. Not allowed, only waited for: an entry goes when its site is drawn with the base.
 */
const PENDING: Record<string, Record<string, string>> = {};

interface Raw {
  path: string;
  line: number;
  /** The first class of the button, or its first `class:` directive when it has no class attribute. */
  key: string;
  radio: boolean;
}

// Markup only: script and style blocks blanked, so line numbers stay.
function markup(text: string): string {
  return text.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, (block) => block.replace(/[^\n]/g, ' '));
}

/** Every raw <button> of `text`; a `>` inside braces or a quoted value does not end its tag. */
function rawButtons(path: string, text: string): Raw[] {
  const html = markup(text);
  const found: Raw[] = [];
  for (const start of html.matchAll(/<button\b/g)) {
    let depth = 0;
    let quote = '';
    let end = start.index + start[0].length;
    for (; end < html.length; end += 1) {
      const c = html[end];
      if (quote) {
        if (c === quote) quote = '';
      } else if (depth === 0 && (c === '"' || c === "'")) quote = c;
      else if (c === '{') depth += 1;
      else if (c === '}') depth -= 1;
      else if (c === '>' && depth === 0) break;
    }
    const tag = html.slice(start.index, end + 1);
    const key = /\sclass="([^"\s]+)/.exec(tag)?.[1] ?? /\sclass:([\w-]+)/.exec(tag)?.[1] ?? '';
    found.push({ path, line: html.slice(0, start.index).split('\n').length, key, radio: /\srole="radio"/.test(tag) });
  }
  return found;
}

const scanned = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !NOT_YET(path) && !BASE.includes(path));
const raw = scanned.flatMap(([path, text]) => rawButtons(path, text));
const listed = (list: Record<string, Record<string, unknown>>, { path, key }: Raw) => key in (list[path] ?? {});
const entries = (list: Record<string, Record<string, unknown>>) => Object.entries(list).flatMap(([path, classes]) => Object.keys(classes).map((key) => ({ path, key })));

describe('no hand-made button look outside the base', () => {
  it('reads the main window and finds its raw buttons', () => {
    expect(scanned.length).toBeGreaterThan(150);
    expect(raw.length).toBeGreaterThan(60);
    expect(scanned.some(([path]) => path.startsWith('routes/showcase/'))).toBe(true);
  });

  it('every raw button is a control of another kind, listed with what it is, or waited for', () => {
    const found = raw.filter((button) => !listed(ALLOWED, button) && !listed(PENDING, button)).map(({ path, line, key }) => `${path}:${line} .${key}`);
    expect(found).toEqual([]);
  });

  it('holds no stale entry: every listed file and class still has a raw button', () => {
    const present = new Set(raw.map(({ path, key }) => `${path} .${key}`));
    const stale = [...entries(ALLOWED), ...entries(PENDING)].filter(({ path, key }) => !present.has(`${path} .${key}`)).map(({ path, key }) => `${path} .${key}`);
    expect(stale).toEqual([]);
    expect(BASE.filter((path) => rawButtons(path, frontendSources[path] ?? '').length === 0)).toEqual([]);
    expect(entries(ALLOWED).filter((entry) => listed(PENDING, { ...entry, line: 0, radio: false })).map(({ path, key }) => `${path} .${key}`)).toEqual([]);
  });

  // The control a Popover renders in its trigger snippet opens a panel: it is not a row, whatever it shows.
  it('a raw button that opens a popover is listed as a panel trigger', () => {
    const triggers = scanned.flatMap(([path, text]) => {
      const html = markup(text);
      return [...html.matchAll(/<Popover\b[\s\S]*?\{#snippet trigger\b[\s\S]*?\{\/snippet\}/g)].flatMap((m) =>
        rawButtons(path, html.slice(0, m.index).replace(/[^\n]/g, ' ') + m[0]),
      );
    });
    expect(triggers.length).toBeGreaterThan(0);
    expect(triggers.filter((button) => ALLOWED[button.path]?.[button.key] !== 'panel trigger').map(({ path, line, key }) => `${path}:${line} .${key}`)).toEqual([]);
  });

  it('a radio tile says it is a radio', () => {
    const tiles = raw.filter((button) => ALLOWED[button.path]?.[button.key] === 'radio tile');
    expect(tiles.length).toBeGreaterThan(10);
    expect(tiles.filter((button) => !button.radio).map(({ path, line }) => `${path}:${line}`)).toEqual([]);
  });

  it('the showcase and the full-screen gates draw their buttons with the base only', () => {
    const none = (prefix: string) => raw.filter(({ path }) => path.startsWith(prefix)).map(({ path, line }) => `${path}:${line}`);
    expect(none('routes/showcase/')).toEqual([]);
    for (const gate of ['LegalAcceptanceScreen', 'TelemetryConsentScreen', 'UpdateOverlay', 'SocialConsentScreen']) expect(none(`lib/components/${gate}.svelte`)).toEqual([]);
  });

  // Printed so the sites waiting for their package stay in sight on every run (a reporter that hides
  // the output of passing tests hides this too; `--reporter=default` shows it).
  it('lists what still waits for its package', () => {
    const waiting = raw.filter((button) => listed(PENDING, button)).map(({ path, line, key }) => `${path}:${line} .${key} -> ${PENDING[path][key]}`);
    if (waiting.length > 0) console.warn(`raw buttons waiting for a base control (${waiting.length}):\n  ${waiting.join('\n  ')}`);
    expect(waiting).toHaveLength(entries(PENDING).length);
  });

  it('reads a button whatever its tag holds', () => {
    const sample = `<script>const b = '<button class="fake">';</script>
<button class="row wide" onclick={() => go({ a: 1 })} title="1 > 0">x</button>
<button type="button" role="radio" class:swatch={on}>y</button>
<Button onclick={go}>z</Button><button>bare</button>`;
    expect(rawButtons('x.svelte', sample)).toEqual([
      { path: 'x.svelte', line: 2, key: 'row', radio: false },
      { path: 'x.svelte', line: 3, key: 'swatch', radio: true },
      { path: 'x.svelte', line: 4, key: '', radio: false },
    ]);
  });
});
