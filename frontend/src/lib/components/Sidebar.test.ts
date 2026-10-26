import { render } from 'svelte/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));

import Sidebar from './Sidebar.svelte';
import { activeTheme } from '../stores/theme';
import type { Theme } from '../services/theme';

const withLayout = (layout: Theme['layout']) => activeTheme.set({ id: 'x', layout } as unknown as Theme);
const html = () => render(Sidebar, { props: {} } as never).body;
const ids = (body: string) => [...body.matchAll(/data-part="sidebar-item" data-id="([a-z]+)"/g)].map((m) => m[1]);

afterEach(() => activeTheme.set(null));

describe('Sidebar layout', () => {
  it('renders the default order with the bottom items', () => {
    activeTheme.set(null);
    const body = html();
    expect(ids(body)).toEqual([
      'library', 'catalog', 'installed', 'diskusage',
      'downloads', 'sources', 'friends', 'activity', 'stats', 'history',
      'support', 'settings',
    ]);
    expect(body.match(/data-part="sidebar-separator"/g)).toHaveLength(1);
    expect(body).toContain('data-part="sidebar-nav"');
    expect(body).toContain('data-part="sidebar-bottom"');
    expect(body).toContain('data-part="sidebar-account"');
    expect(body).not.toMatch(/class="sidebar[^"]*\bright\b/);
  });

  it('follows a theme order, hides items and drops support', () => {
    withLayout({ sidebar: { order: ['stats', 'library'], hidden: ['catalog', 'support'] } });
    const list = ids(html());
    expect(list.slice(0, 2)).toEqual(['stats', 'library']);
    expect(list).not.toContain('catalog');
    expect(list).not.toContain('support');
    expect(list.at(-1)).toBe('settings');
  });

  it('marks the right side', () => {
    withLayout({ sidebar: { side: 'right' } });
    expect(html()).toMatch(/class="sidebar[^"]*\bright\b/);
  });
});
