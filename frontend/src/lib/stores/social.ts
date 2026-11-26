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

let started = false;

export async function initSocial(): Promise<void> {
  if (started) return;
  started = true;
  if (!inWails) return;

  Events.On('social:friends', (event) => {
    heard += 1;
    received(toFriendsPage(event.data));
  });
  Events.On('social:requests', (event) => {
    const signal = event.data as RequestsSignal | null;
    requestsSignal.set(signal ? signal.incoming : 0);
  });

  let previous: string | undefined;
  authState.subscribe((state) => {
    const changed = previous !== state;
    previous = state;
    if (!changed) return;
    session += 1;
    // A read of the session that ended does not hold the next one's.
    inFlight = null;
    followUp = null;
    friendsLoaded.set(false);
    friendsLoadError.set(null);
    resetFeed();
    if (state !== 'authenticated') {
      setPage(emptyFriendsPage());
      return;
    }
    kick().catch((err) => report(err, msg('state.socialRefreshFriendsFailed')));
  });

  let consentWithheld: boolean | undefined;
  needsSocialConsent.subscribe((needs) => {
    const changed = consentWithheld !== needs;
    consentWithheld = needs;
    if (changed && needs) resetFeed();
  });
  // Nothing is read here: a read in the unsettled start belongs to no account (the change of session
  // drops or resets it). The kick of a signed-in session has Go read and send the page itself, and the
  // Friends screen reads on arrival and on a change of session.
}
