import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
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
import { applyTheme as applyThemeDom, clearTheme as clearThemeDom } from '../theme/apply';
import { errorMessage } from '../utils/errors';
import { toast } from './toasts';

export type ThemeMode = 'system' | 'theme';

const CONFIRM_DELAY_MS = 5000;

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
    if (get(activeTheme)?.id !== id) return;
    confirmThemeRequest(id).catch((err) => toast(errorMessage(err), 'danger'));
  }, CONFIRM_DELAY_MS);
}

function resolveSystemId(): string {
  return get(systemPrefersDark) ? 'dark' : 'light';
}

function setActive(theme: Theme) {
  activeTheme.set(theme);
  applyThemeDom(theme);
}

export async function refreshThemes() {
  try {
    themeList.set(await listThemes());
  } catch (err) {
    toast(errorMessage(err), 'danger');
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
    toast(errorMessage(err), 'danger');
  }
}

function onSystemChange() {
  systemPrefersDark.set(media?.matches ?? true);
  if (get(themeMode) !== 'system') return;
  selectTheme('system');
}

export async function resetAppearance() {
  cancelConfirm();
  clearThemeDom();
  try {
    await resetThemeRequest();
  } catch (err) {
    toast(errorMessage(err), 'danger');
    return;
  }
  try {
    const applied = await fetchActiveTheme();
    setActive(applied);
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export async function initTheme() {
  media = window.matchMedia('(prefers-color-scheme: dark)');
  systemPrefersDark.set(media.matches);
  media.addEventListener('change', onSystemChange);

  try {
    setActive(await fetchActiveTheme());
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }

  if (!inWails) return;

  await refreshThemes();

  Events.On('theme:updated', (event) => {
    const theme = toTheme(event.data as BindingTheme);
    themeList.update((list) => {
      const index = list.findIndex((t) => t.id === theme.id);
      if (index < 0) return [...list, theme];
      const next = [...list];
      next[index] = theme;
      return next;
    });
    if (get(activeTheme)?.id === theme.id) setActive(theme);
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
