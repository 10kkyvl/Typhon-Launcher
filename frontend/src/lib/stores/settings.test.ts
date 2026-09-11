import { msg } from '../i18n';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Settings } from '../services/settings';

globalThis.document = {
  documentElement: {
    style: { setProperty: () => {}, removeProperty: () => {} },
    classList: { toggle: () => {} },
  },
} as unknown as Document;

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettings: vi.fn(),
  saveConsent: vi.fn(),
  setupLibrary: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));

const { getSettings, saveSettings } = await import('../services/settings');
const { settings, initSettings, updateSettings } = await import('./settings');

function makeSettings(): Settings {
  return {
    uiScale: 1,
    animationsEnabled: true,
    sourcesNoticeAccepted: true,
    anonymousUsageStats: false,
    anonymousDiagnostics: false,
    telemetryConsentVersion: 2,
  } as Settings;
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(async () => {
  vi.mocked(getSettings).mockResolvedValue(makeSettings());
  vi.mocked(saveSettings).mockReset();
  vi.mocked(saveSettings).mockResolvedValue(undefined as never);
  await initSettings();
});

describe('updateSettings', () => {
  it('does not undo a field another call already saved', async () => {
    const first = deferred<void>();
    vi.mocked(saveSettings)
      .mockImplementationOnce(() => first.promise as never)
      .mockImplementationOnce(() => Promise.resolve(undefined as never));

    const failing = updateSettings({ anonymousUsageStats: true });
    const succeeding = updateSettings({ anonymousDiagnostics: true });
    first.reject(new Error('disk full'));
    await Promise.allSettled([failing, succeeding]);

    const final = get(settings)!;
    expect(final.anonymousDiagnostics).toBe(true);
    expect(final.anonymousUsageStats).toBe(false);
  });

  it('does not let a second click overtake the first in flight', async () => {
    const first = deferred<void>();
    const sent: boolean[] = [];
    vi.mocked(saveSettings)
      .mockImplementationOnce((s: Settings) => {
        sent.push(s.anonymousDiagnostics);
        return first.promise as never;
      })
      .mockImplementationOnce(async (s: Settings) => {
        sent.push(s.anonymousDiagnostics);
      });

    const a = updateSettings({ anonymousDiagnostics: true });
    const b = updateSettings({ anonymousDiagnostics: false });
    await Promise.resolve();
    expect(sent).toHaveLength(1);

    first.resolve();
    await Promise.all([a, b]);
    expect(sent).toHaveLength(2);
    expect(sent.at(-1)).toBe(false);
    expect(get(settings)!.anonymousDiagnostics).toBe(false);
  });
});

it('restores the last confirmed value when two successive saves fail', async () => {
 const first = deferred<void>();
 vi.mocked(saveSettings).mockImplementationOnce(() => first.promise as never).mockRejectedValueOnce(new Error('still full'));
 const a=updateSettings({uiScale:1.1});await Promise.resolve();
 const b=updateSettings({uiScale:1.2});first.reject(new Error('full'));
 await Promise.all([a,b]);expect(get(settings)!.uiScale).toBe(1);
});

it('rolls back personal accent and icon after a failed save and shows a translated failure', async () => {
  settings.set({ ...makeSettings(), accentColor: '#6673F2', tintLogo: false });
  const write = deferred<void>();
  vi.mocked(saveSettings).mockImplementationOnce(() => write.promise);
  const pending = updateSettings({ accentColor: '#FFFF00', tintLogo: true });
  expect(get(settings)?.accentColor).toBe('#FFFF00');
  write.reject(new Error('disk full'));
  await pending;
  expect(get(settings)?.accentColor).toBe('#6673F2');
  expect(get(settings)?.tintLogo).toBe(false);
  const { toast } = await import('./toasts');
  expect(toast).toHaveBeenLastCalledWith(msg('state.settingsSaveFailed'), 'danger');
});
