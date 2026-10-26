import { describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));

import { navBlocks, themeLayout } from './layout';
import { activeTheme } from './theme';
import { sidebarEntries } from '../theme/layout';
import type { Theme } from '../services/theme';

const withLayout = (layout: Theme['layout']) => ({ id: 'x', layout }) as unknown as Theme;

describe('themeLayout', () => {
  it('falls back to the default layout without a theme', () => {
    activeTheme.set(null);
    const view = get(themeLayout);
    expect(view.side).toBe('left');
    expect(view.search).toBe('left');
    expect(view.supportHidden).toBe(false);
    expect(view.actions).toEqual(['bigpicture', 'notifications', 'friends']);
    expect(view.sections.main[0]).toBe('summary');
    expect(view.sections.side).toEqual(['about', 'achievements', 'reviewSummary', 'friends']);
  });

  it('follows the active theme layout', () => {
    activeTheme.set(
      withLayout({
        sidebar: { side: 'right', hidden: ['support'] },
        topbar: { search: 'center', order: ['friends'], hidden: ['bigpicture'] },
        gamePage: { side: ['reviews'], hidden: ['tags'] },
      }),
    );
    const view = get(themeLayout);
    expect(view.side).toBe('right');
    expect(view.supportHidden).toBe(true);
    expect(view.search).toBe('center');
    expect(view.actions).toEqual(['friends', 'notifications']);
    expect(view.sections.side[0]).toBe('reviews');
    expect(view.sections.main).not.toContain('reviews');
    expect(view.sections.main).not.toContain('tags');
    activeTheme.set(null);
  });
});

describe('navBlocks', () => {
  const ids = (blocks: ReturnType<typeof navBlocks>) =>
    blocks.map((block) => (block.kind === 'group' ? block.ids.join(',') : '|'));

  it('splits the default order into three groups', () => {
    expect(ids(navBlocks(sidebarEntries(null), true))).toEqual([
      'library,catalog,installed,diskusage',
      '|',
      'downloads,sources,friends,activity,stats,history',
      '|',
      'lan',
    ]);
  });

  it('drops the lan group and its separator when LAN sharing is off', () => {
    expect(ids(navBlocks(sidebarEntries(null), false))).toEqual([
      'library,catalog,installed,diskusage',
      '|',
      'downloads,sources,friends,activity,stats,history',
    ]);
  });

  it('never leaves a leading, doubled or trailing separator', () => {
    const layout = { sidebar: { order: ['lan', '-', 'library', '-', '-', 'catalog', '-'] } };
    const out = ids(navBlocks(sidebarEntries(layout), false));
    expect(out[0]).not.toBe('|');
    expect(out.at(-1)).not.toBe('|');
    expect(out.join(' ')).not.toContain('| |');
  });
});
