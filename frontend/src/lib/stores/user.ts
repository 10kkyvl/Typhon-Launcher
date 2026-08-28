import { derived, get, writable } from 'svelte/store';
import { resetHistory } from './router';
import {
  AccountError,
  bootstrapSession,
  continueAsGuest,
  login,
  logout,
  pickAvatar,
  register,
  removeAvatar,
  updateProfile,
  uploadAvatar,
  type CurrentUser,
  type LoginInput,
  type ProfilePatch,
  type RegisterInput,
} from '../services/account';

export type AuthState = 'bootstrapping' | 'authenticated' | 'unauthenticated' | 'unavailable' | 'guest' | 'offline';
export type AuthView = 'login' | 'register';

export const currentUser = writable<CurrentUser | null>(null);
export const authState = writable<AuthState>('bootstrapping');
export const authReason = writable('');
export const authView = writable<AuthView>('login');
export const savingProfile = writable(false);
export const pickingAvatar = writable(false);
export const uploadingAvatar = writable(false);
export const removingAvatar = writable(false);
export const isOffline = derived(authState, (state) => state === 'offline');

let bootstrapping: Promise<void> | null = null;

export function initAuth(): Promise<void> {
  if (bootstrapping) return bootstrapping;
  bootstrapping = runBootstrap().finally(() => {
    bootstrapping = null;
  });
  return bootstrapping;
}

export function retryBootstrap(): Promise<void> {
  if (bootstrapping) return bootstrapping;
  if (get(authState) === 'offline') {
    reconnectAttempt = 0;
    return performReconnectAttempt();
  }
  authState.set('bootstrapping');
  return initAuth();
}

async function runBootstrap(): Promise<void> {
  try {
    const state = await bootstrapSession();
    if (state.status === 'authenticated') {
      currentUser.set(state.user);
      authReason.set('');
      authState.set('authenticated');
      return;
    }
    if (state.status === 'offline') {
      currentUser.set(state.user);
      authReason.set(state.reason);
      authState.set('offline');
      return;
    }
    currentUser.set(null);
    authReason.set(state.reason);
    if (state.status === 'unavailable' || state.status === 'guest') {
      authState.set(state.status);
      return;
    }
    authState.set('unauthenticated');
  } catch (err) {
    currentUser.set(null);
    authReason.set(err instanceof AccountError ? err.code : 'server_error');
    authState.set('unavailable');
  }
}

const RECONNECT_DELAYS_MS = [15000, 30000, 60000];

let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempt = 0;
let onlineListenerAttached = false;

function nextReconnectDelay(): number {
  const delay = RECONNECT_DELAYS_MS[Math.min(reconnectAttempt, RECONNECT_DELAYS_MS.length - 1)];
  reconnectAttempt += 1;
  return delay;
}

function clearReconnectTimer(): void {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

function scheduleReconnect(): void {
  clearReconnectTimer();
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    void performReconnectAttempt();
  }, nextReconnectDelay());
}

async function performReconnectAttempt(): Promise<void> {
  if (get(authState) !== 'offline') return;
  clearReconnectTimer();
  await initAuth();
  if (get(authState) === 'offline') scheduleReconnect();
}

function onWindowOnline(): void {
  if (get(authState) !== 'offline') return;
  void performReconnectAttempt();
}

function startReconnectLoop(): void {
  reconnectAttempt = 0;
  if (!onlineListenerAttached) {
    window.addEventListener('online', onWindowOnline);
    onlineListenerAttached = true;
  }
  scheduleReconnect();
}

function stopReconnectLoop(): void {
  clearReconnectTimer();
  reconnectAttempt = 0;
  if (onlineListenerAttached) {
    window.removeEventListener('online', onWindowOnline);
    onlineListenerAttached = false;
  }
}

authState.subscribe((state) => {
  if (state === 'offline') startReconnectLoop();
  else stopReconnectLoop();
});

export async function enterAsGuest(): Promise<void> {
  await continueAsGuest();
  currentUser.set(null);
  authReason.set('');
  resetHistory();
  authState.set('guest');
}

export async function leaveGuest(view: AuthView = 'login'): Promise<void> {
  try {
    await logout();
  } finally {
    currentUser.set(null);
    authReason.set('');
    authView.set(view);
    resetHistory();
    authState.set('unauthenticated');
  }
}

export async function signUp(input: RegisterInput): Promise<void> {
  const user = await register(input);
  currentUser.set(user);
  authReason.set('');
  resetHistory();
  authState.set('authenticated');
}

export async function signIn(input: LoginInput): Promise<void> {
  const user = await login(input);
  currentUser.set(user);
  authReason.set('');
  resetHistory();
  authState.set('authenticated');
}

export async function signOut(): Promise<void> {
  try {
    await logout();
  } finally {
    currentUser.set(null);
    authReason.set('');
    authView.set('login');
    resetHistory();
    authState.set('unauthenticated');
  }
}

function onUnauthenticated(err: unknown) {
  if (err instanceof AccountError && err.code === 'unauthenticated') {
    currentUser.set(null);
    authReason.set('');
    authView.set('login');
    resetHistory();
    authState.set('unauthenticated');
  }
}

export async function saveProfile(patch: ProfilePatch): Promise<void> {
  if (get(savingProfile)) return;
  savingProfile.set(true);
  try {
    currentUser.set(await updateProfile(patch));
  } catch (err) {
    onUnauthenticated(err);
    throw err;
  } finally {
    savingProfile.set(false);
  }
}

export async function chooseAvatar(): Promise<string> {
  if (get(pickingAvatar)) return '';
  pickingAvatar.set(true);
  try {
    const image = await pickAvatar();
    if (!image.data || !image.mime) return '';
    return `data:${image.mime};base64,${image.data}`;
  } catch (err) {
    onUnauthenticated(err);
    throw err;
  } finally {
    pickingAvatar.set(false);
  }
}

export async function saveAvatar(encoded: string): Promise<void> {
  if (!encoded) throw new AccountError('invalid_avatar');
  if (get(uploadingAvatar)) return;
  uploadingAvatar.set(true);
  try {
    currentUser.set(await uploadAvatar(encoded));
  } catch (err) {
    onUnauthenticated(err);
    throw err;
  } finally {
    uploadingAvatar.set(false);
  }
}

export async function deleteAvatar(): Promise<void> {
  if (get(removingAvatar)) return;
  removingAvatar.set(true);
  try {
    currentUser.set(await removeAvatar());
  } catch (err) {
    onUnauthenticated(err);
    throw err;
  } finally {
    removingAvatar.set(false);
  }
}
