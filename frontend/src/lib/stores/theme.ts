import { updateSettings } from './settings';
import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { msg } from '../i18n';
import { inWails } from '../services/backend';
import {
  activeTheme as fetchActiveTheme,
  applyTheme as applyThemeRequest,
  confirmTheme as confirmThemeRequest,
  listThemes,
  resetTheme as resetThemeRequest,
  toTheme,
  type BindingTheme,
  type Theme,
} from '../services/theme';
import { applyTheme as applyThemeDom, clearTheme as clearThemeDom, displayedAccent } from '../theme/apply';
import { themeCodeText, themeErrorText } from '../theme/themeErrors';
import { validateCss } from '../theme/validate';
import { toast } from './toasts';
import { confirmAccent } from './confirmedAccent';

export type ThemeMode = 'system' | 'theme';

const CONFIRM_DELAY_MS = 5000;

export const appearanceResetVersion = writable(0);
export const activeTheme = writable<Theme | null>(null);
export const themeList = writable<Theme[]>([]);
export const themeMode = writable<ThemeMode>('theme');
export const systemPrefersDark = writable(true);

let confirmTimer: ReturnType<typeof setTimeout> | null = null;
let media: MediaQueryList | null = null;

function cancelConfirm() {
  if (confirmTimer === null) return;
  clearTimeout(confirmTimer);
  confirmTimer = null;
}

function scheduleConfirm(id: string) {
  cancelConfirm();
  confirmTimer = setTimeout(() => {
    confirmTimer = null;
    // The window shows a draft during a trial, and the displayed accent is the
    // draft's, so confirming now would record the draft; wait until it ends.
    if (trialRestore !== null) {
      if (trialRestore.id === id) deferredConfirm = id;
      return;
    }
    if (get(activeTheme)?.id !== id) return;
    const accent = get(displayedAccent);
    confirmThemeRequest(id).then(() => confirmAccent(accent), (err) => toast(themeErrorText(err), 'danger'));
  }, CONFIRM_DELAY_MS);
}

function resolveSystemId(): string {
  return get(systemPrefersDark) ? 'dark' : 'light';
}

export interface ThemeTrial {
  name: string;
  endsAt: number;
  kept: boolean;
}

export const TRIAL_MS = 15000;
export const themeTrial = writable<ThemeTrial | null>(null);

let trialRestore: Theme | null = null;
let trialTimer: ReturnType<typeof setTimeout> | null = null;
let deferredConfirm: string | null = null;

function clearTrial() {
  if (trialTimer !== null) clearTimeout(trialTimer);
  trialTimer = null;
  trialRestore = null;
  deferredConfirm = null;
  themeTrial.set(null);
}

// The theme the backend has active, which a trial hides behind the draft.
export function realActiveTheme(): Theme | null {
  return trialRestore ?? get(activeTheme);
}

// Any real theme change (a pick, a save, a reset, an event from the backend)
// replaces what was being tried, so the trial is dropped without going back.
function setActive(theme: Theme, trial = false) {
  if (!trial) clearTrial();
  activeTheme.set(theme);
  applyThemeDom(theme);
}

// A draft is shown on the whole window without being saved or applied on the
// backend. Unless it is kept, the window goes back on its own, so CSS that
// hides the interface cannot lock the user out.
export function tryTheme(theme: Theme, ms: number = TRIAL_MS): string | null {
  const current = get(activeTheme);
  if (!current) return null;
  // The draft's CSS reaches the window at once, so it gets the same check the
  // backend applies on save before it is shown.
  const cssError = validateCss(theme.css ?? '');
  if (cssError) return cssError;
  if (trialRestore === null) trialRestore = current;
  const kept = get(themeTrial)?.kept ?? false;
  setActive(theme, true);
  if (trialTimer !== null) clearTimeout(trialTimer);
  trialTimer = null;
  themeTrial.set({ name: theme.name, endsAt: kept ? 0 : Date.now() + ms, kept });
  if (!kept) trialTimer = setTimeout(endTrial, ms);
  return null;
}

export function keepTrial() {
  const trial = get(themeTrial);
  if (!trial) return;
  if (trialTimer !== null) clearTimeout(trialTimer);
  trialTimer = null;
  themeTrial.set({ ...trial, endsAt: 0, kept: true });
}

export function endTrial() {
  const back = trialRestore;
  const confirm = deferredConfirm;
  clearTrial();
  if (!back) return;
  setActive(back);
  if (confirm === back.id) scheduleConfirm(back.id);
}

// A deleted theme may be the real active one (the backend then falls back to
// the default) or only the draft being tried; either way the window must not
// keep or go back to it.
export async function forgetTheme(id: string) {
  const real = realActiveTheme();
  if (real?.id === id) {
    setActive(await fetchActiveTheme());
  } else if (trialRestore !== null && get(activeTheme)?.id === id) {
    endTrial();
  }
}

export async function refreshThemes() {
  try {
    themeList.set(await listThemes());
  } catch (err) {
    toast(themeErrorText(err), 'danger');
  }
}

export async function selectTheme(id: string) {
  if (!inWails) return;
  cancelConfirm();
  const system = id === 'system';
  themeMode.set(system ? 'system' : 'theme');
  const targetId = system ? resolveSystemId() : id;
  try {
    await applyThemeRequest(targetId);
    const applied = await fetchActiveTheme();
    setActive(applied);
    scheduleConfirm(applied.id);
  } catch (err) {
    toast(themeErrorText(err), 'danger');
  }
}

function onSystemChange() {
  systemPrefersDark.set(media?.matches ?? true);
  if (get(themeMode) !== 'system') return;
  selectTheme('system');
}

export async function resetAppearance() {
  appearanceResetVersion.update(value => value + 1);
  cancelConfirm();
  const previous = get(activeTheme);
  // Keep the emergency reset usable even when custom CSS hides the interface.
  clearThemeDom();
  try {
    await resetThemeRequest();
    await updateSettings({ accentColor: '', tintLogo: false });
  } catch (err) {
    if (previous) setActive(previous);
    toast(themeErrorText(err), 'danger');
    return;
  }
  try {
    const applied = await fetchActiveTheme();
    setActive(applied);
  } catch (err) {
    toast(themeErrorText(err), 'danger');
  }
}

export async function initTheme() {
  media = window.matchMedia('(prefers-color-scheme: dark)');
  systemPrefersDark.set(media.matches);
  media.addEventListener('change', onSystemChange);

  // The backend may re-read a watched theme file a second after start, so the
  // events are subscribed before the first read: nothing in between is lost.
  if (inWails) subscribeThemeEvents();

  try {
    setActive(await fetchActiveTheme());
    confirmAccent();
  } catch (err) {
    toast(themeErrorText(err), 'danger');
  }

  if (!inWails) return;

  await refreshThemes();
}

function subscribeThemeEvents() {
  Events.On('theme:updated', (event) => {
    const theme = toTheme(event.data as BindingTheme);
    themeList.update((list) => {
      const index = list.findIndex((t) => t.id === theme.id);
      if (index < 0) return [...list, theme];
      const next = [...list];
      next[index] = theme;
      return next;
    });
    // During a trial the window shows a draft; an update to the real theme only
    // moves the point the trial goes back to, and nothing becomes active
    // without Apply and Confirm on the backend.
    if (trialRestore !== null) {
      if (trialRestore.id === theme.id) trialRestore = theme;
      return;
    }
    if (get(activeTheme)?.id === theme.id) {
      setActive(theme);
      confirmAccent();
    }
  });

  Events.On('theme:source-failed', (event) => {
    const info = event.data as { id: string; name: string; code: string; message: string };
    toast(msg('settings.appearanceSourceFailedToast', { name: info.name, reason: themeCodeText(info.code, info.message) }), 'danger');
  });

  Events.On('theme:list', (event) => {
    const list = (event.data as BindingTheme[] | null) ?? [];
    themeList.set(list.map(toTheme));
  });

  Events.On('theme:reverted', (event) => {
    cancelConfirm();
    setActive(toTheme(event.data as BindingTheme));
  });
}
