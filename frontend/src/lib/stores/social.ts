import { derived, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { AccountError } from '../services/account';
import { accountErrorText } from '../services/accountMessages';
import { inWails } from '../services/backend';
import {
  emptyFriendsPage,
  friends as fetchFriends,
  kick,
  toFriendsPage,
  type FriendsPage,
  type RequestsSignal,
} from '../services/social';
import { resetFeed } from './feed';
import { settings } from './settings';
import { toast } from './toasts';
import { authState } from './user';

export const friendsPage = writable<FriendsPage>(emptyFriendsPage());

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

export async function loadFriends(): Promise<void> {
  try {
    setPage(await fetchFriends());
  } catch (err) {
    report(err, 'Не удалось загрузить список друзей');
  }
}

let started = false;

export async function initSocial(): Promise<void> {
  if (started) return;
  started = true;
  if (!inWails) return;

  Events.On('social:friends', (event) => {
    setPage(toFriendsPage(event.data));
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
    resetFeed();
    if (state !== 'authenticated') {
      setPage(emptyFriendsPage());
      return;
    }
    kick().catch((err) => report(err, 'Не удалось обновить список друзей'));
  });

  await loadFriends();
}
