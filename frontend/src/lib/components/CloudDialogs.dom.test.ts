import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { cloudAllowLaunchOnce, cloudResolveConflict, loadCloudStatus, type CloudConflict, type CloudStatus } from '../services/cloudsaves';
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
  cloudAllowLaunchOnce: vi.fn(),
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
  vi.mocked(cloudAllowLaunchOnce).mockReset();
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

const offlineText = (code = 'cloud.network') => msg('cloudsaves.conflictOffline', { reason: msg(code as 'cloud.network') });
const offline = () => new Error('typhon:cloud.network: network_error');
const listedOffline = (): CloudStatus => ({ ...listed(), error: 'cloud.network' });

function buttonsOf(dialog: HTMLElement) {
  return {
    mine: () => within(dialog).queryByRole('button', { name: msg('cloudsaves.conflictKeepMine') }),
    remote: () => within(dialog).queryByRole('button', { name: msg('cloudsaves.conflictTakeRemote') }),
    play: () => within(dialog).queryByRole('button', { name: msg('cloudsaves.conflictPlayHere') }),
    retry: () => within(dialog).queryByRole('button', { name: msg('common.retry') }),
    later: () => within(dialog).getByRole('button', { name: msg('cloudsaves.conflictLater') }),
  };
}

// The cloud cannot be reached: the dialog says a copy can be chosen only with a connection, keeps the
// copies in view, and offers to try again, to leave, and, when a launch waits on it, to play on this
// computer's saves for that one launch.
describe('CloudConflictModal without the cloud', () => {
  it('is offline at once when the status lists the conflict while the cloud cannot be reached', async () => {
    cloudStatus.set(listedOffline());
    const { question, dialog } = askFromLaunch();
    const buttons = buttonsOf(dialog);

    expect(within(dialog).getByText(offlineText())).toBeTruthy();
    expect(within(dialog).getByText(deviceLine)).toBeTruthy();
    expect(buttons.mine()).toBeNull();
    expect(buttons.remote()).toBeNull();
    expect(buttons.retry()).not.toBeNull();
    const play = buttons.play() as HTMLElement;
    expect(play).not.toBeNull();
    const note = document.getElementById(play.getAttribute('aria-describedby') ?? '');
    expect(note?.textContent?.trim()).toBe(msg('cloudsaves.conflictPlayHereNote'));
    expect(vi.mocked(loadCloudStatus)).not.toHaveBeenCalled();
    expect(await answer(question)).toBe('waiting');
  });

  it('offers no play when no launch waits on it: the explanation, try again and later', () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
    cloudStatus.set(listedOffline());
    openCloudConflict('g1');
    render(CloudConflictModal, { target: app });
    const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    const buttons = buttonsOf(dialog);

    expect(within(dialog).getByText(offlineText())).toBeTruthy();
    expect(buttons.play()).toBeNull();
    expect(within(dialog).queryByText(msg('cloudsaves.conflictPlayHereNote'))).toBeNull();
    expect(buttons.retry()).not.toBeNull();
    expect(buttons.later()).toBeTruthy();
  });

  it('goes offline when a choice fails without the cloud, keeps the copies, and hands the focus to try again', async () => {
    cloudStatus.set(listed());
    vi.mocked(cloudResolveConflict).mockRejectedValueOnce(offline());
    const { question, dialog } = askFromLaunch();
    const buttons = buttonsOf(dialog);
    const keys = user();
    (buttons.mine() as HTMLElement).focus();

    await keys.keyboard('{Enter}');

    expect(await within(dialog).findByText(offlineText())).toBeTruthy();
    expect(within(dialog).getByText(deviceLine)).toBeTruthy();
    expect(buttons.mine()).toBeNull();
    expect(within(dialog).queryByRole('alert')).toBeNull();
    await vi.waitFor(() => expect(document.activeElement).toBe(buttons.retry()));
    expect(await answer(question)).toBe('waiting');
  });

  it('goes offline when the account cannot be confirmed without the server, with that reason', async () => {
    cloudStatus.set(listed());
    vi.mocked(cloudResolveConflict).mockRejectedValueOnce(new Error('typhon:cloud.account_unknown: not bound'));
    const { dialog } = askFromLaunch();

    await user().click(buttonsOf(dialog).remote() as HTMLElement);

    expect(await within(dialog).findByText(offlineText('cloud.account_unknown'))).toBeTruthy();
    expect(buttonsOf(dialog).play()).not.toBeNull();
  });

  it('tries the failed choice again: with the cloud back it is applied and the launch answered "resolved"', async () => {
    cloudStatus.set(listed());
    vi.mocked(cloudResolveConflict).mockRejectedValueOnce(offline()).mockResolvedValueOnce(undefined);
    const { question, dialog } = askFromLaunch();
    const keys = user();
    await keys.click(buttonsOf(dialog).mine() as HTMLElement);
    await within(dialog).findByText(offlineText());

    await keys.click(buttonsOf(dialog).retry() as HTMLElement);

    expect(await answer(question)).toBe(true);
    expect(vi.mocked(cloudResolveConflict).mock.calls).toEqual([
      ['g1', true],
      ['g1', true],
    ]);
    expect(get(toasts).map((item) => item.message)).toEqual([msg('cloudsaves.conflictKeptMine')]);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('leaves the offline state the status put it in on try again, and the choice works with the cloud back', async () => {
    cloudStatus.set(listedOffline());
    vi.mocked(cloudResolveConflict).mockResolvedValueOnce(undefined);
    const { question, dialog } = askFromLaunch();
    const keys = user();
    const retry = buttonsOf(dialog).retry() as HTMLElement;
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(within(dialog).queryByText(offlineText())).toBeNull());
    const remote = buttonsOf(dialog).remote() as HTMLElement;
    expect(remote).not.toBeNull();
    expect(document.activeElement).toBe(buttonsOf(dialog).mine());
    expect(vi.mocked(cloudResolveConflict)).not.toHaveBeenCalled();

    await keys.click(remote);
    expect(await answer(question)).toBe(true);
    expect(vi.mocked(cloudResolveConflict)).toHaveBeenCalledWith('g1', false);
  });

  it('keeps the offline state and the focus on try again when the cloud is still out of reach, and says it again', async () => {
    cloudStatus.set(listed());
    vi.mocked(cloudResolveConflict).mockRejectedValue(offline());
    const { question, dialog } = askFromLaunch();
    const keys = user();
    await keys.click(buttonsOf(dialog).mine() as HTMLElement);
    await within(dialog).findByText(offlineText());
    const retry = buttonsOf(dialog).retry() as HTMLElement;
    const notice = retry.closest('.notice') as HTMLElement;
    const said = sayingAgain(notice);
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(offlineText()));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(vi.mocked(cloudResolveConflict)).toHaveBeenCalledTimes(2);
    expect(await answer(question)).toBe('waiting');
  });

  it('plays on this computer: grants the one launch, answers it "resolved", and says nothing about a resolved conflict', async () => {
    cloudStatus.set(listedOffline());
    vi.mocked(cloudAllowLaunchOnce).mockResolvedValueOnce(undefined);
    const { question, dialog } = askFromLaunch();

    await user().click(buttonsOf(dialog).play() as HTMLElement);

    expect(await answer(question)).toBe(true);
    expect(vi.mocked(cloudAllowLaunchOnce)).toHaveBeenCalledWith('g1');
    expect(vi.mocked(cloudResolveConflict)).not.toHaveBeenCalled();
    expect(get(toasts)).toEqual([]);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('keeps the focus on play, busy, while the permission is asked for; leaving waits for it', async () => {
    const asking = deferred<void>();
    cloudStatus.set(listedOffline());
    vi.mocked(cloudAllowLaunchOnce).mockReturnValueOnce(asking.promise);
    const { question, dialog } = askFromLaunch();
    const keys = user();
    const play = buttonsOf(dialog).play() as HTMLElement;
    play.focus();

    await keys.keyboard('{Enter}');

    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(play);
    expect(buttonsOf(dialog).later()).toHaveProperty('disabled', true);
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);
    await keys.keyboard('{Enter}');
    expect(vi.mocked(cloudAllowLaunchOnce)).toHaveBeenCalledTimes(1);

    asking.resolve();
    expect(await answer(question)).toBe(true);
  });

  it('says a refused permission in the dialog and leaves the launch waiting', async () => {
    const refused = new Error('typhon:cloud.no_conflict: none');
    cloudStatus.set(listedOffline());
    vi.mocked(cloudAllowLaunchOnce).mockRejectedValueOnce(refused);
    const { question, dialog } = askFromLaunch();
    const keys = user();
    const play = buttonsOf(dialog).play() as HTMLElement;
    play.focus();

    await keys.keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(cloudErrorText(refused));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(await answer(question)).toBe('waiting');
    expect(play.getAttribute('aria-busy')).toBeNull();
    expect(document.activeElement).toBe(play);
  });

  it('gives the launch up on "Later"', async () => {
    cloudStatus.set(listedOffline());
    const { question, dialog } = askFromLaunch();

    await user().click(buttonsOf(dialog).later());

    expect(await answer(question)).toBe(false);
    expect(vi.mocked(cloudAllowLaunchOnce)).not.toHaveBeenCalled();
  });

  it('goes offline when the details cannot be loaded without the cloud, and still offers play to the launch', async () => {
    vi.mocked(loadCloudStatus).mockRejectedValueOnce(offline()).mockResolvedValueOnce(listed());
    const { dialog } = askFromLaunch();
    const keys = user();

    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });
    expect(buttonsOf(dialog).play()).not.toBeNull();
    expect(buttonsOf(dialog).mine()).toBeNull();

    await keys.click(retry);

    expect(await within(dialog).findByText(deviceLine)).toBeTruthy();
    expect(within(dialog).getByText(offlineText())).toBeTruthy();
  });

  it('starts every opening from the choice: an earlier offline state is not carried over', async () => {
    cloudStatus.set(listed());
    vi.mocked(cloudResolveConflict).mockRejectedValueOnce(offline());
    const first = askFromLaunch();
    const keys = user();
    await keys.click(buttonsOf(first.dialog).mine() as HTMLElement);
    await within(first.dialog).findByText(offlineText());
    await keys.click(buttonsOf(first.dialog).later());
    expect(await answer(first.question)).toBe(false);

    const second = askToResolveConflict('g1');
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });

    expect(within(dialog).queryByText(offlineText())).toBeNull();
    expect(buttonsOf(dialog).mine()).not.toBeNull();
    expect(buttonsOf(dialog).play()).toBeNull();
    expect(await answer(second)).toBe('waiting');
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
