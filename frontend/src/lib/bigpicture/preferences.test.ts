import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Settings } from '../services/settings';

globalThis.document = {
  documentElement: {
    style: { setProperty: () => {}, removeProperty: () => {} },
    classList: { toggle: () => {} },
  },
} as unknown as Document;

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettingsPatch: vi.fn(),
  saveConsent: vi.fn(),
  setupLibrary: vi.fn(),
}));
vi.mock('../stores/toasts', () => ({ toast: vi.fn() }));

const { getSettings, saveSettingsPatch } = await import('../services/settings');
const { settings, initSettings } = await import('../stores/settings');
const { BIG_PICTURE_DOWNLOAD_LIMITS, bigPictureDownloadChoices, saveBigPicturePreference } = await import('./preferences');

function makeSettings(): Settings {
  return {
    uiScale: 1,
    animationsEnabled: true,
    launchOnStartup: false,
    minimizeToTray: true,
    maxActiveDownloads: 2,
    sourcesNoticeAccepted: true,
  } as Settings;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

beforeEach(async () => {
  vi.mocked(getSettings).mockResolvedValue(makeSettings());
  vi.mocked(saveSettingsPatch).mockReset();
  vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...makeSettings(), ...patch }));
  await initSettings();
});

describe('Big Picture concurrency choices', () => {
  it('are the four when the stored value is one of them', () => {
    for (const stored of BIG_PICTURE_DOWNLOAD_LIMITS) {
      expect(bigPictureDownloadChoices(stored)).toEqual([1, 2, 3, 5]);
    }
  });

  it('add a value another screen stored, in its place in order, and keep the four', () => {
    expect(bigPictureDownloadChoices(4)).toEqual([1, 2, 3, 4, 5]);
    expect(bigPictureDownloadChoices(6)).toEqual([1, 2, 3, 5, 6]);
    expect(bigPictureDownloadChoices(10)).toEqual([1, 2, 3, 5, 10]);
  });

  it('are the four while nothing usable is stored', () => {
    expect(bigPictureDownloadChoices(undefined)).toEqual([1, 2, 3, 5]);
    expect(bigPictureDownloadChoices(0)).toEqual([1, 2, 3, 5]);
    expect(bigPictureDownloadChoices(2.5)).toEqual([1, 2, 3, 5]);
  });
});

describe('Big Picture preferences', () => {
  it('reports success only after the persisted setting has been written', async () => {
    expect(await saveBigPicturePreference({ animationsEnabled: false })).toBe('saved');
    expect(vi.mocked(saveSettingsPatch)).toHaveBeenCalledOnce();
    expect(vi.mocked(saveSettingsPatch).mock.calls[0][0]).toEqual({ animationsEnabled: false });
    expect(get(settings)?.animationsEnabled).toBe(false);
  });

  it('shows a failed result and restores the previous value when storage rejects', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('disk full'));

    expect(await saveBigPicturePreference({ animationsEnabled: false })).toBe('failed');
    expect(vi.mocked(saveSettingsPatch)).toHaveBeenCalledOnce();
    expect(get(settings)?.animationsEnabled).toBe(true);
  });

  it('serializes concurrent patches and does not include a later change in a failed write', async () => {
    const firstWrite = deferred<Settings>();
    vi.mocked(saveSettingsPatch)
      .mockImplementationOnce(() => firstWrite.promise)
      .mockImplementationOnce(async (patch) => ({ ...makeSettings(), ...patch }));

    const first = saveBigPicturePreference({ animationsEnabled: false });
    const second = saveBigPicturePreference({ launchOnStartup: true });
    await Promise.resolve();

    expect(vi.mocked(saveSettingsPatch)).toHaveBeenCalledOnce();
    expect(vi.mocked(saveSettingsPatch).mock.calls[0][0]).toEqual({ animationsEnabled: false });
    firstWrite.reject(new Error('disk full'));

    expect(await first).toBe('failed');
    expect(await second).toBe('saved');
    expect(vi.mocked(saveSettingsPatch)).toHaveBeenCalledTimes(2);
    expect(vi.mocked(saveSettingsPatch).mock.calls[1][0]).toEqual({ launchOnStartup: true });
    expect(get(settings)).toMatchObject({ animationsEnabled: true, launchOnStartup: true });
  });
});
