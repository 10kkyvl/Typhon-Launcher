import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { applyRollback, noRollback, SelfUpdateError } from '../services/selfupdate';
import { releaseNotes, rollbackInfo } from '../stores/selfupdate';
import { toasts } from '../stores/toasts';
import { endUpdateShow, takeOverShow } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import ReleaseNotesModal from './ReleaseNotesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/selfupdate')>()),
  applyRollback: vi.fn(),
}));

const NOTE = { version: '1.5.0', publishedAt: '2026-11-20T12:00:00Z', changes: [{ kind: 'fixed' as const, text: 'fixed a thing' }] };
const title = () => msg('modals.releaseNotesTitle', { version: '1.5.0' });

afterEach(() => {
  endUpdateShow();
  releaseNotes.set({ currentVersion: '', unseen: [], history: [] });
  rollbackInfo.set(noRollback());
  toasts.set([]);
  vi.mocked(applyRollback).mockReset();
  document.body.replaceChildren();
});

describe('the rollback offered beside the notes', () => {
  it('keeps its dialog open with the reason when running work refuses it', async () => {
    vi.mocked(applyRollback).mockRejectedValue(new SelfUpdateError('typhon:selfupdate.work_running: selfupdate: an install is running'));
    releaseNotes.set({ currentVersion: '1.5.0', unseen: [NOTE], history: [NOTE] });
    rollbackInfo.set({ available: true, version: '1.4.1', reason: '', reasonCode: '' });
    const { user } = renderDom(ReleaseNotesModal);
    const notes = screen.getByRole('dialog', { name: title() });
    await user.click(within(notes).getByRole('button', { name: msg('ui.rollbackButton', { version: '1.4.1' }) }));
    const dialog = await screen.findByRole('dialog', { name: msg('ui.rollbackConfirmTitle', { version: '1.4.1' }) });

    await user.click(within(dialog).getByRole('button', { name: msg('ui.rollbackButton', { version: '1.4.1' }) }));

    expect(await within(dialog).findByText(msg('state.selfupdateReasonWorkRunning'))).toBeTruthy();
    expect(dialog.isConnected).toBe(true);
    expect(get(toasts)).toEqual([]);
    expect(applyRollback).toHaveBeenCalledOnce();
  });
});

describe('the notes of the version the launcher was updated to', () => {
  it('open at once when no update show is up', () => {
    releaseNotes.set({ currentVersion: '1.5.0', unseen: [NOTE], history: [NOTE] });
    renderDom(ReleaseNotesModal);
    expect(screen.getByRole('dialog', { name: title() })).toBeTruthy();
  });

  it('wait for the update show to end: a dialog under it would take focus behind the curtain', async () => {
    takeOverShow(Date.now() - 10_000, '1.5.0');
    releaseNotes.set({ currentVersion: '1.5.0', unseen: [NOTE], history: [NOTE] });
    renderDom(ReleaseNotesModal);
    expect(screen.queryByRole('dialog', { name: title() })).toBeNull();

    endUpdateShow();
    await tick();
    expect(screen.getByRole('dialog', { name: title() })).toBeTruthy();
  });
});
