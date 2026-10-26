import { describe, expect, it } from 'vitest';
import {
  GAME_MAIN_SECTIONS,
  GAME_SIDE_SECTIONS,
  LAYOUT_PARTS,
  MAX_SEPARATORS,
  PARTS_WITH_ID,
  SEARCH_POSITIONS,
  SEPARATOR,
  SIDEBAR_DEFAULT_ORDER,
  SIDEBAR_HIDEABLE,
  SIDEBAR_ITEMS,
  SIDEBAR_SIDES,
  TOPBAR_ACTIONS,
  gameSections,
  searchPosition,
  sidebarEntries,
  sidebarSide,
  supportHidden,
  topbarActions,
} from './layout';

const raw = Object.values(
  import.meta.glob<string>('../../../../internal/theme/layout.json', { query: '?raw', import: 'default', eager: true }),
)[0];
const spec = JSON.parse(raw);

const ids = (entries: ReturnType<typeof sidebarEntries>) => entries.map((e) => (e.kind === 'item' ? e.id : SEPARATOR));

describe('layout ids match internal/theme/layout.json', () => {
  it.each([
    ['separator', SEPARATOR, spec.separator],
    ['maxSeparators', MAX_SEPARATORS, spec.maxSeparators],
    ['sidebar sides', SIDEBAR_SIDES, spec.sidebar.sides],
    ['sidebar items', SIDEBAR_ITEMS, spec.sidebar.items],
    ['sidebar default order', SIDEBAR_DEFAULT_ORDER, spec.sidebar.defaultOrder],
    ['sidebar hideable', SIDEBAR_HIDEABLE, spec.sidebar.hideable],
    ['search positions', SEARCH_POSITIONS, spec.topbar.searchPositions],
    ['topbar actions', TOPBAR_ACTIONS, spec.topbar.actions],
    ['game main', GAME_MAIN_SECTIONS, spec.gamePage.main],
    ['game side', GAME_SIDE_SECTIONS, spec.gamePage.side],
    ['parts', LAYOUT_PARTS, spec.parts],
    ['parts with id', PARTS_WITH_ID, spec.partsWithId],
  ])('%s', (_name, ours, theirs) => {
    expect(ours).toEqual(theirs);
  });

  it('never lets settings be hidden', () => {
    expect(SIDEBAR_HIDEABLE).not.toContain('settings');
  });
});

describe('sidebarEntries', () => {
  it.each([
    { name: 'no layout keeps the default', layout: undefined, want: SIDEBAR_DEFAULT_ORDER },
    {
      name: 'custom order appends the items it leaves out',
      layout: { sidebar: { order: ['downloads', '-', 'library'] } },
      want: ['downloads', '-', 'library', 'catalog', 'installed', 'diskusage', 'sources', 'friends', 'activity', 'stats', 'history', 'lan'],
    },
    {
      name: 'unknown ids and duplicates are dropped',
      layout: { sidebar: { order: ['library', 'nope', 'library', 'settings'] } },
      want: ['library', 'catalog', 'installed', 'diskusage', 'downloads', 'sources', 'friends', 'activity', 'stats', 'history', 'lan'],
    },
    {
      name: 'leading, trailing and doubled separators collapse',
      layout: { sidebar: { order: ['-', 'library', '-', '-', 'catalog', '-'], hidden: [...SIDEBAR_ITEMS].filter((id) => id !== 'library' && id !== 'catalog') } },
      want: ['library', '-', 'catalog'],
    },
    {
      name: 'a separator left next to a hidden item does not double',
      layout: { sidebar: { order: ['library', '-', 'catalog', '-', 'installed'], hidden: ['catalog', ...SIDEBAR_ITEMS.slice(3)] } },
      want: ['library', '-', 'installed'],
    },
  ])('$name', ({ layout, want }) => {
    expect(ids(sidebarEntries(layout))).toEqual(want);
  });

  it(`keeps at most ${MAX_SEPARATORS} separators`, () => {
    const order = SIDEBAR_ITEMS.flatMap((id) => [id, SEPARATOR]);
    expect(ids(sidebarEntries({ sidebar: { order } })).filter((id) => id === SEPARATOR)).toHaveLength(MAX_SEPARATORS);
  });

  it('reads side and support', () => {
    expect(sidebarSide(undefined)).toBe('left');
    expect(sidebarSide({ sidebar: { side: 'right' } })).toBe('right');
    expect(sidebarSide({ sidebar: { side: 'top' } })).toBe('left');
    expect(supportHidden({ sidebar: { hidden: ['support'] } })).toBe(true);
    expect(supportHidden(undefined)).toBe(false);
  });
});

describe('topbar', () => {
  it.each([
    { layout: undefined, want: ['bigpicture', 'notifications', 'friends'] },
    { layout: { topbar: { order: ['friends'] } }, want: ['friends', 'bigpicture', 'notifications'] },
    { layout: { topbar: { order: ['friends', 'x'], hidden: ['bigpicture'] } }, want: ['friends', 'notifications'] },
  ])('actions for %j', ({ layout, want }) => {
    expect(topbarActions(layout)).toEqual(want);
  });

  it('reads the search position', () => {
    expect(searchPosition({ topbar: { search: 'center' } })).toBe('center');
    expect(searchPosition({ topbar: { search: 'right' } })).toBe('left');
  });
});

describe('gameSections', () => {
  it('keeps the defaults', () => {
    expect(gameSections(undefined)).toEqual({ main: [...GAME_MAIN_SECTIONS], side: [...GAME_SIDE_SECTIONS] });
  });

  it('moves a section to the other column and hides one', () => {
    const got = gameSections({ gamePage: { main: ['notes', 'achievements'], side: ['summary'], hidden: ['reviews'] } });
    expect(got.main.slice(0, 2)).toEqual(['notes', 'achievements']);
    expect(got.main).not.toContain('reviews');
    expect(got.main).not.toContain('summary');
    expect(got.side[0]).toBe('summary');
    expect(got.side).not.toContain('achievements');
    expect([...got.main, ...got.side, 'reviews'].toSorted()).toEqual([...GAME_MAIN_SECTIONS, ...GAME_SIDE_SECTIONS].toSorted());
  });

  it('shows a section the main button needs even when the theme hides it', () => {
    const layout = { gamePage: { side: ['update'], hidden: ['update', 'reviews'] } };
    expect(gameSections(layout).side).not.toContain('update');
    const kept = gameSections(layout, ['update']);
    expect(kept.side[0]).toBe('update');
    expect(kept.main).not.toContain('reviews');
  });

  it('places a section listed in both columns once, where it comes first', () => {
    const got = gameSections({ gamePage: { main: ['about'], side: ['about'] } });
    expect(got.main).toContain('about');
    expect(got.side).not.toContain('about');
  });
});
