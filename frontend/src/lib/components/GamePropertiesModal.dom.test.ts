import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { errorCode, hasMessage, msg } from '../i18n';
import { installErrorText } from '../install/installErrors';
import type { LibraryGame } from '../services/library';
import { enableWrapper } from '../services/wrappers';
import { closeGameProperties, openGameProperties } from '../stores/gameProperties';
import { toasts } from '../stores/toasts';
import { get } from 'svelte/store';
import { within } from '@testing-library/svelte';
import { libraryGames } from '../stores/library';
import { user } from '../testing/dom';
import GamePropertiesModal from './GamePropertiesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/wrappers', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/wrappers')>()),
  inspectWrapper: vi.fn(async () => ({ supported: true, arch: 'x86', imports: ['d3d9.dll'], suggested: 'dgvoodoo', active: null, problem: null })),
  enableWrapper: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([
    { id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame,
    { id: 'g2', title: 'Celeste', uninstalled: false, installDir: '/games/celeste', executable: 'Celeste.exe' } as LibraryGame,
  ]);
});

afterEach(() => {
  closeGameProperties();
  libraryGames.set([]);
  toasts.set([]);
  app.remove();
});

describe('GamePropertiesModal while compatibility mode downloads', () => {
  it('can be closed mid-download (nothing to cancel, it may take minutes); reopened, it shows the work still running and starts nothing new', async () => {
    let finish!: () => void;
    vi.mocked(enableWrapper).mockReturnValueOnce(new Promise<void>((resolve) => (finish = resolve)));
    render(GamePropertiesModal, { target: app });
    const keys = user();
    openGameProperties('g1');
    const toggle = await screen.findByRole('switch', { name: msg('modals.compatToggle') });

    await keys.click(toggle);
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();

    openGameProperties('g1');
    const again = await screen.findByRole('switch', { name: msg('modals.compatToggle') });
    expect(again).toHaveProperty('disabled', true);
    await keys.click(again);
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);

    finish();
    await vi.waitFor(() => expect(screen.getByRole('switch', { name: msg('modals.compatToggle') })).toHaveProperty('disabled', false));
  });
});

describe('GamePropertiesModal reports a failed switch wherever the user is', () => {
  const failure = new Error('typhon:wrappers.download_failed: offline');
  // The dialog's own wording for a failed enable: the code's message, else the install table's, else the fallback.
  const failText = () => {
    const code = errorCode(failure);
    return hasMessage(code) ? msg(code) : installErrorText(failure, msg('modals.compatEnableFailed'));
  };

  async function startEnable() {
    let fail!: (err: unknown) => void;
    vi.mocked(enableWrapper).mockReturnValueOnce(new Promise<void>((_resolve, reject) => (fail = reject)));
    render(GamePropertiesModal, { target: app });
    const keys = user();
    openGameProperties('g1');
    await keys.click(await screen.findByRole('switch', { name: msg('modals.compatToggle') }));
    return { keys, fail };
  }

  const toastTexts = () => get(toasts).map((item) => item.message);

  it('closed mid-enable: the failure arrives as a toast', async () => {
    const { keys, fail } = await startEnable();
    await keys.keyboard('{Escape}');

    fail(failure);

    await vi.waitFor(() => expect(toastTexts()).toHaveLength(1));
    expect(get(toasts)[0].kind).toBe('danger');
    expect(toastTexts()[0]).toBe(failText());
  });

  it("open for another game: a toast, and that game's dialog is left alone", async () => {
    const { keys, fail } = await startEnable();
    await keys.keyboard('{Escape}');
    openGameProperties('g2');
    const other = await screen.findByRole('dialog', { name: msg('modals.propertiesTitle', { title: 'Celeste' }) });

    fail(failure);

    await vi.waitFor(() => expect(toastTexts()).toEqual([failText()]));
    expect(within(other).queryByText(failText())).toBeNull();
  });

  it('still shown: the failure stays inline in the dialog, no toast', async () => {
    const { fail } = await startEnable();

    fail(failure);

    const dialog = screen.getByRole('dialog');
    expect(await within(dialog).findByText(failText())).toBeTruthy();
    expect(toastTexts()).toEqual([]);
  });
});
