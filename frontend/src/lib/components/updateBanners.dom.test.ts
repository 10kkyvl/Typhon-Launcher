import { screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { requestDismiss, requestDownload, retryFailed, selfUpdateDownloading, selfUpdateStatus } from '../stores/selfupdate';
import { renderDom } from '../testing/dom';
import StableOffer from './StableOffer.svelte';
import UpdateBanner from './UpdateBanner.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/selfupdate')>()),
  retryFailed: vi.fn(),
  requestDownload: vi.fn(),
  requestDismiss: vi.fn(),
}));

afterEach(() => {
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  selfUpdateDownloading.set(false);
  vi.mocked(retryFailed).mockReset();
  vi.mocked(requestDownload).mockReset();
  vi.mocked(requestDismiss).mockReset();
  document.body.replaceChildren();
});

const AVAILABLE = { state: 'available', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;
const READY = { state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;

describe('UpdateBanner', () => {
  it('keeps a failed check and its focused Retry on screen while the retry runs', async () => {
    let finish = () => {};
    vi.mocked(retryFailed).mockReturnValue(new Promise<void>((done) => (finish = done)));
    selfUpdateStatus.set({ state: 'failed', currentVersion: '1.4.0', error: 'offline' });
    const { user } = renderDom(UpdateBanner);
    const notice = document.querySelector('.notice[data-tone="danger"]') as HTMLElement;
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

  it('shows a downloaded update in a success notice with its two actions', () => {
    selfUpdateStatus.set(READY);
    renderDom(UpdateBanner);
    const notice = screen.getByText(msg('ui.launcherRestartNewVersion')).closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('success');
    expect(notice.textContent).toContain(msg('ui.updateReadyMessage', { version: '1.5.0' }));
    expect(within(notice).getAllByRole('button').map((button) => button.textContent?.trim())).toEqual([
      msg('ui.restartAndUpdate'),
      msg('ui.notNow'),
    ]);
  });
});

describe('UpdateBanner: what it announces', () => {
  it('reads a state that was there when the page opened with the page, without announcing it', () => {
    for (const state of [AVAILABLE, READY, { state: 'failed', currentVersion: '1.4.0', error: 'offline' } as const]) {
      selfUpdateStatus.set(state);
      const { unmount } = renderDom(UpdateBanner);
      // A live region that says nothing yet (where a failed retry would be said again) may stand by.
      const heard = [...screen.queryAllByRole('status'), ...screen.queryAllByRole('alert')].filter((region) => region.textContent?.trim());
      expect(heard, state.state).toEqual([]);
      unmount();
    }
  });

  it('announces the answer to a check made while the page is open', async () => {
    renderDom(UpdateBanner);
    selfUpdateStatus.set(AVAILABLE);
    await vi.waitFor(() => expect(screen.getByRole('status').textContent).toContain(msg('ui.versionAvailable', { version: '1.5.0' })));
  });

  it('announces a download that has just finished', async () => {
    selfUpdateStatus.set({ ...AVAILABLE, state: 'downloading' });
    renderDom(UpdateBanner);
    selfUpdateStatus.set(READY);
    await vi.waitFor(() => expect(screen.getByRole('status').textContent).toContain(msg('ui.updateReadyMessage', { version: '1.5.0' })));
  });
});

describe('UpdateBanner: focus when a pressed button gives way', () => {
  it('Download moves focus to the download\'s Cancel', async () => {
    vi.mocked(requestDownload).mockImplementation(() => {
      selfUpdateDownloading.set(true);
      return new Promise<void>(() => {});
    });
    selfUpdateStatus.set(AVAILABLE);
    const { user } = renderDom(UpdateBanner);
    screen.getByRole('button', { name: msg('ui.download') }).focus();

    await user.keyboard('{Enter}');

    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('ui.cancelVerb') }));
  });

  it('"Not now" moves focus on to what follows the banner once it goes', async () => {
    vi.mocked(requestDismiss).mockImplementation(async () => selfUpdateStatus.set({ state: 'idle', currentVersion: '1.4.0' }));
    selfUpdateStatus.set(READY);
    const { user } = renderDom(UpdateBanner);
    const after = document.createElement('button');
    after.textContent = 'Check for updates';
    document.body.append(after);
    screen.getByRole('button', { name: msg('ui.notNow') }).focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(document.activeElement).toBe(after));
  });

  it('a Retry that works moves focus to the first control of what replaced the failure', async () => {
    vi.mocked(retryFailed).mockImplementation(async () => selfUpdateStatus.set(AVAILABLE));
    selfUpdateStatus.set({ state: 'failed', currentVersion: '1.4.0', error: 'offline' });
    const { user } = renderDom(UpdateBanner);

    await user.click(screen.getByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('ui.download') })));
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
