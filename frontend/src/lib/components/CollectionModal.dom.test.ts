import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { msg } from '../i18n';
import { saveCollection } from '../services/library';
import { user } from '../testing/dom';
import CollectionModal from './CollectionModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  allTags: vi.fn(async () => []),
  saveCollection: vi.fn(async () => {}),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.mocked(saveCollection).mockClear();
  document.body.replaceChildren();
});

describe('CollectionModal: the status field', () => {
  it('is named by its label, chosen from the keyboard inside the dialog, and saved', async () => {
    render(CollectionModal, { target: app, props: { open: true, genres: [] } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });
    const status = screen.getByRole('combobox', { name: msg('modals.collectionStatusLabel') });
    expect(status.textContent?.trim()).toBe(msg('modals.collectionStatusAny'));

    status.focus();
    await keys.keyboard('{ArrowDown}');
    const list = screen.getByRole('listbox', { name: msg('modals.collectionStatusLabel') });
    expect(dialog.contains(list)).toBe(false);
    await keys.keyboard('{ArrowDown}{Enter}');
    expect(status.textContent?.trim()).toBe(msg('games.statusPlaying'));

    await keys.keyboard('{ArrowDown}{Escape}');
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(screen.getByRole('dialog')).toBe(dialog);

    await keys.type(screen.getByPlaceholderText(msg('modals.collectionNamePlaceholder')), 'Evenings');
    await keys.click(screen.getByRole('button', { name: msg('common.save') }));
    expect(saveCollection).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ name: 'Evenings', status: 'playing' }));
  });
});
