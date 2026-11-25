import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/lan', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/lan')>()),
  // The screen reads the offers again while it is shown: Go answers what the test set.
  getOffers: vi.fn(async () => get((await import('../../lib/stores/lan')).offers)),
  receive: vi.fn(),
  unshare: vi.fn(),
  cancel: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { cancel, receive, unshare, type Transfer } from '../../lib/services/lan';
import { offers, shares, transfers } from '../../lib/stores/lan';
import { toasts } from '../../lib/stores/toasts';
import { renderDom, user } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import Lan from './Lan.svelte';
import { offer, ready, resetScreen, sectionOf, share, transfer } from './lanTesting';

const ALREADY = 'typhon:lan.already_have: lan: this game is already shared or being received';
const UNKNOWN = 'typhon:lan.unknown_transfer: lan: unknown transfer';

let scrolled: Element[];

beforeEach(() => {
  ready();
  scrolled = [];
  Element.prototype.scrollIntoView = vi.fn(function (this: Element) {
    scrolled.push(this);
  });
});

afterEach(() => {
  resetScreen();
  vi.mocked(receive).mockReset();
  vi.mocked(unshare).mockReset();
  vi.mocked(cancel).mockReset();
});

const said = () => get(toasts).map((item) => [item.kind, item.message]);
const download = (title: string, host = 'PC-1') => screen.getByRole('button', { name: msg('transfers.lanDownloadFor', { title, host }) });
const stop = (title: string) => screen.getByRole('button', { name: msg('transfers.lanStopSharingFor', { title }) });
const cancelOf = (title: string) => screen.getByRole('button', { name: msg('transfers.lanCancelFor', { title }) });
const cancelDialog = (title: string) => screen.queryByRole('dialog', { name: msg('transfers.lanCancelTitle', { title }) });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('Lan: Download', () => {
  it('is busy from the press to Go’s answer: a second press asks nothing more', async () => {
    const answer = deferred<Transfer>();
    vi.mocked(receive).mockReturnValue(answer.promise);
    offers.set([offer()]);
    renderDom(Lan);
    const keys = user();
    download('Portal 2').focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(download('Portal 2').getAttribute('aria-busy')).toBe('true');
    expect(receive).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(download('Portal 2'));

    answer.resolve(transfer({ id: 'new', title: 'Portal 2', infoHash: 'a'.repeat(40) }));
    await waitFor(() => expect(download('Portal 2').getAttribute('aria-busy')).not.toBe('true'));
    expect(within(sectionOf('transfers.lanTransfersHeading')).getByRole('progressbar', { name: 'Portal 2' })).toBeTruthy();
  });

  it('a refusal is said once in the words of its code, and the row stays as it was', async () => {
    vi.mocked(receive).mockRejectedValue(new Error(ALREADY));
    offers.set([offer()]);
    const { user: keys } = renderDom(Lan);

    await keys.click(download('Portal 2'));

    await waitFor(() => expect(said()).toEqual([['danger', msg('errSources.lanAlreadyHave')]]));
    expect(download('Portal 2').getAttribute('aria-busy')).not.toBe('true');
    expect(screen.getByText(msg('transfers.lanNoTransfers'))).toBeTruthy();
  });
});

describe('Lan: Stop sharing', () => {
  // It deletes nothing (Go keeps the files and the prepared torrent): it acts at once, as a pause does.
  it('acts at once, without a confirmation, and is busy until Go answers', async () => {
    const answer = deferred<void>();
    vi.mocked(unshare).mockReturnValue(answer.promise);
    shares.set([share()]);
    const { user: keys } = renderDom(Lan);

    await keys.click(stop('Hades'));
    await keys.click(stop('Hades'));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(unshare).toHaveBeenCalledTimes(1);
    expect(stop('Hades').getAttribute('aria-busy')).toBe('true');
    answer.resolve();
    await waitFor(() => expect(screen.queryByRole('button', { name: msg('transfers.lanStopSharingFor', { title: 'Hades' }) })).toBeNull());
  });

  it('focus goes to the same control of the row that took the place of the one that left', async () => {
    vi.mocked(unshare).mockResolvedValue(undefined);
    shares.set([share(), share({ gameId: 'g2', title: 'Inside' }), share({ gameId: 'g3', title: 'Limbo' })]);
    renderDom(Lan);
    const keys = user();
    stop('Hades').focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(document.activeElement).toBe(stop('Inside')));
    expect(scrolled).toContain(stop('Inside').closest('[data-lan-row]'));
  });

  it('focus goes to the row above when the last row left', async () => {
    vi.mocked(unshare).mockResolvedValue(undefined);
    shares.set([share(), share({ gameId: 'g2', title: 'Inside' })]);
    renderDom(Lan);
    const keys = user();
    stop('Inside').focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(document.activeElement).toBe(stop('Hades')));
  });

  it('focus goes to the page heading when the section has nothing left to focus', async () => {
    vi.mocked(unshare).mockResolvedValue(undefined);
    shares.set([share()]);
    renderDom(Lan);
    const keys = user();
    stop('Hades').focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });

  it('a refusal keeps the row and the focus on it, said in the words of its code', async () => {
    vi.mocked(unshare).mockRejectedValue(new Error('typhon:lan.sharing_disabled: lan: sharing is disabled'));
    shares.set([share()]);
    renderDom(Lan);
    const keys = user();
    stop('Hades').focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(said()).toEqual([['danger', msg('errSources.lanSharingDisabled')]]));
    expect(document.activeElement).toBe(stop('Hades'));
  });
});

describe('Lan: rows that leave by Go’s doing', () => {
  it('an offer Go no longer knows takes focus to the same control of the offer now in its place', async () => {
    offers.set([offer(), offer({ title: 'Inside', infoHash: 'e'.repeat(40) })]);
    renderDom(Lan);
    download('Portal 2').focus();

    offers.set([offer({ title: 'Inside', infoHash: 'e'.repeat(40) })]);

    await waitFor(() => expect(document.activeElement).toBe(download('Inside')));
  });

  it('a second computer appearing regroups the offers and focus stays on the same offer’s Download', async () => {
    offers.set([offer()]);
    renderDom(Lan);
    download('Portal 2').focus();

    offers.set([offer(), offer({ peerId: 'p2', host: 'PC-2', title: 'Inside', infoHash: 'e'.repeat(40) })]);

    await waitFor(() => expect(within(sectionOf('transfers.lanNearbyHeading')).getAllByRole('heading', { level: 3 })).toHaveLength(2));
    await waitFor(() => expect(document.activeElement).toBe(download('Portal 2')));
  });

  it('a transfer that ended takes focus to the next transfer’s Cancel', async () => {
    transfers.set([transfer(), transfer({ id: 't2', title: 'Inside' })]);
    renderDom(Lan);
    cancelOf('Celeste').focus();

    transfers.set([transfer({ id: 't2', title: 'Inside' })]);

    await waitFor(() => expect(document.activeElement).toBe(cancelOf('Inside')));
  });
});

describe('Lan: Cancel', () => {
  it('with nothing received yet acts at once', async () => {
    vi.mocked(cancel).mockResolvedValue(undefined);
    transfers.set([transfer({ downloaded: 0 })]);
    const { user: keys } = renderDom(Lan);

    await keys.click(cancelOf('Celeste'));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(cancel).toHaveBeenCalledExactlyOnceWith('t1');
  });

  it('with data received asks first, saying what is deleted; keeping it asks Go nothing and focus comes back', async () => {
    transfers.set([transfer({ downloaded: 50 * 1024 ** 2 })]);
    renderDom(Lan);
    const keys = user();
    cancelOf('Celeste').focus();

    await keys.keyboard('{Enter}');

    const dialog = cancelDialog('Celeste') as HTMLElement;
    expect(dialog.textContent).toContain(msg('transfers.lanCancelText', { size: bytesSize(50 * 1024 ** 2) }));
    await keys.click(within(dialog).getByRole('button', { name: msg('ui.dontCancel') }));

    await waitFor(() => expect(cancelDialog('Celeste')).toBeNull());
    expect(cancel).not.toHaveBeenCalled();
    await waitFor(() => expect(document.activeElement).toBe(cancelOf('Celeste')));
  });

  it('with data received, confirming asks Go to cancel; a refusal stays in the dialog in the words of its code', async () => {
    vi.mocked(cancel).mockRejectedValueOnce(new Error(UNKNOWN)).mockResolvedValueOnce(undefined);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    transfers.set([transfer({ downloaded: 10 })]);
    const { user: keys } = renderDom(Lan);

    await keys.click(cancelOf('Celeste'));
    const dialog = cancelDialog('Celeste') as HTMLElement;
    await keys.click(within(dialog).getByRole('button', { name: msg('transfers.lanCancelConfirm') }));

    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toContain(msg('errSources.lanUnknownTransfer')));
    expect(said()).toEqual([]);
    await keys.click(within(dialog).getByRole('button', { name: msg('common.retry') }));

    await waitFor(() => expect(cancelDialog('Celeste')).toBeNull());
    expect(cancel).toHaveBeenCalledTimes(2);
    expect(cancel).toHaveBeenLastCalledWith('t1');
    vi.mocked(console.error).mockRestore();
  });

  it('is busy from the press to Go’s answer', async () => {
    const answer = deferred<void>();
    vi.mocked(cancel).mockReturnValue(answer.promise);
    transfers.set([transfer({ downloaded: 0 })]);
    const { user: keys } = renderDom(Lan);

    await keys.click(cancelOf('Celeste'));
    await keys.click(cancelOf('Celeste'));

    expect(cancel).toHaveBeenCalledTimes(1);
    expect(cancelOf('Celeste').getAttribute('aria-busy')).toBe('true');
    answer.resolve();
    await waitFor(() => expect(cancelOf('Celeste').getAttribute('aria-busy')).not.toBe('true'));
  });
});
