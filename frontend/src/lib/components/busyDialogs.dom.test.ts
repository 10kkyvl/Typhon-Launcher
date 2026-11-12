import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { libraryErrorText } from '../library/errors';
import { AccountError } from '../services/account';
import { cloudResolveConflict, type CloudConflict } from '../services/cloudsaves';
import { selectEmulatorFile } from '../services/emulators';
import { cloudConflictTarget, cloudStatus } from '../stores/cloudsaves';
import { askForEmulator, emulatorPrompt } from '../stores/emulatorPrompt';
import { toasts } from '../stores/toasts';
import { authState, deleteAccount } from '../stores/user';
import { user } from '../testing/dom';
import CloudConflictModal from './CloudConflictModal.svelte';
import DeleteAccountModal from './DeleteAccountModal.svelte';
import EmulatorNeededModal from './EmulatorNeededModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/user')>()),
  deleteAccount: vi.fn(),
}));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudResolveConflict: vi.fn(),
}));
vi.mock('../services/emulators', () => ({
  listPlatforms: vi.fn(async () => [{ id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2', status: 'not_set' }]),
  openOfficialPage: vi.fn(async () => {}),
  selectEmulatorFile: vi.fn(),
  setEmulatorPath: vi.fn(async () => {}),
}));

// The operation each dialog waits on: held open by the test, then failed.
function held() {
  let fail!: (err: unknown) => void;
  const promise = new Promise<never>((_resolve, reject) => {
    fail = reject;
  });
  return { promise, fail };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  get(emulatorPrompt)?.settle(false);
  cloudConflictTarget.set(null);
  cloudStatus.set(null);
  authState.set('bootstrapping');
  toasts.set([]);
  app.remove();
  vi.clearAllMocks();
});

describe('a dialog busy with its operation', () => {
  it('DeleteAccountModal: Escape does nothing while the deletion runs, and a refusal shows in the dialog', async () => {
    authState.set('authenticated');
    const deletion = held();
    vi.mocked(deleteAccount).mockReturnValueOnce(deletion.promise);
    const onclose = vi.fn();
    render(DeleteAccountModal, { target: app, props: { onclose } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('profile.deleteAccountConfirmTitle') });
    await keys.type(within(dialog).getByLabelText(msg('profile.deleteAccountPassword')), 'secret');
    await keys.click(within(dialog).getByRole('button', { name: msg('profile.deleteAccountConfirmButton') }));

    await keys.keyboard('{Escape}');
    expect(onclose).not.toHaveBeenCalled();

    deletion.fail(new AccountError('invalid_credentials'));
    expect((await within(dialog).findByRole('alert')).textContent).toBe(msg('profile.deleteAccountWrongPassword'));
    expect(dialog.isConnected).toBe(true);
    expect(onclose).not.toHaveBeenCalled();
  });

  it('CloudConflictModal: Escape does nothing while the conflict is resolved, and a failure shows in the dialog', async () => {
    const conflict = { localUpdatedAt: '2026-11-01T10:00:00Z', remoteUpdatedAt: '2026-11-02T10:00:00Z', localSize: 10, remoteSize: 20, remoteDevice: 'Deck' } as CloudConflict;
    cloudStatus.set({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games: [{ gameId: 'g1', state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict }] });
    cloudConflictTarget.set({ gameId: 'g1', title: 'Hades' });
    const resolving = held();
    vi.mocked(cloudResolveConflict).mockReturnValueOnce(resolving.promise);
    render(CloudConflictModal, { target: app });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') }));

    await keys.keyboard('{Escape}');
    expect(get(cloudConflictTarget)).not.toBeNull();

    const failure = new Error('typhon:cloudsaves.quota_exceeded: full');
    resolving.fail(failure);
    expect(await within(dialog).findByText(cloudErrorText(failure))).toBeTruthy();
    expect(dialog.isConnected).toBe(true);
  });

  it('EmulatorNeededModal: Escape does nothing while the emulator is being chosen, and a failure shows in the dialog', async () => {
    const choosing = held();
    vi.mocked(selectEmulatorFile).mockReturnValueOnce(choosing.promise);
    render(EmulatorNeededModal, { target: app });
    const answer = askForEmulator('ps2', 'emulators.not_configured');
    const settled = vi.fn();
    void answer.then(settled);
    const keys = user();
    const dialog = await screen.findByRole('dialog', { name: msg('emul.neededTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('emul.neededChoose') }));

    await keys.keyboard('{Escape}');
    await Promise.resolve();
    expect(settled).not.toHaveBeenCalled();

    const failure = new Error('typhon:emulators.not_found: gone');
    choosing.fail(failure);
    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(libraryErrorText(failure, msg('emul.saveFailed')));
    expect(dialog.isConnected).toBe(true);
  });
});
