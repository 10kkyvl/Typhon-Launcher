import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { toasts } from '../stores/toasts';
import { renderDom } from '../testing/dom';
import BetaBadge from './BetaBadge.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/system')>()),
  appInfo: vi.fn(async () => ({ channel: 'beta', version: '1.5.0-beta.2' })),
}));

afterEach(() => {
  toasts.set([]);
});

describe('BetaBadge in a DOM', () => {
  it('names the version in the badge itself, without a native title repeating it', async () => {
    const view = renderDom(BetaBadge);
    const badge = await view.findByRole('button', { name: msg('ui.betaBadgeTitle', { version: '1.5.0-beta.2' }) });

    expect(badge.textContent).toBe(msg('ui.betaBadge'));
    expect(badge.hasAttribute('title')).toBe(false);
    expect(badge.getAttribute('aria-haspopup')).toBe('menu');
  });

  it('opens the report menu, runs "Report a problem" and gives focus back to the badge', async () => {
    const view = renderDom(BetaBadge);
    const badge = await view.findByRole('button', { name: msg('ui.betaBadgeTitle', { version: '1.5.0-beta.2' }) });

    await view.user.click(badge);
    await view.user.click(view.getByRole('menuitem', { name: msg('ui.betaReport') }));

    expect(view.queryByRole('menu')).toBeNull();
    expect(document.activeElement).toBe(badge);
    expect(get(toasts).map((t) => t.message)).toEqual([msg('settings.aboutLogsDesktopOnly')]);
  });
});
