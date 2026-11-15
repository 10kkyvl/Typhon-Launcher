import { fireEvent, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { followOrder } from '../../lib/downloads/order';
import { msg } from '../../lib/i18n';
import { installErrorText } from '../../lib/install/installErrors';
import { cancelDownload, downloadNow, forceStartDownload, moveDownloadTo, removeDownload, type Download } from '../../lib/services/downloads';
import { idlePlayThrottle } from '../../lib/services/playThrottle';
import { downloads } from '../../lib/stores/downloads';
import { playThrottle } from '../../lib/stores/playThrottle';
import { resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { sources } from '../../lib/stores/sources';
import type { Source } from '../../lib/services/sources';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import QueueList from './QueueList.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  moveDownloadTo: vi.fn(),
  cancelDownload: vi.fn(),
  removeDownload: vi.fn(),
  downloadNow: vi.fn(),
  forceStartDownload: vi.fn(),
}));

function queued(id: string, name: string, patch: Partial<Download> = {}): Download {
  return {
    id,
    name,
    type: 'torrent',
    source: '',
    infoHash: id,
    destination: `D:/Games/${name}`,
    status: 'queued',
    progress: 0,
    downloaded: 0,
    total: 4 * 1024 ** 3,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    addedAt: '2026-11-01T00:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...patch,
  };
}

function deferred() {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// What Go sends: its whole order after a move, the id of a download it dropped.
const goReorders = (ids: string[]) => downloads.update((list) => followOrder(list, ids));
const goRemoves = (id: string) => downloads.update((list) => list.filter((d) => d.id !== id));

const gripName = (title: string, position: number, total: number) => msg('downloads.queue.grip', { title, position, total });
const grip = (title: string) => screen.getByRole('button', { name: new RegExp(`^«${title}», `) });
/** The grips in the order the list shows them, by their names: who stands where, as the list says it. */
const shownOrder = () => screen.getAllByRole('button', { name: /^«.+», позиция \d+ из \d+$/ }).map((b) => b.getAttribute('aria-label'));
const cancelX = (title: string) => screen.getByRole('button', { name: msg('downloads.queue.cancelFor', { title }) });

let onannounce: ReturnType<typeof vi.fn<(text: string) => void>>;
let onopen: ReturnType<typeof vi.fn<(id: string) => void>>;
let heading: HTMLElement;

function mount() {
  return renderDom(QueueList, { onannounce, onopen });
}

beforeEach(() => {
  vi.mocked(moveDownloadTo).mockReset();
  vi.mocked(cancelDownload).mockReset();
  vi.mocked(removeDownload).mockReset();
  vi.mocked(downloadNow).mockReset();
  vi.mocked(forceStartDownload).mockReset();
  onannounce = vi.fn<(text: string) => void>();
  onopen = vi.fn<(id: string) => void>();
  heading = document.createElement('h1');
  heading.tabIndex = -1;
  heading.textContent = 'Загрузки';
  document.body.append(heading);
});

afterEach(() => {
  downloads.set([]);
  sources.set([]);
  toasts.set([]);
  playThrottle.set(idlePlayThrottle);
  resetHistory();
  document.body.replaceChildren();
});

describe('QueueList: the grip', () => {
  it('is a button named by the download and its place, described by the keys it takes', () => {
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    mount();

    const g = grip('Gamma');
    expect(g.getAttribute('aria-label')).toBe(gripName('Gamma', 3, 3));
    const help = document.getElementById(g.getAttribute('aria-describedby') ?? '');
    expect(help?.textContent).toBe(msg('downloads.queue.help'));
  });
});

describe('QueueList: reorder from the keyboard', () => {
  it('Arrow Up sends one moveTo with the new place, shows it at once, says it when Go confirms; focus stays on the grip', async () => {
    const call = deferred();
    vi.mocked(moveDownloadTo).mockReturnValue(call.promise);
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    const { user } = mount();
    grip('Gamma').focus();

    await user.keyboard('{ArrowUp}');

    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['c', 1]]);
    expect(shownOrder()).toEqual([gripName('Alpha', 1, 3), gripName('Gamma', 2, 3), gripName('Beta', 3, 3)]);
    expect(onannounce).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(document.activeElement).toBe(grip('Gamma')));

    // Go's answer first: the asked order stays on screen until Go's order arrives, and nothing is said yet.
    call.resolve();
    await new Promise((done) => setTimeout(done, 0));
    expect(onannounce).not.toHaveBeenCalled();
    expect(shownOrder()).toEqual([gripName('Alpha', 1, 3), gripName('Gamma', 2, 3), gripName('Beta', 3, 3)]);
    goReorders(['a', 'c', 'b']);

    await vi.waitFor(() => expect(onannounce).toHaveBeenCalledWith(msg('downloads.queue.moved', { title: 'Gamma', position: 2, total: 3 })));
    expect(onannounce).toHaveBeenCalledTimes(1);
    expect(shownOrder()).toEqual([gripName('Alpha', 1, 3), gripName('Gamma', 2, 3), gripName('Beta', 3, 3)]);
    expect(document.activeElement).toBe(grip('Gamma'));
  });

  it('says the move when Go’s order arrives before its answer too', async () => {
    const call = deferred();
    vi.mocked(moveDownloadTo).mockReturnValue(call.promise);
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    const { user } = mount();
    grip('Alpha').focus();

    await user.keyboard('{End}');
    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['a', 2]]);
    goReorders(['b', 'c', 'a']);
    expect(onannounce).not.toHaveBeenCalled();
    call.resolve();

    await vi.waitFor(() => expect(onannounce).toHaveBeenCalledWith(msg('downloads.queue.moved', { title: 'Alpha', position: 3, total: 3 })));
    expect(document.activeElement).toBe(grip('Alpha'));
  });

  it('Home and End ask for the ends; a key at the end it already holds sends nothing', async () => {
    vi.mocked(moveDownloadTo).mockResolvedValue();
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    const { user } = mount();

    grip('Alpha').focus();
    await user.keyboard('{Home}{ArrowUp}');
    expect(moveDownloadTo).not.toHaveBeenCalled();

    grip('Beta').focus();
    await user.keyboard('{Home}');
    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['b', 0]]);
  });

  it('a move Go refuses puts Go’s order back and says why by its code, not as a toast; focus stays on the grip', async () => {
    const err = new Error('typhon:download.unavailable');
    vi.mocked(moveDownloadTo).mockRejectedValue(err);
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    const { user } = mount();
    grip('Gamma').focus();

    await user.keyboard('{ArrowUp}');

    await vi.waitFor(() =>
      expect(onannounce).toHaveBeenCalledWith(msg('downloads.queue.moveFailed', { title: 'Gamma', reason: installErrorText(err) })),
    );
    expect(shownOrder()).toEqual([gripName('Alpha', 1, 3), gripName('Beta', 2, 3), gripName('Gamma', 3, 3)]);
    expect(get(toasts)).toEqual([]);
    await vi.waitFor(() => expect(document.activeElement).toBe(grip('Gamma')));
  });

  it('held keys send one move at a time, and the last place asked wins', async () => {
    const first = deferred();
    const second = deferred();
    vi.mocked(moveDownloadTo).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma'), queued('d', 'Delta')]);
    const { user } = mount();
    grip('Delta').focus();

    await user.keyboard('{ArrowUp}{ArrowUp}{ArrowUp}');

    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['d', 2]]);
    expect(shownOrder()[0]).toBe(gripName('Delta', 1, 4));

    first.resolve();
    goReorders(['a', 'b', 'd', 'c']);
    await vi.waitFor(() => expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['d', 2], ['d', 0]]));
    expect(shownOrder()[0]).toBe(gripName('Delta', 1, 4));
    expect(onannounce).not.toHaveBeenCalled();

    second.resolve();
    goReorders(['d', 'a', 'b', 'c']);
    await vi.waitFor(() => expect(onannounce).toHaveBeenCalledWith(msg('downloads.queue.moved', { title: 'Delta', position: 1, total: 4 })));
    expect(onannounce).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(grip('Delta'));
  });
});

describe('QueueList: to top', () => {
  it('is offered on every row but the first, sends moveTo place 0, and focus follows the download to its grip', async () => {
    vi.mocked(moveDownloadTo).mockResolvedValue();
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    const { user } = mount();

    expect(screen.queryByRole('button', { name: msg('downloads.queue.toTopFor', { title: 'Alpha' }) })).toBeNull();
    const top = screen.getByRole('button', { name: msg('downloads.queue.toTopFor', { title: 'Gamma' }) });
    top.focus();
    await user.keyboard('{Enter}');

    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['c', 0]]);
    await vi.waitFor(() => expect(document.activeElement).toBe(grip('Gamma')));
    expect(screen.queryByRole('button', { name: msg('downloads.queue.toTopFor', { title: 'Gamma' }) })).toBeNull();

    goReorders(['c', 'a', 'b']);
    await vi.waitFor(() => expect(onannounce).toHaveBeenCalledWith(msg('downloads.queue.moved', { title: 'Gamma', position: 1, total: 3 })));
  });
});

describe('QueueList: pointer drag on the grip', () => {
  // Rows of 50 px, 10 px apart: middles at 25, 85, 145.
  beforeEach(() => {
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
      const row = this.closest('li');
      const index = row?.parentElement ? Array.from(row.parentElement.children).indexOf(row) : 0;
      return new DOMRect(0, index * 60, 400, 50);
    });
    HTMLElement.prototype.setPointerCapture = vi.fn();
    HTMLElement.prototype.releasePointerCapture = vi.fn();
    HTMLElement.prototype.hasPointerCapture = vi.fn(() => true);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const rowOf = (title: string) => grip(title).closest('li') as HTMLElement;

  async function lift(title: string, dy: number) {
    const g = grip(title);
    const start = 25 + 60 * Array.from((rowOf(title).parentElement as HTMLElement).children).indexOf(rowOf(title));
    await fireEvent.pointerDown(g, { pointerId: 1, button: 0, clientY: start });
    await fireEvent.pointerMove(g, { pointerId: 1, clientY: start + dy });
    return { g, at: start + dy };
  }

  it('the row follows the pointer, the others make room, and a drop sends one moveTo', async () => {
    vi.mocked(moveDownloadTo).mockResolvedValue();
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    mount();

    const { g, at } = await lift('Gamma', -125);

    expect(g.setPointerCapture).toHaveBeenCalledWith(1);
    expect(rowOf('Gamma').style.transform).toBe('translateY(-125px)');
    expect(rowOf('Alpha').style.transform).toBe('translateY(60px)');
    expect(rowOf('Beta').style.transform).toBe('translateY(60px)');
    expect(moveDownloadTo).not.toHaveBeenCalled();

    await fireEvent.pointerUp(g, { pointerId: 1, clientY: at });

    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['c', 0]]);
    expect(shownOrder()).toEqual([gripName('Gamma', 1, 3), gripName('Alpha', 2, 3), gripName('Beta', 3, 3)]);
    for (const title of ['Alpha', 'Beta', 'Gamma']) expect(rowOf(title).style.transform, title).toBe('');
  });

  it('Escape cancels the drag: nothing moves and nothing is sent', async () => {
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    mount();

    const { g, at } = await lift('Gamma', -125);
    await fireEvent.keyDown(g, { key: 'Escape' });
    await fireEvent.pointerUp(g, { pointerId: 1, clientY: at });

    expect(moveDownloadTo).not.toHaveBeenCalled();
    expect(rowOf('Alpha').style.transform).toBe('');
    expect(shownOrder()).toEqual([gripName('Alpha', 1, 3), gripName('Beta', 2, 3), gripName('Gamma', 3, 3)]);
  });

  it('a lost capture or a cancelled pointer ends the drag with nothing sent', async () => {
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    mount();

    let held = await lift('Gamma', -125);
    await fireEvent(held.g, new PointerEvent('lostpointercapture', { pointerId: 1 }));
    await fireEvent.pointerUp(held.g, { pointerId: 1, clientY: held.at });

    held = await lift('Gamma', -125);
    await fireEvent.pointerCancel(held.g, { pointerId: 1 });
    await fireEvent.pointerUp(held.g, { pointerId: 1, clientY: held.at });

    expect(moveDownloadTo).not.toHaveBeenCalled();
    expect(rowOf('Beta').style.transform).toBe('');
  });

  it('a drop where the drag began sends nothing', async () => {
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    mount();

    const { g, at } = await lift('Beta', 20);
    await fireEvent.pointerUp(g, { pointerId: 1, clientY: at });

    expect(moveDownloadTo).not.toHaveBeenCalled();
  });

  it('a list that changes under the pointer cancels the drag, and the drop sends nothing', async () => {
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    mount();

    const { g, at } = await lift('Gamma', -125);
    downloads.update((list) => list.map((d) => (d.id === 'a' ? { ...d, status: 'downloading' } : d)));
    await vi.waitFor(() => expect(rowOf('Beta').style.transform).toBe(''));
    await fireEvent.pointerUp(g, { pointerId: 1, clientY: at });

    expect(moveDownloadTo).not.toHaveBeenCalled();
  });

  it('while the move is in flight the list shows the asked order, then Go’s when Go refuses it', async () => {
    const call = deferred();
    vi.mocked(moveDownloadTo).mockReturnValue(call.promise);
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    mount();

    const { g, at } = await lift('Alpha', 130);
    await fireEvent.pointerUp(g, { pointerId: 1, clientY: at });
    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['a', 2]]);
    expect(shownOrder()).toEqual([gripName('Beta', 1, 3), gripName('Gamma', 2, 3), gripName('Alpha', 3, 3)]);

    call.reject(new Error('typhon:download.unavailable'));

    await vi.waitFor(() =>
      expect(shownOrder()).toEqual([gripName('Alpha', 1, 3), gripName('Beta', 2, 3), gripName('Gamma', 3, 3)]),
    );
  });
});

describe('QueueList: cancel', () => {
  it('a download with data on disk asks first, naming it and what is deleted, then sends Go’s Cancel, not Remove', async () => {
    vi.mocked(cancelDownload).mockResolvedValue();
    downloads.set([queued('a', 'Alpha', { downloaded: 300 * 1024 ** 2, progress: 0.07 }), queued('b', 'Beta')]);
    const { user } = mount();

    cancelX('Alpha').focus();
    await user.keyboard('{Enter}');

    const dialog = await screen.findByRole('dialog', { name: msg('downloads.cancel.title', { name: 'Alpha' }) });
    expect(dialog.textContent).toContain(msg('downloads.cancel.deletes', { size: bytesSize(300 * 1024 ** 2) }));
    expect(cancelDownload).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: msg('ui.cancelDownload') }));

    expect(vi.mocked(cancelDownload).mock.calls).toEqual([['a']]);
    expect(removeDownload).not.toHaveBeenCalled();
  });

  it('a download that keeps its files on Cancel (in place) says the files stay, and where', async () => {
    downloads.set([queued('a', 'Alpha', { downloaded: 1024 ** 2, destination: 'D:/Games/Alpha', inPlace: true } as Partial<Download>)]);
    const { user } = mount();

    await user.click(cancelX('Alpha'));

    const dialog = await screen.findByRole('dialog');
    expect(dialog.textContent).toContain(msg('downloads.cancel.keeps', { size: bytesSize(1024 ** 2), path: 'D:/Games/Alpha' }));
    expect(dialog.textContent).not.toContain(msg('downloads.cancel.deletes', { size: bytesSize(1024 ** 2) }));
  });

  it('when Go refuses, the dialog stays open with the error by its code, and the row stays', async () => {
    const err = new Error('typhon:download.not_found');
    vi.mocked(cancelDownload).mockRejectedValue(err);
    downloads.set([queued('a', 'Alpha', { downloaded: 1024 })]);
    const { user } = mount();
    vi.spyOn(console, 'error').mockImplementation(() => {});

    await user.click(cancelX('Alpha'));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('ui.cancelDownload') }));

    await vi.waitFor(() => expect(dialog.textContent).toContain(installErrorText(err)));
    expect(screen.getByRole('dialog')).toBe(dialog);
    expect(cancelX('Alpha')).toBeTruthy();
    expect(onannounce).not.toHaveBeenCalled();
  });

  it('a download with nothing on disk is cancelled at once, with no dialog, and the cancel is said', async () => {
    vi.mocked(cancelDownload).mockResolvedValue();
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta')]);
    const { user } = mount();

    await user.click(cancelX('Beta'));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(vi.mocked(cancelDownload).mock.calls).toEqual([['b']]);
    expect(removeDownload).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(onannounce).toHaveBeenCalledWith(msg('downloads.cancel.done', { name: 'Beta' })));
  });

  it('a double press sends one cancel', async () => {
    const call = deferred();
    vi.mocked(cancelDownload).mockReturnValue(call.promise);
    downloads.set([queued('a', 'Alpha')]);
    const { user } = mount();
    const x = cancelX('Alpha');
    x.focus();

    await user.keyboard('{Enter}{Enter}');

    expect(cancelDownload).toHaveBeenCalledTimes(1);
    expect(x.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(x);
    call.resolve();
    await vi.waitFor(() => expect(onannounce).toHaveBeenCalled());
  });

  it('focus goes to the same control of the row now in its place, else the one above, else the page heading', async () => {
    vi.mocked(cancelDownload).mockResolvedValue();
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta'), queued('c', 'Gamma')]);
    const { user } = mount();

    cancelX('Beta').focus();
    await user.keyboard('{Enter}');
    goRemoves('b');
    await vi.waitFor(() => expect(document.activeElement).toBe(cancelX('Gamma')));

    await user.keyboard('{Enter}');
    goRemoves('c');
    await vi.waitFor(() => expect(document.activeElement).toBe(cancelX('Alpha')));

    await user.keyboard('{Enter}');
    goRemoves('a');
    await vi.waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it('after a confirmed cancel the dialog hands focus to the row now in its place', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id: string) => {
      goRemoves(id);
    });
    downloads.set([queued('a', 'Alpha', { downloaded: 1024 }), queued('b', 'Beta')]);
    const { user } = mount();

    cancelX('Alpha').focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('ui.cancelDownload') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(cancelX('Beta')));
  });
});

describe('QueueList: a row', () => {
  it('says its place, its state in words and how much is already downloaded', () => {
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta', { downloaded: 1024, progress: 0.42 })]);
    mount();

    const [first, second] = screen.getAllByRole('listitem');
    expect(first.textContent).toContain(msg('downloads.queue.position', { n: 1 }));
    expect(first.textContent).toContain(msg('downloads.queue.stateNext'));
    expect(first.textContent).not.toContain('%');
    expect(second.textContent).toContain(msg('downloads.queue.position', { n: 2 }));
    expect(second.textContent).toContain(msg('downloads.queue.stateWaiting'));
    expect(second.textContent).toContain(bytesSize(4 * 1024 ** 3));
    expect(within(first).queryByRole('progressbar')).toBeNull();
  });

  it('says the part already downloaded once: a bar named for the download, its percent drawn beside it', () => {
    downloads.set([queued('b', 'Beta', { downloaded: 1024, progress: 0.42 })]);
    mount();

    const bar = screen.getByRole('progressbar', { name: msg('downloads.queue.downloadedFor', { title: 'Beta' }) });
    expect(bar.getAttribute('aria-valuenow')).toBe('0.42');
    expect(bar.getAttribute('aria-valuemax')).toBe('1');
    const percent = screen.getByText(msg('downloads.queue.downloaded', { percent: 42 }));
    expect(percent.closest('[aria-hidden="true"]')).not.toBeNull();
  });

  it('while the schedule holds the queue: “waiting for the schedule”, Download now, and no pointless Start now', async () => {
    vi.mocked(downloadNow).mockResolvedValue();
    playThrottle.set({ ...idlePlayThrottle, active: true, schedule: { blocked: true, nextOpenAt: '' } });
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta')]);
    const { user } = mount();

    for (const row of screen.getAllByRole('listitem')) expect(row.textContent).toContain(msg('downloads.queue.stateSchedule'));
    await user.click(screen.getByRole('button', { name: msg('transfers.downloadsQueueActionsFor', { name: 'Beta' }) }));
    expect(screen.queryByRole('menuitem', { name: msg('transfers.downloadsStartNow') })).toBeNull();
    await user.keyboard('{Escape}');

    await user.click(screen.getByRole('button', { name: msg('downloads.queue.downloadNowFor', { title: 'Beta' }) }));
    expect(vi.mocked(downloadNow).mock.calls).toEqual([['b']]);
  });

  it('with no hold: no Download now; the menu has Details, Start now and the moves that fit its place', async () => {
    vi.mocked(forceStartDownload).mockResolvedValue();
    vi.mocked(moveDownloadTo).mockResolvedValue();
    downloads.set([queued('a', 'Alpha'), queued('b', 'Beta')]);
    const { user } = mount();

    expect(screen.queryByRole('button', { name: msg('downloads.queue.downloadNowFor', { title: 'Beta' }) })).toBeNull();
    await user.click(screen.getByRole('button', { name: msg('transfers.downloadsQueueActionsFor', { name: 'Beta' }) }));
    expect(screen.getAllByRole('menuitem').map((item) => item.textContent?.trim())).toEqual([
      msg('downloads.queue.details'),
      msg('transfers.downloadsStartNow'),
      msg('transfers.downloadsMoveUp'),
    ]);
    await user.click(screen.getByRole('menuitem', { name: msg('downloads.queue.details') }));
    expect(onopen).toHaveBeenCalledWith('b');

    await user.click(screen.getByRole('button', { name: msg('transfers.downloadsQueueActionsFor', { name: 'Beta' }) }));
    await user.click(screen.getByRole('menuitem', { name: msg('transfers.downloadsMoveUp') }));
    expect(vi.mocked(moveDownloadTo).mock.calls).toEqual([['b', 0]]);

    await user.click(screen.getByRole('button', { name: msg('transfers.downloadsQueueActionsFor', { name: 'Alpha' }) }));
    await user.click(screen.getByRole('menuitem', { name: msg('transfers.downloadsStartNow') }));
    expect(vi.mocked(forceStartDownload).mock.calls).toEqual([['a']]);
  });

  it('keeps the tags the row had: the kind of download, its source, and a download let outside the schedule', () => {
    sources.set([{ id: 'src-1', name: 'Main feed' } as Source]);
    downloads.set([queued('a', 'Alpha', { origin: { gameId: 'g-alpha', sourceId: 'src-1', purpose: 'update' }, ignoreSchedule: true })]);
    mount();

    const row = screen.getByRole('listitem');
    for (const label of [msg('transfers.downloadsTypeUpdate'), 'Main feed', msg('transfers.outsideSchedule')]) {
      expect(row.textContent, label).toContain(label);
    }
  });

  it('the title leads to the game’s page; there is no separate game page button', async () => {
    downloads.set([queued('a', 'Alpha', { origin: { gameId: 'g-alpha' } })]);
    const { user } = mount();

    expect(screen.queryByRole('button', { name: msg('transfers.gamePage') })).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Alpha' }));

    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-alpha' } });
  });
});

describe('QueueList: the cancel dialog follows its download', () => {
  it('says what the download holds now, not at the press', async () => {
    downloads.set([queued('a', 'Alpha', { downloaded: 1024 }), queued('b', 'Beta')]);
    const { user } = mount();

    await user.click(cancelX('Alpha'));
    const dialog = await screen.findByRole('dialog');
    expect(dialog.textContent).toContain(msg('downloads.cancel.deletes', { size: bytesSize(1024) }));
    downloads.update((list) => list.map((d) => (d.id === 'a' ? { ...d, downloaded: 900 * 1024 ** 2 } : d)));

    await vi.waitFor(() => expect(dialog.textContent).toContain(msg('downloads.cancel.deletes', { size: bytesSize(900 * 1024 ** 2) })));
    expect(dialog.textContent).not.toContain(msg('downloads.cancel.deletes', { size: bytesSize(1024) }));
  });

  it('closes when the download leaves the queue, and focus goes on by the rule; nothing is sent', async () => {
    downloads.set([queued('a', 'Alpha', { downloaded: 1024 }), queued('b', 'Beta')]);
    const { user } = mount();

    cancelX('Alpha').focus();
    await user.keyboard('{Enter}');
    await screen.findByRole('dialog');
    downloads.update((list) => list.map((d) => (d.id === 'a' ? { ...d, status: 'downloading' } : d)));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(cancelX('Beta')));
    expect(cancelDownload).not.toHaveBeenCalled();
    downloads.update((list) => list.map((d) => (d.id === 'a' ? { ...d, status: 'queued' } : d)));
    await new Promise((done) => setTimeout(done, 0));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('a confirmed cancel Go answers before its removal: focus waits for the row to leave, then the row in its place', async () => {
    vi.mocked(cancelDownload).mockResolvedValue();
    downloads.set([queued('a', 'Alpha', { downloaded: 1024 }), queued('b', 'Beta')]);
    const { user } = mount();

    cancelX('Alpha').focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('ui.cancelDownload') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(document.activeElement).toBe(cancelX('Alpha'));

    goRemoves('a');

    await vi.waitFor(() => expect(document.activeElement).toBe(cancelX('Beta')));
    expect(onannounce).toHaveBeenCalledWith(msg('downloads.cancel.done', { name: 'Alpha' }));
  });
});
