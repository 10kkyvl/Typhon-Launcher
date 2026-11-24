import { screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { releaseNotes } from '../stores/selfupdate';
import { endUpdateShow, takeOverShow } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import ReleaseNotesModal from './ReleaseNotesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

const NOTE = { version: '1.5.0', publishedAt: '2026-11-20T12:00:00Z', changes: [{ kind: 'fixed' as const, text: 'fixed a thing' }] };
const title = () => msg('modals.releaseNotesTitle', { version: '1.5.0' });

afterEach(() => {
  endUpdateShow();
  releaseNotes.set({ currentVersion: '', unseen: [], history: [] });
  document.body.replaceChildren();
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
