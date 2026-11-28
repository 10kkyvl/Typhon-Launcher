import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../i18n';
import type { GridBlock } from '../../profile/layoutView';
import { calls, resetBindings } from '../../testing/fakeBinding';
import { user } from '../../testing/dom';
import BlockGrid from './BlockGrid.svelte';
import { blockLabel } from './blockLabels';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: false }));
vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));

function genres(id: string): GridBlock {
  return { id, index: 0, type: 'genres', width: 'full', hidden: false, issue: false, body: { kind: 'genres', breakdown: { genres: [], other: 0, unknown: 0 } } };
}

function text(id: string): GridBlock {
  return { id, index: 0, type: 'text', width: 'full', hidden: false, issue: false, body: { kind: 'text', title: '', body: id } };
}

function unknown(id: string): GridBlock {
  return { id, index: 0, type: 'future', width: 'full', hidden: false, issue: false, body: { kind: 'unknown' } };
}

const indexed = (blocks: GridBlock[]) => blocks.map((block, index) => ({ ...block, index }));

/** A grid in the editor whose page applies every move, width and removal, as the profile page does. */
function editor(start: GridBlock[]) {
  let blocks = indexed(start);
  const moves: [number, number][] = [];
  const onmove = vi.fn((from: number, to: number) => {
    moves.push([from, to]);
    const next = [...blocks];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    blocks = indexed(next);
    void rendered.rerender({ blocks });
  });
  const onremove = vi.fn((id: string) => {
    blocks = indexed(blocks.filter((block) => block.id !== id));
    void rendered.rerender({ blocks });
  });
  const onselect = vi.fn();
  const onwidth = vi.fn();
  const rendered = render(BlockGrid, { props: { blocks, editing: true, onmove, onremove, onselect, onwidth } });
  return { moves, onmove, onremove, onselect, onwidth, order: () => blocks.map((block) => block.id) };
}

const handle = (id: string) => document.querySelector<HTMLElement>(`[data-handle="${id}"]`) as HTMLElement;
const text$ = () => blockLabel('text');

beforeEach(() => {
  locale.set('en');
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
});

describe('reordering the blocks with the keyboard', () => {
  it('moves the block whose handle has focus with the arrows, and keeps focus on that handle', async () => {
    const grid = editor([text('a'), text('b'), text('c')]);
    const keys = user();
    handle('a').focus();

    await keys.keyboard('{ArrowDown}');
    await vi.waitFor(() => expect(grid.order()).toEqual(['b', 'a', 'c']));
    await vi.waitFor(() => expect(document.activeElement).toBe(handle('a')));
    await keys.keyboard('{ArrowRight}');
    await vi.waitFor(() => expect(grid.order()).toEqual(['b', 'c', 'a']));
    await keys.keyboard('{ArrowLeft}');
    await keys.keyboard('{ArrowUp}');
    await vi.waitFor(() => expect(grid.order()).toEqual(['a', 'b', 'c']));
    await vi.waitFor(() => expect(document.activeElement).toBe(handle('a')));
    expect(grid.moves).toEqual([[0, 1], [1, 2], [2, 1], [1, 0]]);
  });

  it('does nothing at either end, or for another key', async () => {
    const grid = editor([text('a'), text('b')]);
    const keys = user();
    handle('a').focus();

    await keys.keyboard('{ArrowUp}');
    await keys.keyboard('{Home}');
    await keys.keyboard('x');
    handle('b').focus();
    await keys.keyboard('{ArrowDown}');
    expect(grid.onmove).not.toHaveBeenCalled();
  });

  it('passes over a block of a kind this launcher does not know, moving by the indexes of the layout', async () => {
    const grid = editor([text('a'), unknown('x'), text('b')]);
    handle('a').focus();

    await user().keyboard('{ArrowDown}');
    expect(grid.moves).toEqual([[0, 2]]);
  });

  it('moves a block with its move buttons too', async () => {
    const grid = editor([text('a'), genres('g')]);
    const clicks = user();

    await clicks.click(screen.getByRole('button', { name: msg('social.moveDown', { title: text$() }) }));
    expect(grid.moves).toEqual([[0, 1]]);
    await clicks.click(screen.getByRole('button', { name: msg('social.moveDown', { title: blockLabel('genres') }) }));
    expect(grid.moves).toEqual([[0, 1], [0, 1]]);
  });

  it('names each handle by its block, and gives every block its width, settings and remove controls', async () => {
    const grid = editor([text('a'), genres('g')]);
    const clicks = user();

    expect(handle('a').getAttribute('aria-label')).toBe(msg('profile.dragBlock', { name: text$() }));
    await clicks.click(screen.getByRole('button', { name: msg('profile.blockSettings', { name: text$() }) }));
    await clicks.click(screen.getAllByRole('radio', { name: msg('profile.widthHalf') })[0]);
    await clicks.click(screen.getByRole('button', { name: msg('profile.removeBlock', { name: blockLabel('genres') }) }));
    expect([grid.onselect.mock.calls, grid.onwidth.mock.calls, grid.onremove.mock.calls]).toEqual([[['a']], [['a', 'half']], [['g']]]);
  });
});

describe('dragging a block', () => {
  it('drops the grabbed block where the pointer is let go, and an Escape lets go of it unmoved', async () => {
    const grid = editor([text('a'), text('b')]);
    const cells = [...document.querySelectorAll<HTMLElement>('[data-index]')];
    cells.forEach((cell, index) => {
      cell.getBoundingClientRect = () => ({ left: 0, right: 100, top: index * 100, bottom: index * 100 + 90, width: 100, height: 90, x: 0, y: index * 100, toJSON: () => ({}) });
    });
    const grip = handle('a');
    grip.setPointerCapture = () => {};

    await fireEvent.pointerDown(grip, { button: 0, pointerId: 1 });
    await fireEvent.keyDown(grip, { key: 'Escape' });
    await fireEvent.pointerUp(grip);
    expect(grid.onmove).not.toHaveBeenCalled();

    await fireEvent.pointerDown(grip, { button: 0, pointerId: 1 });
    await fireEvent.pointerMove(grip, { clientX: 50, clientY: 150 });
    await fireEvent.pointerUp(grip);
    expect(grid.moves).toEqual([[0, 1]]);
  });
});
