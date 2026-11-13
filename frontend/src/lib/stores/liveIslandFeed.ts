import { CircleAlert, CircleCheck, Gamepad2, Info, TriangleAlert } from '@lucide/svelte';
import { get } from 'svelte/store';
import { msg } from '../i18n';
import { openGameByIGDB } from '../social/openGame';
import { isPlaying } from '../social/presence';
import { downloads, failedMessageId } from './downloads';
import { liveIsland, liveIslandCurrent, liveIslandHeld, liveIslandMessages, type LiveIslandMessage, type LiveIslandTone } from './liveIsland';
import { friendsPage } from './social';
import { ACTION_MS, dismissToast, pauseToast, resumeToast, toasts, toastsInIsland, type Toast, type ToastKind } from './toasts';

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
 *   and says nothing; an empty list (signed out) starts that over.
 */

const TOAST_ICON = { info: Info, success: CircleCheck, warning: TriangleAlert, danger: CircleAlert } as const;
const TOAST_TONE: Record<ToastKind, LiveIslandTone> = { info: 'accent', success: 'success', warning: 'warning', danger: 'danger' };

const toastId = (n: number) => `toast:${n}`;

function fromToast(t: Toast): LiveIslandMessage {
  return {
    id: toastId(t.id),
    text: t.message,
    tone: TOAST_TONE[t.kind],
    icon: TOAST_ICON[t.kind],
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
  let before: Map<string, number> | null = null;
  return friendsPage.subscribe((page) => {
    const now = new Map<string, number>();
    for (const f of page.friends) {
      const game = f.presence?.gameId;
      if (isPlaying(f.presence) && game != null) now.set(f.id, game);
    }
    const known = before;
    before = page.friends.length > 0 ? now : null;
    if (known === null) return;
    for (const f of page.friends) {
      const game = now.get(f.id);
      if (game === undefined || known.get(f.id) === game) continue;
      const name = f.displayName || f.username;
      const title = f.presence?.gameTitle ?? '';
      liveIsland.show({
        id: `friend:${f.id}`,
        kind: 'friend',
        icon: Gamepad2,
        avatar: f.avatarUrl,
        text: msg('ui.liveIslandFriendPlaying', { name }),
        detail: title || undefined,
        ttl: ACTION_MS,
        actions: [{ label: msg('ui.liveIslandOpenGame'), run: () => void openGameByIGDB(game, title) }],
      });
    }
  });
}

/**
 * Starts every producer; returns the disposer. When the island goes (motion off), what it still holds
 * besides the toasts is said by its fallback (a failed download: its plain toast) or let go.
 */
export function initLiveIslandFeed(): () => void {
  const offs = [trackToasts(), trackFailedDownloads(), trackFriends()];
  return () => {
    for (const off of offs) off();
    for (const m of get(liveIslandMessages)) {
      liveIsland.hide(m.id);
      m.fallback?.();
    }
  };
}
