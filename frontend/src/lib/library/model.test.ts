import { describe, expect, it } from 'vitest';
import type { Download } from '../services/downloads';
import type { LibraryCollection, LibraryGame } from '../services/library';
import { emptyReason, type EmptyInput } from './empty';
import { createEntryCache, entryCover, entryHeroArt, gameEntry } from './entries';
import type { LibraryFilter } from './filters';
import { genreOptions, queryLibrary, sourceOptions, type LibraryQuery, type LibrarySort } from './query';
import { buildSlides, recentEntries, RECENT_LIMIT } from './spotlight';
import type { StatusFilter } from './statuses';

function game(id: string, overrides: Partial<LibraryGame> = {}): LibraryGame {
  return {
    id,
    title: id,
    executable: `${id}.exe`,
    installDir: `C:\\Games\\${id}`,
    cover: '',
    version: '',
    sizeBytes: 0,
    lastPlayed: null,
    playtimeSeconds: 0,
    installedAt: '',
    ...overrides,
  };
}

function download(gameId: string, overrides: Partial<Download> = {}): Download {
  return {
    id: `d-${gameId}`,
    name: `${gameId}.torrent`,
    status: 'downloading',
    total: 100,
    downloaded: 1,
    addedAt: '2026-11-01T00:00:00Z',
    origin: { gameId },
    ...overrides,
  } as Download;
}

const query = (patch: Partial<LibraryQuery & { sort: LibrarySort }> = {}) => ({
  filter: 'all' as LibraryFilter,
  source: 'all' as const,
  status: 'all' as StatusFilter,
  search: '',
  sort: 'title' as LibrarySort,
  ...patch,
});

const ids = (list: readonly { id: string }[]) => list.map((item) => item.id);

// One library for every table below: installed and removed games, hidden ones, two stores, statuses,
// tags and catalog genres, favourites, play dates, a download not installed yet and one watched game.
const fixture: LibraryGame[] = [
  game('hades', { title: 'Hades', lastPlayed: '2026-11-10T10:00:00Z', playtimeSeconds: 900, sizeBytes: 15, status: 'playing', tags: ['rogue'], canonicalGameId: 'c-hades', favorite: true, installedAt: '2026-01-01T00:00:00Z' }),
  game('celeste', { title: 'Celeste', lastPlayed: '2026-11-12T10:00:00Z', playtimeSeconds: 300, sizeBytes: 5, status: 'completed', completedAt: '2026-11-12T11:00:00Z', canonicalGameId: 'c-celeste', installedAt: '2026-03-01T00:00:00Z' }),
  game('witcher', { title: 'Ведьмак 3', lastPlayed: '2026-10-01T10:00:00Z', playtimeSeconds: 5000, sizeBytes: 50, status: 'paused', external: { provider: 'steam', appId: '292030' }, canonicalGameId: 'c-witcher', installedAt: '2026-02-01T00:00:00Z' }),
  game('arma', { title: 'Arma', uninstalled: true, lastPlayed: '2026-11-13T10:00:00Z', playtimeSeconds: 10, tags: ['rogue'], favorite: true }),
  game('secret', { title: 'Secret Hades', hidden: true, lastPlayed: '2026-11-14T10:00:00Z', playtimeSeconds: 99999, status: 'playing', source: 'discovered', canonicalGameId: 'c-secret', tags: ['rogue'] }),
  game('zeta', { title: 'Zeta', source: 'discovered', status: 'backlog', canonicalGameId: 'c-zeta' }),
];
const genres = { 'c-hades': ['Action', 'Roguelike'], 'c-celeste': ['Platformer'], 'c-witcher': ['RPG'], 'c-secret': ['Horror'], 'c-zeta': ['Puzzle'] };

function library(games = fixture, downloads: Download[] = [download('c-new')], titles = {}) {
  return createEntryCache().library(games, downloads, titles);
}
const watched = createEntryCache().watched([{ gameId: 'c-watch', title: 'Watched Game' }]);

describe('library entries', () => {
  it('lists library games, then a download per catalog game the library does not hold, newest first', () => {
    const list = library(fixture, [
      download('c-new', { id: 'old', addedAt: '2026-10-01T00:00:00Z', name: 'old.torrent' }),
      download('c-new', { id: 'new', addedAt: '2026-11-01T00:00:00Z', name: 'new.torrent' }),
      download('c-hades'),
      download('', { origin: {} as Download['origin'] }),
    ]);
    expect(ids(list)).toEqual([...ids(fixture), 'c-new']);
    expect(list.at(-1)).toMatchObject({ kind: 'download', title: 'new.torrent', installed: false, source: 'typhon' });
  });

  it('names a download by its catalog title once it is known', () => {
    expect(library(fixture, [download('c-new')], { 'c-new': { title: 'New Game' } }).at(-1)?.title).toBe('New Game');
    expect(library(fixture, [download('c-new')], { 'c-new': null }).at(-1)?.title).toBe('c-new.torrent');
  });

  it('looks art up by catalog id and falls back to the game cover', () => {
    const entry = gameEntry(game('g', { canonicalGameId: 'c-g', cover: 'own.png' }));
    expect(entryCover(entry, {})).toBe('own.png');
    expect(entryCover(entry, { 'c-g': { cover: 'meta.png', hero: 'hero.png' } })).toBe('meta.png');
    expect(entryHeroArt(entry, { 'c-g': { cover: '', hero: 'hero.png' } })).toBe('hero.png');
    expect(entryHeroArt(entry, {})).toBe('');
  });
});

describe('cheap updates', () => {
  it('a download progress tick keeps every entry and the list itself', () => {
    const cache = createEntryCache();
    const first = cache.library(fixture, [download('c-new', { downloaded: 1 })], {});
    const second = cache.library(fixture, [download('c-new', { downloaded: 50, downloadSpeed: 9 })], {});
    expect(second).toBe(first);
  });

  it('a changed download status or one changed game rebuilds only that entry', () => {
    const cache = createEntryCache();
    const first = cache.library(fixture, [download('c-new')], {});
    const second = cache.library(fixture, [download('c-new', { status: 'paused' })], {});
    expect(second).not.toBe(first);
    expect(second.slice(0, -1).every((entry, i) => entry === first[i])).toBe(true);
    expect(second.at(-1)).not.toBe(first.at(-1));

    const changed = fixture.map((item) => (item.id === 'zeta' ? { ...item, title: 'Zeta 2' } : item));
    const third = cache.library(changed, [download('c-new', { status: 'paused' })], {});
    expect(third.filter((entry, i) => entry !== second[i]).map((entry) => entry.id)).toEqual(['zeta']);
  });
});

describe('filters, source, status, collection, search', () => {
  const entries = library();
  const cozy: Pick<LibraryCollection, 'tags' | 'genres' | 'status'> = { tags: ['Rogue'], genres: [] };
  const action: Pick<LibraryCollection, 'tags' | 'genres' | 'status'> = { genres: ['roguelike', 'rpg'] };

  it.each<[string, Partial<LibraryQuery & { sort: LibrarySort }>, string[]]>([
    ['all leaves hidden games out', {}, ['arma', 'c-new', 'celeste', 'hades', 'zeta', 'witcher']],
    ['installed', { filter: 'installed' }, ['celeste', 'hades', 'zeta', 'witcher']],
    ['uninstalled', { filter: 'uninstalled' }, ['arma']],
    ['hidden holds only hidden games', { filter: 'hidden' }, ['secret']],
    ['favourites', { filter: 'favorites' }, ['arma', 'hades']],
    ['recent: played installed games in play order whatever the sort', { filter: 'recent', sort: 'size' }, ['celeste', 'hades', 'witcher']],
    ['watched holds the watched games only', { filter: 'watched', source: 'steam', status: 'playing' }, ['c-watch']],
    ['source steam', { source: 'steam' }, ['witcher']],
    ['source disk without the hidden disk game', { source: 'disk' }, ['zeta']],
    ['source typhon takes the download too', { source: 'typhon' }, ['arma', 'c-new', 'celeste', 'hades']],
    ['status playing without the hidden one', { status: 'playing' }, ['hades']],
    ['status none: library games only', { status: 'none' }, ['arma']],
    ['collection by tag', { collection: cozy }, ['arma', 'hades']],
    ['collection by genre', { collection: action }, ['hades', 'witcher']],
    ['collection inside hidden', { filter: 'hidden', collection: cozy }, ['secret']],
    ['search, case-insensitive', { search: '  HADES ' }, ['hades']],
    ['search, form-insensitive (full-width letters)', { search: 'ＨＡＤＥＳ' }, ['hades']],
    ['search inside hidden', { filter: 'hidden', search: 'hades' }, ['secret']],
    ['search in Cyrillic', { search: 'ВЕДЬ' }, ['witcher']],
    ['installed × source × status × collection × search', { filter: 'installed', source: 'typhon', status: 'playing', collection: cozy, search: 'ha' }, ['hades']],
    ['installed × status × sort by playtime', { filter: 'installed', status: 'none', sort: 'playtime' }, []],
    ['all × sort by recent', { sort: 'recent' }, ['arma', 'celeste', 'hades', 'witcher', 'c-new', 'zeta']],
    ['all × sort by playtime', { sort: 'playtime' }, ['witcher', 'hades', 'celeste', 'arma', 'c-new', 'zeta']],
    ['all × sort by size', { sort: 'size' }, ['c-new', 'witcher', 'hades', 'celeste', 'arma', 'zeta']],
    ['all × sort by status', { sort: 'status' }, ['hades', 'zeta', 'witcher', 'celeste', 'arma', 'c-new']],
    ['all × sort by completion', { sort: 'completed' }, ['celeste', 'arma', 'c-new', 'hades', 'zeta', 'witcher']],
    ['all × sort by date added', { sort: 'added' }, ['celeste', 'witcher', 'hades', 'arma', 'c-new', 'zeta']],
  ])('%s', (_name, patch, expected) => {
    expect(ids(queryLibrary(entries, watched, query(patch), { genres, locale: 'en' }))).toEqual(expected);
  });

  it('leaves the input lists as they were', () => {
    const before = [...entries];
    queryLibrary(entries, watched, query({ sort: 'playtime' }), { genres, locale: 'en' });
    expect(entries).toEqual(before);
  });
});

describe('text order follows the interface language', () => {
  const titled = ['Witcher', 'Ведьмак', 'arma', 'Яблоко', 'Éclair', 'eclair'].map((title, i) => gameEntry(game(`g${i}`, { title })));

  it.each([
    ['en', ['arma', 'eclair', 'Éclair', 'Witcher', 'Ведьмак', 'Яблоко']],
    ['ru', ['Ведьмак', 'Яблоко', 'arma', 'eclair', 'Éclair', 'Witcher']],
  ])('%s', (locale, expected) => {
    expect(queryLibrary(titled, [], query(), { genres: {}, locale }).map((entry) => entry.title)).toEqual(expected);
  });

  it('breaks a tie between equal titles by id, whatever the input order', () => {
    const twins = [gameEntry(game('b', { title: 'Same' })), gameEntry(game('a', { title: 'Same' }))];
    expect(ids(queryLibrary(twins, [], query(), { genres: {}, locale: 'en' }))).toEqual(['a', 'b']);
    expect(ids(queryLibrary(twins.toReversed(), [], query(), { genres: {}, locale: 'en' }))).toEqual(['a', 'b']);
  });
});

describe('options leave hidden games out', () => {
  const entries = library();

  it('offers the sources of games listed outside "Hidden"', () => {
    expect(sourceOptions(entries)).toEqual(['typhon', 'steam', 'disk']);
    const onlyHiddenDisk = library(fixture.filter((item) => item.id !== 'zeta'));
    expect(sourceOptions(onlyHiddenDisk)).toEqual(['typhon', 'steam']);
  });

  it('offers inside "Hidden" the sources of the hidden games only', () => {
    expect(sourceOptions(entries, { hidden: true })).toEqual(['disk']);
  });

  it('keeps the selected source among the options when no listed game has it', () => {
    expect(sourceOptions(entries, { selected: 'gog' })).toEqual(['typhon', 'steam', 'gog', 'disk']);
    expect(sourceOptions(entries, { hidden: true, selected: 'steam' })).toEqual(['steam', 'disk']);
    expect(sourceOptions(entries, { selected: 'all' })).toEqual(['typhon', 'steam', 'disk']);
  });

  it('offers the genres of games that are not hidden', () => {
    expect(genreOptions(entries, genres)).toEqual(['Action', 'Roguelike', 'Platformer', 'RPG', 'Puzzle']);
  });
});

describe('hero slides and the recent row', () => {
  const entries = library();
  const none = new Set<string>();

  it('takes up to three last played, installed, launchable, not hidden games, newest first', () => {
    const extra = library([...fixture, game('old', { lastPlayed: '2025-01-01T00:00:00Z' }), game('noexe', { executable: '', lastPlayed: '2026-11-13T00:00:00Z' })]);
    expect(ids(buildSlides(extra, { running: none, locale: 'en' }))).toEqual(['celeste', 'hades', 'witcher']);
  });

  it('puts a running game first, played or not', () => {
    expect(ids(buildSlides(entries, { running: new Set(['zeta']), locale: 'en' }))).toEqual(['zeta', 'celeste', 'hades']);
    expect(ids(buildSlides(entries, { running: new Set(['witcher']), locale: 'en' }))).toEqual(['witcher', 'celeste', 'hades']);
    expect(ids(buildSlides(entries, { running: new Set(['secret']), locale: 'en' }))).toEqual(['celeste', 'hades', 'witcher']);
  });

  it('does not depend on the sort or the filters of the list', () => {
    expect(buildSlides(entries, { running: none, locale: 'en' })).toEqual(buildSlides(entries.toReversed(), { running: none, locale: 'en' }));
  });

  it('the row leaves out only the game the hero shows, and keeps it when the hero is hidden', () => {
    const [hero] = buildSlides(entries, { running: none, locale: 'en' });
    expect(ids(recentEntries(entries, { shown: hero.id, locale: 'en' }))).toEqual(['hades', 'witcher']);
    expect(ids(recentEntries(entries, { locale: 'en' }))).toEqual(['celeste', 'hades', 'witcher']);
  });

  it('the row holds the games of the "Recent" filter in the same order', () => {
    expect(ids(recentEntries(entries, { locale: 'en' }))).toEqual(
      ids(queryLibrary(entries, [], query({ filter: 'recent', sort: 'title' }), { genres, locale: 'en' })),
    );
  });

  it('keeps a game that just stopped first, in the hero and the row, until the list records its session', () => {
    const stopped = new Map([['witcher', 1]]);
    expect(ids(buildSlides(entries, { running: none, stopped, locale: 'en' }))).toEqual(['witcher', 'celeste', 'hades']);
    expect(ids(recentEntries(entries, { shown: 'witcher', stopped, locale: 'en' }))).toEqual(['celeste', 'hades']);
    const twice = new Map([['witcher', 1], ['hades', 2]]);
    expect(ids(buildSlides(entries, { running: none, stopped: twice, locale: 'en' }))).toEqual(['hades', 'witcher', 'celeste']);
    expect(ids(queryLibrary(entries, [], query({ filter: 'recent' }), { genres, locale: 'en', stopped: twice }))).toEqual(['hades', 'witcher', 'celeste']);
  });

  it('counts a game that stopped after its first session as recent before the list records it', () => {
    const stopped = new Map([['zeta', 1]]);
    expect(ids(buildSlides(entries, { running: none, stopped, locale: 'en' }))).toEqual(['zeta', 'celeste', 'hades']);
    expect(ids(recentEntries(entries, { stopped, locale: 'en' }))[0]).toBe('zeta');
  });

  it(`caps the row at ${RECENT_LIMIT}`, () => {
    const many = library(Array.from({ length: 20 }, (_, i) => game(`g${String(i).padStart(2, '0')}`, { lastPlayed: `2026-10-${String(i + 1).padStart(2, '0')}T00:00:00Z` })));
    const row = recentEntries(many, { locale: 'en' });
    expect(row).toHaveLength(12);
    expect(row[0].id).toBe('g19');
  });
});

describe('why the list is empty', () => {
  const base: EmptyInput = { loaded: true, failed: false, entries: 3, watched: 0, shown: 0, query: query() };

  it.each<[string, Partial<EmptyInput>, ReturnType<typeof emptyReason>]>([
    ['not empty', { shown: 2 }, null],
    ['still loading', { loaded: false, entries: 0 }, { kind: 'loading' }],
    ['the load failed', { loaded: false, failed: true, entries: 0 }, { kind: 'failed' }],
    ['the library is empty', { entries: 0 }, { kind: 'empty' }],
    ['the library is empty, a search typed', { entries: 0, query: query({ search: 'x' }) }, { kind: 'empty' }],
    ['every game is hidden', {}, { kind: 'all-hidden' }],
    ['nothing matches the search', { query: query({ search: 'zz' }) }, { kind: 'no-match', narrowing: ['search'] }],
    ['nothing hidden', { query: query({ filter: 'hidden' }) }, { kind: 'no-match', narrowing: ['filter'] }],
    [
      'search, filter, source, status and collection together',
      { query: query({ search: 'a', filter: 'installed', source: 'steam', status: 'playing', collection: { tags: ['x'] } }) },
      { kind: 'no-match', narrowing: ['search', 'filter', 'source', 'status', 'collection'] },
    ],
    [
      'watched with a collection: source and status do not narrow it',
      { watched: 1, query: query({ filter: 'watched', source: 'steam', status: 'playing', collection: { tags: ['x'] } }) },
      { kind: 'no-match', narrowing: ['filter', 'collection'] },
    ],
  ])('%s', (_name, patch, expected) => {
    expect(emptyReason({ ...base, ...patch })).toEqual(expected);
  });
});
