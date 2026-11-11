import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { msg } from '../../lib/i18n';
import { saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { user } from '../../lib/testing/dom';
import Settings from './Settings.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  // jsdom has no ResizeObserver; the settings tabs measure their indicator with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  // Zoom 120 % is what Big Picture sets; the settings page offers 90, 100, 110 and 125.
  settings.set({ language: 'system', uiScale: 1.2, animationsEnabled: true } as SettingsData);
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  document.body.replaceChildren();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const shown = (field: HTMLElement) => field.textContent?.trim() ?? '';

describe('Settings: a select field', () => {
  it('is named by the label of its row', () => {
    render(Settings, { target: app });
    expect(screen.getByRole('combobox', { name: msg('settings.language') })).toBeTruthy();
    expect(screen.getByRole('combobox', { name: msg('settings.generalUiScaleLabel') })).toBeTruthy();
  });

  it('shows a zoom it has no option for, shows the new one while it saves, and goes back when the save fails', async () => {
    let refuse: (err: Error) => void = () => {};
    vi.mocked(saveSettingsPatch).mockImplementation(
      () =>
        new Promise<SettingsData>((_resolve, reject) => {
          refuse = reject;
        }),
    );
    render(Settings, { target: app });
    const keys = user();
    const scale = screen.getByRole('combobox', { name: msg('settings.generalUiScaleLabel') });
    expect(shown(scale)).toBe('120');

    await keys.click(scale);
    await keys.click(screen.getByRole('option', { name: '110%' }));

    await vi.waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ uiScale: 1.1 }));
    expect(shown(scale)).toBe('110%');

    refuse(new Error('disk full'));

    await vi.waitFor(() => expect(shown(scale)).toBe('120'));
    expect(get(toasts).map((t) => t.message)).toContain(msg('state.settingsSaveFailed'));
    expect(document.activeElement).toBe(scale);
  });
});
