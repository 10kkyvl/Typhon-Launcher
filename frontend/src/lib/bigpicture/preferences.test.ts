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
  saveSettings: vi.fn(),
  saveConsent: vi.fn(),
  setupLibrary: vi.fn(),
}));
vi.mock('../stores/toasts', () => ({ toast: vi.fn() }));

const { getSettings, saveSettings } = await import('../services/settings');
const { settings, initSettings } = await import('../stores/settings');
const { saveBigPicturePreference } = await import('./preferences');

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
  vi.mocked(saveSettings).mockReset();
  vi.mocked(saveSettings).mockResolvedValue(undefined as never);
  await initSettings();
});

describe('Big Picture preferences', () => {
  it('reports success only after the persisted setting has been written', async () => {
    expect(await saveBigPicturePreference({ animationsEnabled: false })).toBe('saved');
    expect(vi.mocked(saveSettings)).toHaveBeenCalledOnce();
    expect(vi.mocked(saveSettings).mock.calls[0][0].animationsEnabled).toBe(false);
    expect(get(settings)?.animationsEnabled).toBe(false);
  });

  it('shows a failed result and restores the previous value when storage rejects', async () => {
    vi.mocked(saveSettings).mockRejectedValueOnce(new Error('disk full'));

    expect(await saveBigPicturePreference({ animationsEnabled: false })).toBe('failed');
    expect(vi.mocked(saveSettings)).toHaveBeenCalledOnce();
    expect(get(settings)?.animationsEnabled).toBe(true);
  });

  it('serializes concurrent patches and does not include a later change in a failed write', async () => {
    const firstWrite = deferred<void>();
    vi.mocked(saveSettings)
      .mockImplementationOnce(() => firstWrite.promise)
      .mockResolvedValueOnce(undefined as never);

    const first = saveBigPicturePreference({ animationsEnabled: false });
    const second = saveBigPicturePreference({ launchOnStartup: true });
    await Promise.resolve();

    expect(vi.mocked(saveSettings)).toHaveBeenCalledOnce();
    expect(vi.mocked(saveSettings).mock.calls[0][0]).toMatchObject({ animationsEnabled: false, launchOnStartup: false });
    firstWrite.reject(new Error('disk full'));

    expect(await first).toBe('failed');
    expect(await second).toBe('saved');
    expect(vi.mocked(saveSettings)).toHaveBeenCalledTimes(2);
    expect(vi.mocked(saveSettings).mock.calls[1][0]).toMatchObject({ animationsEnabled: true, launchOnStartup: true });
    expect(get(settings)).toMatchObject({ animationsEnabled: true, launchOnStartup: true });
  });
});
