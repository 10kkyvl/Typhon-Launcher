import { screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { retryFailed, selfUpdateStatus } from '../stores/selfupdate';
import { renderDom } from '../testing/dom';
import StableOffer from './StableOffer.svelte';
import UpdateBanner from './UpdateBanner.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/selfupdate')>()),
  retryFailed: vi.fn(),
}));

afterEach(() => {
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  vi.mocked(retryFailed).mockReset();
});

describe('UpdateBanner', () => {
  it('keeps a failed check and its focused Retry on screen while the retry runs', async () => {
    let finish = () => {};
    vi.mocked(retryFailed).mockReturnValue(new Promise<void>((done) => (finish = done)));
    selfUpdateStatus.set({ state: 'failed', currentVersion: '1.4.0', error: 'offline' });
    const { user } = renderDom(UpdateBanner);
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });

    await user.click(retry);

    expect(retryFailed).toHaveBeenCalledOnce();
    expect(notice.isConnected).toBe(true);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    finish();
    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
  });

  it('states an available version in a notice of the page with its Download', () => {
    selfUpdateStatus.set({ state: 'available', currentVersion: '1.4.0', availableVersion: '1.5.0' });
    renderDom(UpdateBanner);
    const text = screen.getByText(msg('ui.versionAvailable', { version: '1.5.0' }));
    const notice = text.closest('.notice') as HTMLElement;
    expect(notice).toBeTruthy();
    expect(text.closest('[role="status"], [role="alert"]')).toBeNull();
    expect(within(notice).getByRole('button', { name: msg('ui.download') })).toBeTruthy();
  });

  it('announces a downloaded update in a success notice with its two actions', () => {
    selfUpdateStatus.set({ state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0' });
    renderDom(UpdateBanner);
    const notice = screen.getByRole('status').closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('success');
    expect(notice.textContent).toContain(msg('ui.updateReadyMessage', { version: '1.5.0' }));
    expect(within(notice).getAllByRole('button').map((button) => button.textContent?.trim())).toEqual([
      msg('ui.restartAndUpdate'),
      msg('ui.notNow'),
    ]);
  });
});

describe('StableOffer', () => {
  it('offers the stable version in a notice of the page with its button', () => {
    selfUpdateStatus.set({ state: 'idle', currentVersion: '1.5.0-beta.2', stableOffer: '1.4.1' });
    renderDom(StableOffer);
    const text = screen.getByText(msg('ui.stableOfferText', { current: '1.5.0-beta.2', version: '1.4.1' }));
    const notice = text.closest('.notice') as HTMLElement;
    expect(notice).toBeTruthy();
    expect(text.closest('[role="status"], [role="alert"]')).toBeNull();
    expect(within(notice).getByRole('button', { name: msg('ui.stableOfferButton', { version: '1.4.1' }) })).toBeTruthy();
  });
});
