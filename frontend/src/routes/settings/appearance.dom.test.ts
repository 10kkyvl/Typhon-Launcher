import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));
vi.mock('../../lib/services/theme', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/theme')>()),
  saveTheme: vi.fn(),
}));
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
import { saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { saveTheme, type Theme } from '../../lib/services/theme';
import { settings } from '../../lib/stores/settings';
import { activeTheme, selectTheme, themeList, themeMode } from '../../lib/stores/theme';
import { accentPresets } from '../../lib/theme/accent';
import { renderDom } from '../../lib/testing/dom';
import AccentPicker from './AccentPicker.svelte';
import AppearanceTab from './AppearanceTab.svelte';

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
  settings.set({ accentColor: '', tintLogo: false } as SettingsData);
  vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...get(settings), ...patch }) as SettingsData);
  themeList.set([DARK, MINE]);
  activeTheme.set(DARK);
  themeMode.set('theme');
});

afterEach(() => {
  document.body.replaceChildren();
  settings.set(null);
  themeList.set([]);
  activeTheme.set(null);
  vi.mocked(saveSettingsPatch).mockReset();
  vi.mocked(selectTheme).mockClear();
  vi.mocked(saveTheme).mockReset();
});

describe('AccentPicker: the swatches', () => {
  it('are one radio group: Tab reaches the chosen swatch, an arrow chooses the next', async () => {
    const { user } = renderDom(AccentPicker, {});
    const group = screen.getByRole('radiogroup', { name: msg('settings.accentTitle') });
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['true', ...accentPresets.map(() => 'false'), 'false']);
    expect(radios.map((radio) => radio.tabIndex)).toEqual([0, ...accentPresets.map(() => -1), -1]);
    const first = within(group).getByRole('radio', { name: msg('settings.accentPreset1') });

    radios[0].focus();
    await user.keyboard('{ArrowRight}');

    expect(document.activeElement).toBe(first);
    expect(vi.mocked(saveSettingsPatch)).toHaveBeenCalledWith({ accentColor: accentPresets[0] });
    await waitFor(() => expect(first.getAttribute('aria-checked')).toBe('true'));
    expect(first.tabIndex).toBe(0);
    expect(radios[0].getAttribute('aria-checked')).toBe('false');
  });

  // While the choice is saved the swatches refuse presses, but the focused one keeps focus.
  it('keeps focus on the swatch while its choice is saved', async () => {
    let finish: (value: SettingsData) => void = () => {};
    vi.mocked(saveSettingsPatch).mockImplementation(() => new Promise((resolve) => (finish = resolve)));
    const { user } = renderDom(AccentPicker, {});
    const second = screen.getByRole('radio', { name: msg('settings.accentPreset2') });

    await user.click(second);

    expect(document.activeElement).toBe(second);
    expect(second.getAttribute('aria-disabled')).toBe('true');
    await user.keyboard('{ArrowRight}');
    expect(vi.mocked(saveSettingsPatch)).toHaveBeenCalledTimes(1);
    finish({ accentColor: accentPresets[1] } as SettingsData);
    await waitFor(() => expect(second.hasAttribute('aria-disabled')).toBe(false));
    expect(document.activeElement).toBe(second);
  });
});

describe('AppearanceTab', () => {
  it('offers the themes as one radio group that an arrow moves through', async () => {
    const { user } = renderDom(AppearanceTab, {});
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

  async function editName(user: ReturnType<typeof renderDom>['user']) {
    const name = screen.getByRole('textbox', { name: msg('settings.appearanceNameFieldLabel') }) as HTMLInputElement;
    await user.click(name);
    await user.keyboard('X');
    return name;
  }

  it('asks before an arrow drops unsaved edits, and Stay keeps the edits, the choice and focus', async () => {
    const { user } = renderDom(AppearanceTab, {});
    const name = await editName(user);
    const radios = within(screen.getByRole('radiogroup', { name: msg('settings.appearanceTab') })).getAllByRole('radio');
    radios[1].focus();

    await user.keyboard('{ArrowRight}');

    const dialog = await screen.findByRole('dialog', { name: msg('settings.themeDiscardTitle') });
    expect(vi.mocked(selectTheme)).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: msg('settings.themeDiscardStay') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(selectTheme)).not.toHaveBeenCalled();
    expect(radios[1].getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(radios[1]);
    expect(name.value).toBe('DarkX');
  });

  it('chooses the tile once the edits are given up, and puts focus on it', async () => {
    const { user } = renderDom(AppearanceTab, {});
    await editName(user);
    const radios = within(screen.getByRole('radiogroup', { name: msg('settings.appearanceTab') })).getAllByRole('radio');

    await user.click(radios[2]);
    const dialog = await screen.findByRole('dialog', { name: msg('settings.themeDiscardTitle') });
    expect(dialog.textContent).toContain(msg('settings.themeDiscardText', { name: 'DarkX' }));
    await user.click(within(dialog).getByRole('button', { name: msg('settings.themeCustomRevert') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(selectTheme)).toHaveBeenCalledWith('mine');
    await waitFor(() => expect(document.activeElement).toBe(radios[2]));
    expect(radios[2].getAttribute('aria-checked')).toBe('true');
    expect((screen.getByRole('textbox', { name: msg('settings.appearanceNameFieldLabel') }) as HTMLInputElement).value).toBe('Mine');
  });

  it('refuses another choice while one is applied, and keeps focus on the tile', async () => {
    let finish: () => void = () => {};
    vi.mocked(selectTheme).mockImplementationOnce(async (id: string) => {
      await new Promise<void>((resolve) => (finish = resolve));
      themeMode.set('theme');
      activeTheme.set(id === 'mine' ? MINE : DARK);
    });
    const { user } = renderDom(AppearanceTab, {});
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
    renderDom(AppearanceTab, {});
    const remove = screen.getByRole('button', { name: msg('settings.appearanceDeleteTile', { name: 'Mine' }) });
    expect(remove.classList.contains('icon-btn')).toBe(true);
  });

  it('shows the custom CSS through a disclosure that says it is expanded', async () => {
    const { user } = renderDom(AppearanceTab, {});
    const toggle = screen.getByRole('button', { name: msg('settings.appearanceShowAdvanced') });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(toggle.hasAttribute('aria-pressed')).toBe(false);
    const region = document.getElementById(toggle.getAttribute('aria-controls') ?? '');
    expect(region).not.toBeNull();

    toggle.focus();
    await user.keyboard('{Enter}');

    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(toggle.textContent?.trim()).toBe(msg('settings.appearanceHideAdvanced'));
    expect(within(region as HTMLElement).getByRole('textbox')).toBeTruthy();
  });

  it('reports a refused save in an alert', async () => {
    const { user } = renderDom(AppearanceTab, {});
    await user.click(screen.getByRole('button', { name: msg('settings.appearanceShowAdvanced') }));
    const css = screen.getByRole('textbox', { name: msg('settings.appearanceCustomCssLabel') });
    await user.click(css);
    await user.paste('@import "x";');

    await user.click(screen.getByRole('button', { name: msg('common.save') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).not.toBe('');
    expect(vi.mocked(saveTheme)).not.toHaveBeenCalled();
  });
});
