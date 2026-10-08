import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { AccountError, fetchCurrentUser } from '../lib/services/account';
import { inWails } from '../lib/services/backend';
import { emptyFriendsPage, friends as fetchFriends, toFriendsPage } from '../lib/services/social';
import { activePeer, loadMessages, refreshConversations } from '../lib/stores/messaging';
import { friendsPage } from '../lib/stores/social';
import { authState, currentUser } from '../lib/stores/user';

export type OverlayAccount = 'loading' | 'ready' | 'signedOut' | 'error';

export const overlayAccount = writable<OverlayAccount>('loading');
export const overlayFriendsFailed = writable(false);

let loading: Promise<void> | null = null;

async function loadAccount(): Promise<boolean> {
  try {
    const user = await fetchCurrentUser();
    currentUser.set(user);
    authState.set('authenticated');
    overlayAccount.set('ready');
    return true;
  } catch (err) {
    if (err instanceof AccountError && err.code === 'unauthenticated') {
      currentUser.set(null);
      authState.set('unauthenticated');
      friendsPage.set(emptyFriendsPage());
      overlayAccount.set('signedOut');
      return false;
    }
    console.warn('overlay account failed', err);
    overlayAccount.set('error');
    return false;
  }
}

async function loadFriendsList(): Promise<void> {
  try {
    friendsPage.set(await fetchFriends());
    overlayFriendsFailed.set(false);
  } catch (err) {
    console.warn('overlay friends failed', err);
    overlayFriendsFailed.set(true);
  }
}

async function loadAll(): Promise<void> {
  if (!(await loadAccount())) return;
  const peer = get(activePeer);
  await Promise.all([
    loadFriendsList(),
    refreshConversations(),
    peer ? loadMessages(peer.id, '', false, true).catch(() => undefined) : Promise.resolve(),
  ]);
}

export function refreshOverlay(): Promise<void> {
  if (loading) return loading;
  loading = loadAll().finally(() => {
    loading = null;
  });
  return loading;
}

export function retryFriends(): Promise<void> {
  return loadFriendsList();
}

export function listenFriends(): () => void {
  if (!inWails) return () => {};
  return Events.On('social:friends', (event) => {
    friendsPage.set(toFriendsPage(event.data));
    overlayFriendsFailed.set(false);
  });
}
