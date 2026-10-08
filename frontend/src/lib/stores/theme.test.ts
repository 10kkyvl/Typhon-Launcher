import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Theme } from '../services/theme';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('./settings', () => ({ updateSettings: vi.fn(async () => true) }));
vi.mock('../theme/apply', () => ({ applyTheme: vi.fn(), clearTheme: vi.fn() }));

const api = {
  applyTheme: vi.fn(),
  confirmTheme: vi.fn(),
  activeTheme: vi.fn(),
  listThemes: vi.fn(),
  resetTheme: vi.fn(),
};

vi.mock('../services/theme', async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

function theme(id: string, patch: Partial<Theme> = {}): Theme {
  return { id, name: id, base: 'dark', tokens: {}, builtIn: true, updatedAt: '2026-09-01T00:00:00Z', ...patch };
}

function raw(id: string, patch: Record<string, unknown> = {}) {
  return { id, name: id, base: 'dark', tokens: {}, css: '', builtIn: false, updatedAt: '2026-09-01T00:00:00Z', ...patch };
}

function mediaList(matches: boolean) {
  return { matches, addEventListener: vi.fn(), removeEventListener: vi.fn() };
}

async function load(prefersDark = true) {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  vi.stubGlobal('window', { matchMedia: vi.fn(() => mediaList(prefersDark)) });
  api.activeTheme.mockResolvedValue(theme('dark'));
  api.listThemes.mockResolvedValue([theme('dark'), theme('light', { base: 'light' })]);
  api.applyTheme.mockResolvedValue(undefined);
  api.confirmTheme.mockResolvedValue(undefined);
  api.resetTheme.mockResolvedValue(undefined);
  const store = await import('./theme');
  const dom = await import('../theme/apply');
  const toasts = await import('./toasts');
  const settings = await import('./settings');
  await store.initTheme();
  vi.clearAllMocks();
  api.activeTheme.mockResolvedValue(theme('dark'));
  return {
    store,
    dom: vi.mocked(dom),
    toast: vi.mocked(toasts.toast),
    updateSettings: vi.mocked(settings.updateSettings),
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetAllMocks();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('choosing a theme', () => {
  it('applies the theme on the backend, shows what the backend reports and paints it', async () => {
    const { store, dom } = await load();
    api.activeTheme.mockResolvedValue(theme('light', { base: 'light' }));

    await store.selectTheme('light');

    expect(api.applyTheme).toHaveBeenCalledWith('light');
    expect(get(store.activeTheme)?.id).toBe('light');
    expect(dom.applyTheme).toHaveBeenCalledTimes(1);
    expect(get(store.themeMode)).toBe('theme');
  });

  it('asks the backend to confirm a theme only after five quiet seconds', async () => {
    const { store } = await load();
    api.activeTheme.mockResolvedValue(theme('light', { base: 'light' }));

    await store.selectTheme('light');
    vi.advanceTimersByTime(4999);
    expect(api.confirmTheme).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(api.confirmTheme).toHaveBeenCalledWith('light');
  });

  it('cancels the first confirmation when another theme is chosen before it fires', async () => {
    const { store } = await load();
    api.activeTheme.mockResolvedValueOnce(theme('light', { base: 'light' }));
    await store.selectTheme('light');
    vi.advanceTimersByTime(3000);

    api.activeTheme.mockResolvedValueOnce(theme('dark'));
    await store.selectTheme('dark');
    vi.advanceTimersByTime(3000);
    expect(api.confirmTheme).not.toHaveBeenCalled();

    vi.advanceTimersByTime(2000);
    expect(api.confirmTheme).toHaveBeenCalledTimes(1);
    expect(api.confirmTheme).toHaveBeenCalledWith('dark');
  });

  it('does not confirm a theme that is no longer the active one', async () => {
    const { store } = await load();
    api.activeTheme.mockResolvedValue(theme('light', { base: 'light' }));
    await store.selectTheme('light');

    store.activeTheme.set(theme('dark'));
    vi.advanceTimersByTime(6000);

    expect(api.confirmTheme).not.toHaveBeenCalled();
  });

  it('follows the system colour scheme in system mode', async () => {
    const { store } = await load(false);

    await store.selectTheme('system');

    expect(get(store.themeMode)).toBe('system');
    expect(api.applyTheme).toHaveBeenCalledWith('light');
  });

  it('picks the dark theme in system mode when the system prefers dark', async () => {
    const { store } = await load(true);

    await store.selectTheme('system');

    expect(api.applyTheme).toHaveBeenCalledWith('dark');
  });

  it('tells the player and schedules no confirmation when the backend refuses the theme', async () => {
    const { store, toast } = await load();
    api.applyTheme.mockRejectedValueOnce(new Error('typhon:theme.not_found: no such theme'));

    await store.selectTheme('ghost');
    vi.advanceTimersByTime(10_000);

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
    expect(String(toast.mock.calls[0][0])).not.toContain('typhon:');
    expect(api.confirmTheme).not.toHaveBeenCalled();
  });

  it('reports a failed confirmation instead of dropping it', async () => {
    const { store, toast } = await load();
    api.activeTheme.mockResolvedValue(theme('light', { base: 'light' }));
    api.confirmTheme.mockRejectedValueOnce(new Error('typhon:theme.not_found: gone'));
    await store.selectTheme('light');

    vi.advanceTimersByTime(5000);
    await vi.advanceTimersByTimeAsync(0);

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });
});

describe('backend theme events', () => {
  it('forgets the pending confirmation when the backend reverts the theme', async () => {
    const { store, dom } = await load();
    api.activeTheme.mockResolvedValue(theme('light', { base: 'light' }));
    await store.selectTheme('light');
    dom.applyTheme.mockClear();

    handlers['theme:reverted']({ data: raw('dark', { builtIn: true }) });
    vi.advanceTimersByTime(6000);

    expect(get(store.activeTheme)?.id).toBe('dark');
    expect(dom.applyTheme).toHaveBeenCalledTimes(1);
    expect(api.confirmTheme).not.toHaveBeenCalled();
  });

  it('replaces a theme in the list and repaints when it is the active one', async () => {
    const { store, dom } = await load();
    await store.refreshThemes();
    store.activeTheme.set(theme('light', { base: 'light' }));
    dom.applyTheme.mockClear();

    handlers['theme:updated']({ data: raw('light', { name: 'Light v2', base: 'light' }) });

    expect(get(store.themeList).find((item) => item.id === 'light')?.name).toBe('Light v2');
    expect(dom.applyTheme).toHaveBeenCalledTimes(1);
  });

  it('adds a theme it has not seen and leaves the painting alone', async () => {
    const { store, dom } = await load();
    await store.refreshThemes();
    dom.applyTheme.mockClear();

    handlers['theme:updated']({ data: raw('mine') });

    expect(get(store.themeList).map((item) => item.id)).toContain('mine');
    expect(dom.applyTheme).not.toHaveBeenCalled();
  });

  it('takes the whole list from theme:list, and an empty one from null', async () => {
    const { store } = await load();

    handlers['theme:list']({ data: [raw('a'), raw('b')] });
    expect(get(store.themeList).map((item) => item.id)).toEqual(['a', 'b']);

    handlers['theme:list']({ data: null });
    expect(get(store.themeList)).toEqual([]);
  });
});

describe('resetting the appearance', () => {
  it('clears the painting first, then resets on the backend, then repaints the active theme', async () => {
    const { store, dom, updateSettings } = await load();
    const order: string[] = [];
    dom.clearTheme.mockImplementation(() => void order.push('clear'));
    api.resetTheme.mockImplementation(async () => void order.push('reset'));
    dom.applyTheme.mockImplementation(() => void order.push('apply'));

    await store.resetAppearance();

    expect(order).toEqual(['clear', 'reset', 'apply']);
    expect(updateSettings).toHaveBeenCalledWith({ accentColor: '', tintLogo: false });
    expect(get(store.appearanceResetVersion)).toBe(1);
  });

  it('puts the previous theme back and tells the player when the backend cannot reset', async () => {
    const { store, dom, toast } = await load();
    store.activeTheme.set(theme('mine', { builtIn: false }));
    api.resetTheme.mockRejectedValueOnce(new Error('typhon:theme.built_in: nope'));

    await store.resetAppearance();

    expect(get(store.activeTheme)?.id).toBe('mine');
    expect(dom.applyTheme).toHaveBeenCalledTimes(1);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });
});

describe('outside the desktop app', () => {
  it('does not try to change themes without a backend', async () => {
    vi.resetModules();
    vi.doMock('../services/backend', () => ({ inWails: false }));
    vi.stubGlobal('window', { matchMedia: vi.fn(() => mediaList(true)) });
    const store = await import('./theme');

    await store.selectTheme('light');

    expect(api.applyTheme).not.toHaveBeenCalled();
    vi.doMock('../services/backend', () => ({ inWails: true }));
  });
});
