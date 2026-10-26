import { render } from 'svelte/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));

import Topbar from './Topbar.svelte';
import { activeTheme } from '../stores/theme';
import type { Theme } from '../services/theme';

const withLayout = (layout: Theme['layout']) => activeTheme.set({ id: 'x', layout } as unknown as Theme);
const html = () => render(Topbar, { props: {} } as never).body;
const ids = (body: string) => [...body.matchAll(/data-part="topbar-action" data-id="([a-z]+)"/g)].map((m) => m[1]);

afterEach(() => activeTheme.set(null));

describe('Topbar layout', () => {
  it('renders every action in the default order and keeps the window controls', () => {
    activeTheme.set(null);
    const body = html();
    expect(ids(body)).toEqual(['bigpicture', 'notifications', 'friends']);
    for (const part of ['topbar', 'topbar-history', 'topbar-search', 'topbar-actions', 'window-controls']) {
      expect(body).toContain(`data-part="${part}"`);
    }
    expect(body).not.toContain('centered');
  });

  it('orders and hides actions but keeps the window controls', () => {
    withLayout({ topbar: { order: ['friends', 'bigpicture'], hidden: ['notifications'] } });
    const body = html();
    expect(ids(body)).toEqual(['friends', 'bigpicture']);
    expect(body).toContain('data-part="window-controls"');
  });

  it('centres the search', () => {
    withLayout({ topbar: { search: 'center' } });
    expect(html()).toContain('centered');
  });
});
