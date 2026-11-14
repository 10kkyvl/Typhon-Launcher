// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { fireEvent, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import GameCard from '../../lib/components/GameCard.svelte';
import { msg } from '../../lib/i18n';
import { getCatalogGames } from '../../lib/services/sources';
import type { Download } from '../../lib/services/downloads';
import { downloads } from '../../lib/stores/downloads';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { gameArt, requestArt } from '../../lib/stores/metadata';
import { libraryView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';
import { carousel, game, played, recentSection, results, shownTitles } from './libraryTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
}));
vi.mock('../../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/sources')>()),
  getCatalogGames: vi.fn(async () => []),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  requestArt: vi.fn(),
}));

/** What a generated picture shows of its game: the sky's colours and the lands' shades. */
function scene(root: Element | null | undefined): string[] {
  const svg = root?.querySelector('svg.generated-art');
  expect(svg).toBeTruthy();
  const stops = [...(svg as Element).querySelectorAll('linearGradient stop')].map((stop) => stop.getAttribute('stop-color') ?? '');
  const lands = [...(svg as Element).querySelectorAll(':scope > path[fill]:not([fill="currentColor"])')].map((path) => path.getAttribute('fill') ?? '');
  return [...stops, ...lands];
}

const itemOf = (title: string) => (results() as HTMLElement).children[shownTitles().indexOf(title)];

function download(gameId: string, downloaded: number): Download {
  return { id: `d-${gameId}`, name: `${gameId}.torrent`, status: 'downloading', total: 100, downloaded, addedAt: '2026-11-01T00:00:00Z', origin: { gameId } } as Download;
}

beforeEach(() => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  libraryView.set('grid');
  libraryLoaded.set(true);
  downloads.set([]);
  gameArt.set({});
  vi.mocked(requestArt).mockClear();
  vi.mocked(getCatalogGames).mockClear();
});

afterEach(() => {
  libraryGames.set([]);
  downloads.set([]);
  gameArt.set({});
  document.body.replaceChildren();
});

describe('Library: one generated picture per game', () => {
  it('the card, the row and the slide of a game without art draw the picture of its catalog id', async () => {
    libraryGames.set([played('g-hades', 'Hades', 9, { canonicalGameId: 'c-hades' })]);
    const seeded = renderDom(GameCard, { id: 'another-id', title: 'Hades', artSeed: 'c-hades' }).container;
    renderDom(Library);
    const card = scene(itemOf('Hades'));
    const slide = scene(carousel());

    libraryView.set('list');
    await tick();
    const row = scene(itemOf('Hades'));

    expect(card).toEqual(scene(seeded));
    expect(row).toEqual(card);
    expect(slide).toEqual(card);
  });
});

describe('Library: a cover that fails to load', () => {
  const own = played('g-hades', 'Hades', 9, { canonicalGameId: 'c-hades', cover: 'own.png' });
  // Played before the carousel's three: Hades' capsule is in the "Recent" row once the carousel is away.
  beforeEach(() => {
    libraryGames.set([own]);
    gameArt.set({ 'c-hades': { cover: 'meta.png', hero: '', heroIsPoster: false } });
  });

  it.each(['grid', 'list'] as const)('falls back to the game\'s own cover in the %s', async (view) => {
    libraryView.set(view);
    renderDom(Library);
    const img = () => itemOf('Hades').querySelector('img') as HTMLImageElement;
    expect(img().getAttribute('src')).toBe('meta.png');

    await fireEvent.error(img());

    expect(img().getAttribute('src')).toBe('own.png');
  });

  it('falls back to the game\'s own cover on its "Recent" capsule', async () => {
    libraryChoices.set({ ...DEFAULT_CHOICES, heroHidden: true });
    renderDom(Library);
    const img = () => (recentSection() as HTMLElement).querySelector('.recent-item img') as HTMLImageElement;
    expect(img().getAttribute('src')).toBe('meta.png');

    await fireEvent.error(img());

    expect(img().getAttribute('src')).toBe('own.png');
  });
});

describe('Library: a big library stays cheap', () => {
  const titles = ['Alpha', 'Bravo', 'Charlie', 'Delta', 'Echo'];
  beforeEach(() => libraryGames.set(titles.map((title) => game(`g-${title.toLowerCase()}`, title, { canonicalGameId: `c-${title.toLowerCase()}` }))));

  const nodes = () => new Map([...(results() as HTMLElement).children].map((item, index) => [shownTitles()[index], item.firstElementChild]));

  it('typing in the search keeps the cards that stay on screen', async () => {
    const { user } = renderDom(Library);
    const before = nodes();

    await user.type(screen.getByLabelText(msg('games.librarySearchPlaceholder')), 'ha');

    expect(shownTitles()).toEqual(['Alpha', 'Charlie']);
    for (const [title, node] of nodes()) expect(node, title).toBe(before.get(title));
  });

  it('download ticks and an art update re-create no card and request nothing again', async () => {
    downloads.set([download('c-ticking', 1)]);
    renderDom(Library);
    await tick();
    const before = nodes();
    const artCalls = vi.mocked(requestArt).mock.calls.length;
    const catalogCalls = vi.mocked(getCatalogGames).mock.calls.length;

    for (let i = 2; i < 12; i += 1) {
      downloads.set([download('c-ticking', i)]);
      await tick();
    }
    gameArt.set({ 'c-bravo': { cover: 'bravo.png', hero: '', heroIsPoster: false } });
    await tick();

    expect(nodes().size).toBe(before.size);
    for (const [title, node] of nodes()) expect(node, title).toBe(before.get(title));
    expect(itemOf('Bravo').querySelector('img')?.getAttribute('src')).toBe('bravo.png');
    expect(vi.mocked(requestArt).mock.calls.length).toBe(artCalls);
    expect(vi.mocked(getCatalogGames).mock.calls.length).toBe(catalogCalls);
  });
});
