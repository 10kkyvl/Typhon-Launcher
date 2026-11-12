import { render, screen, waitFor, within } from '@testing-library/svelte';
import type { Writable } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../i18n';
import { isOffline, retryBootstrap } from '../stores/user';
import { user } from '../testing/dom';
import OfflineBanner from './OfflineBanner.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', async (importOriginal) => {
  const { writable } = await import('svelte/store');
  return {
    ...(await importOriginal<typeof import('../stores/user')>()),
    isOffline: writable(false),
    retryBootstrap: vi.fn(async () => {}),
  };
});

const offline = isOffline as unknown as Writable<boolean>;

describe('the lost connection banner', () => {
  beforeEach(() => {
    locale.set('en');
  });

  it('is a warning notice with its two actions, and its close takes it away', async () => {
    const { container } = render(OfflineBanner);

    const notice = container.querySelector('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('warning');
    expect(screen.getByRole('status')).not.toBeNull();
    const buttons = within(notice).getAllByRole('button').map((button) => button.textContent?.trim());
    expect(buttons).toEqual(['Service status', 'Retry', '']);

    await user().click(within(notice).getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('status')).toBeNull();
  });

  // The region holds the message; Retry turning into "Checking connection…" is a change of a button,
  // not of the message, so it stays outside and is not read out with the message again.
  it('keeps the Retry button out of its live region while the connection is checked', async () => {
    let finish = () => {};
    vi.mocked(retryBootstrap).mockImplementation(() => new Promise<void>((resolve) => (finish = resolve)));
    render(OfflineBanner);
    const region = screen.getByRole('status');
    await waitFor(() => expect(region.textContent).toContain(msg('ui.offlineBannerText')));
    const message = region.textContent;

    await user().click(screen.getByRole('button', { name: 'Retry' }));
    expect(screen.getByRole('button', { name: 'Checking connection…' })).not.toBeNull();
    expect(region.textContent).toBe(message);
    expect(within(region).queryAllByRole('button')).toEqual([]);

    finish();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Retry' })).not.toBeNull());
    expect(region.textContent).toBe(message);
  });

  // Still offline after the check: the banner says so again, though its words are the same.
  it('says it is still offline when the check finds no connection', async () => {
    vi.mocked(retryBootstrap).mockImplementation(async () => offline.set(true));
    offline.set(true);
    render(OfflineBanner);
    const region = screen.getByRole('status');
    const before = region.firstElementChild;

    await user().click(screen.getByRole('button', { name: 'Retry' }));

    // The message comes into the live region again: new nodes, the same words.
    await waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent?.trim()).toBe(msg('ui.offlineBannerText'));
    offline.set(false);
  });
});
