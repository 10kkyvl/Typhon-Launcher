import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import { locale } from '../i18n';
import type { ProfileBlock, ProfileLayout, ProfileSettings } from '../services/account';
import { EMPTY_SNAPSHOT, type GameRef, type ProfileSnapshot } from '../services/profile';
import type { GameCard, PublicBlock, PublicProfile } from '../services/social';
import { defaultLayout } from './layout';
import {
  ownAutoArt,
  publicLayout,
  resolveOwn,
  resolvePublic,
  type GridBlock,
  type OwnContext,
  type PublicContext,
} from './layoutView';

beforeEach(() => {
  locale.set('en');
});

function game(id: string, patch: Partial<GameRef> = {}): GameRef {
  return { id, title: `Game ${id}`, cover: `cover-${id}`, canonicalGameId: `canon-${id}`, playtimeSeconds: 3600, status: '', ...patch };
}

function card(igdbId: number, patch: Partial<GameCard> = {}): GameCard {
  return { igdbId, title: `Card ${igdbId}`, coverUrl: `c${igdbId}`, heroUrl: `h${igdbId}`, ...patch };
}

function block(id: string, type: string, config: Record<string, unknown> = {}): ProfileBlock {
  return { id, type, width: 'full', config };
}

function layoutOf(...blocks: ProfileBlock[]): ProfileLayout {
  return { version: 1, blocks };
}

const flags = { showPlaying: true, showLibrary: true, showActivity: true, showStats: true };

function ownContext(patch: Partial<OwnContext> = {}, snapshot: Partial<ProfileSnapshot> = {}): OwnContext {
  return {
    flags,
    snapshot: { ...EMPTY_SNAPSHOT, ...snapshot },
    preview: null,
    art: {},
    picked: {},
    open: vi.fn(),
    empty: () => false,
    ...patch,
  };
}

function bodyOf(blocks: GridBlock[], id: string) {
  return blocks.find((item) => item.id === id)!.body;
}

describe('resolveOwn', () => {
  it('keeps the layout order and the array index of every block', () => {
    const blocks = resolveOwn(layoutOf(block('a', 'about'), block('g', 'genres'), block('x', 'future')), ownContext());

    expect(blocks.map((item) => [item.id, item.index, item.body.kind])).toEqual([
      ['a', 0, 'external'],
      ['g', 1, 'genres'],
      ['x', 2, 'unknown'],
    ]);
  });

  it('shows the saved game of a pinned block with its hours, status and caption', () => {
    const open = vi.fn();
    const saved = game('1', { playtimeSeconds: 7200, status: 'completed', statusAt: '2026-09-01T10:00:00Z', canonicalGameId: 'canon-1' });
    const ctx = ownContext(
      { open, art: { 'canon-1': { cover: 'art-cover', hero: 'art-hero' } } },
      { layoutGames: { '42': { game: saved } } },
    );

    const body = bodyOf(resolveOwn(layoutOf(block('p', 'pinned', { igdbId: 42, caption: 'best' })), ctx), 'p');

    expect(body).toMatchObject({ kind: 'pinned', caption: 'best' });
    if (body.kind !== 'pinned' || !body.game) throw new Error('no game');
    expect(body.game).toMatchObject({ title: 'Game 1', cover: 'art-cover', hero: 'art-hero', seconds: 7200, completedAt: '2026-09-01T10:00:00Z' });
    body.game.open();
    expect(open).toHaveBeenCalledWith(saved);
  });

  it('prefers a game just picked in the draft over the saved snapshot', () => {
    const ctx = ownContext({ picked: { 42: game('new') } }, { layoutGames: { '42': { game: game('old') } } });

    const body = bodyOf(resolveOwn(layoutOf(block('p', 'pinned', { igdbId: 42 })), ctx), 'p');

    expect(body).toMatchObject({ kind: 'pinned', game: { title: 'Game new' } });
  });

  it('shows an error state when the backend could not resolve the pinned game', () => {
    const ctx = ownContext({}, { layoutGames: { '42': { error: 'unresolved' } } });

    expect(bodyOf(resolveOwn(layoutOf(block('p', 'pinned', { igdbId: 42 })), ctx), 'p')).toEqual({ kind: 'error' });
  });

  it('leaves a pinned block without a game empty instead of failing', () => {
    expect(bodyOf(resolveOwn(layoutOf(block('p', 'pinned', { igdbId: 0 })), ownContext()), 'p')).toMatchObject({ kind: 'pinned', game: null });
    expect(bodyOf(resolveOwn(layoutOf(block('p', 'pinned', { igdbId: 9 })), ownContext()), 'p')).toMatchObject({ kind: 'pinned', game: null });
  });

  it('takes an auto collection from the snapshot, or from the full preview while editing', () => {
    const saved = { kind: 'favorites', games: [game('a')] };
    const wide = { kind: 'favorites', games: [game('a'), game('b')] };
    const snapshot = { showcase: [saved] };
    const layout = layoutOf(block('c', 'collection', { source: 'favorites' }));

    const plain = bodyOf(resolveOwn(layout, ownContext({}, snapshot)), 'c');
    const editing = bodyOf(resolveOwn(layout, ownContext({ preview: { ...EMPTY_SNAPSHOT, showcase: [wide] } }, snapshot)), 'c');

    expect(plain).toMatchObject({ kind: 'collection', title: 'Favorite games', hearts: true, dated: false });
    expect(plain.kind === 'collection' && plain.games).toHaveLength(1);
    expect(editing.kind === 'collection' && editing.games).toHaveLength(2);
  });

  it('dates the recently completed shelf', () => {
    const shelf = { kind: 'recently_completed', games: [game('a', { status: 'completed', statusAt: '2026-09-01T10:00:00Z' })] };

    const body = bodyOf(resolveOwn(layoutOf(block('c', 'collection', { source: 'recently_completed' })), ownContext({}, { showcase: [shelf] })), 'c');

    expect(body).toMatchObject({ kind: 'collection', dated: true });
    expect(body.kind === 'collection' && body.games[0].completedAt).toBe('2026-09-01T10:00:00Z');
  });

  it('builds a manual collection in the order of its ids and skips games it cannot find', () => {
    const ctx = ownContext({ picked: { 3: game('three') } }, { layoutGames: { '1': { game: game('one') }, '2': { error: 'unresolved' } } });

    const body = bodyOf(resolveOwn(layoutOf(block('c', 'collection', { source: 'manual', title: 'Mine', igdbIds: [3, 2, 1] })), ctx), 'c');

    expect(body.kind === 'collection' && body.games.map((item) => item.title)).toEqual(['Game three', 'Game one']);
    expect(body).toMatchObject({ title: 'Mine' });
  });

  it('turns a manual collection into an error when none of its games resolved', () => {
    const ctx = ownContext({}, { layoutGames: { '1': { error: 'unresolved' } } });

    expect(bodyOf(resolveOwn(layoutOf(block('c', 'collection', { source: 'manual', title: 'Mine', igdbIds: [1] })), ctx), 'c')).toEqual({ kind: 'error' });
  });

  it('reads genres and fingerprint from the snapshot', () => {
    const genres = { genres: [{ name: 'RPG', share: 0.5 }], other: 0.1, unknown: 0.4 };
    const ctx = ownContext({}, { genres, fingerprint: { hours: 12, games: 3, completed: 1 } });

    const blocks = resolveOwn(layoutOf(block('g', 'genres'), block('f', 'fingerprint')), ctx);

    expect(bodyOf(blocks, 'g')).toEqual({ kind: 'genres', breakdown: genres });
    expect(bodyOf(blocks, 'f')).toEqual({ kind: 'fingerprint', breakdown: genres, hours: 12, games: 3, completed: 1 });
  });

  it('survives a snapshot whose genre list came back null', () => {
    const ctx = ownContext({}, { genres: { genres: null, other: 0, unknown: 0 } as unknown as ProfileSnapshot['genres'] });

    expect(bodyOf(resolveOwn(layoutOf(block('g', 'genres')), ctx), 'g')).toEqual({
      kind: 'genres',
      breakdown: { genres: [], other: 0, unknown: 0 },
    });
  });

  it('passes text through untouched', () => {
    const body = bodyOf(resolveOwn(layoutOf(block('t', 'text', { title: 'Hi', body: 'a\nb <b>x</b>' })), ownContext()), 't');

    expect(body).toEqual({ kind: 'text', title: 'Hi', body: 'a\nb <b>x</b>' });
  });

  it('asks the page whether each external block has anything to show', () => {
    const empty = vi.fn((type: string) => type === 'recent');

    const blocks = resolveOwn(layoutOf(block('r', 'recent'), block('s', 'stats')), ownContext({ empty }));

    expect(bodyOf(blocks, 'r')).toEqual({ kind: 'external', empty: true });
    expect(bodyOf(blocks, 's')).toEqual({ kind: 'external', empty: false });
  });

  it('marks blocks the visibility flags hide from others and blocks that are not finished', () => {
    const ctx = ownContext({ flags: { ...flags, showStats: false } });

    const blocks = resolveOwn(layoutOf(block('g', 'genres'), block('p', 'pinned', { igdbId: 0 }), block('a', 'about')), ctx);

    expect(blocks.map((item) => [item.id, item.hidden, item.issue])).toEqual([
      ['g', true, false],
      ['p', false, true],
      ['a', false, false],
    ]);
  });
});

describe('ownAutoArt', () => {
  const running = [game('run', { canonicalGameId: 'run' })];
  const art = { run: { cover: 'run-cover', hero: 'run-hero' }, top: { cover: 'top-cover', hero: '' } };

  it('uses the running game for playing, preferring its wide art', () => {
    expect(ownAutoArt('playing', { running, mostPlayed: undefined, blocks: [], art })).toBe('run-hero');
    expect(ownAutoArt('playing', { running: [], mostPlayed: undefined, blocks: [], art })).toBe('');
  });

  it('falls back to the cover when there is no wide art', () => {
    expect(ownAutoArt('most_played', { running: [], mostPlayed: game('top', { canonicalGameId: 'top' }), blocks: [], art })).toBe('top-cover');
  });

  it('uses the first pinned game for pinned', () => {
    const blocks = resolveOwn(
      layoutOf(block('a', 'about'), block('p', 'pinned', { igdbId: 1 })),
      ownContext({ art }, { layoutGames: { '1': { game: game('run', { canonicalGameId: 'run' }) } } }),
    );

    expect(ownAutoArt('pinned', { running: [], mostPlayed: undefined, blocks, art })).toBe('run-hero');
    expect(ownAutoArt('pinned', { running: [], mostPlayed: undefined, blocks: [], art })).toBe('');
  });
});

function profileOf(patch: Partial<PublicProfile> = {}): PublicProfile {
  return {
    id: 'u1',
    username: 'neo',
    displayName: 'Neo',
    avatarUrl: '',
    bio: 'hello',
    relation: 'friend',
    visibility: 'public',
    stats: { games: 10, completed: 2, hours: 50 },
    favorites: [],
    showcase: [],
    recentlyPlayed: [],
    recentActivity: [],
    common: null,
    mutualFriends: [],
    mutualCount: 0,
    createdAt: '',
    ...patch,
  } as PublicProfile;
}

function publicContext(profile: PublicProfile, patch: Partial<PublicContext> = {}): PublicContext {
  return { profile, open: vi.fn(), empty: () => false, ...patch };
}

describe('publicLayout without a layout from the server', () => {
  it('rebuilds the page the old backend used to give: showcases, then recent, then activity', () => {
    const profile = profileOf({
      showcase: [
        { kind: 'favorites', games: [card(1)] },
        { kind: 'most_played', games: [card(2)] },
      ],
    });

    const layout = publicLayout(profile);

    expect(layout.blocks.map((item) => [item.type, item.width, item.config.source])).toEqual([
      ['playing', 'full', undefined],
      ['collection', 'full', 'favorites'],
      ['collection', 'full', 'most_played'],
      ['recent', 'full', undefined],
      ['activity', 'full', undefined],
    ]);
  });

  it('is built from defaultLayout', () => {
    const profile = profileOf({ showcase: [{ kind: 'favorites', games: [] }] });
    const wanted = defaultLayout({ showcase: ['favorites'] }).blocks.filter((item) => item.type !== 'stats' && item.type !== 'about');

    expect(publicLayout(profile).blocks.map((item) => item.id)).toEqual(wanted.map((item) => item.id));
  });

  it('renders each showcase from the public showcase field', () => {
    const profile = profileOf({
      showcase: [
        { kind: 'favorites', games: [card(1), card(2)] },
        { kind: 'recently_completed', games: [card(3)] },
      ],
    });

    const blocks = resolvePublic(publicLayout(profile), publicContext(profile));

    const favorites = bodyOf(blocks, 'col1');
    expect(favorites).toMatchObject({ kind: 'collection', title: 'Favorite games', hearts: true });
    expect(favorites.kind === 'collection' && favorites.games.map((item) => item.title)).toEqual(['Card 1', 'Card 2']);
    expect(bodyOf(blocks, 'col2')).toMatchObject({ kind: 'collection', dated: true });
  });

  it('uses the layout from the server when there is one', () => {
    const server = layoutOf(block('b1', 'about'));

    expect(publicLayout(profileOf({ layout: server }))).toBe(server);
  });
});

describe('resolvePublic', () => {
  function resolve(blocks: PublicBlock[], profile = profileOf()) {
    return resolvePublic({ version: 1, blocks }, publicContext(profile));
  }

  it('turns the server data of a pinned block into a game with hours and a completed date', () => {
    const data = { game: { ...card(5), playtimeSeconds: 600, status: 'completed', favorite: false, lastPlayedAt: null, completedAt: '2026-08-01T00:00:00Z' } };

    const body = bodyOf(resolve([{ ...block('p', 'pinned', { igdbId: 5, caption: 'c' }), data }]), 'p');

    expect(body).toMatchObject({ kind: 'pinned', caption: 'c' });
    expect(body.kind === 'pinned' && body.game).toMatchObject({ title: 'Card 5', cover: 'c5', hero: 'h5', seconds: 600, completedAt: '2026-08-01T00:00:00Z' });
  });

  it('shows the error state for data the server could not resolve, and for data that is missing', () => {
    const blocks = resolve([
      { ...block('p', 'pinned', { igdbId: 5 }), data: { error: 'unresolved' } },
      { ...block('c', 'collection', { source: 'manual', title: 'T', igdbIds: [1] }), data: { error: 'unresolved' } },
      block('g', 'genres'),
      block('f', 'fingerprint'),
      block('m', 'collection', { source: 'manual', title: 'T', igdbIds: [1] }),
    ]);

    expect(blocks.map((item) => item.body.kind)).toEqual(['error', 'error', 'error', 'error', 'error']);
  });

  it('takes the collection title from the server and falls back to the source label', () => {
    const blocks = resolve([
      { ...block('a', 'collection', { source: 'manual', title: 'Mine', igdbIds: [1] }), data: { title: 'Best of 2025', games: [card(1)] } },
      { ...block('b', 'collection', { source: 'most_played' }), data: { games: [card(2)] } },
    ]);

    expect(bodyOf(blocks, 'a')).toMatchObject({ title: 'Best of 2025' });
    expect(bodyOf(blocks, 'b')).toMatchObject({ title: 'Most played' });
  });

  it('opens a game by its catalog id', () => {
    const open = vi.fn();
    const blocks = resolvePublic(
      { version: 1, blocks: [{ ...block('c', 'collection', { source: 'favorites' }), data: { games: [card(8)] } }] },
      publicContext(profileOf(), { open }),
    );

    const body = bodyOf(blocks, 'c');
    if (body.kind === 'collection') body.games[0].open();

    expect(open).toHaveBeenCalledWith(card(8));
  });

  it('reads genres and the fingerprint, taking the game count from stats when the server sends a list there', () => {
    const genres = [{ name: 'RPG', share: 0.7 }];
    const blocks = resolve([
      { ...block('g', 'genres'), data: { genres, other: 0.1, unknown: 0.2 } },
      { ...block('f', 'fingerprint'), data: { genres, other: 0, unknown: 0, hours: 412, games: 38 as unknown as GameCard[], completed: 12 } },
      { ...block('s', 'fingerprint'), data: { genres, hours: 5, games: [] as GameCard[], completed: 1 } },
    ]);

    expect(bodyOf(blocks, 'g')).toEqual({ kind: 'genres', breakdown: { genres, other: 0.1, unknown: 0.2 } });
    expect(bodyOf(blocks, 'f')).toMatchObject({ kind: 'fingerprint', hours: 412, games: 38, completed: 12 });
    expect(bodyOf(blocks, 's')).toMatchObject({ kind: 'fingerprint', games: 10 });
  });

  it('keeps hidden playtime as null instead of turning it into zero hours', () => {
    const genres = [{ name: 'RPG', share: 0.7 }];
    const hidden = profileOf({ stats: { games: 10, completed: 2, hours: null } });

    const blocks = resolve([{ ...block('f', 'fingerprint'), data: { genres, games: 38 as unknown as GameCard[], completed: 12 } }], hidden);

    expect(bodyOf(blocks, 'f')).toMatchObject({ kind: 'fingerprint', hours: null, games: 38, completed: 12 });
  });

  it('keeps every counter null when neither the block nor the stats carry it', () => {
    const blocks = resolve([{ ...block('f', 'fingerprint'), data: { genres: [] } }], profileOf({ stats: null }));

    expect(bodyOf(blocks, 'f')).toMatchObject({ kind: 'fingerprint', hours: null, games: null, completed: null });
  });

  it('renders text from the config and an unknown type as nothing', () => {
    const blocks = resolve([block('t', 'text', { title: 'T', body: 'B' }), block('x', 'future')]);

    expect(bodyOf(blocks, 't')).toEqual({ kind: 'text', title: 'T', body: 'B' });
    expect(bodyOf(blocks, 'x')).toEqual({ kind: 'unknown' });
  });

  it('never marks a server-filtered block as hidden or unfinished', () => {
    const blocks = resolve([block('g', 'genres'), block('p', 'pinned', { igdbId: 0 })]);

    expect(blocks.every((item) => !item.hidden && !item.issue)).toBe(true);
  });
});
