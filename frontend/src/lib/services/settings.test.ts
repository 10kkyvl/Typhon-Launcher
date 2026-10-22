import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  GetSettings: vi.fn(),
  SaveSettingsPatch: vi.fn(),
  ProposeLibraryPath: vi.fn(),
  SetupLibrary: vi.fn(),
  SaveConsent: vi.fn(),
  LibraryFolders: vi.fn(),
  AddLibraryFolder: vi.fn(),
  RemoveLibraryFolder: vi.fn(),
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

    const saved = await settings.saveSettingsPatch({ theme: 'light', uiScale: 1.25 });
    const again = await settings.getSettings();

    expect(again).toMatchObject({ ...current, theme: 'light', uiScale: 1.25 });
    expect(saved).toEqual(again);
    expect(bindings.SaveSettingsPatch).not.toHaveBeenCalled();
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

    await expect(settings.getSettings()).resolves.toEqual({ theme: 'light', downloadSchedule: [], extraLibraryPaths: [] });
  });

  it('turns the nil extra folder list into an empty one and keeps a real one', async () => {
    bindings.GetSettings.mockResolvedValueOnce({ extraLibraryPaths: null }).mockResolvedValueOnce({ extraLibraryPaths: ['E:\\Games'] });
    const settings = await load(true);

    await expect(settings.getSettings()).resolves.toMatchObject({ extraLibraryPaths: [] });
    await expect(settings.getSettings()).resolves.toMatchObject({ extraLibraryPaths: ['E:\\Games'] });
  });

  it('turns the nil schedule Go sends into an empty list and keeps a real one', async () => {
    const window = { days: [1, 2], from: '23:00', to: '07:00' };
    bindings.GetSettings.mockResolvedValueOnce({ downloadSchedule: null }).mockResolvedValueOnce({ downloadSchedule: [window] });
    const settings = await load(true);

    await expect(settings.getSettings()).resolves.toEqual({ downloadSchedule: [], extraLibraryPaths: [] });
    await expect(settings.getSettings()).resolves.toEqual({ downloadSchedule: [window], extraLibraryPaths: [] });
  });

  it('sends exactly the patch to the backend, returns what it stored and lets a refusal reach the caller', async () => {
    const stored = { theme: 'dark', proxyPort: 1080, uiScale: 1 };
    bindings.SaveSettingsPatch.mockResolvedValueOnce(stored).mockRejectedValueOnce(new Error('typhon:settings.network_mode_invalid: x'));
    const settings = await load(true);
    const patch = { proxyPort: 1080 };

    await expect(settings.saveSettingsPatch(patch)).resolves.toEqual({ ...stored, downloadSchedule: [], extraLibraryPaths: [] });
    expect(bindings.SaveSettingsPatch).toHaveBeenCalledWith({ proxyPort: 1080 });

    await expect(settings.saveSettingsPatch(patch)).rejects.toThrow('network_mode_invalid');
  });

  it('passes the library questions through to the backend', async () => {
    bindings.ProposeLibraryPath.mockResolvedValue('D:\\TyphonLibrary');
    bindings.SetupLibrary.mockResolvedValue({ libraryPath: 'D:\\TyphonLibrary' });
    bindings.SaveConsent.mockResolvedValue({ telemetryConsentVersion: 2 });
    const settings = await load(true);

    await expect(settings.proposeLibraryPath('D:\\')).resolves.toBe('D:\\TyphonLibrary');
    await expect(settings.setupLibrary('D:\\')).resolves.toEqual({
      libraryPath: 'D:\\TyphonLibrary',
      downloadSchedule: [],
      extraLibraryPaths: [],
    });
    await expect(settings.saveConsent(false, true)).resolves.toEqual({
      telemetryConsentVersion: 2,
      downloadSchedule: [],
      extraLibraryPaths: [],
    });
    expect(bindings.SaveConsent).toHaveBeenCalledWith(false, true);
  });

  it('lists, adds and removes game folders through the backend and lets a refusal reach the caller', async () => {
    const folder = { path: 'D:\\Games', primary: true, freeBytes: 1, totalBytes: 2, available: true };
    bindings.LibraryFolders.mockResolvedValueOnce([folder]).mockResolvedValueOnce(null).mockRejectedValueOnce(new Error('disk'));
    bindings.AddLibraryFolder.mockResolvedValueOnce({ extraLibraryPaths: ['E:\\Games'] }).mockRejectedValueOnce(
      new Error('typhon:settings.extra_library_nested: x'),
    );
    bindings.RemoveLibraryFolder.mockResolvedValueOnce({ extraLibraryPaths: null });
    const settings = await load(true);

    await expect(settings.libraryFolders()).resolves.toEqual([folder]);
    await expect(settings.libraryFolders()).resolves.toEqual([]);
    await expect(settings.libraryFolders()).rejects.toThrow('disk');
    await expect(settings.addLibraryFolder('E:\\Games')).resolves.toMatchObject({ extraLibraryPaths: ['E:\\Games'] });
    expect(bindings.AddLibraryFolder).toHaveBeenCalledWith('E:\\Games');
    await expect(settings.addLibraryFolder('E:\\Inside')).rejects.toThrow('extra_library_nested');
    await expect(settings.removeLibraryFolder('E:\\Games')).resolves.toMatchObject({ extraLibraryPaths: [] });
    expect(bindings.RemoveLibraryFolder).toHaveBeenCalledWith('E:\\Games');
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
