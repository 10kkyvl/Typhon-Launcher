import { derived, get, type Readable } from 'svelte/store';
import { msg } from '../i18n';
import type { CloudGameStatus } from '../services/cloudsaves';
import { openGameByIGDB } from '../social/openGame';
import { isPlaying } from '../social/presence';
import { cloudConflictTarget, cloudStatus, openCloudConflict } from './cloudsaves';
import { downloads, failedMessageId } from './downloads';
import { libraryGames } from './library';
import { liveIsland, liveIslandCurrent, liveIslandHeld, liveIslandMessages, type LiveIslandMessage, type LiveIslandTone } from './liveIsland';
import { friendsPage } from './social';
import { ACTION_MS, dismissToast, pauseToast, resumeToast, toast, toasts, toastsInIsland, type Toast, type ToastKind } from './toasts';

/**
 * What the live island is told, wired once by LiveIsland.svelte while it is mounted (motion on):
 *
 * - toasts: the toast store stays their source and keeps their clocks, the queue and the repeats; the
 *   island is the second way to show what it holds, and the stack shows none of them meanwhile. A
 *   toast's clock runs only while the island shows it and nobody reads it. When the island goes
 *   (motion off) the toasts it held go back to the stack with their clocks running;
 * - a download that fails: one message with Retry and "Open downloads", posted by the downloads store
 *   in place of its toast; here it goes once the download is no longer failed (retried elsewhere,
 *   removed);
 * - a friend who starts playing (presence in the friends list): one message with the friend's
 *   picture and "Open the game". The first list that has friends in it is what was going on already
 *   and says nothing; an empty list (signed out) starts that over; a friend new to the list says
 *   nothing either;
 * - a save conflict (the cloud saves status lists a game in conflict that its dialog does not show):
 *   one message per game with "choose a version", which opens the dialog. It is said
 *   once a session, and goes once the conflict is resolved or its dialog opens. Where no island is on
 *   screen the shell says it as a toast (`initConflictToasts`).
 */

const TOAST_TONE: Record<ToastKind, LiveIslandTone> = { info: 'accent', success: 'success', warning: 'warning', danger: 'danger' };

const toastId = (n: number) => `toast:${n}`;

function fromToast(t: Toast): LiveIslandMessage {
  return {
    id: toastId(t.id),
    text: t.message,
    tone: TOAST_TONE[t.kind],
    icon: t.kind,
    count: t.count,
    actions: t.action ? [{ label: t.action.label, run: t.action.run }] : undefined,
    close: () => dismissToast(t.id),
  };
}

function trackToasts(): () => void {
  const mirrored = new Map<number, Toast>();

  const sync = () => {
    const list = get(toasts);
    const present = new Set(list.map((t) => t.id));
    for (const n of [...mirrored.keys()]) {
      if (present.has(n)) continue;
      mirrored.delete(n);
      liveIsland.hide(toastId(n));
    }
    for (const t of list) {
      const before = mirrored.get(t.id);
      mirrored.set(t.id, t);
      if (!before) liveIsland.show(fromToast(t));
      else if (before.count !== t.count) liveIsland.update(toastId(t.id), { count: t.count });
    }
    const shown = get(liveIslandCurrent)?.id;
    const reading = get(liveIslandHeld);
    for (const t of list) {
      if (shown === toastId(t.id) && !reading) resumeToast(t.id);
      else pauseToast(t.id);
    }
  };

  toastsInIsland(true);
  const offs = [toasts.subscribe(sync), liveIslandCurrent.subscribe(sync), liveIslandHeld.subscribe(sync)];
  return () => {
    for (const off of offs) off();
    for (const n of mirrored.keys()) {
      liveIsland.hide(toastId(n));
      resumeToast(n);
    }
    mirrored.clear();
    toastsInIsland(false);
  };
}

// The message itself comes from the downloads store's `download:failed` handler: an event, so a
// download that had failed before (in the list as it loads) is never taken for news.
function trackFailedDownloads(): () => void {
  return downloads.subscribe((list) => {
    const failed = new Set(list.filter((d) => d.status === 'failed').map((d) => failedMessageId(d.id)));
    for (const m of get(liveIslandMessages)) {
      if (m.id.startsWith(failedMessageId('')) && !failed.has(m.id)) liveIsland.hide(m.id);
    }
  });
}

function trackFriends(): () => void {
  // Every friend on the list, with the game they are in or null: a friend new to the list who is
  // already in a game is someone added, not someone who just started playing.
  let before: Map<string, number | null> | null = null;
  return friendsPage.subscribe((page) => {
    const now = new Map<string, number | null>();
    for (const f of page.friends) {
      const game = f.presence?.gameId;
      now.set(f.id, isPlaying(f.presence) && game != null ? game : null);
    }
    const known = before;
    before = page.friends.length > 0 ? now : null;
    if (known === null) return;
    for (const f of page.friends) {
      const game = now.get(f.id);
      if (game == null || !known.has(f.id) || known.get(f.id) === game) continue;
      const name = f.displayName || f.username;
      const title = f.presence?.gameTitle ?? '';
      liveIsland.show({
        id: `friend:${f.id}`,
        kind: 'friend',
        icon: 'game',
        avatar: f.avatarUrl,
        text: msg('ui.liveIslandFriendPlaying', { name }),
        detail: title || undefined,
        ttl: ACTION_MS,
        actions: [{ label: msg('ui.liveIslandOpenGame'), run: () => void openGameByIGDB(game, title) }],
      });
    }
  });
}

const CONFLICT_MS = 12_000;
const CONFLICT_PREFIX = 'save-conflict:';

const conflictMessageId = (gameId: string) => `${CONFLICT_PREFIX}${gameId}`;

/** The games whose save conflict the island shows or holds: the sidebar's status line leaves them to it. */
export const conflictsInIsland: Readable<ReadonlySet<string>> = derived(
  liveIslandMessages,
  (list) => new Set(list.filter((m) => m.id.startsWith(CONFLICT_PREFIX)).map((m) => m.id.slice(CONFLICT_PREFIX.length))),
);

// For the session, by game: the conflict that was said or seen in the dialog, as the recorded copies
// it was about ('' while the status has not listed them: an event says "conflict" before the status
// is loaded again). Kept when the game leaves the conflict state, so that a pass that fails and finds
// the same conflict again says nothing; a conflict with other copies is a new one.
const saidConflicts = new Map<string, string>();
// The toast that says a game's conflict where the island is not on screen, to take it back once resolved.
const conflictToasts = new Map<string, number>();

const copiesOf = (g: CloudGameStatus) => (g.conflict ? `${g.conflict.localDigest}:${g.conflict.remoteDigest}` : '');

function unsayConflict(gameId: string): void {
  const id = conflictMessageId(gameId);
  if (liveIsland.has(id)) liveIsland.hide(id);
  const shown = conflictToasts.get(gameId);
  if (shown === undefined) return;
  conflictToasts.delete(gameId);
  dismissToast(shown);
}

function sayConflictAsToast(gameId: string, title: string): void {
  unsayConflict(gameId);
  const id = toast(msg('ui.statusCloudConflict', { title }), 'warning', {
    label: msg('ui.liveIslandChooseVersion'),
    run: () => void openCloudConflict(gameId),
  });
  conflictToasts.set(gameId, id);
}

function sayConflictInIsland(gameId: string, title: string): void {
  unsayConflict(gameId);
  liveIsland.show({
    id: conflictMessageId(gameId),
    kind: 'conflict',
    tone: 'warning',
    icon: 'warning',
    text: msg('ui.liveIslandSaveConflict'),
    detail: title,
    ttl: CONFLICT_MS,
    // Putting it away is the island's own "dismiss", drawn for every message.
    actions: [{ label: msg('ui.liveIslandChooseVersion'), run: () => void openCloudConflict(gameId) }],
    fallback: () => sayConflictAsToast(gameId, title),
  });
}

/**
 * Says each save conflict once, in the way `say` gives: a conflict the dialog shows is taken as seen,
 * a game the library does not have waits until it does, and what was said goes once the conflict is
 * resolved or its dialog opens.
 */
function trackConflicts(say: (gameId: string, title: string) => void): () => void {
  const sync = () => {
    const open = get(cloudConflictTarget)?.gameId;
    const titles = new Map(get(libraryGames).map((g) => [g.id, g.title]));
    const conflicts = new Map((get(cloudStatus)?.games ?? []).filter((g) => g.state === 'conflict').map((g) => [g.gameId, copiesOf(g)]));
    for (const [gameId, copies] of [...saidConflicts]) {
      if (conflicts.has(gameId)) continue;
      unsayConflict(gameId);
      // Without its copies it cannot be told from the next conflict of the game.
      if (copies === '') saidConflicts.delete(gameId);
    }
    for (const [gameId, copies] of conflicts) {
      const before = saidConflicts.get(gameId);
      const newer = before !== undefined && before !== '' && copies !== '' && before !== copies;
      if (gameId === open) {
        unsayConflict(gameId);
        saidConflicts.set(gameId, copies || before || '');
        continue;
      }
      if (before !== undefined && !newer) {
        if (copies !== '') saidConflicts.set(gameId, copies);
        continue;
      }
      const title = titles.get(gameId);
      if (!title) continue;
      saidConflicts.set(gameId, copies);
      say(gameId, title);
    }
  };
  const offs = [cloudStatus.subscribe(sync), cloudConflictTarget.subscribe(sync), libraryGames.subscribe(sync)];
  return () => {
    for (const off of offs) off();
  };
}

/**
 * Says each save conflict once as a toast with "choose a version": for a shell that shows no island
 * (motion off, Big Picture). The island says them itself while it is mounted; the two share what was
 * said, so a conflict is not said again when the one hands over to the other.
 */
export function initConflictToasts(): () => void {
  return trackConflicts(sayConflictAsToast);
}

/**
 * Starts every producer; returns the disposer. When the island goes (motion off), what it still holds
 * besides the toasts is said by its fallback (a failed download, a save conflict: its toast) or let go.
 */
export function initLiveIslandFeed(): () => void {
  const offs = [trackToasts(), trackFailedDownloads(), trackFriends(), trackConflicts(sayConflictInIsland)];
  return () => {
    for (const off of offs) off();
    for (const m of get(liveIslandMessages)) {
      liveIsland.hide(m.id);
      m.fallback?.();
    }
  };
}
