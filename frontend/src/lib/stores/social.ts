import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { AccountError } from '../services/account';
import { accountErrorText } from '../services/accountMessages';
import { inWails } from '../services/backend';
import {
  emptyFriendsPage,
  friends as fetchFriends,
  kick,
  refresh,
  toFriendsPage,
  type FriendsPage,
  type RequestsSignal,
} from '../services/social';
import { msg } from '../i18n';
import { resetFeed } from './feed';
import { settings } from './settings';
import { toast } from './toasts';
import { authState } from './user';

export const friendsPage = writable<FriendsPage>(emptyFriendsPage());

/** A page of friends has been read for this account: until then `friendsPage` says nothing about who is there. */
export const friendsLoaded = writable(false);
/** Why the first read for this account failed; null once a page has arrived. */
export const friendsLoadError = writable<unknown>(null);

// The account the reads belong to: an answer that comes after a sign-in or a sign-out is dropped.
let session = 0;

const requestsSignal = writable<number | null>(null);

export const incomingCount = derived(
  [friendsPage, requestsSignal],
  ([$friendsPage, $requestsSignal]) => $requestsSignal ?? $friendsPage.incoming.length,
);

let peak = 0;

export const incomingPeak = derived(incomingCount, ($incomingCount) => {
  if ($incomingCount <= 0) peak = 0;
  else if ($incomingCount > peak) peak = $incomingCount;
  return peak;
});

export const needsSocialConsent = derived(
  [settings, authState],
  ([$settings, $authState]) => $authState === 'authenticated' && !!$settings && !$settings.accountSync,
);

function report(err: unknown, fallback: string) {
  if (err instanceof AccountError && err.code === 'unauthenticated') return;
  toast(accountErrorText(err, fallback), 'danger');
}

function setPage(page: FriendsPage) {
  friendsPage.set(page);
  requestsSignal.set(null);
}

function received(page: FriendsPage) {
  setPage(page);
  friendsLoaded.set(true);
  friendsLoadError.set(null);
}

// The screens mounted that show a failed first read themselves (the Friends screen, its notice).
let shown = 0;

/**
 * Held while a screen that shows `friendsLoadError` is mounted: a failed first read of `loadFriends`
 * (the consent dialog's) is then said by that screen, not in a toast too. Returns the release.
 */
export function showFriendsState(): () => void {
  shown += 1;
  let held = true;
  return () => {
    if (held) shown -= 1;
    held = false;
  };
}

/**
 * Before any page a failed read is the state the Friends screen shows, and `said` asks for a toast
 * too unless that screen is there; after a page the lists stay and a toast says it.
 */
function readFailed(err: unknown, fallback: string, said = false) {
  if (get(friendsLoaded)) {
    report(err, fallback);
    return;
  }
  friendsLoadError.set(err);
  if (said && shown === 0) report(err, fallback);
}

// Every page Go sent with `social:friends`: an answer asked for before one of them is older than it.
let heard = 0;

/** Takes an answer of `Friends()` asked for when `heard` was `before`, unless a newer page came since. */
function answered(page: FriendsPage, before: number) {
  if (heard === before) received(page);
}

/**
 * Reads the friends Go holds, for a screen that shows no state of its own (the consent dialog's read):
 * a failure is kept as the state and said once in a toast, unless the Friends screen shows it.
 */
export async function loadFriends(): Promise<void> {
  const started = session;
  try {
    const before = heard;
    const page = await fetchFriends();
    if (started === session) answered(page, before);
  } catch (err) {
    if (started === session) readFailed(err, msg('state.socialLoadFriendsFailed'), true);
  }
}

// The read in flight, and the one more asked while it runs: an answer may predate what asked (an action
// Go answered after the read began), so any number of asks during a read make one read after it.
let inFlight: Promise<boolean> | null = null;
let followUp: Promise<boolean> | null = null;

/**
 * Has Go read the friends again and takes the page it then holds. Never rejects: answers whether a
 * page is there now (a failure is kept or said as `readFailed` does). Asked while a read runs, it
 * waits for one read that starts after it.
 */
export function reloadFriends(): Promise<boolean> {
  const running = inFlight;
  if (!running) return startRead();
  if (!followUp) {
    const asked = session;
    const next = running.then(() => {
      if (followUp === next) followUp = null;
      return asked === session ? startRead() : false;
    });
    followUp = next;
  }
  return followUp;
}

function startRead(): Promise<boolean> {
  const flight = readAgain();
  inFlight = flight;
  void flight.then(() => {
    if (inFlight === flight) inFlight = null;
  });
  return flight;
}

// The start's read while it runs: the Friends screen that arrives meanwhile takes it as its own.
let opening: Promise<boolean> | null = null;

/**
 * The Friends screen's read on arrival. The start's read, while it runs, stands for it: asked when the
 * launcher started, it predates nothing the player did. Otherwise as `reloadFriends`.
 */
export function readFriendsOnArrival(): Promise<boolean> {
  return opening ?? reloadFriends();
}

/**
 * Reads the page Go holds while the stored session is checked, so that every reader of the store (the
 * badges, the request notification, Activity, the game page) has it as the splash goes, not at Go's
 * first poll after the kick. Its failure is the state only: at boot nobody says it. A read asked while
 * it runs (after an action) waits for one more, as with any read in flight.
 */
function readAtStart(): Promise<boolean> {
  const started = session;
  const before = heard;
  const flight = fetchFriends().then(
    (page) => {
      if (started === session) answered(page, before);
      return get(friendsLoaded);
    },
    (err: unknown) => {
      if (started === session && !get(friendsLoaded)) friendsLoadError.set(err);
      return get(friendsLoaded);
    },
  );
  opening = flight;
  inFlight = flight;
  void flight.then(() => {
    if (opening === flight) opening = null;
    if (inFlight === flight) inFlight = null;
  });
  return flight;
}

async function readAgain(): Promise<boolean> {
  const started = session;
  try {
    await refresh();
    const before = heard;
    const page = await fetchFriends();
    if (started === session) answered(page, before);
  } catch (err) {
    if (started !== session) return get(friendsLoaded);
    readFailed(err, msg('social.friendsRefreshFailed'));
    return false;
  }
  return get(friendsLoaded);
}

/**
 * Ends the session the reads and the page belong to: an answer asked for in it is dropped, and the
 * page goes before the next session's first read, so one account's friends never reach the next.
 */
function forget() {
  session += 1;
  // A read of the session that ended does not hold the next one's.
  inFlight = null;
  followUp = null;
  opening = null;
  setPage(emptyFriendsPage());
  friendsLoaded.set(false);
  friendsLoadError.set(null);
}

let started = false;

export async function initSocial(): Promise<void> {
  if (started) return;
  started = true;
  if (!inWails) return;

  Events.On('social:friends', (event) => {
    // Go's poll can answer after a sign-out, and its page is the account that left.
    if (get(authState) !== 'authenticated') return;
    heard += 1;
    received(toFriendsPage(event.data));
  });
  Events.On('social:requests', (event) => {
    const signal = event.data as RequestsSignal | null;
    requestsSignal.set(signal ? signal.incoming : 0);
  });

  let previous: string | undefined;
  authState.subscribe((state) => {
    const before = previous;
    previous = state;
    if (before === state) return;
    resetFeed();
    if (before === 'bootstrapping' && state === 'authenticated') {
      // The stored session checked at the start is confirmed: the same account, so the start's read
      // and its page stay. A failure it had before is forgotten: the kick has Go read again.
      if (!get(friendsLoaded)) friendsLoadError.set(null);
      kick().catch((err) => report(err, msg('state.socialRefreshFriendsFailed')));
      return;
    }
    forget();
    if (state === 'authenticated') kick().catch((err) => report(err, msg('state.socialRefreshFriendsFailed')));
  });

  let consentWithheld: boolean | undefined;
  needsSocialConsent.subscribe((needs) => {
    const changed = consentWithheld !== needs;
    consentWithheld = needs;
    if (!changed || !needs) return;
    // Without the consent nothing of the account's friends may be shown, and a read asked with it is
    // dropped: the badges, Activity and the game page see an empty page as after a sign-out.
    resetFeed();
    forget();
  });

  await readAtStart();
}
