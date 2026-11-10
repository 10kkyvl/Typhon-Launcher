import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AppInfo } from '../services/system';

const handlers = new Map<string, () => void>();
const off = vi.fn();
let maximised: Promise<boolean>;
let info: Promise<AppInfo>;
let wails = true;

const Types = {
  Common: {
    WindowMaximise: 'common:WindowMaximise',
    WindowUnMaximise: 'common:WindowUnMaximise',
    WindowRestore: 'common:WindowRestore',
    WindowFullscreen: 'common:WindowFullscreen',
    WindowUnFullscreen: 'common:WindowUnFullscreen',
  },
  Windows: {
    WindowMaximise: 'windows:WindowMaximise',
    WindowUnMaximise: 'windows:WindowUnMaximise',
    WindowRestore: 'windows:WindowRestore',
    WindowFullscreen: 'windows:WindowFullscreen',
    WindowUnFullscreen: 'windows:WindowUnFullscreen',
  },
  Mac: {
    WindowMaximise: 'mac:WindowMaximise',
    WindowUnMaximise: 'mac:WindowUnMaximise',
    WindowDidEnterFullScreen: 'mac:WindowDidEnterFullScreen',
    WindowDidExitFullScreen: 'mac:WindowDidExitFullScreen',
  },
};

const windowApi = {
  Minimise: vi.fn(async () => {}),
  ToggleMaximise: vi.fn(async () => {}),
  Close: vi.fn(async () => {}),
  IsMaximised: vi.fn(() => maximised),
};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    Types,
    On: vi.fn((name: string, cb: () => void) => {
      handlers.set(name, cb);
      return off;
    }),
  },
  Window: windowApi,
}));
vi.mock('../services/backend', () => ({
  get inWails() {
    return wails;
  },
}));
vi.mock('../services/system', () => ({ appInfo: vi.fn(() => info) }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));

async function load() {
  vi.resetModules();
  handlers.clear();
  const store = await import('./window');
  const { toast } = await import('./toasts');
  const { msg } = await import('../i18n');
  return { store, toast: vi.mocked(toast), msg };
}

function fire(name: string) {
  const handler = handlers.get(name);
  expect(handler, name).toBeDefined();
  handler?.();
}

const appInfoFor = (platform: string) => ({ platform }) as AppInfo;

beforeEach(() => {
  vi.clearAllMocks();
  wails = true;
  maximised = Promise.resolve(false);
  info = Promise.resolve(appInfoFor('windows'));
});

describe('window platform', () => {
  it.each(['windows', 'linux', 'darwin', 'browser'])('takes %s from the app info', async (platform) => {
    info = Promise.resolve(appInfoFor(platform));
    const { store } = await load();
    await store.initPlatform();
    expect(get(store.platform)).toBe(platform);
  });

  it('keeps the window buttons when the platform lookup fails', async () => {
    info = Promise.reject(new Error('binding down'));
    const { store } = await load();
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(store.initPlatform()).resolves.toBeUndefined();
    expect(get(store.windowControls)).toBe('buttons');
    expect(error).toHaveBeenCalled();
    error.mockRestore();
  });

  it('draws the buttons on every platform while macOS keeps the default', async () => {
    for (const platform of ['windows', 'linux', 'darwin', 'browser']) {
      info = Promise.resolve(appInfoFor(platform));
      const { store } = await load();
      await store.initPlatform();
      expect(get(store.windowControls), platform).toBe('buttons');
    }
  });

  it('leaves room for the native controls only on macOS when they are switched on', async () => {
    const { store } = await load();
    expect(store.windowControlsFor('darwin', true)).toBe('native');
    expect(store.windowControlsFor('windows', true)).toBe('buttons');
    expect(store.windowControlsFor('linux', true)).toBe('buttons');
    expect(store.windowControlsFor('browser', true)).toBe('buttons');
    expect(store.windowControlsFor('darwin', false)).toBe('buttons');
  });
});

describe('window maximised state', () => {
  it('reads the state once at startup', async () => {
    maximised = Promise.resolve(true);
    const { store } = await load();
    store.initWindowState();
    await vi.waitFor(() => expect(get(store.windowMaximised)).toBe(true));
    expect(windowApi.IsMaximised).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['windows:WindowMaximise', 'windows:WindowRestore'],
    ['windows:WindowMaximise', 'windows:WindowUnMaximise'],
    ['mac:WindowMaximise', 'mac:WindowUnMaximise'],
    ['common:WindowMaximise', 'common:WindowUnMaximise'],
    ['common:WindowMaximise', 'common:WindowRestore'],
  ])('follows %s and %s without asking the window again', async (on, offEvent) => {
    const { store } = await load();
    store.initWindowState();
    await vi.waitFor(() => expect(windowApi.IsMaximised).toHaveBeenCalledTimes(1));
    fire(on);
    expect(get(store.windowMaximised)).toBe(true);
    fire(offEvent);
    expect(get(store.windowMaximised)).toBe(false);
    expect(windowApi.IsMaximised).toHaveBeenCalledTimes(1);
  });

  it('asks the window again after it leaves fullscreen', async () => {
    const { store } = await load();
    store.initWindowState();
    await vi.waitFor(() => expect(windowApi.IsMaximised).toHaveBeenCalledTimes(1));
    maximised = Promise.resolve(true);
    fire('windows:WindowUnFullscreen');
    await vi.waitFor(() => expect(get(store.windowMaximised)).toBe(true));
    maximised = Promise.resolve(false);
    fire('mac:WindowDidExitFullScreen');
    await vi.waitFor(() => expect(get(store.windowMaximised)).toBe(false));
  });

  it('does not let a late startup read override a newer event', async () => {
    let answer: (value: boolean) => void = () => {};
    maximised = new Promise((resolve) => (answer = resolve));
    const { store } = await load();
    store.initWindowState();
    fire('windows:WindowMaximise');
    answer(false);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(get(store.windowMaximised)).toBe(true);
  });

  it('logs a failed read and keeps the last known state', async () => {
    maximised = Promise.reject(new Error('gone'));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { store } = await load();
    store.initWindowState();
    await vi.waitFor(() => expect(error).toHaveBeenCalled());
    expect(get(store.windowMaximised)).toBe(false);
    error.mockRestore();
  });

  it('unsubscribes every event it listens to', async () => {
    const { store } = await load();
    const dispose = store.initWindowState();
    const subscribed = handlers.size;
    expect(subscribed).toBeGreaterThan(0);
    dispose();
    expect(off).toHaveBeenCalledTimes(subscribed);
  });

  it('listens to nothing outside the desktop app', async () => {
    wails = false;
    const { store } = await load();
    store.initWindowState();
    expect(handlers.size).toBe(0);
    expect(windowApi.IsMaximised).not.toHaveBeenCalled();
  });
});

describe('window actions', () => {
  it.each([
    ['minimise', windowApi.Minimise],
    ['toggleMaximise', windowApi.ToggleMaximise],
    ['close', windowApi.Close],
  ] as const)('%s calls the window', async (action, call) => {
    const { store, toast } = await load();
    await store.runWindowAction(action);
    expect(call).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalled();
  });

  it('answers with the desktop-only toast in a browser', async () => {
    wails = false;
    const { store, toast, msg } = await load();
    await store.runWindowAction('close');
    expect(windowApi.Close).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith(msg('ui.desktopOnly'));
  });

  it('shows an error toast when the window call rejects', async () => {
    windowApi.Minimise.mockRejectedValueOnce(new Error('denied'));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { store, toast, msg } = await load();
    await expect(store.runWindowAction('minimise')).resolves.toBeUndefined();
    expect(toast).toHaveBeenCalledWith(msg('common.error'), 'danger');
    expect(error).toHaveBeenCalled();
    error.mockRestore();
  });
});
