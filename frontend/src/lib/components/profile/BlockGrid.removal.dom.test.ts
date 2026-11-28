import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../i18n';
import type { ProfileBlock } from '../../services/account';
import type { GridBlock } from '../../profile/layoutView';
import { calls, resetBindings } from '../../testing/fakeBinding';
import { user } from '../../testing/dom';
import BlockGrid from './BlockGrid.svelte';
import BlockSettings from './BlockSettings.svelte';
import { blockLabel } from './blockLabels';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: false }));
vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));

const NONE = { genres: [], other: 0, unknown: 0 };

/** Three kinds of block, so each has names of its own. */
function cell(id: string): GridBlock {
  const base = { id, index: 0, width: 'full' as const, hidden: false, issue: false };
  if (id === 'a') return { ...base, type: 'genres', body: { kind: 'genres', breakdown: NONE } };
  if (id === 'b') return { ...base, type: 'fingerprint', body: { kind: 'fingerprint', breakdown: NONE, hours: 1, games: 1, completed: 0 } };
  return { ...base, type: 'text', body: { kind: 'text', title: '', body: 'words' } };
}

const TYPES: Record<string, string> = { a: 'genres', b: 'fingerprint', c: 'text' };

const indexed = (blocks: GridBlock[]) => blocks.map((block, index) => ({ ...block, index }));

/**
 * The editor as the profile page lays it out: the grid, the control that adds a block after it, and
 * the settings of the selected block beside it; the page applies every removal and closes the
 * settings of a removed block.
 */
function editor(ids: string[], selected = '') {
  let blocks = indexed(ids.map(cell));
  const page = document.createElement('div');
  const side = document.createElement('div');
  document.body.append(page, side);
  const remove = (id: string) => {
    blocks = indexed(blocks.filter((block) => block.id !== id));
    void grid.rerender({ blocks });
    if (id === selected) settings?.unmount();
  };
  const grid = render(BlockGrid, { target: page, props: { blocks, editing: true, onremove: remove } });
  const add = document.createElement('button');
  add.textContent = 'Add block';
  page.append(add);
  const target: ProfileBlock = { id: selected, type: TYPES[selected] ?? 'genres', width: 'full', config: {} };
  const settings = selected
    ? render(BlockSettings, {
        target: side,
        props: { block: target, layout: { version: 1, blocks: [target] }, titleOf: () => '', onconfig: () => {}, onremember: () => {}, onremove: () => remove(selected), onclose: () => {} },
      })
    : null;
  return { add };
}

const named = (key: 'profile.removeBlock' | 'profile.blockSettings', id: string) =>
  screen.getAllByRole('button', { name: msg(key, { name: blockLabel(TYPES[id]) }) }).filter((button) => !button.closest('.settings'));
const removeOf = (id: string) => named('profile.removeBlock', id)[0];
const gone = (id: string) => screen.queryAllByRole('button', { name: msg('profile.blockSettings', { name: blockLabel(TYPES[id]) }) }).length === 0;

beforeEach(() => {
  locale.set('en');
  resetBindings();
});

afterEach(() => {
  document.body.replaceChildren();
  expect(calls).toEqual([]);
});

describe('removing a block', () => {
  it('from its own button hands focus to the same button of the block now in its place, else the one above, else the control after the grid', async () => {
    const { add } = editor(['a', 'b', 'c']);
    const keys = user();

    removeOf('b').focus();
    await keys.keyboard('{Enter}');
    await vi.waitFor(() => expect(document.activeElement).toBe(removeOf('c')));
    await keys.keyboard('{Enter}');
    await vi.waitFor(() => expect(document.activeElement).toBe(removeOf('a')));
    await keys.keyboard('{Enter}');
    await vi.waitFor(() => expect(gone('a')).toBe(true));
    await vi.waitFor(() => expect(document.activeElement).toBe(add));
  });

  it('from its settings hands focus to the settings button of the block now in its place', async () => {
    editor(['a', 'b', 'c'], 'b');
    const region = screen.getByRole('region', { name: msg('profile.blockSettings', { name: blockLabel('fingerprint') }) });
    (region.querySelector('.foot button') as HTMLElement).focus();

    await user().keyboard('{Enter}');
    await vi.waitFor(() => expect(region.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(named('profile.blockSettings', 'c')[0]));
  });

  it('pressed with no focus held leaves focus alone', async () => {
    editor(['a', 'b']);

    // A pointer press in the macOS view focuses no button.
    await fireEvent.click(removeOf('a'));
    await vi.waitFor(() => expect(gone('a')).toBe(true));
    await tick();
    expect(document.activeElement).toBe(document.body);
  });
});
