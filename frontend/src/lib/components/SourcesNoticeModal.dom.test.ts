import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { saveSettingsPatch, type Settings } from '../services/settings';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import SourcesNoticeModal from './SourcesNoticeModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
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
  const sent = sentToGo();
  resetBindings();
  settings.set(null);
  toasts.set([]);
  vi.restoreAllMocks();
  vi.mocked(saveSettingsPatch).mockReset();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function openDialog(props: Record<string, unknown> = {}) {
  const onaccepted = vi.fn();
  const result = render(SourcesNoticeModal, { target: app, props: { open: true, onaccepted, ...props } });
  const title = props.mode === 'review' ? msg('modals.sourcesNoticeReviewTitle') : msg('modals.sourcesNoticeGateTitle');
  return { ...result, onaccepted, keys: user(), dialog: screen.getByRole('dialog', { name: title }) };
}

describe('SourcesNoticeModal: the safe way out', () => {
  it('opens with the focus on Cancel before a source is added, never on Continue', () => {
    const { dialog } = openDialog();

    expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: msg('common.cancel') }));
  });

  it('opens with the focus on Close when it is only read again', () => {
    const { dialog } = openDialog({ mode: 'review' });

    const footerClose = within(dialog)
      .getAllByRole('button', { name: msg('common.close') })
      .find((button) => !button.closest('[data-skip-autofocus]'));
    expect(footerClose).toBeTruthy();
    expect(document.activeElement).toBe(footerClose);
    expect(within(dialog).queryByRole('button', { name: msg('modals.sourcesNoticeContinue') })).toBeNull();
  });

  it('closes on Escape as Cancel does: nothing saved, nothing opened after it', async () => {
    const { keys, onaccepted } = openDialog();

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(vi.mocked(saveSettingsPatch)).not.toHaveBeenCalled();
    expect(onaccepted).not.toHaveBeenCalled();
  });
});

describe('SourcesNoticeModal: continuing', () => {
  it('saves once for two presses, Continue busy and focused, Cancel held with its reason and Escape held with it', async () => {
    const saving = held<Settings>();
    vi.mocked(saveSettingsPatch).mockReturnValueOnce(saving.promise);
    const { dialog, keys, onaccepted } = openDialog();
    const go = within(dialog).getByRole('button', { name: msg('modals.sourcesNoticeContinue') });
    go.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(saveSettingsPatch)).toHaveBeenCalledTimes(1);
    expect(go.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(go);
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel.hasAttribute('disabled')).toBe(false);
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    const reason = document.getElementById(cancel.getAttribute('aria-describedby') ?? '');
    expect(reason?.textContent?.trim()).toBe(msg('modals.actionRunning'));
    await keys.click(cancel);
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);

    settings.set({ uiScale: 1, animationsEnabled: true, sourcesNoticeAccepted: true } as Settings);
    saving.resolve({ uiScale: 1, animationsEnabled: true, sourcesNoticeAccepted: true } as Settings);
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(onaccepted).toHaveBeenCalledTimes(1);
  });

  it('a failed save keeps the notice open with the reason in it, the focus on the reason, and its buttons work again', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('disk full'));
    const { dialog, keys, onaccepted } = openDialog();

    await keys.click(within(dialog).getByRole('button', { name: msg('modals.sourcesNoticeContinue') }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('state.settingsSaveFailed'));
    await vi.waitFor(() => expect(document.activeElement).toBe(alert.closest('.notice')));
    expect(dialog.isConnected).toBe(true);
    expect(onaccepted).not.toHaveBeenCalled();
    expect(get(toasts)).toEqual([]);
    for (const name of [msg('modals.sourcesNoticeContinue'), msg('common.cancel')]) {
      const button = within(dialog).getByRole('button', { name });
      expect(button.hasAttribute('disabled')).toBe(false);
      expect(button.getAttribute('aria-disabled')).toBeNull();
    }
  });
});
