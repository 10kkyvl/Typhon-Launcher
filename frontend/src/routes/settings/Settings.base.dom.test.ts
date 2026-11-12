import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
  // jsdom has no ResizeObserver; the settings tabs measure their marker with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ language: 'system', uiScale: 1, animationsEnabled: true, minimizeToTray: true, overlayEnabled: true } as SettingsData);
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  document.body.replaceChildren();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('Settings: the general tab', () => {
  it('opens the privacy policy from a text link, by the keyboard', async () => {
    render(Settings, { target: app });
    const link = screen.getByRole('button', { name: msg('settings.generalPrivacyPolicyLinkLabel') });
    expect(link.classList.contains('text-link')).toBe(true);

    link.focus();
    await user().keyboard('{Enter}');

    expect(await screen.findByRole('dialog', { name: msg('settings.generalPrivacyPolicyLinkLabel') })).toBeTruthy();
  });

  it('says there is no library yet as an empty state of the library card', () => {
    render(Settings, { target: app });
    const empty = screen.getByText(msg('settings.generalLibraryEmptyHint'));
    expect(empty.closest('.empty')).not.toBeNull();
  });

  it('reports an overlay setting that was not saved on a notice', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('locked'));
    render(Settings, { target: app });
    const overlay = screen.getByRole('switch', { name: msg('settings.overlayEnabledLabel') });

    overlay.focus();
    await user().keyboard(' ');

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('settings.overlaySaveError', { reason: 'locked' }));
    expect(alert.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
  });
});

describe('Settings: the about tab', () => {
  it('says the legal documents are unavailable on a notice of the legal card', async () => {
    render(Settings, { target: app });
    await user().click(screen.getByRole('tab', { name: msg('settings.aboutTab') }));
    const card = (await screen.findByText(msg('settings.aboutLegalCardTitle'))).closest('.card') as HTMLElement;
    const text = await within(card).findByText(msg('settings.aboutLegalUnavailable'));
    expect(text.closest('[data-tone]')?.getAttribute('data-tone')).toBe('info');
    // A fact of this build, read with the card: not announced on every opening of the tab.
    expect(text.closest('[role="status"], [role="alert"]')).toBeNull();
  });
});
