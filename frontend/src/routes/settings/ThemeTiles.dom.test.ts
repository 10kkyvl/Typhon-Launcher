import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// The real store applies a theme through the backend; here choosing one only records the choice.
vi.mock('../../lib/stores/theme', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../lib/stores/theme')>();
  return {
    ...original,
    selectTheme: vi.fn(async (id: string) => {
      original.themeMode.set(id === 'system' ? 'system' : 'theme');
      const picked = (await import('svelte/store')).get(original.themeList).find((theme) => theme.id === id);
      if (picked) original.activeTheme.set(picked);
    }),
  };
});

import { msg } from '../../lib/i18n';
import type { Theme } from '../../lib/services/theme';
import { activeTheme, selectTheme, themeList, themeMode } from '../../lib/stores/theme';
import { renderDom } from '../../lib/testing/dom';
import ThemeTiles from './ThemeTiles.svelte';

const theme = (id: string, name: string, builtIn: boolean): Theme => ({
  id,
  name,
  base: 'dark',
  tokens: { '--bg': '#101010', '--accent': '#4F6BFF' },
  builtIn,
  updatedAt: '',
});
const DARK = theme('dark', 'Dark', true);
const MINE = theme('mine', 'Mine', false);

beforeEach(() => {
  themeList.set([DARK, MINE]);
  activeTheme.set(DARK);
  themeMode.set('theme');
});

afterEach(() => {
  document.body.replaceChildren();
  themeList.set([]);
  activeTheme.set(null);
  vi.mocked(selectTheme).mockClear();
});

describe('ThemeTiles', () => {
  it('offers the themes as one radio group that an arrow moves through', async () => {
    const { user } = renderDom(ThemeTiles, { unsaved: false, name: '', deleting: false, onpick: () => {}, onremove: () => {} });
    const group = screen.getByRole('radiogroup', { name: msg('settings.appearanceTab') });
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'true', 'false']);
    expect(radios.map((radio) => radio.tabIndex)).toEqual([-1, 0, -1]);

    radios[1].focus();
    await user.keyboard('{ArrowRight}');

    expect(vi.mocked(selectTheme)).toHaveBeenCalledWith('mine');
    expect(document.activeElement).toBe(radios[2]);
    await waitFor(() => expect(radios[2].getAttribute('aria-checked')).toBe('true'));

    await user.keyboard('{ArrowRight}');
    expect(vi.mocked(selectTheme)).toHaveBeenLastCalledWith('system');
    expect(document.activeElement).toBe(radios[0]);
  });

  it('refuses another choice while one is applied, and keeps focus on the tile', async () => {
    let finish: () => void = () => {};
    vi.mocked(selectTheme).mockImplementationOnce(async (id: string) => {
      await new Promise<void>((resolve) => (finish = resolve));
      themeMode.set('theme');
      activeTheme.set(id === 'mine' ? MINE : DARK);
    });
    const { user } = renderDom(ThemeTiles, { unsaved: false, name: '', deleting: false, onpick: () => {}, onremove: () => {} });
    const radios = within(screen.getByRole('radiogroup', { name: msg('settings.appearanceTab') })).getAllByRole('radio');

    await user.click(radios[2]);
    expect(radios.every((radio) => radio.getAttribute('aria-disabled') === 'true')).toBe(true);
    await user.keyboard('{ArrowRight}');
    expect(document.activeElement).toBe(radios[2]);
    await user.click(radios[0]);

    expect(vi.mocked(selectTheme)).toHaveBeenCalledTimes(1);
    finish();
    await waitFor(() => expect(radios[2].hasAttribute('aria-disabled')).toBe(false));
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'true']);
  });

  it('deletes a custom theme from a base icon button named for the theme', () => {
    renderDom(ThemeTiles, { unsaved: false, name: '', deleting: false, onpick: () => {}, onremove: () => {} });
    const remove = screen.getByRole('button', { name: msg('settings.appearanceDeleteTile', { name: 'Mine' }) });
    expect(remove.classList.contains('icon-btn')).toBe(true);
  });
});
