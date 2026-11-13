import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { cloudResolveConflict, loadCloudStatus, type CloudConflict, type CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import { askToResolveConflict, closeCloudConflict, cloudConflictTarget, cloudStatus, openCloudConflict } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import CloudConflictModal from './CloudConflictModal.svelte';
import CloudConsentModal from './CloudConsentModal.svelte';
import CloudDisableModal from './CloudDisableModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudResolveConflict: vi.fn(),
  loadCloudStatus: vi.fn(),
}));

const conflict = { localUpdatedAt: '2026-11-01T10:00:00Z', remoteUpdatedAt: '2026-11-02T10:00:00Z', localSize: 10, remoteSize: 20, remoteDevice: 'Deck' } as CloudConflict;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(loadCloudStatus).mockReturnValue(new Promise(() => {}));
});

afterEach(() => {
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([]);
  toasts.set([]);
  vi.mocked(cloudResolveConflict).mockReset();
  vi.mocked(loadCloudStatus).mockReset();
  document.body.replaceChildren();
});

function statusWith(games: CloudStatus['games']): CloudStatus {
  return { enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games };
}

const listed = () => statusWith([{ gameId: 'g1', state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict }]);

// What the waiting launch has been told so far: still waiting, or its answer.
async function answer(question: Promise<boolean> | null): Promise<boolean | 'waiting'> {
  expect(question).not.toBeNull();
  return await Promise.race([question as Promise<boolean>, new Promise<'waiting'>((resolve) => setTimeout(() => resolve('waiting'), 0))]);
}

// A launch the conflict held back opens the dialog; nothing about the conflict is loaded yet.
function askFromLaunch() {
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
  const question = askToResolveConflict('g1');
  render(CloudConflictModal, { target: app });
  return { question, dialog: screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') }) };
}

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

const deviceLine = `${msg('cloudsaves.conflictDevice')}: Deck`;

function showConflict(withDetails = true) {
  cloudStatus.set({
    enabled: true,
    signedIn: true,
    usedBytes: 0,
    quotaBytes: 0,
    maxBytes: 0,
    error: '',
    games: withDetails ? [{ gameId: 'g1', state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict }] : [],
  });
  cloudConflictTarget.set({ gameId: 'g1', title: 'Hades' });
  render(CloudConflictModal, { target: app });
  return screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
}

describe('CloudConflictModal', () => {
  it('states what each choice loses in a warning notice read with the dialog', () => {
    const dialog = showConflict();
    const mine = within(dialog).getByText(msg('cloudsaves.conflictKeepMineWarning'));
    const notice = mine.closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(within(notice).getByText(msg('cloudsaves.conflictTakeRemoteWarning'))).toBeTruthy();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('says it is loading the copies with a labelled spinner', () => {
    const dialog = showConflict(false);
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('cloudsaves.conflictLoading'));
  });

  it('keeps the focus on the pressed choice, busy, and says a failure in a danger notice', async () => {
    const resolving = deferred<void>();
    vi.mocked(cloudResolveConflict).mockReturnValueOnce(resolving.promise);
    const dialog = showConflict();
    const keys = user();
    const mine = within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') });
    const remote = within(dialog).getByRole('button', { name: msg('cloudsaves.conflictTakeRemote') });
    mine.focus();

    await keys.keyboard('{Enter}');

    expect(mine.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(mine);
    expect(remote).toHaveProperty('disabled', true);

    const failure = new Error('typhon:cloudsaves.quota_exceeded: full');
    resolving.reject(failure);
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(cloudErrorText(failure));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

describe('CloudConflictModal opened by a launch', () => {
  it('loads the details itself when no status lists them, and shows them', async () => {
    let loaded!: (status: CloudStatus) => void;
    vi.mocked(loadCloudStatus).mockReturnValueOnce(new Promise((resolve) => (loaded = resolve)));
    const { dialog } = askFromLaunch();
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('cloudsaves.conflictLoading'));

    loaded(listed());

    expect(await within(dialog).findByText(deviceLine)).toBeTruthy();
    expect(vi.mocked(loadCloudStatus)).toHaveBeenCalledTimes(1);
  });

  it('says the details could not be loaded, and Retry loads them', async () => {
    const failure = new Error('typhon:cloud.network: offline');
    vi.mocked(loadCloudStatus).mockRejectedValueOnce(failure).mockResolvedValueOnce(listed());
    const { dialog } = askFromLaunch();
    const keys = user();

    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('[data-tone="danger"]') as HTMLElement;
    expect(within(notice).getByRole('alert').textContent?.trim()).toBe(cloudErrorText(failure));
    await keys.click(retry);

    expect(await within(dialog).findByText(deviceLine)).toBeTruthy();
    expect(notice.isConnected).toBe(false);
  });

  it('keeps the notice and the focus on Retry when it fails again, and says it again; "Later" still gives the launch up', async () => {
    vi.mocked(loadCloudStatus).mockRejectedValue(new Error('typhon:cloud.network: offline'));
    const { question, dialog } = askFromLaunch();
    const keys = user();
    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    const said = sayingAgain(notice);
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('cloud.network')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(vi.mocked(loadCloudStatus)).toHaveBeenCalledTimes(2);

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictLater') }));
    expect(await answer(question)).toBe(false);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('starts the next opening from the loading state, with a working Retry, after "Later" during a Retry', async () => {
    const held = deferred<CloudStatus>();
    vi.mocked(loadCloudStatus)
      .mockRejectedValueOnce(new Error('typhon:cloud.network: offline'))
      .mockReturnValueOnce(held.promise)
      .mockReturnValueOnce(new Promise(() => {}));
    const { question, dialog } = askFromLaunch();
    const keys = user();
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.retry') }));
    expect(vi.mocked(loadCloudStatus)).toHaveBeenCalledTimes(2);

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictLater') }));
    expect(await answer(question)).toBe(false);
    held.reject(new Error('typhon:cloud.network: offline'));
    await held.promise.catch(() => undefined);
    // The Retry of the closed dialog has answered before the next opening.
    await new Promise((resolve) => setTimeout(resolve, 0));

    const reopened = deferred<CloudStatus>();
    vi.mocked(loadCloudStatus).mockReset();
    vi.mocked(loadCloudStatus).mockReturnValueOnce(reopened.promise).mockReturnValueOnce(new Promise(() => {}));
    askToResolveConflict('g1');
    const again = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    expect(within(again).getByRole('status').textContent?.trim()).toBe(msg('cloudsaves.conflictLoading'));
    expect(within(again).queryByRole('alert')).toBeNull();

    reopened.reject(new Error('typhon:cloud.network: offline'));
    const retry = await within(again).findByRole('button', { name: msg('common.retry') });
    expect(retry.getAttribute('aria-busy')).toBeNull();
    await keys.click(retry);
    expect(vi.mocked(loadCloudStatus)).toHaveBeenCalledTimes(2);
  });

  it('starts from the loading state when an event moves a retrying dialog to another game', async () => {
    const held = deferred<CloudStatus>();
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    vi.mocked(loadCloudStatus)
      .mockRejectedValueOnce(new Error('typhon:cloud.network: offline'))
      .mockReturnValueOnce(held.promise)
      .mockReturnValueOnce(new Promise(() => {}));
    const question = askToResolveConflict('g1');
    render(CloudConflictModal, { target: app });
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    await user().click(await within(dialog).findByRole('button', { name: msg('common.retry') }));

    openCloudConflict('g2');
    expect(await answer(question)).toBe(false);
    held.reject(new Error('typhon:cloud.network: offline'));
    await held.promise.catch(() => undefined);

    await vi.waitFor(() => expect(within(dialog).getByText(msg('cloudsaves.conflictIntro', { title: 'Celeste' }))).toBeTruthy());
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('cloudsaves.conflictLoading'));
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('shows the dialog’s current game, not a late failure of the game it was showing before', async () => {
    const first = deferred<CloudStatus>();
    const second = deferred<CloudStatus>();
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    vi.mocked(loadCloudStatus).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    askToResolveConflict('g1');
    render(CloudConflictModal, { target: app });
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') });

    openCloudConflict('g2');
    await vi.waitFor(() => expect(vi.mocked(loadCloudStatus)).toHaveBeenCalledTimes(2));
    first.reject(new Error('typhon:cloud.network: offline'));
    await first.promise.catch(() => undefined);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('cloudsaves.conflictLoading'));
    expect(within(dialog).queryByRole('alert')).toBeNull();

    second.resolve(statusWith([{ gameId: 'g2', state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict }]));
    expect(await within(dialog).findByText(deviceLine)).toBeTruthy();
  });

  it('says so when the loaded status lists no conflict for the game, instead of waiting on a spinner', async () => {
    vi.mocked(loadCloudStatus).mockResolvedValueOnce(statusWith([]));
    const { dialog } = askFromLaunch();

    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });

    expect(within(retry.closest('.notice') as HTMLElement).getByRole('alert').textContent?.trim()).toBe(msg('cloudsaves.statusFailed'));
    expect(within(dialog).queryByRole('status')).toBeNull();
  });

  it('answers the launch "resolved" once the choice is applied, and closes', async () => {
    vi.mocked(loadCloudStatus).mockResolvedValueOnce(listed());
    vi.mocked(cloudResolveConflict).mockResolvedValueOnce(undefined);
    const { question, dialog } = askFromLaunch();
    const keys = user();
    await within(dialog).findByText(deviceLine);

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictTakeRemote') }));

    expect(await answer(question)).toBe(true);
    expect(vi.mocked(cloudResolveConflict)).toHaveBeenCalledWith('g1', false);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('answers the launch "given up" on "Later" and on Escape', async () => {
    vi.mocked(loadCloudStatus).mockResolvedValue(listed());
    const first = askFromLaunch();
    const keys = user();
    await within(first.dialog).findByText(deviceLine);
    await keys.click(within(first.dialog).getByRole('button', { name: msg('cloudsaves.conflictLater') }));
    expect(await answer(first.question)).toBe(false);

    const second = askToResolveConflict('g1');
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    await within(dialog).findByText(deviceLine);
    await keys.keyboard('{Escape}');
    expect(await answer(second)).toBe(false);
    expect(vi.mocked(cloudResolveConflict)).not.toHaveBeenCalled();
  });

  it('answers nothing when the choice fails: the dialog stays with the error, and the player can choose again', async () => {
    const failure = new Error('typhon:cloud.server: down');
    vi.mocked(loadCloudStatus).mockResolvedValue(listed());
    vi.mocked(cloudResolveConflict).mockRejectedValueOnce(failure).mockResolvedValueOnce(undefined);
    const { question, dialog } = askFromLaunch();
    const keys = user();
    await within(dialog).findByText(deviceLine);

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') }));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(cloudErrorText(failure));
    expect(await answer(question)).toBe('waiting');
    expect(dialog.isConnected).toBe(true);

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') }));
    expect(await answer(question)).toBe(true);
  });
});

describe('CloudConsentModal', () => {
  it('opens the privacy policy from a text link in its sentence', async () => {
    const onpolicy = vi.fn();
    render(CloudConsentModal, { target: app, props: { open: true, onenable: vi.fn(), onpolicy } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.consentTitle') });
    const link = within(dialog).getByRole('button', { name: msg('cloudsaves.consentPolicyLink') });
    expect(link.classList.contains('text-link')).toBe(true);
    link.focus();

    await keys.keyboard('{Enter}');

    expect(onpolicy).toHaveBeenCalledTimes(1);
  });

  it('keeps the focus on "enable", busy, while it turns cloud saves on', async () => {
    const { rerender } = render(CloudConsentModal, { target: app, props: { open: true, onenable: vi.fn(), onpolicy: vi.fn() } });
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.consentTitle') });
    const enable = within(dialog).getByRole('button', { name: msg('cloudsaves.consentEnable') });
    enable.focus();

    await rerender({ busy: true });

    expect(enable.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(enable);
  });
});

describe('CloudDisableModal', () => {
  it('keeps the focus on the pressed choice, busy, while cloud saves turn off', async () => {
    let busy = false;
    const props = { open: true, busy, ondelete: vi.fn(), onkeep: vi.fn(() => (busy = true)) };
    const { rerender } = render(CloudDisableModal, { target: app, props });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    const keep = within(dialog).getByRole('button', { name: msg('cloudsaves.disableKeep') });
    keep.focus();

    await keys.keyboard('{Enter}');
    await rerender({ busy });

    expect(keep.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(keep);
    expect(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDeleting') })).toHaveProperty('disabled', true);
  });
});
