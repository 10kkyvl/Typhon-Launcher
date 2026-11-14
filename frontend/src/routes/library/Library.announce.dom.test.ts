// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { DEFAULT_CHOICES, libraryChoices, librarySort } from '../../lib/stores/libraryPrefs';
import { libraryView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Library, { ANNOUNCE_MS } from './Library.svelte';
import { game, shownTitles } from './libraryTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
}));

const hades = game('g-hades', 'Hades', { sizeBytes: 100 });
const celeste = game('g-celeste', 'Celeste', { uninstalled: true });
const inside = game('g-inside', 'Inside', { hidden: true });

const filterGroup = () => screen.getByRole('radiogroup', { name: msg('library.filterLabel') });
const option = (name: string) => within(filterGroup()).getByRole('radio', { name });

/** Everything the page's polite status region says, in order, from the moment this is called. */
function listen(): string[] {
  const region = [...document.querySelectorAll('[role="status"]')].find((el) => el.closest('[data-library-status]'));
  expect(region).toBeTruthy();
  const said: string[] = [];
  // Each announcement comes in as new nodes, so the same words said twice count twice.
  new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        const text = node.textContent?.trim() ?? '';
        if (text) said.push(text);
      }
    }
  }).observe(region as Element, { childList: true, subtree: true });
  return said;
}

async function settle() {
  vi.advanceTimersByTime(ANNOUNCE_MS);
  await tick();
  await Promise.resolve();
}

beforeEach(() => {
  vi.useFakeTimers();
  libraryChoices.set({ ...DEFAULT_CHOICES });
  librarySort.set('title');
  libraryView.set('list');
  libraryLoaded.set(true);
  libraryGames.set([hades, celeste, inside]);
});

afterEach(() => {
  vi.useRealTimers();
  libraryGames.set([]);
  document.body.replaceChildren();
});

describe('Library: the filter choice', () => {
  it('is one choice among several: a named radio group, the chosen one checked', () => {
    renderDom(Library);

    const radios = within(filterGroup()).getAllByRole('radio');
    expect(radios.map((radio) => radio.textContent?.trim())).toEqual([
      msg('games.filterAll'),
      msg('games.filterInstalled'),
      msg('games.recentLabel'),
      msg('library.filterFavorites'),
      msg('games.filterHidden'),
    ]);
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['true', 'false', 'false', 'false', 'false']);
    expect(screen.queryByRole('button', { name: msg('games.filterInstalled') })).toBeNull();
  });

  it('moves and chooses with the arrow keys, and pressing the chosen one keeps it', async () => {
    const { user } = renderDom(Library);
    option(msg('games.filterAll')).focus();

    await user.keyboard('{ArrowRight}');

    expect(document.activeElement).toBe(option(msg('games.filterInstalled')));
    expect(option(msg('games.filterInstalled')).getAttribute('aria-checked')).toBe('true');
    expect(shownTitles()).toEqual(['Hades']);

    await user.click(option(msg('games.filterInstalled')));
    expect(option(msg('games.filterInstalled')).getAttribute('aria-checked')).toBe('true');
  });
});

describe('Library: what the list shows is said', () => {
  it('once the filter settles, with the count', async () => {
    const { user } = renderDom(Library);
    const said = listen();

    await user.click(option(msg('games.filterInstalled')));
    expect(said).toEqual([]);
    await settle();

    expect(said).toEqual([msg('library.shownCount', { count: 1 })]);
  });

  it('once after typing, not on every key', async () => {
    const { user } = renderDom(Library);
    const said = listen();

    await user.type(screen.getByLabelText(msg('games.librarySearchPlaceholder')), 'cel');
    await settle();

    expect(said).toEqual([msg('library.shownCount', { count: 1 })]);
  });

  it('says why the list is empty', async () => {
    const { user } = renderDom(Library);
    const said = listen();

    await user.type(screen.getByLabelText(msg('games.librarySearchPlaceholder')), 'zzz');
    await settle();

    expect(said).toHaveLength(1);
    expect(said[0]).toContain(msg('games.nothingFoundTitle'));
  });

  it('when the sort changes, though the count stays', async () => {
    const { user } = renderDom(Library);
    const said = listen();

    await user.click(option(msg('games.filterInstalled')));
    await settle();
    await user.click(screen.getByRole('button', { name: msg('games.sortAlpha') }));
    await user.click(screen.getByRole('menuitemradio', { name: msg('games.sortSize') }));
    await settle();

    expect(said).toEqual([msg('library.shownCount', { count: 1 }), msg('library.shownCount', { count: 1 })]);
  });

  it('says nothing when the page opens, or when a game merely updates', async () => {
    renderDom(Library);
    const said = listen();
    await settle();

    libraryGames.set([{ ...hades, playtimeSeconds: 600 }, celeste, inside]);
    await tick();
    await settle();

    expect(said).toEqual([]);
  });
});
