import { get, writable } from 'svelte/store';
import { resetHistory } from './router';
import {
  AccountError,
  bootstrapSession,
  continueAsGuest,
  login,
  logout,
  register,
  removeAvatar,
  selectAvatarFile,
  updateProfile,
  uploadAvatar,
  type CurrentUser,
  type LoginInput,
  type ProfilePatch,
  type RegisterInput,
} from '../services/account';

export type AuthState = 'bootstrapping' | 'authenticated' | 'unauthenticated' | 'unavailable' | 'guest';
export type AuthView = 'login' | 'register';

export const currentUser = writable<CurrentUser | null>(null);
export const authState = writable<AuthState>('bootstrapping');
export const authReason = writable('');
export const authView = writable<AuthView>('login');
export const savingProfile = writable(false);
export const uploadingAvatar = writable(false);
export const removingAvatar = writable(false);

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

export async function changeAvatar(): Promise<void> {
  if (get(uploadingAvatar)) return;
  uploadingAvatar.set(true);
  try {
    const path = await selectAvatarFile();
    if (!path) return;
    currentUser.set(await uploadAvatar(path));
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
