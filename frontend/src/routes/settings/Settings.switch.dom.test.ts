import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { msg } from '../../lib/i18n';
import { saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { user } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import Settings from './Settings.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  // jsdom has no ResizeObserver; the settings tabs measure their marker with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ language: 'system', uiScale: 1, animationsEnabled: true, minimizeToTray: true } as SettingsData);
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  document.body.replaceChildren();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('Settings: a switch row', () => {
  it('names the switch by the row label and describes it by the row text', () => {
    render(Settings, { target: app });
    const tray = screen.getByRole('switch', { name: msg('settings.generalMinimizeToTrayLabel') });
    expect(tray.hasAttribute('aria-label')).toBe(false);
    expect(description(tray)).toBe(msg('settings.generalMinimizeToTraySub'));
    expect(screen.getByRole('switch', { name: msg('settings.generalAnimationsLabel') })).toBeTruthy();
  });

  it('shows the new state while it saves and the held one again when the save fails', async () => {
    let refuse: (err: Error) => void = () => {};
    vi.mocked(saveSettingsPatch).mockImplementation(
      () =>
        new Promise<SettingsData>((_resolve, reject) => {
          refuse = reject;
        }),
    );
    render(Settings, { target: app });
    const tray = screen.getByRole('switch', { name: msg('settings.generalMinimizeToTrayLabel') });
    expect(tray.getAttribute('aria-checked')).toBe('true');

    await user().click(tray);

    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ minimizeToTray: false });
    expect(tray.getAttribute('aria-checked')).toBe('false');

    refuse(new Error('disk full'));
    await vi.waitFor(() => expect(tray.getAttribute('aria-checked')).toBe('true'));
  });
});
