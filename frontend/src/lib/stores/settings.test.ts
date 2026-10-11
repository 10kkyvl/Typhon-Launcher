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

const listeners: ((event: { data: unknown }) => void)[] = [];
vi.mock('@wailsio/runtime', () => ({
  Events: { On: (_name: string, fn: (event: { data: unknown }) => void) => { listeners.push(fn); } },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettingsPatch: vi.fn(),
  saveConsent: vi.fn(),
  setupLibrary: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));

const { getSettings, saveSettingsPatch } = await import('../services/settings');
const { settings, initSettings, updateSettings, updateSettingsResult, updateSettingsReporting } = await import('./settings');

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

let stored: Settings;

const applyPatch = async (patch: Partial<Settings>): Promise<Settings> => {
  stored = { ...stored, ...patch };
  return { ...stored };
};

beforeEach(async () => {
  stored = makeSettings();
  vi.mocked(getSettings).mockImplementation(async () => ({ ...stored }));
  vi.mocked(saveSettingsPatch).mockReset();
  vi.mocked(saveSettingsPatch).mockImplementation(applyPatch);
  listeners.length = 0;
  await initSettings();
});

function emitUpdated(data: Settings) {
  for (const fn of listeners) fn({ data });
}

describe('updateSettings', () => {
  it('sends each queued patch with only its own key, never a stale copy of the other field', async () => {
    const first = deferred<Settings>();
    vi.mocked(saveSettingsPatch).mockImplementationOnce(() => first.promise);

    const a = updateSettings({ uiScale: 1.5 });
    const b = updateSettings({ animationsEnabled: false });
    await Promise.resolve();
    first.resolve(await applyPatch({ uiScale: 1.5 }));
    await Promise.all([a, b]);

    expect(vi.mocked(saveSettingsPatch).mock.calls).toEqual([[{ uiScale: 1.5 }], [{ animationsEnabled: false }]]);
    expect(stored).toMatchObject({ uiScale: 1.5, animationsEnabled: false });
  });

  it('keeps what the backend returned as the confirmed baseline for a later rollback', async () => {
    await updateSettings({ uiScale: 1.4 });
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('disk full'));

    await updateSettings({ uiScale: 1.9 });

    expect(get(settings)!.uiScale).toBe(1.4);
  });

  it('rolls back to the value the backend stored, not the one that was sent', async () => {
    const first = deferred<Settings>();
    vi.mocked(saveSettingsPatch)
      .mockImplementationOnce(() => first.promise)
      .mockRejectedValueOnce(new Error('disk full'));

    const normalized = updateSettings({ accentColor: '#abcdef' });
    const failing = updateSettings({ accentColor: '#000000' });
    first.resolve({ ...makeSettings(), accentColor: '#ABCDEF' });
    await Promise.all([normalized, failing]);

    expect(get(settings)!.accentColor).toBe('#ABCDEF');
  });

  it('reports each write result without persisting later optimistic patches early', async () => {
    const first = deferred<Settings>();
    vi.mocked(saveSettingsPatch).mockImplementationOnce(() => first.promise);

    const failing = updateSettingsResult({ uiScale: 1.2 });
    const succeeding = updateSettingsResult({ animationsEnabled: false });
    await Promise.resolve();
    expect(vi.mocked(saveSettingsPatch).mock.calls[0][0]).toEqual({ uiScale: 1.2 });

    first.reject(new Error('disk full'));
    expect(await failing).toBe(false);
    expect(await succeeding).toBe(true);
    expect(vi.mocked(saveSettingsPatch).mock.calls[1][0]).toEqual({ animationsEnabled: false });
    expect(get(settings)).toMatchObject({ uiScale: 1, animationsEnabled: false });
  });

  it('does not undo a field another call already saved', async () => {
    const first = deferred<Settings>();
    vi.mocked(saveSettingsPatch)
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(applyPatch);

    const failing = updateSettings({ anonymousUsageStats: true });
    const succeeding = updateSettings({ anonymousDiagnostics: true });
    first.reject(new Error('disk full'));
    await Promise.allSettled([failing, succeeding]);

    const final = get(settings)!;
    expect(final.anonymousDiagnostics).toBe(true);
    expect(final.anonymousUsageStats).toBe(false);
  });

  it('does not let a second click overtake the first in flight', async () => {
    const first = deferred<Settings>();
    const sent: (boolean | undefined)[] = [];
    vi.mocked(saveSettingsPatch)
      .mockImplementationOnce((patch) => {
        sent.push(patch.anonymousDiagnostics);
        return first.promise;
      })
      .mockImplementationOnce(async (patch) => {
        sent.push(patch.anonymousDiagnostics);
        return applyPatch(patch);
      });

    const a = updateSettings({ anonymousDiagnostics: true });
    const b = updateSettings({ anonymousDiagnostics: false });
    await Promise.resolve();
    expect(sent).toHaveLength(1);

    first.resolve(await applyPatch({ anonymousDiagnostics: true }));
    await Promise.all([a, b]);
    expect(sent).toHaveLength(2);
    expect(sent.at(-1)).toBe(false);
    expect(get(settings)!.anonymousDiagnostics).toBe(false);
  });
});

it('restores the last confirmed value when two successive saves fail', async () => {
 const first = deferred<Settings>();
 vi.mocked(saveSettingsPatch).mockImplementationOnce(() => first.promise).mockRejectedValueOnce(new Error('still full'));
 const a=updateSettings({uiScale:1.1});await Promise.resolve();
 const b=updateSettings({uiScale:1.2});first.reject(new Error('full'));
 await Promise.all([a,b]);expect(get(settings)!.uiScale).toBe(1);
});

it('rolls back personal accent and icon after a failed save and shows a translated failure', async () => {
  settings.set({ ...makeSettings(), accentColor: '#6673F2', tintLogo: false });
  const write = deferred<Settings>();
  vi.mocked(saveSettingsPatch).mockImplementationOnce(() => write.promise);
  const pending = updateSettings({ accentColor: '#FFFF00', tintLogo: true });
  expect(get(settings)?.accentColor).toBe('#FFFF00');
  write.reject(new Error('disk full'));
  await pending;
  expect(get(settings)?.accentColor).toBe('#6673F2');
  expect(get(settings)?.tintLogo).toBe(false);
  const { toast } = await import('./toasts');
  expect(toast).toHaveBeenLastCalledWith(msg('state.settingsSaveFailed'), 'danger');
});

describe('settings:updated while saves are pending', () => {
  it('rolls a failed save back to the value Go changed meanwhile, not the stale baseline', async () => {
    const write = deferred<Settings>();
    vi.mocked(saveSettingsPatch).mockImplementationOnce(() => write.promise);
    const pending = updateSettings({ uiScale: 1.5 });

    emitUpdated({ ...makeSettings(), uiScale: 1.25 });
    write.reject(new Error('disk full'));
    await pending;

    expect(get(settings)!.uiScale).toBe(1.25);
  });

  it('keeps the optimistic value of a queued patch when the event arrives', async () => {
    const write = deferred<Settings>();
    vi.mocked(saveSettingsPatch).mockImplementationOnce(() => write.promise);
    const pending = updateSettings({ animationsEnabled: false });

    emitUpdated({ ...makeSettings(), uiScale: 1.25 });
    const shown = get(settings);
    write.resolve(await applyPatch({ animationsEnabled: false }));
    await pending;

    expect(shown).toMatchObject({ animationsEnabled: false, uiScale: 1.25 });
  });

  it('applies the event as is once nothing is pending', async () => {
    await updateSettings({ uiScale: 1.5 });

    emitUpdated({ ...makeSettings(), uiScale: 1.25 });

    expect(get(settings)!.uiScale).toBe(1.25);
  });

  it('does not reapply a patch that has already been saved', async () => {
    const second = deferred<Settings>();
    vi.mocked(saveSettingsPatch).mockImplementationOnce(applyPatch).mockImplementationOnce(() => second.promise);
    const a = updateSettings({ uiScale: 1.5 });
    const b = updateSettings({ animationsEnabled: false });
    await a;

    emitUpdated({ ...makeSettings(), uiScale: 1.25, animationsEnabled: true });
    const shown = get(settings);
    second.resolve(await applyPatch({ animationsEnabled: false }));
    await b;

    expect(shown).toMatchObject({ uiScale: 1.25, animationsEnabled: false });
  });
});

describe('error callback', () => {
  it('rolls back even when the caller onError throws', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('disk full'));

    const ok = await updateSettingsReporting({ uiScale: 1.9 }, () => { throw new Error('handler broke'); });

    expect(ok).toBe(false);
    expect(get(settings)!.uiScale).toBe(1);
  });

  it('calls onError after the rollback', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('disk full'));
    let seen: number | undefined;

    await updateSettingsReporting({ uiScale: 1.9 }, () => { seen = get(settings)!.uiScale; });

    expect(seen).toBe(1);
  });
});
