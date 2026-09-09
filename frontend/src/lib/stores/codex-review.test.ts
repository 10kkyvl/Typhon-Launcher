import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Settings } from '../services/settings';

globalThis.document = {
  documentElement: {
    style: { setProperty: () => {} },
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

it('preserves the latest edit of the same setting when an earlier save fails', async () => {
 const first = deferred<void>();
 vi.mocked(saveSettings).mockImplementationOnce(() => first.promise as never);
 const a = updateSettings({uiScale: 1.1});
 await Promise.resolve();
 const b = updateSettings({uiScale: 1.2});
 first.reject(new Error('disk full'));
 await Promise.all([a,b]);
 expect(get(settings)!.uiScale).toBe(1.2);
});
