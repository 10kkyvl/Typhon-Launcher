// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { focusableIn } from '../../lib/layers/focus';
import { playGame } from '../../lib/services/library';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { libraryView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';
import { game, results } from './libraryTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
  playGame: vi.fn(async () => {}),
}));

// Never played: no carousel and no "Recent" row, so the toolbar leads straight into the list.
const titles = ['Alpha', 'Bravo', 'Charlie', 'Delta', 'Echo', 'Foxtrot'];
const COLUMNS = 3;

/** Lays the items out as the grid would on screen: three to a row. */
function layOut() {
  [...(results() as HTMLElement).children].forEach((item, index) => {
    const left = (index % COLUMNS) * 200;
    const top = Math.floor(index / COLUMNS) * 300;
    item.getBoundingClientRect = () => new DOMRect(left, top, 180, 280);
  });
}

/** The game whose item holds focus. */
function focusedTitle(): string | undefined {
  const list = results() as HTMLElement;
  const item = [...list.children].find((child) => child.contains(document.activeElement));
  return item?.querySelector('[id$="-title"]')?.textContent?.trim();
}

/** The control just before the list in the Tab order: the collections row's "+ Collection". */
const beforeList = () => screen.getByRole('button', { name: msg('games.collectionAdd') });

beforeEach(() => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  libraryLoaded.set(true);
  libraryGames.set(titles.map((title) => game(`g-${title.toLowerCase()}`, title)));
  vi.mocked(playGame).mockClear();
});

afterEach(() => {
  libraryGames.set([]);
  document.body.replaceChildren();
});

describe('Library: the grid by keyboard', () => {
  beforeEach(() => libraryView.set('grid'));

  it('is one Tab stop: Tab enters on the first game\'s title, goes through its own controls, then out', async () => {
    const { user } = renderDom(Library);
    const list = results() as HTMLElement;
    const stops = focusableIn(list);
    expect(stops.every((stop) => list.children[0].contains(stop))).toBe(true);
    beforeList().focus();

    await user.tab();
    expect(focusedTitle()).toBe('Alpha');
    expect(document.activeElement?.textContent).toContain('Alpha');

    await user.tab();
    expect(document.activeElement?.getAttribute('aria-label')).toBe(msg('ui.play'));
    expect(focusedTitle()).toBe('Alpha');

    await user.tab();
    expect(list.contains(document.activeElement)).toBe(false);
  });

  it('moves by the rows on screen with the arrows, to the ends with Home and End', async () => {
    const { user } = renderDom(Library);
    layOut();
    beforeList().focus();
    await user.tab();

    const steps: [string, string][] = [
      ['{ArrowRight}', 'Bravo'],
      ['{ArrowDown}', 'Echo'],
      ['{ArrowLeft}', 'Delta'],
      ['{ArrowUp}', 'Alpha'],
      ['{End}', 'Foxtrot'],
      ['{Home}', 'Alpha'],
      ['{ArrowLeft}', 'Alpha'],
    ];
    for (const [key, title] of steps) {
      await user.keyboard(key);
      expect(focusedTitle(), key).toBe(title);
      expect(document.activeElement?.textContent, key).toContain(title);
    }
  });

  it('enters again on the game the player was on last', async () => {
    const { user } = renderDom(Library);
    layOut();
    beforeList().focus();
    await user.tab();
    await user.keyboard('{ArrowDown}{ArrowRight}');
    expect(focusedTitle()).toBe('Echo');

    beforeList().focus();
    await user.tab();

    expect(focusedTitle()).toBe('Echo');
    expect(focusableIn(results() as HTMLElement).every((stop) => (results() as HTMLElement).children[4].contains(stop))).toBe(true);
  });

  it('plays the focused game with Ctrl+Enter', async () => {
    const { user } = renderDom(Library);
    layOut();
    beforeList().focus();
    await user.tab();
    await user.keyboard('{ArrowRight}');

    await user.keyboard('{Control>}{Enter}{/Control}');

    expect(playGame).toHaveBeenCalledWith('g-bravo');
  });
});

describe('Library: the list by keyboard', () => {
  beforeEach(() => libraryView.set('list'));

  it('is one Tab stop and moves up and down, to the ends with Home and End', async () => {
    const { user } = renderDom(Library);
    beforeList().focus();

    await user.tab();
    expect(focusedTitle()).toBe('Alpha');
    expect(document.activeElement?.textContent).toBe('Alpha');

    const steps: [string, string][] = [
      ['{ArrowDown}', 'Bravo'],
      ['{ArrowRight}', 'Bravo'],
      ['{ArrowDown}', 'Charlie'],
      ['{ArrowUp}', 'Bravo'],
      ['{End}', 'Foxtrot'],
      ['{Home}', 'Alpha'],
    ];
    for (const [key, title] of steps) {
      await user.keyboard(key);
      expect(focusedTitle(), key).toBe(title);
      expect(document.activeElement?.textContent, key).toBe(title);
    }

    await user.tab();
    expect(document.activeElement?.getAttribute('aria-label')).toBe(msg('ui.play'));
    await user.tab();
    expect(document.activeElement?.getAttribute('aria-label')).toBe(msg('library.row.more'));
    await user.tab();
    expect((results() as HTMLElement).contains(document.activeElement)).toBe(false);
  });

  it('keeps the remembered game when the view changes', async () => {
    const { user } = renderDom(Library);
    beforeList().focus();
    await user.tab();
    await user.keyboard('{ArrowDown}{ArrowDown}');

    libraryView.set('grid');
    await tick();
    beforeList().focus();
    await user.tab();

    expect(focusedTitle()).toBe('Charlie');
  });
});

describe('Library: the game focus moves to comes into view', () => {
  const into = vi.fn();
  beforeEach(() => {
    into.mockClear();
    HTMLElement.prototype.scrollIntoView = into;
  });
  afterEach(() => {
    delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
  });

  const revealed = () => into.mock.contexts.map((element) => (element as HTMLElement).querySelector('[id$="-title"]')?.textContent?.trim());

  it.each(['grid', 'list'] as const)('in the %s, by the least movement, on End and on an arrow', async (view) => {
    libraryView.set(view);
    const { user } = renderDom(Library);
    if (view === 'grid') layOut();
    beforeList().focus();
    await user.tab();

    await user.keyboard('{End}');
    expect(revealed().at(-1)).toBe('Foxtrot');
    await user.keyboard(view === 'grid' ? '{ArrowUp}' : '{ArrowUp}');
    expect(revealed().at(-1)).toBe(view === 'grid' ? 'Charlie' : 'Echo');
    for (const [options] of into.mock.calls) expect(options).toEqual({ block: 'nearest', inline: 'nearest' });
  });
});
