import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

type Site = [tone: string, announces: boolean | 'dynamic'];

/**
 * Which notices announce. A notice that reports something that has just happened (a load or a request
 * that failed, the connection going away) announces by its tone; one that states a standing fact of the
 * page it is rendered with does not (`announce={false}`): it is read in the flow of the page. A new
 * notice is added here with that decision; a tone or an announce set by an expression (or a spread) is
 * listed as 'dynamic', so the table says so instead of assuming the default. A source tripwire:
 * Notice.dom.test.ts proves the option.
 */
const SITES: Record<string, Site[]> = {
  'lib/components/ChatPanel.svelte': [
    ['danger', true],
    ['danger', true],
    ['warning', true],
    // The conversation cannot be written to: a fact of the conversation, shown whenever it is open.
    ['info', false],
    ['danger', true],
  ],
  // Names missing from the list and unread progress files are facts of the card; a failed load just happened.
  'lib/components/AchievementsCard.svelte': [
    ['warning', false],
    ['warning', false],
    ['danger', true],
  ],
  'lib/components/NetworkBanner.svelte': [['danger', true]],
  'lib/components/OfflineBanner.svelte': [['warning', true]],
  // The stable version on offer to a beta: a fact of the settings page.
  'lib/components/StableOffer.svelte': [['info', false]],
  // A state shown when the settings page opened is read with it; one that comes after is announced.
  'lib/components/UpdateBanner.svelte': [
    ['danger', 'dynamic'],
    ['info', 'dynamic'],
    ['success', 'dynamic'],
    ['info', 'dynamic'],
  ],
  // The stored error of the last update and the running game that holds an update back, shown on
  // every opening of the game.
  'lib/components/UpdateCard.svelte': [
    ['danger', false],
    ['info', false],
  ],
  'lib/components/SystemRequirements.svelte': [['danger', true]],
  // Advice for the chosen platform, also inside InstallModal and AddDownloadModal.
  'lib/components/EmulatorHint.svelte': [['warning', false]],
  // A block whose data failed to load; blocks of a newer launcher filling the layout, a standing fact.
  'lib/components/profile/BlockGrid.svelte': [['danger', true]],
  'lib/components/profile/BlockPicker.svelte': [['warning', false]],
  'lib/components/profile/GamePicker.svelte': [['danger', true]],
  'routes/activity/Activity.svelte': [['danger', true]],
  'routes/auth/AuthScreen.svelte': [['danger', true]],
  // The page fell back to a cached copy; a game just put away from the recommendations, with its undo;
  // the picks that fell back or went stale; the first page or a next page failed.
  'routes/catalog/Catalog.svelte': [
    ['warning', true],
    ['success', true],
    ['info', true],
    ['warning', true],
    ['danger', true],
    ['danger', true],
  ],
  'routes/diskusage/DiskUsage.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  // Safety advice of the requests tab, read with it.
  'routes/friends/Friends.svelte': [['info', false]],
  // Downloads held while a game runs: it starts and ends while the page is open.
  'routes/downloads/Downloads.svelte': [['info', true]],
  // In source order: the downloads and the other versions failed to load (just happened); the stored
  // error of the last download and the uninstalled game, shown on every opening; the metadata search
  // running; "not found" or "failed" of the metadata, its tone set by which, announced when a search
  // ran while the page was open and read with the page when it opens on it.
  'routes/game/GameDetails.svelte': [
    ['danger', true],
    ['danger', true],
    ['danger', false],
    ['info', false],
    ['info', true],
    ['dynamic', 'dynamic'],
  ],
  'routes/game/GameReviews.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  // The library folder missing is a state of the settings, not of this visit.
  'routes/game/GameScreenshots.svelte': [
    ['warning', false],
    ['danger', true],
  ],
  // A review hidden by moderation is a fact of the review.
  'routes/game/ReviewComposer.svelte': [
    ['danger', true],
    ['warning', false],
  ],
  // The reviews list fails on the same request and announces it with its Retry.
  'routes/game/ReviewSummary.svelte': [['danger', false]],
  // The history file could not be written: a state of the page, not an event of this visit.
  'routes/history/History.svelte': [['danger', false]],
  'routes/installed/Installed.svelte': [['danger', true]],
  'routes/profile/Profile.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  // Where sources are fetched: a standing fact of the page.
  'routes/sources/Sources.svelte': [['info', false]],
  'routes/stats/Stats.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  // The art of the auto theme failed; the cover was refused; the worn frame was revoked before this visit.
  'routes/profile/ProfileAppearancePanel.svelte': [
    ['warning', true],
    ['danger', true],
    ['warning', false],
  ],
  'routes/profile/ProfileHeader.svelte': [['danger', true]],
  // The connection went while the dialog is open; a save was refused.
  'routes/profile/ProfileSettingsModal.svelte': [
    ['warning', true],
    ['danger', true],
  ],
  'routes/settings/AppearanceTab.svelte': [
    // The stored theme's CSS breaks a later rule: a fact of the theme, shown whenever it is edited.
    ['warning', false],
    // The errors a kept trial updates at every edit: on screen, not announced while the user types.
    ['danger', false],
    ['danger', true],
  ],
  'routes/settings/DiscordCard.svelte': [
    ['danger', true],
    ['danger', true],
    ['warning', true],
    ['danger', true],
  ],
  'routes/settings/EmulatorsCard.svelte': [
    ['danger', true],
    // A known hang of one emulator on this platform, shown with its row.
    ['warning', false],
  ],
  'routes/settings/LibraryFoldersRow.svelte': [['danger', true]],
  'routes/settings/LibraryLocationRow.svelte': [['danger', true]],
  'routes/settings/NetworkSettingsCard.svelte': [
    ['danger', true],
    ['warning', true],
    // What the proxy mode turns off, shown with the proxy fields.
    ['warning', false],
    ['dynamic', true],
    ['danger', true],
  ],
  'routes/settings/Settings.svelte': [
    // A failed overlay inside the row's status region, which says the change itself.
    ['danger', false],
    ['danger', true],
    ['danger', true],
    ['danger', true],
    ['danger', true],
    // This build has no legal documents to show: a fact read with the card.
    ['info', false],
  ],
  'routes/support/NewTicket.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  'routes/support/TicketList.svelte': [['danger', true]],
  'routes/support/TicketView.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  'routes/user/UserCommonModal.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  // The profile failed to load; a closed or a friends-only profile is a standing fact of the page.
  'routes/user/UserProfile.svelte': [
    ['danger', true],
    ['info', false],
    ['info', false],
  ],
};

// The attributes of a tag up to its closing `>`, skipping what sits inside quotes and braces, so an
// arrow function or a comparison in an expression does not end the tag. A spread is `{...}` by name.
function attributes(source: string, from: number): Map<string, string> {
  const out = new Map<string, string>();
  let i = from;
  while (i < source.length && source[i] !== '>' && !(source[i] === '/' && source[i + 1] === '>')) {
    if (/\s/.test(source[i])) {
      i++;
      continue;
    }
    const start = i;
    let name = '';
    if (source[i] === '{') {
      name = '{...}';
    } else {
      while (i < source.length && !/[\s=>/{]/.test(source[i])) i++;
      name = source.slice(start, i);
      if (source[i] !== '=') {
        out.set(name, 'true');
        continue;
      }
      i++;
    }
    const valueStart = i;
    if (source[i] === '"' || source[i] === "'") {
      const quote = source[i];
      i = source.indexOf(quote, i + 1) + 1;
    } else if (source[i] === '{') {
      let depth = 0;
      let quote = '';
      for (; i < source.length; i++) {
        const c = source[i];
        if (quote) {
          if (c === '\\') i++;
          else if (c === quote) quote = '';
        } else if (c === '"' || c === "'" || c === '`') quote = c;
        else if (c === '{') depth++;
        else if (c === '}' && --depth === 0) break;
      }
      i++;
    } else {
      while (i < source.length && !/[\s>]/.test(source[i])) i++;
    }
    out.set(name, source.slice(valueStart, i));
  }
  return out;
}

function notices(source: string): Site[] {
  const out: Site[] = [];
  for (const m of source.matchAll(/<Notice\b/g)) {
    const attrs = attributes(source, (m.index as number) + m[0].length);
    const spread = attrs.has('{...}');
    const tone = attrs.get('tone');
    const announce = attrs.get('announce');
    out.push([
      spread || (tone !== undefined && !/^"\w+"$/.test(tone)) ? 'dynamic' : (tone?.slice(1, -1) ?? 'info'),
      spread || (announce !== undefined && !/^\{(true|false)\}$/.test(announce)) ? 'dynamic' : announce !== '{false}',
    ]);
  }
  return out;
}

function sites(): Record<string, Site[]> {
  const out: Record<string, Site[]> = {};
  for (const [path, source] of Object.entries(frontendSources)) {
    if (!path.endsWith('.svelte')) continue;
    for (const site of notices(source)) (out[path] ??= []).push(site);
  }
  return out;
}

describe('notices', () => {
  it('announce what has just happened and leave standing facts to the flow of the page', () => {
    expect(sites()).toEqual(SITES);
  });

  it('are read whole, past the `>` of an arrow function in an attribute', () => {
    expect(notices('<Notice tone="warning" onclose={() => (shown = false)} announce={false}>text</Notice>')).toEqual([['warning', false]]);
  });

  it('name a tone or an announce set by an expression as dynamic, not as a default', () => {
    expect(notices('<Notice tone={kind} announce={quiet}>text</Notice>')).toEqual([['dynamic', 'dynamic']]);
    expect(notices('<Notice\n  tone="danger"\n  {...rest}\n>text</Notice>')).toEqual([['dynamic', 'dynamic']]);
  });
});
