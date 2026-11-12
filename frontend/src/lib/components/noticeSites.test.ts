import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

/**
 * Which notices announce. A notice that reports something that has just happened (a load or a request
 * that failed, the connection going away) announces by its tone; one that states a standing fact of the
 * page it is rendered with does not (`announce={false}`): it is read in the flow of the page. A new
 * notice is added here with that decision. A source tripwire: Notice.dom.test.ts proves the option.
 */
const SITES: Record<string, [tone: string, announces: boolean][]> = {
  'lib/components/NetworkBanner.svelte': [['danger', true]],
  'lib/components/OfflineBanner.svelte': [['warning', true]],
  'lib/components/UpdateBanner.svelte': [['danger', true]],
  // Advice for the chosen platform, also inside InstallModal and AddDownloadModal.
  'lib/components/EmulatorHint.svelte': [['warning', false]],
  'lib/components/profile/GamePicker.svelte': [['danger', true]],
  'routes/activity/Activity.svelte': [['danger', true]],
  'routes/catalog/Catalog.svelte': [['danger', true]],
  'routes/diskusage/DiskUsage.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  // The stored error of the last download, shown on every opening of the game.
  'routes/game/GameDetails.svelte': [['danger', false]],
  'routes/game/GameReviews.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  'routes/game/GameScreenshots.svelte': [['danger', true]],
  'routes/game/ReviewComposer.svelte': [['danger', true]],
  // The history file could not be written: a state of the page, not an event of this visit.
  'routes/history/History.svelte': [['danger', false]],
  'routes/profile/Profile.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  'routes/stats/Stats.svelte': [['danger', true]],
  'routes/support/NewTicket.svelte': [
    ['danger', true],
    ['danger', true],
  ],
  'routes/support/TicketList.svelte': [['danger', true]],
  'routes/support/TicketView.svelte': [
    ['danger', true],
    ['danger', true],
  ],
};

function sites(): Record<string, [string, boolean][]> {
  const out: Record<string, [string, boolean][]> = {};
  for (const [path, source] of Object.entries(frontendSources)) {
    if (!path.endsWith('.svelte')) continue;
    for (const m of source.matchAll(/<Notice\b([^>]*)>/g)) {
      const tone = /\btone="(\w+)"/.exec(m[1])?.[1] ?? 'info';
      const announces = !/\bannounce=\{false\}/.test(m[1]);
      (out[path] ??= []).push([tone, announces]);
    }
  }
  return out;
}

describe('notices', () => {
  it('announce what has just happened and leave standing facts to the flow of the page', () => {
    expect(sites()).toEqual(SITES);
  });
});
