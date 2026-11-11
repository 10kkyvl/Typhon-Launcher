import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { msg } from '../../lib/i18n';
import { saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { user } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import Settings from './Settings.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));
// Without a backend the Discord card's first request reads as a lost session and signs the account out.
vi.mock('../../lib/services/discord', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../lib/services/discord')>();
  return {
    ...original,
    discordApi: { ...original.discordApi, getLink: async () => ({ linked: false, discordId: '', discordUsername: '', linkedAt: '' }) },
  };
});

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
  authState.set('bootstrapping');
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

  // The settings store refuses a change it cannot save (not loaded) before it changes anything, so the
  // switch must not show the press. Found through its row, not its name, so the case reads the same
  // against a switch that names itself differently.
  it('shows the held state when the page refuses the change', async () => {
    settings.set(null);
    render(Settings, { target: app });
    const row = screen.getByText(msg('settings.generalMinimizeToTrayLabel')).closest('.row');
    expect(row).toBeInstanceOf(HTMLElement);
    const tray = within(row as HTMLElement).getByRole('switch');
    expect(tray.getAttribute('aria-checked')).toBe('true');

    await user().click(tray);

    expect(get(toasts).map((item) => item.message)).toEqual([msg('state.settingsNotLoaded')]);
    expect(saveSettingsPatch).not.toHaveBeenCalled();
    expect(tray.getAttribute('aria-checked')).toBe('true');
  });
});

describe('Settings: the Cloud saves switch', () => {
  it.each(['Cancel', 'Escape'])('keeps focus and the held state when the consent dialog is closed with %s', async (how) => {
    authState.set('authenticated');
    render(Settings, { target: app });
    await user().click(screen.getByRole('tab', { name: msg('settings.downloadsTab') }));
    const cloud = screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') });
    expect(cloud.getAttribute('aria-checked')).toBe('false');

    cloud.focus();
    await user().keyboard(' ');
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.consentTitle') });
    if (how === 'Escape') await user().keyboard('{Escape}');
    else await user().click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(document.activeElement).toBe(cloud);
    expect(cloud.isConnected).toBe(true);
    expect(cloud.getAttribute('aria-checked')).toBe('false');
    expect(saveSettingsPatch).not.toHaveBeenCalled();
  });
});

describe('Settings: the tabs', () => {
  it('name the panel they show, and an arrow key shows the next one', async () => {
    render(Settings, { target: app });
    const tabs = screen.getAllByRole('tab');
    const first = tabs[0];
    expect(first.getAttribute('aria-selected')).toBe('true');
    const panel = screen.getByRole('tabpanel');
    expect(panel.getAttribute('aria-labelledby')).toBe(first.id);
    expect(first.getAttribute('aria-controls')).toBe(panel.id);

    first.focus();
    await user().keyboard('{ArrowRight}');

    expect(tabs[1].getAttribute('aria-selected')).toBe('true');
    expect(document.activeElement).toBe(tabs[1]);
    expect(screen.getByRole('tabpanel').getAttribute('aria-labelledby')).toBe(tabs[1].id);
  });
});
