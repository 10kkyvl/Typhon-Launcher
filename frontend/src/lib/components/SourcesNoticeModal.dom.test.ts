import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { saveSettingsPatch, type Settings } from '../services/settings';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import SourcesNoticeModal from './SourcesNoticeModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ uiScale: 1, animationsEnabled: true, sourcesNoticeAccepted: false } as Settings);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  app.remove();
  vi.restoreAllMocks();
});

describe('SourcesNoticeModal', () => {
  it('a failed save keeps the notice open with the reason in it, and its buttons work again', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('disk full'));
    const onaccepted = vi.fn();
    render(SourcesNoticeModal, { target: app, props: { open: true, onaccepted } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('modals.sourcesNoticeGateTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('modals.sourcesNoticeContinue') }));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('state.settingsSaveFailed'));
    expect(dialog.isConnected).toBe(true);
    expect(onaccepted).not.toHaveBeenCalled();
    expect(get(toasts)).toEqual([]);
    expect(within(dialog).getByRole('button', { name: msg('modals.sourcesNoticeContinue') })).toHaveProperty('disabled', false);
    expect(within(dialog).getByRole('button', { name: msg('common.cancel') })).toHaveProperty('disabled', false);
  });
});
