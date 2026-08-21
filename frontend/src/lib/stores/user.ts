import { get, writable } from 'svelte/store';
import {
  AccountError,
  fetchCurrentUser,
  removeAvatar,
  selectAvatarFile,
  updateProfile,
  uploadAvatar,
  type CurrentUser,
  type ProfilePatch,
} from '../services/account';

export const currentUser = writable<CurrentUser | null>(null);
export const userLoading = writable(false);
export const savingProfile = writable(false);
export const uploadingAvatar = writable(false);
export const removingAvatar = writable(false);

let initPromise: Promise<void> | null = null;

export function initCurrentUser(): Promise<void> {
  if (initPromise) return initPromise;
  initPromise = (async () => {
    userLoading.set(true);
    try {
      currentUser.set(await fetchCurrentUser());
    } catch (err) {
      if (err instanceof AccountError && err.code === 'unauthenticated') {
        currentUser.set(null);
      } else {
        throw err;
      }
    } finally {
      userLoading.set(false);
    }
  })();
  return initPromise;
}

export async function saveProfile(patch: ProfilePatch): Promise<void> {
  if (get(savingProfile)) return;
  savingProfile.set(true);
  try {
    currentUser.set(await updateProfile(patch));
  } catch (err) {
    if (err instanceof AccountError && err.code === 'unauthenticated') {
      currentUser.set(null);
    }
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
    if (err instanceof AccountError && err.code === 'unauthenticated') {
      currentUser.set(null);
    }
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
    if (err instanceof AccountError && err.code === 'unauthenticated') {
      currentUser.set(null);
    }
    throw err;
  } finally {
    removingAvatar.set(false);
  }
}
