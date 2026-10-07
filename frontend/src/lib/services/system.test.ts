import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { AppInfo } from './system';

const bindings = {
  GetAppInfo: vi.fn(),
  GetStorageInfo: vi.fn(),
  GetStorageInfoFor: vi.fn(),
  ExportLogs: vi.fn(),
  SetUILanguage: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/app', () => ({ Service: bindings }));

function info(patch: Partial<AppInfo> = {}): AppInfo {
  return { version: '0.5.2', platform: 'windows', arch: 'amd64', devMock: false, ...patch };
}

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./system');
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('elevationSupported', () => {
  it('is true on Windows and in the mocked build, false elsewhere', async () => {
    const system = await load(true);

    expect(system.elevationSupported(info({ platform: 'windows' }))).toBe(true);
    expect(system.elevationSupported(info({ platform: 'darwin', devMock: true }))).toBe(true);
    expect(system.elevationSupported(info({ platform: 'darwin' }))).toBe(false);
    expect(system.elevationSupported(info({ platform: 'linux' }))).toBe(false);
  });
});

describe('appInfo', () => {
  it('asks the backend once however many callers there are', async () => {
    bindings.GetAppInfo.mockResolvedValue(info());
    const system = await load(true);

    const [first, second] = await Promise.all([system.appInfo(), system.appInfo()]);
    await system.appInfo();

    expect(first).toBe(second);
    expect(bindings.GetAppInfo).toHaveBeenCalledTimes(1);
  });

  it('does not pin every later caller to a one-off failure', async () => {
    bindings.GetAppInfo.mockRejectedValueOnce(new Error('bridge not ready')).mockResolvedValueOnce(info({ version: '0.5.3' }));
    const system = await load(true);

    await expect(system.appInfo()).rejects.toThrow('bridge not ready');

    await expect(system.appInfo()).resolves.toMatchObject({ version: '0.5.3' });
    expect(bindings.GetAppInfo).toHaveBeenCalledTimes(2);
  });

  it('answers a browser preview without a backend', async () => {
    const system = await load(false);

    await expect(system.appInfo()).resolves.toMatchObject({ platform: 'browser', devMock: false });
    expect(bindings.GetAppInfo).not.toHaveBeenCalled();
  });
});

describe('storage and logs', () => {
  it('reads the storage of a given path from the backend', async () => {
    bindings.GetStorageInfoFor.mockResolvedValue({ path: 'E:\\Games' });
    const system = await load(true);

    await expect(system.getStorageInfoFor('E:\\Games')).resolves.toEqual({ path: 'E:\\Games' });
    expect(bindings.GetStorageInfoFor).toHaveBeenCalledWith('E:\\Games');
  });

  it('keeps the asked path in the browser preview', async () => {
    const system = await load(false);

    await expect(system.getStorageInfoFor('E:\\Games')).resolves.toMatchObject({ path: 'E:\\Games' });
  });

  it('refuses to export logs without a backend instead of inventing a bundle', async () => {
    const system = await load(false);

    await expect(system.exportLogs()).rejects.toThrow('unavailable in browser');
  });

  it('hands the exported bundle over untouched', async () => {
    const bundle = { path: 'C:\\logs.zip', name: 'logs.zip', dir: 'C:\\', sizeBytes: 10 };
    bindings.ExportLogs.mockResolvedValue(bundle);
    const system = await load(true);

    await expect(system.exportLogs()).resolves.toEqual(bundle);
  });
});

describe('native language sync', () => {
  it('does nothing in a browser preview', async () => {
    const system = await load(false);

    const stop = system.initNativeLanguage();
    stop();

    expect(bindings.SetUILanguage).not.toHaveBeenCalled();
  });

  it('tells the backend the language now and on every change, in order', async () => {
    const order: string[] = [];
    bindings.SetUILanguage.mockImplementation(async (language: string) => {
      order.push(language);
    });
    const system = await load(true);
    const { locale } = await import('../i18n/locale');
    locale.set('ru');

    const stop = system.initNativeLanguage();
    locale.set('en');
    locale.set('ru');
    await new Promise((resolve) => setTimeout(resolve, 0));
    stop();

    expect(order).toEqual(['ru', 'en', 'ru']);
    expect(get(locale)).toBe('ru');
  });

  it('keeps syncing after one call fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const seen: string[] = [];
    bindings.SetUILanguage.mockImplementation(async (language: string) => {
      seen.push(language);
      if (seen.length === 1) throw new Error('native side not ready');
    });
    const system = await load(true);
    const { locale } = await import('../i18n/locale');
    locale.set('ru');

    const stop = system.initNativeLanguage();
    locale.set('en');
    await new Promise((resolve) => setTimeout(resolve, 0));
    stop();

    expect(seen).toEqual(['ru', 'en']);
    expect(warn).toHaveBeenCalledTimes(1);
  });
});
