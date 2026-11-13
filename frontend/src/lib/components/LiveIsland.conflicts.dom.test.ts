import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CloudGameStatus, CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import { askToResolveConflict, closeCloudConflict, cloudConflictTarget, cloudStatus, openCloudConflict } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { liveIsland, liveIslandCurrent, liveIslandMessages, type LiveIslandMessage } from '../stores/liveIsland';
import { resetHistory } from '../stores/router';
import { holdToasts, toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import IslandShellHarness from '../testing/IslandShellHarness.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
// The shell renders as in the browser (no backend); the cloud saves store starts as in Wails.
const backend = vi.hoisted(() => ({ wails: false }));
vi.mock('../services/backend', () => ({
  get inWails() {
    return backend.wails;
  },
}));
const status = vi.hoisted(() => vi.fn());
vi.mock('../../../bindings/typhon/internal/cloudsaves', () => ({ Service: { Status: status } }));

const SAY = () => msg('ui.liveIslandSaveConflict');
const CHOOSE = () => msg('ui.liveIslandChooseVersion');
const LATER = () => msg('cloudsaves.conflictLater');

// What the island says is session memory: every test gets copies of its own, so that a conflict said
// in one test is news in the next.
let copiesMade = 0;
function conflict(gameId: string, copies = `copy-${++copiesMade}`): CloudGameStatus {
  return {
    gameId,
    state: 'conflict',
    code: '',
    message: '',
    at: '',
    syncedAt: '',
    conflict: { localDigest: copies, localUpdatedAt: '2026-11-01T10:00:00Z', localSize: 10, remoteDigest: `remote-${copies}`, remoteUpdatedAt: '2026-11-02T10:00:00Z', remoteSize: 20, remoteDevice: 'Deck' },
  };
}

const synced = (gameId: string): CloudGameStatus => ({ gameId, state: 'synced', code: '', message: '', at: '', syncedAt: '' });

function cloud(...games: CloudGameStatus[]): CloudStatus {
  return { enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games };
}

const conflictMessages = () => get(liveIslandMessages).filter((m) => m.kind === 'conflict');

let app: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  toasts.set([]);
  holdToasts(false);
  liveIsland.clear();
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
  resetHistory();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  document.documentElement.classList.remove('no-anim');
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([]);
  toasts.set([]);
  liveIsland.clear();
  resetHistory();
  status.mockReset();
  vi.unstubAllGlobals();
});

function setup() {
  const view = render(IslandShellHarness, { target: app });
  const island = () => app.querySelector<HTMLElement>('[data-part="live-island"]');
  const pill = () => island()?.querySelector<HTMLButtonElement>('.head') as HTMLButtonElement;
  const row = () => app.querySelector<HTMLElement>('[data-cloud]');
  return { keys: user(), island, pill, row, view };
}

async function motion(on: boolean) {
  document.documentElement.classList.toggle('no-anim', !on);
  await Promise.resolve();
  await tick();
}

describe('a save conflict in the island', () => {
  it('is one message: "save conflict", the game as its detail, a warning, "choose a version" and "later", 12 s', async () => {
    const { island } = setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();

    const shown = get(liveIslandCurrent) as LiveIslandMessage;
    expect(shown).toMatchObject({ kind: 'conflict', tone: 'warning', icon: 'warning', text: SAY(), detail: 'Hades', ttl: 12_000 });
    expect(shown.actions?.map((a) => a.label)).toEqual([CHOOSE(), LATER()]);
    expect(island()?.dataset.kind).toBe('conflict');
    expect(island()?.classList.contains('warning')).toBe(true);
    expect(within(screen.getByTestId('live-island-alert')).getByText(`${SAY()}. Hades`)).toBeTruthy();
    expect(get(toasts)).toEqual([]);
    expect(get(cloudConflictTarget)).toBeNull();
  });

  it('leaves by itself after 12 s on screen', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    vi.advanceTimersByTime(11_900);
    expect(conflictMessages()).toHaveLength(1);
    vi.advanceTimersByTime(100);
    expect(conflictMessages()).toEqual([]);
  });

  it('"choose a version" closes the message and opens the dialog for that game, focus inside it', async () => {
    const { keys, island, pill } = setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    await keys.click(pill());
    await tick();

    await keys.click(within(island() as HTMLElement).getByRole('button', { name: CHOOSE() }));

    expect(get(cloudConflictTarget)).toEqual({ gameId: 'g1', title: 'Hades' });
    expect(conflictMessages()).toEqual([]);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
  });

  it('"later" puts it away and nothing else happens; it is not said again', async () => {
    const { keys, island, pill } = setup();
    const listed = cloud(conflict('g1'));
    cloudStatus.set(listed);
    await tick();
    await keys.click(pill());
    await tick();

    await keys.click(within(island() as HTMLElement).getByRole('button', { name: LATER() }));
    await tick();

    expect(get(liveIslandMessages)).toEqual([]);
    expect(get(cloudConflictTarget)).toBeNull();
    expect(get(toasts)).toEqual([]);
    expect(screen.queryByRole('dialog', { name: msg('cloudsaves.conflictTitle') })).toBeNull();
    cloudStatus.set({ ...listed, games: [...listed.games] });
    await tick();
    expect(get(liveIslandMessages)).toEqual([]);
  });

  it('is said once: not again after it timed out, was dismissed, or the same conflict was listed again after a failed pass', async () => {
    const { keys, island, pill } = setup();
    const game = conflict('g1');
    cloudStatus.set(cloud(game));
    await tick();
    await keys.click(pill());
    await tick();
    await keys.click(within(island() as HTMLElement).getByRole('button', { name: msg('ui.toastDismiss') }));
    await tick();
    expect(get(liveIslandMessages)).toEqual([]);

    cloudStatus.set(cloud({ ...game }));
    cloudStatus.set(cloud({ ...synced('g1'), state: 'failed', code: 'cloud.network' }));
    // As a conflict event leaves it until the status is loaded again: no details yet.
    cloudStatus.set(cloud({ ...game, conflict: undefined }));
    cloudStatus.set(cloud({ ...game }));
    await tick();

    expect(get(liveIslandMessages)).toEqual([]);
  });

  it('a newer conflict of the same game (its recorded copies changed) is said again', async () => {
    setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    liveIsland.hide(get(liveIslandCurrent)?.id as string);
    await tick();

    cloudStatus.set(cloud(conflict('g1')));
    await tick();

    expect(conflictMessages().map((m) => m.detail)).toEqual(['Hades']);
  });

  it('a conflict first listed without its details is not said again when they come', async () => {
    // A game with no conflict said before in this session.
    const gameId = `fresh-${++copiesMade}`;
    libraryGames.set([{ id: gameId, title: 'Hades' } as LibraryGame]);
    setup();
    const game = conflict(gameId);
    cloudStatus.set(cloud({ ...game, conflict: undefined }));
    await tick();
    expect(conflictMessages()).toHaveLength(1);
    liveIsland.hide(conflictMessages()[0].id);

    cloudStatus.set(cloud(game));
    await tick();

    expect(conflictMessages()).toEqual([]);
  });

  it('a game the library does not have is not said; it is once the library lists it', async () => {
    libraryGames.set([]);
    setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    expect(get(liveIslandMessages)).toEqual([]);

    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
    await tick();

    expect(conflictMessages().map((m) => m.detail)).toEqual(['Hades']);
  });

  it('a conflict listed before the island came (startup) is said once it comes', async () => {
    cloudStatus.set(cloud(conflict('g1')));
    setup();
    await tick();
    expect(conflictMessages().map((m) => m.detail)).toEqual(['Hades']);
    expect(get(toasts)).toEqual([]);
  });

  it('is not said while the dialog shows that conflict (opened from the status line), nor after "Later" closes it', async () => {
    setup();
    openCloudConflict('g1');
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    expect(get(liveIslandMessages)).toEqual([]);

    closeCloudConflict();
    await tick();

    expect(get(liveIslandMessages)).toEqual([]);
    expect(get(toasts)).toEqual([]);
  });

  it('is not said while a held-back launch asks about it, nor after the launch is given up', async () => {
    setup();
    const question = askToResolveConflict('g1');
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    expect(get(liveIslandMessages)).toEqual([]);

    closeCloudConflict();
    expect(await question).toBe(false);
    await tick();

    expect(get(liveIslandMessages)).toEqual([]);
    expect(get(toasts)).toEqual([]);
  });

  it('goes when the dialog for that game opens another way (the status line, Play), shown or waiting', async () => {
    setup();
    cloudStatus.set(cloud(conflict('g1'), conflict('g2')));
    await tick();
    expect(conflictMessages()).toHaveLength(2);

    openCloudConflict('g2');
    await tick();
    expect(conflictMessages().map((m) => m.detail)).toEqual(['Hades']);
    openCloudConflict('g1');
    await tick();

    expect(conflictMessages()).toEqual([]);
  });

  it('goes once resolved, whether on screen or still waiting', async () => {
    setup();
    const first = conflict('g1');
    cloudStatus.set(cloud(first, conflict('g2')));
    await tick();
    expect(get(liveIslandCurrent)?.detail).toBe('Hades');

    cloudStatus.set(cloud(first, synced('g2')));
    await tick();
    expect(conflictMessages().map((m) => m.detail)).toEqual(['Hades']);

    cloudStatus.set(cloud(synced('g1'), synced('g2')));
    await tick();
    expect(get(liveIslandMessages)).toEqual([]);
  });

  it('several games: one message per game, the count of the waiting one, and "dismiss all"', async () => {
    const { keys, island, pill } = setup();
    cloudStatus.set(cloud(conflict('g1'), conflict('g2')));
    await tick();
    expect(conflictMessages().map((m) => m.detail)).toEqual(['Hades', 'Celeste']);
    expect(within(pill()).getByText(msg('ui.liveIslandWaiting', { count: 1 }))).toBeTruthy();

    await keys.click(pill());
    await tick();
    await keys.click(within(island() as HTMLElement).getByRole('button', { name: msg('ui.liveIslandDismissAll') }));
    await tick();

    expect(get(liveIslandMessages)).toEqual([]);
  });

  it('goes before a failed download and a friend already waiting, and takes the screen from the failure', async () => {
    setup();
    liveIsland.show({ id: 'download-failed:dl1', kind: 'error', tone: 'danger', text: 'Download failed', ttl: 9000, actions: [{ label: 'Retry', run: () => {} }] });
    liveIsland.show({ id: 'friend:bob', kind: 'friend', text: 'Bob is playing', ttl: 9000 });
    cloudStatus.set(cloud(conflict('g1')));
    await tick();

    expect(get(liveIslandCurrent)?.kind).toBe('conflict');
    liveIsland.hide(get(liveIslandCurrent)?.id as string);
    expect(get(liveIslandCurrent)?.id).toBe('download-failed:dl1');
  });
});

describe('a save conflict with no island on screen', () => {
  it('motion off: said once as a warning toast whose action opens the dialog', async () => {
    await motion(false);
    const { island } = setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();

    expect(island()).toBeNull();
    expect(get(toasts).map((t) => [t.kind, t.message, t.action?.label])).toEqual([['warning', msg('ui.statusCloudConflict', { title: 'Hades' }), CHOOSE()]]);
    cloudStatus.set(cloud(...(get(cloudStatus) as CloudStatus).games));
    await tick();
    expect(get(toasts)).toHaveLength(1);

    await user().click(screen.getByRole('button', { name: CHOOSE() }));

    expect(get(cloudConflictTarget)).toEqual({ gameId: 'g1', title: 'Hades' });
    expect(get(toasts)).toEqual([]);
  });

  it('motion off: the toast goes once the conflict is resolved', async () => {
    await motion(false);
    setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    expect(get(toasts)).toHaveLength(1);

    cloudStatus.set(cloud(synced('g1')));
    await tick();

    expect(get(toasts)).toEqual([]);
  });

  it('the island going with the messages still there: one toast each with the action, and nothing again when it comes back', async () => {
    setup();
    cloudStatus.set(cloud(conflict('g1'), conflict('g2')));
    await tick();
    expect(conflictMessages()).toHaveLength(2);

    await motion(false);

    expect(get(liveIslandMessages)).toEqual([]);
    expect(get(toasts).map((t) => [t.kind, t.message, t.action?.label])).toEqual([
      ['warning', msg('ui.statusCloudConflict', { title: 'Hades' }), CHOOSE()],
      ['warning', msg('ui.statusCloudConflict', { title: 'Celeste' }), CHOOSE()],
    ]);

    await motion(true);

    expect(conflictMessages()).toEqual([]);
    expect(get(toasts)).toHaveLength(2);
  });

  it('motion off at startup: said once as a toast, and turning motion on says nothing more', async () => {
    cloudStatus.set(cloud(conflict('g1')));
    await motion(false);
    setup();
    await tick();
    expect(get(toasts).map((t) => t.action?.label)).toEqual([CHOOSE()]);
    toasts.set([]);

    await motion(true);

    expect(conflictMessages()).toEqual([]);
    expect(get(toasts)).toEqual([]);
  });
});
