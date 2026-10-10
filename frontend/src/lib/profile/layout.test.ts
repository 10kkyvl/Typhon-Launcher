import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import type { ProfileBlock, ProfileLayout, ProfileSettings } from '../services/account';
import { DEFAULT_PROFILE } from '../services/account';
import {
  addBlock,
  blockIssue,
  canAdd,
  ADD_PRESETS,
  defaultLayout,
  effectiveLayout,
  hiddenFromOthers,
  layoutPatch,
  moveBlock,
  newBlockId,
  removeBlock,
  runeCount,
  sameLayout,
  setWidth,
  updateConfig,
  validateLayout,
  visibleBlocks,
} from './layout';

function block(id: string, type: string, config: Record<string, unknown> = {}, width: ProfileBlock['width'] = 'full'): ProfileBlock {
  return { id, type, width, config };
}

function layoutOf(...blocks: ProfileBlock[]): ProfileLayout {
  return { version: 1, blocks };
}

function ok<T extends { ok: boolean }>(result: T): Extract<T, { ok: true }> {
  expect(result.ok).toBe(true);
  return result as Extract<T, { ok: true }>;
}

function code(result: { ok: boolean; error?: { code: string } }): string | undefined {
  return result.ok ? undefined : result.error?.code;
}

const unknown = block('x1', 'future', { shape: 'star' });

describe('defaultLayout', () => {
  it('builds the legacy showcase into blocks and leaves bio and stats to the header', () => {
    const layout = defaultLayout({ showcase: ['favorites', 'most_played'] });

    expect(layout.version).toBe(1);
    expect(layout.blocks.map((item) => [item.type, item.width, item.config.source])).toEqual([
      ['playing', 'full', undefined],
      ['collection', 'full', 'favorites'],
      ['collection', 'full', 'most_played'],
      ['recent', 'full', undefined],
      ['activity', 'full', undefined],
    ]);
  });

  it('has no collection when the showcase is empty', () => {
    const layout = defaultLayout({ showcase: [] });

    expect(layout.blocks.map((item) => item.type)).toEqual(['playing', 'recent', 'activity']);
  });

  it('gives every block a valid unique id', () => {
    const layout = defaultLayout({ showcase: ['favorites', 'recently_completed', 'most_played'] });
    const ids = layout.blocks.map((item) => item.id);

    expect(new Set(ids).size).toBe(ids.length);
    expect(ids.every((id) => /^[a-z0-9]{1,16}$/.test(id))).toBe(true);
  });

  it('passes validation', () => {
    expect(validateLayout(defaultLayout(DEFAULT_PROFILE))).toEqual([]);
  });
});

describe('effectiveLayout', () => {
  it('builds the default for a profile that was never customised', () => {
    expect(effectiveLayout({ showcase: ['favorites'], layout: null })).toEqual(defaultLayout({ showcase: ['favorites'] }));
    expect(effectiveLayout({ showcase: ['favorites'] })).toEqual(defaultLayout({ showcase: ['favorites'] }));
  });

  it('prefers the stored layout over the showcase', () => {
    const stored = layoutOf(block('a1', 'about'));

    expect(effectiveLayout({ showcase: ['favorites'], layout: stored })).toBe(stored);
  });
});

describe('newBlockId', () => {
  it.each([
    [[], 'b1'],
    [['b1'], 'b2'],
    [['b1', 'b2', 'b4'], 'b3'],
    [['b1', 'b2', 'b3', 'b4', 'b5', 'b6', 'b7', 'b8', 'b9'], 'ba'],
  ])('picks the first free id for %j', (taken, expected) => {
    expect(newBlockId(layoutOf(...taken.map((id) => block(id, 'about'))))).toBe(expected);
  });
});

describe('addBlock', () => {
  it('appends a block with the preset width and config', () => {
    const result = ok(addBlock(layoutOf(), 'genres'));

    expect(result.layout.blocks).toEqual([{ id: 'b1', type: 'genres', width: 'half', config: {} }]);
    expect(result.id).toBe('b1');
  });

  it('does not touch the layout it was given', () => {
    const before = layoutOf(block('a1', 'about'));
    const snapshot = JSON.stringify(before);

    addBlock(before, 'genres');

    expect(JSON.stringify(before)).toBe(snapshot);
  });

  it('does not share the preset config with the new block', () => {
    const first = ok(addBlock(layoutOf(), 'text'));
    first.layout.blocks[0].config.body = 'changed';

    expect(ok(addBlock(layoutOf(), 'text')).layout.blocks[0].config.body).toBe('');
  });

  it.each(['genres', 'fingerprint', 'recent', 'activity', 'stats', 'playing', 'about'])('allows %s only once', (type) => {
    const first = ok(addBlock(layoutOf(), type));

    expect(code(addBlock(first.layout, type))).toBe('duplicate');
  });

  it('allows several pinned blocks and several manual collections', () => {
    let layout = layoutOf();
    layout = ok(addBlock(layout, 'pinned')).layout;
    layout = ok(addBlock(layout, 'pinned')).layout;
    layout = ok(addBlock(layout, 'collection')).layout;
    layout = ok(addBlock(layout, 'collection')).layout;

    expect(layout.blocks).toHaveLength(4);
  });

  it('allows each auto collection source only once', () => {
    const favorites = ADD_PRESETS.find((preset) => preset.id === 'favorites')!;
    const first = ok(addBlock(layoutOf(), 'collection', { config: favorites.config }));

    const second = addBlock(first.layout, 'collection', { config: favorites.config });

    expect(code(second)).toBe('duplicate_source');
    expect(code(addBlock(first.layout, 'collection', { config: { source: 'most_played' } }))).toBeUndefined();
  });

  it('allows at most four text blocks', () => {
    let layout = layoutOf();
    for (let i = 0; i < 4; i++) layout = ok(addBlock(layout, 'text')).layout;

    expect(code(addBlock(layout, 'text'))).toBe('max_text');
  });

  it('allows at most sixteen blocks, unknown ones included', () => {
    const filled = layoutOf(...Array.from({ length: 15 }, (_, i) => block(`p${i}`, 'pinned', { igdbId: i + 1, caption: '' })), unknown);

    expect(code(addBlock(filled, 'pinned'))).toBe('max_blocks');
  });

  it('refuses an unknown type', () => {
    expect(code(addBlock(layoutOf(), 'future'))).toBe('unknown_type');
  });

  it('refuses a width outside the contract', () => {
    expect(code(addBlock(layoutOf(), 'genres', { width: 'wide' as 'full' }))).toBe('invalid_width');
  });

  it('refuses a config that breaks the limits instead of cutting it', () => {
    const caption = 'я'.repeat(141);

    expect(addBlock(layoutOf(), 'pinned', { config: { igdbId: 1, caption } })).toEqual({
      ok: false,
      error: { code: 'invalid_config', field: 'caption' },
    });
  });

  it('keeps unknown blocks exactly where and as they were', () => {
    const before = layoutOf(block('a1', 'about'), unknown);

    const after = ok(addBlock(before, 'genres')).layout;

    expect(after.blocks[1]).toBe(unknown);
  });
});

describe('canAdd', () => {
  it('names the reason a preset is unavailable', () => {
    const layout = layoutOf(block('a1', 'about'));
    const about = ADD_PRESETS.find((preset) => preset.id === 'about')!;
    const genres = ADD_PRESETS.find((preset) => preset.id === 'genres')!;

    expect(canAdd(layout, about)).toEqual({ code: 'duplicate' });
    expect(canAdd(layout, genres)).toBeNull();
  });
});

describe('removeBlock', () => {
  it('removes one block and keeps the order of the rest', () => {
    const layout = layoutOf(block('a1', 'about'), block('g1', 'genres'), block('s1', 'stats'));

    expect(ok(removeBlock(layout, 'g1')).layout.blocks.map((item) => item.id)).toEqual(['a1', 's1']);
  });

  it('reports a missing id', () => {
    expect(code(removeBlock(layoutOf(block('a1', 'about')), 'zz'))).toBe('not_found');
  });

  it('refuses to remove a block of an unknown type', () => {
    expect(code(removeBlock(layoutOf(unknown), 'x1'))).toBe('unknown_type');
  });
});

describe('moveBlock', () => {
  const layout = layoutOf(block('a', 'about'), block('g', 'genres'), unknown, block('s', 'stats'));

  it.each([
    [0, 1, ['g', 'a', 'x1', 's']],
    [0, 3, ['g', 'x1', 's', 'a']],
    [3, 0, ['s', 'a', 'g', 'x1']],
    [1, 1, ['a', 'g', 'x1', 's']],
  ])('moves %i to %i', (from, to, ids) => {
    expect(ok(moveBlock(layout, from, to)).layout.blocks.map((item) => item.id)).toEqual(ids);
  });

  it.each([
    [-1, 0],
    [0, 4],
    [1.5, 0],
    [0, Number.NaN],
  ])('rejects %s to %s', (from, to) => {
    expect(code(moveBlock(layout, from, to))).toBe('out_of_range');
  });

  it('keeps the unknown block untouched', () => {
    expect(ok(moveBlock(layout, 0, 3)).layout.blocks).toContain(unknown);
  });
});

describe('setWidth', () => {
  const layout = layoutOf(block('a', 'about'), unknown);

  it.each(['full', 'half', 'third'])('sets %s', (width) => {
    expect(ok(setWidth(layout, 'a', width)).layout.blocks[0].width).toBe(width);
  });

  it('rejects other widths without clamping', () => {
    expect(code(setWidth(layout, 'a', 'quarter'))).toBe('invalid_width');
  });

  it('reports a missing id and refuses unknown blocks', () => {
    expect(code(setWidth(layout, 'zz', 'half'))).toBe('not_found');
    expect(code(setWidth(layout, 'x1', 'half'))).toBe('unknown_type');
  });

  it('leaves the other blocks as they were', () => {
    expect(ok(setWidth(layout, 'a', 'half')).layout.blocks[1]).toBe(unknown);
  });
});

describe('updateConfig', () => {
  it('merges the patch into the config', () => {
    const layout = layoutOf(block('p', 'pinned', { igdbId: 0, caption: '' }));

    const next = ok(updateConfig(layout, 'p', { igdbId: 7 })).layout.blocks[0];

    expect(next.config).toEqual({ igdbId: 7, caption: '' });
  });

  it('removes a key set to undefined', () => {
    const layout = layoutOf(block('c', 'collection', { source: 'manual', title: 'T', igdbIds: [1] }));

    const next = ok(updateConfig(layout, 'c', { source: 'favorites', title: undefined, igdbIds: undefined })).layout.blocks[0];

    expect(next.config).toEqual({ source: 'favorites' });
  });

  it.each([
    ['caption over the limit', 'pinned', { igdbId: 1, caption: 'x'.repeat(141) }, { caption: 'x'.repeat(141) }, 'caption'],
    ['negative game id', 'pinned', { igdbId: 1, caption: '' }, { igdbId: -3 }, 'igdbId'],
    ['fractional game id', 'pinned', { igdbId: 1, caption: '' }, { igdbId: 1.5 }, 'igdbId'],
    ['title over the limit', 'text', { title: '', body: 'b' }, { title: 'x'.repeat(41) }, 'title'],
    ['body over the limit', 'text', { title: '', body: 'b' }, { body: 'x'.repeat(1001) }, 'body'],
    ['collection title over the limit', 'collection', { source: 'manual', title: '', igdbIds: [] }, { title: 'x'.repeat(41) }, 'title'],
    ['thirteen games', 'collection', { source: 'manual', title: 't', igdbIds: [] }, { igdbIds: Array.from({ length: 13 }, (_, i) => i + 1) }, 'igdbIds'],
    ['repeated game', 'collection', { source: 'manual', title: 't', igdbIds: [] }, { igdbIds: [4, 4] }, 'igdbIds'],
    ['zero game id in a list', 'collection', { source: 'manual', title: 't', igdbIds: [] }, { igdbIds: [0] }, 'igdbIds'],
    ['unknown source', 'collection', { source: 'manual', title: 't', igdbIds: [] }, { source: 'nope' }, 'source'],
    ['title on an auto source', 'collection', { source: 'favorites' }, { title: 'T' }, 'title'],
    ['games on an auto source', 'collection', { source: 'favorites' }, { igdbIds: [1] }, 'igdbIds'],
  ])('refuses %s', (_name, type, config, patch, field) => {
    const layout = layoutOf(block('b', type, config));

    expect(updateConfig(layout, 'b', patch)).toEqual({ ok: false, error: { code: 'invalid_config', field } });
  });

  it('accepts values exactly at the limits, counting runes', () => {
    const layout = layoutOf(block('p', 'pinned', { igdbId: 1, caption: '' }), block('t', 'text', { title: '', body: '' }));

    expect(updateConfig(layout, 'p', { caption: '😀'.repeat(140) }).ok).toBe(true);
    expect(updateConfig(layout, 't', { body: 'ы'.repeat(1000), title: 'ы'.repeat(40) }).ok).toBe(true);
    expect(runeCount('😀😀')).toBe(2);
  });

  it('refuses a second collection with the same auto source', () => {
    const layout = layoutOf(block('c1', 'collection', { source: 'favorites' }), block('c2', 'collection', { source: 'manual', title: 't', igdbIds: [1] }));

    expect(code(updateConfig(layout, 'c2', { source: 'favorites', title: undefined, igdbIds: undefined }))).toBe('duplicate_source');
  });

  it('lets a collection keep its own source', () => {
    const layout = layoutOf(block('c1', 'collection', { source: 'favorites' }));

    expect(updateConfig(layout, 'c1', { source: 'favorites' }).ok).toBe(true);
  });

  it('reports a missing id and refuses unknown blocks', () => {
    const layout = layoutOf(unknown);

    expect(code(updateConfig(layout, 'zz', {}))).toBe('not_found');
    expect(code(updateConfig(layout, 'x1', { shape: 'moon' }))).toBe('unknown_type');
  });

  it('does not touch the layout it was given', () => {
    const layout = layoutOf(block('p', 'pinned', { igdbId: 0, caption: '' }));
    const snapshot = JSON.stringify(layout);

    updateConfig(layout, 'p', { igdbId: 9 });

    expect(JSON.stringify(layout)).toBe(snapshot);
  });
});

describe('blockIssue and validateLayout', () => {
  it.each([
    ['empty pinned', block('p', 'pinned', { igdbId: 0, caption: '' }), 'igdbId'],
    ['manual collection without a title', block('c', 'collection', { source: 'manual', title: '  ', igdbIds: [1] }), 'title'],
    ['manual collection without games', block('c', 'collection', { source: 'manual', title: 'T', igdbIds: [] }), 'igdbIds'],
    ['text without a body', block('t', 'text', { title: 'T', body: ' \n ' }), 'body'],
  ])('flags %s as incomplete', (_name, item, field) => {
    expect(blockIssue(item)).toEqual({ code: 'incomplete', field });
  });

  it.each([
    block('p', 'pinned', { igdbId: 5, caption: '' }),
    block('c', 'collection', { source: 'manual', title: 'T', igdbIds: [1, 2] }),
    block('c', 'collection', { source: 'favorites' }),
    block('t', 'text', { title: '', body: 'hello' }),
    block('g', 'genres'),
    unknown,
  ])('accepts a finished block', (item) => {
    expect(blockIssue(item)).toBeNull();
  });

  it('collects an issue per broken block', () => {
    const layout = layoutOf(block('p', 'pinned', { igdbId: 0 }), block('g', 'genres'), block('t', 'text', { body: '' }));

    expect(validateLayout(layout).map((issue) => issue.id)).toEqual(['p', 't']);
  });

  it('catches duplicates that came from outside', () => {
    const layout = layoutOf(
      block('g1', 'genres'),
      block('g2', 'genres'),
      block('c1', 'collection', { source: 'favorites' }),
      block('c2', 'collection', { source: 'favorites' }),
    );

    expect(validateLayout(layout).map((issue) => [issue.id, issue.error.code])).toEqual([
      ['g2', 'duplicate'],
      ['c2', 'duplicate_source'],
    ]);
  });

  it('catches a fifth text block and a seventeenth block', () => {
    const texts = layoutOf(...Array.from({ length: 5 }, (_, i) => block(`t${i}`, 'text', { body: 'x' })));
    const many = layoutOf(...Array.from({ length: 17 }, (_, i) => block(`p${i}`, 'pinned', { igdbId: i + 1 })));

    expect(validateLayout(texts).map((issue) => issue.error.code)).toEqual(['max_text']);
    expect(validateLayout(many).map((issue) => issue.error.code)).toEqual(['max_blocks']);
  });
});

describe('visibility flags', () => {
  const flags = { showPlaying: true, showLibrary: true, showActivity: true, showStats: true };
  const everything = layoutOf(
    block('a', 'activity'),
    block('s', 'stats'),
    block('g', 'genres'),
    block('f', 'fingerprint'),
    block('p', 'playing'),
    block('r', 'recent'),
    block('n', 'pinned', { igdbId: 1 }),
    block('c', 'collection', { source: 'favorites' }),
    block('t', 'text', { body: 'x' }),
    block('o', 'about'),
    unknown,
  );

  it('keeps everything when everything is shown', () => {
    expect(visibleBlocks(everything, flags)).toHaveLength(11);
  });

  it.each([
    [{ showActivity: false }, ['a']],
    [{ showStats: false }, ['s', 'g', 'f']],
    [{ showPlaying: false }, ['p']],
    [{ showLibrary: false }, ['r', 'n', 'c']],
  ])('hides what %j hides', (patch, hidden) => {
    const shown = visibleBlocks(everything, { ...flags, ...patch }).map((item) => item.id);

    expect(everything.blocks.map((item) => item.id).filter((id) => !shown.includes(id))).toEqual(hidden);
  });

  it('never hides text, about or an unknown block', () => {
    const none = { showPlaying: false, showLibrary: false, showActivity: false, showStats: false };

    expect(visibleBlocks(everything, none).map((item) => item.id)).toEqual(['t', 'o', 'x1']);
    expect(hiddenFromOthers('future', none)).toBe(false);
  });
});

describe('sameLayout', () => {
  it('ignores key order and undefined values', () => {
    const left = layoutOf({ id: 'a', type: 'pinned', width: 'full', config: { igdbId: 1, caption: 'c' } });
    const right = layoutOf({ id: 'a', type: 'pinned', width: 'full', config: { caption: 'c', igdbId: 1, extra: undefined } });

    expect(sameLayout(left, right)).toBe(true);
  });

  it('tells a changed width, config or order apart', () => {
    const base = layoutOf(block('a', 'about'), block('g', 'genres'));

    expect(sameLayout(base, layoutOf(block('a', 'about', {}, 'half'), block('g', 'genres')))).toBe(false);
    expect(sameLayout(base, layoutOf(block('a', 'about', { x: 1 }), block('g', 'genres')))).toBe(false);
    expect(sameLayout(base, layoutOf(block('g', 'genres'), block('a', 'about')))).toBe(false);
  });

  it('treats a missing layout as null', () => {
    expect(sameLayout(null, undefined)).toBe(true);
    expect(sameLayout(null, layoutOf())).toBe(false);
  });
});

describe('layoutPatch', () => {
  const never: Pick<ProfileSettings, 'showcase' | 'layout'> = { showcase: ['favorites'], layout: null };
  const custom: Pick<ProfileSettings, 'showcase' | 'layout'> = { showcase: ['favorites'], layout: layoutOf(block('a', 'about')) };

  it('sends nothing when the draft is the layout the profile already has', () => {
    expect(layoutPatch(never, defaultLayout(never), false)).toEqual({ send: false });
    expect(layoutPatch(custom, custom.layout!, false)).toEqual({ send: false });
  });

  it('sends the draft when it differs', () => {
    const edited = layoutOf(block('a', 'about'), block('g', 'genres'));

    expect(layoutPatch(custom, edited, false)).toEqual({ send: true, layout: edited });
    expect(layoutPatch(never, edited, false)).toEqual({ send: true, layout: edited });
  });

  it('sends null to reset a stored layout', () => {
    expect(layoutPatch(custom, defaultLayout(custom), true)).toEqual({ send: true, layout: null });
  });

  it('sends nothing for a reset when there was nothing stored', () => {
    expect(layoutPatch(never, defaultLayout(never), true)).toEqual({ send: false });
  });

  it('sends the layout when the reset was followed by an edit', () => {
    const edited = layoutOf(...defaultLayout(custom).blocks, block('g', 'genres'));

    expect(layoutPatch(custom, edited, true)).toEqual({ send: true, layout: edited });
  });
});
