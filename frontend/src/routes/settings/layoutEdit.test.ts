import { describe, expect, it } from 'vitest';
import { MAX_SEPARATORS, SEPARATOR, SIDEBAR_DEFAULT_ORDER } from '../../lib/theme/layout';
import {
  addSeparator,
  canAddSeparator,
  dropOn,
  fromState,
  moveToColumn,
  removeSeparator,
  step,
  toState,
  toggleHidden,
} from './layoutEdit';

describe('layout normalization', () => {
  it('omits a layout that equals the default', () => {
    expect(fromState(toState(undefined))).toBeUndefined();
    expect(fromState(toState({ sidebar: { side: 'left', order: [...SIDEBAR_DEFAULT_ORDER] } }))).toBeUndefined();
  });

  it('keeps only the blocks that differ', () => {
    const moved = step(toState(undefined), 'topbar', 0, 1);
    expect(fromState(moved)).toEqual({ topbar: { order: ['notifications', 'bigpicture', 'friends'] } });
    const right = fromState({ ...toState(undefined), sidebarSide: 'right' });
    expect(right).toEqual({ sidebar: { side: 'right' } });
  });

  it('moves a sidebar item and round-trips', () => {
    const next = step(toState(undefined), 'sidebar', 0, 1);
    expect(next.sidebar.slice(0, 2)).toEqual(['catalog', 'library']);
    expect(toState(fromState(next)).sidebar).toEqual(next.sidebar);
  });

  it('ignores moves off the ends', () => {
    const state = toState(undefined);
    expect(step(state, 'sidebar', 0, -1)).toBe(state);
  });

  it('hides and shows items, support included', () => {
    const hidden = toggleHidden(toggleHidden(toState(undefined), 'sidebar', 'stats'), 'sidebar', 'support');
    expect(fromState(hidden)?.sidebar?.hidden).toEqual(['stats', 'support']);
    expect(fromState(toggleHidden(hidden, 'sidebar', 'stats'))?.sidebar?.hidden).toEqual(['support']);
  });

  it('limits separators', () => {
    let state = toState(undefined);
    const start = state.sidebar.filter((id) => id === SEPARATOR).length;
    for (let i = start; i < MAX_SEPARATORS + 3; i++) state = addSeparator(state);
    expect(state.sidebar.filter((id) => id === SEPARATOR)).toHaveLength(MAX_SEPARATORS);
    expect(canAddSeparator(state)).toBe(false);
    const index = state.sidebar.indexOf(SEPARATOR);
    expect(removeSeparator(state, index).sidebar.filter((id) => id === SEPARATOR)).toHaveLength(MAX_SEPARATORS - 1);
    expect(removeSeparator(state, 0)).toBe(state);
  });

  it('moves a game section to the other column', () => {
    const state = toState(undefined);
    const next = moveToColumn(state, 'main', state.main.indexOf('notes'), 'side', 1);
    expect(next.main).not.toContain('notes');
    expect(next.side[1]).toBe('notes');
    const layout = fromState(next);
    expect(layout?.gamePage?.side).toContain('notes');
    expect(toState(layout).side).toEqual(next.side);
    expect(toState(layout).main).toEqual(next.main);
  });

  it('drops within a list and between columns, never across unrelated lists', () => {
    const state = toState(undefined);
    expect(dropOn(state, { key: 'main', index: 0 }, { key: 'main', index: 2 }).main[2]).toBe(state.main[0]);
    expect(dropOn(state, { key: 'side', index: 0 }, { key: 'main' }).main.at(-1)).toBe(state.side[0]);
    expect(dropOn(state, { key: 'sidebar', index: 0 }, { key: 'topbar', index: 0 })).toBe(state);
  });

  it('records hidden game sections', () => {
    const next = toggleHidden(toState(undefined), 'game', 'reviews');
    expect(fromState(next)).toEqual({ gamePage: { hidden: ['reviews'] } });
  });
});
