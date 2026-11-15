import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { maxActiveDownloadOptions, saveSettingsPatch, type Settings } from '../../lib/services/settings';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

function stored(patch: Partial<Settings> = {}): Settings {
  return { language: 'system', uiScale: 1, animationsEnabled: true, maxActiveDownloads: 2, downloadRateLimit: 0, downloadSchedule: [], ...patch } as Settings;
}

const header = () => document.querySelector('header.page-header') as HTMLElement;

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  vi.mocked(saveSettingsPatch).mockReset();
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe('Downloads header: how many download at once', () => {
  const trigger = () => within(header()).getByRole('button', { name: msg('transfers.downloadsConcurrency', { value: '4' }) });

  it('offers every number Go accepts, 1 to 10, from the list Settings reads too', () => {
    expect(maxActiveDownloadOptions.map((o) => o.id)).toEqual(['1', '2', '3', '4', '5', '6', '7', '8', '9', '10']);
  });

  it('marks the stored value, a 4 included', async () => {
    settings.set(stored({ maxActiveDownloads: 4 }));
    const { user } = renderDom(Downloads);

    await user.click(trigger());
    const options = screen.getAllByRole('menuitemradio');
    expect(options.map((o) => o.textContent?.trim())).toEqual(['1', '2', '3', '4', '5', '6', '7', '8', '9', '10']);
    expect(options.filter((o) => o.getAttribute('aria-checked') === 'true').map((o) => o.textContent?.trim())).toEqual(['4']);
  });

  it('a refused save puts the stored value back and says why by its code', async () => {
    settings.set(stored({ maxActiveDownloads: 4 }));
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('typhon:settings.library_not_writable: denied'));
    const { user } = renderDom(Downloads);

    await user.click(trigger());
    await user.click(screen.getByRole('menuitemradio', { name: '6' }));

    await waitFor(() => expect(get(settings)?.maxActiveDownloads).toBe(4));
    expect(trigger()).toBeTruthy();
    expect(get(toasts).map((t) => t.message)).toEqual([
      msg('downloads.page.concurrencySaveFailed', { reason: msg('settings.library_not_writable') }),
    ]);
  });

  it('a failed save without a code says the plain failure', async () => {
    settings.set(stored({ maxActiveDownloads: 4 }));
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('disk gone'));
    const { user } = renderDom(Downloads);

    await user.click(trigger());
    await user.click(screen.getByRole('menuitemradio', { name: '6' }));

    await waitFor(() => expect(get(toasts).map((t) => t.message)).toEqual([msg('downloads.page.concurrencySaveFailedPlain')]));
    expect(get(settings)?.maxActiveDownloads).toBe(4);
  });
});
