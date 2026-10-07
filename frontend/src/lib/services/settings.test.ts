import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Settings } from './settings';

const bindings = {
  GetSettings: vi.fn(),
  SaveSettings: vi.fn(),
  ProposeLibraryPath: vi.fn(),
  SetupLibrary: vi.fn(),
  SaveConsent: vi.fn(),
};

const app = {
  SelectFolder: vi.fn(),
  OpenFolder: vi.fn(),
  OpenGameFolder: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/settings', () => ({ Service: bindings }));
vi.mock('../../../bindings/typhon/internal/app', () => ({ Service: app }));

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./settings');
}

function memoryStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  return {
    getItem: vi.fn((key: string) => data.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => void data.set(key, value)),
    data,
  };
}

beforeEach(() => {
  vi.resetAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('settings in a browser preview', () => {
  it('starts from the defaults when nothing was saved', async () => {
    vi.stubGlobal('localStorage', memoryStorage());
    const settings = await load(false);

    const loaded = await settings.getSettings();

    expect(loaded).toMatchObject({ theme: 'dark', language: 'system', maxActiveDownloads: 2, minimizeToTray: true });
  });

  it('lays what was saved over the defaults, so a newly added setting still has its default', async () => {
    vi.stubGlobal('localStorage', memoryStorage({ 'typhon.settings': JSON.stringify({ theme: 'light', maxActiveDownloads: 5 }) }));
    const settings = await load(false);

    const loaded = await settings.getSettings();

    expect(loaded.theme).toBe('light');
    expect(loaded.maxActiveDownloads).toBe(5);
    expect(loaded.animationsEnabled).toBe(true);
    expect(loaded.language).toBe('system');
  });

  it('falls back to the defaults for a saved value that cannot be read', async () => {
    vi.stubGlobal('localStorage', memoryStorage({ 'typhon.settings': '{not json' }));
    const settings = await load(false);

    await expect(settings.getSettings()).resolves.toMatchObject({ theme: 'dark', maxActiveDownloads: 2 });
  });

  it('does not hand out one shared object that a caller could change for everyone', async () => {
    vi.stubGlobal('localStorage', memoryStorage());
    const settings = await load(false);

    const first = await settings.getSettings();
    first.theme = 'changed';
    const second = await settings.getSettings();

    expect(second.theme).toBe('dark');
  });

  it('keeps what is saved and reads it back', async () => {
    const storage = memoryStorage();
    vi.stubGlobal('localStorage', storage);
    const settings = await load(false);
    const current = await settings.getSettings();

    await settings.saveSettings({ ...current, theme: 'light', uiScale: 1.25 });
    const again = await settings.getSettings();

    expect(again).toMatchObject({ theme: 'light', uiScale: 1.25 });
    expect(bindings.SaveSettings).not.toHaveBeenCalled();
  });

  it('refuses the actions that need the desktop app', async () => {
    const settings = await load(false);

    await expect(settings.openFolder('C:\\')).rejects.toThrow('unavailable in browser');
    await expect(settings.proposeLibraryPath('C:\\')).rejects.toThrow('unavailable in browser');
    await expect(settings.setupLibrary('C:\\')).rejects.toThrow('unavailable in browser');
    await expect(settings.saveConsent(true, true)).rejects.toThrow('unavailable in browser');
    await expect(settings.openGameFolder('C:\\g', 'g.exe')).rejects.toThrow('unavailable in browser');
    await expect(settings.selectFolder('Pick')).resolves.toBe('');
  });
});

describe('settings in the desktop app', () => {
  it('reads what the backend holds', async () => {
    bindings.GetSettings.mockResolvedValue({ theme: 'light' });
    const settings = await load(true);

    await expect(settings.getSettings()).resolves.toEqual({ theme: 'light' });
  });

  it('sends the whole object to the backend and lets a refusal reach the caller', async () => {
    bindings.SaveSettings.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('typhon:settings.network_mode_invalid: x'));
    const settings = await load(true);
    const current = { theme: 'dark', proxyPort: 1080 } as unknown as Settings;

    await settings.saveSettings(current);
    expect(bindings.SaveSettings).toHaveBeenCalledWith(current);

    await expect(settings.saveSettings(current)).rejects.toThrow('network_mode_invalid');
  });

  it('passes the library questions through to the backend', async () => {
    bindings.ProposeLibraryPath.mockResolvedValue('D:\\TyphonLibrary');
    bindings.SetupLibrary.mockResolvedValue({ libraryPath: 'D:\\TyphonLibrary' });
    bindings.SaveConsent.mockResolvedValue({ telemetryConsentVersion: 2 });
    const settings = await load(true);

    await expect(settings.proposeLibraryPath('D:\\')).resolves.toBe('D:\\TyphonLibrary');
    await expect(settings.setupLibrary('D:\\')).resolves.toEqual({ libraryPath: 'D:\\TyphonLibrary' });
    await expect(settings.saveConsent(false, true)).resolves.toEqual({ telemetryConsentVersion: 2 });
    expect(bindings.SaveConsent).toHaveBeenCalledWith(false, true);
  });

  it('opens folders through the app service', async () => {
    app.SelectFolder.mockResolvedValue('E:\\Pick');
    const settings = await load(true);

    await expect(settings.selectFolder('Pick')).resolves.toBe('E:\\Pick');
    await settings.openFolder('E:\\Pick');
    await settings.openGameFolder('E:\\Pick', 'game.exe');

    expect(app.OpenFolder).toHaveBeenCalledWith('E:\\Pick');
    expect(app.OpenGameFolder).toHaveBeenCalledWith('E:\\Pick', 'game.exe');
  });
});
