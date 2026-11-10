import { render } from 'svelte/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));

import Topbar from './Topbar.svelte';
import { activeTheme } from '../stores/theme';
import type { Theme } from '../services/theme';
import { msg } from '../i18n';

const withLayout = (layout: Theme['layout']) => activeTheme.set({ id: 'x', layout } as unknown as Theme);
const html = () => render(Topbar, { props: {} } as never).body;
const ids = (body: string) => [...body.matchAll(/data-part="topbar-action" data-id="([a-z]+)"/g)].map((m) => m[1]);

afterEach(() => activeTheme.set(null));

describe('Topbar layout', () => {
  it('renders every action in the default order', () => {
    activeTheme.set(null);
    const body = html();
    expect(ids(body)).toEqual(['bigpicture', 'notifications', 'friends']);
    for (const part of ['topbar', 'topbar-history', 'topbar-search', 'topbar-actions']) {
      expect(body).toContain(`data-part="${part}"`);
    }
    expect(body).not.toContain('centered');
  });

  it('orders and hides actions', () => {
    withLayout({ topbar: { order: ['friends', 'bigpicture'], hidden: ['notifications'] } });
    expect(ids(html())).toEqual(['friends', 'bigpicture']);
  });

  it('leaves the window buttons to the title bar and stays a drag region', () => {
    const body = html();
    expect(body).not.toContain('data-part="window-controls"');
    for (const label of [msg('ui.minimize'), msg('ui.maximize'), msg('common.close')]) {
      expect(body).not.toContain(`aria-label="${label}"`);
    }
    const header = /<header[^>]*data-part="topbar"[^>]*>/.exec(body)?.[0] ?? '';
    expect(header).toContain('--wails-draggable: drag');
    for (const part of ['topbar-history', 'topbar-search']) {
      expect(new RegExp(`class="[^"]*\\bno-drag\\b[^"]*"[^>]*data-part="${part}"`).test(body), part).toBe(true);
    }
  });

  it('centres the search', () => {
    withLayout({ topbar: { search: 'center' } });
    expect(html()).toContain('centered');
  });
});
