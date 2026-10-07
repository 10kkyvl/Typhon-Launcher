import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { BindingTheme } from './theme';

const bindings = {
  List: vi.fn(),
  Active: vi.fn(),
  Get: vi.fn(),
  Save: vi.fn(),
  Apply: vi.fn(),
  SelectThemeFile: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/theme', () => ({ Service: bindings }));

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./theme');
}

function raw(patch: Record<string, unknown> = {}): BindingTheme {
  return {
    id: 'mine',
    name: 'Mine',
    base: 'light',
    tokens: { '--accent': '#fff' },
    css: '.a{}',
    builtIn: false,
    updatedAt: '2026-09-01T00:00:00Z',
    ...patch,
  } as BindingTheme;
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('toTheme', () => {
  it('copies the fields the interface uses', async () => {
    const { toTheme } = await load(true);

    expect(toTheme(raw())).toEqual({
      id: 'mine',
      name: 'Mine',
      base: 'light',
      tokens: { '--accent': '#fff' },
      css: '.a{}',
      builtIn: false,
      updatedAt: '2026-09-01T00:00:00Z',
    });
  });

  it('treats any base but light as dark', async () => {
    const { toTheme } = await load(true);

    expect(toTheme(raw({ base: 'sepia' })).base).toBe('dark');
    expect(toTheme(raw({ base: '' })).base).toBe('dark');
    expect(toTheme(raw({ base: 'dark' })).base).toBe('dark');
  });

  it('never lets a theme carry the UI scale, which belongs to the settings', async () => {
    const { toTheme } = await load(true);

    expect(toTheme(raw({ tokens: { '--ui-scale': '2', '--accent': '#fff' } })).tokens).toEqual({ '--accent': '#fff' });
  });

  it('drops tokens without a value and survives a theme without tokens at all', async () => {
    const { toTheme } = await load(true);

    expect(toTheme(raw({ tokens: { '--a': undefined, '--b': 'x' } })).tokens).toEqual({ '--b': 'x' });
    expect(toTheme(raw({ tokens: null })).tokens).toEqual({});
  });
});

describe('theme calls', () => {
  it('lists nothing and offers a fallback theme in a browser preview', async () => {
    const theme = await load(false);

    await expect(theme.listThemes()).resolves.toEqual([]);
    await expect(theme.activeTheme()).resolves.toMatchObject({ id: 'dark', base: 'dark', builtIn: true });
    expect(bindings.List).not.toHaveBeenCalled();
  });

  it('refuses to change anything in a browser preview', async () => {
    const theme = await load(false);

    await expect(theme.applyTheme('light')).rejects.toThrow('unavailable in browser');
    await expect(theme.resetTheme()).rejects.toThrow('unavailable in browser');
    await expect(theme.confirmTheme('light')).rejects.toThrow('unavailable in browser');
    await expect(theme.selectThemeFile()).resolves.toBe('');
  });

  it('converts every theme the backend lists and copes with an empty answer', async () => {
    const theme = await load(true);
    bindings.List.mockResolvedValueOnce([raw({ id: 'a' }), raw({ id: 'b', base: 'x' })]).mockResolvedValueOnce(null);

    const list = await theme.listThemes();
    expect(list.map((item) => [item.id, item.base])).toEqual([
      ['a', 'light'],
      ['b', 'dark'],
    ]);
    await expect(theme.listThemes()).resolves.toEqual([]);
  });

  it('saves the theme with the fields the backend expects and returns the stored one', async () => {
    const theme = await load(true);
    bindings.Save.mockResolvedValueOnce(raw({ name: 'Stored' }));

    const stored = await theme.saveTheme({
      id: 'mine',
      name: 'Mine',
      base: 'light',
      tokens: { '--accent': '#fff' },
      css: '.a{}',
      builtIn: false,
      updatedAt: '2026-09-01T00:00:00Z',
    });

    expect(bindings.Save).toHaveBeenCalledWith({
      id: 'mine',
      name: 'Mine',
      base: 'light',
      tokens: { '--accent': '#fff' },
      css: '.a{}',
      builtIn: false,
      updatedAt: '2026-09-01T00:00:00Z',
    });
    expect(stored.name).toBe('Stored');
  });
});
