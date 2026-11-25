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

/** Before any page a failed read is the state the screens show; after one the lists stay and a toast says it. */
function readFailed(err: unknown, fallback: string) {
  if (get(friendsLoaded)) report(err, fallback);
  else friendsLoadError.set(err);
}

export async function loadFriends(): Promise<void> {
  const started = session;
  try {
    const page = await fetchFriends();
    if (started === session) received(page);
  } catch (err) {
    if (started === session) readFailed(err, msg('state.socialLoadFriendsFailed'));
  }
}

/**
 * Has Go read the friends again and takes the page it then holds. Never rejects: answers whether a
 * page is there now (a failure is kept or said as `readFailed` does).
 */
export async function reloadFriends(): Promise<boolean> {
  const started = session;
  try {
    await refresh();
    const page = await fetchFriends();
    if (started === session) received(page);
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

  await loadFriends();
}
