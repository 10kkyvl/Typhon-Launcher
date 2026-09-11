import { writable } from 'svelte/store';
import { accentPalette, validAccent } from './accent';
import type { Theme } from '../../../bindings/typhon/internal/theme';
import { msg, type MessageKey } from '../i18n';

const STYLE_ELEMENT_ID = 'typhon-theme';
export const displayedAccent = writable('#6673F2');

interface NamedTheme {
  id: string;
  name: string;
  builtIn: boolean;
}

// internal/theme/presets.go hardcodes each built-in preset's `Name` in
// Russian — it stays the stored value and the fallback (no migration needed
// for themes already saved by users), but the window resolves a built-in
// theme's label from its stable `id` through the catalog instead, so it
// matches the current interface language and switches immediately when the
// language does. A new preset id needs a new key here or its label stays in
// the stored language. An imported theme (`builtIn: false`) has no catalog
// entry and nothing else to translate to, so its own name passes through.
const builtInThemeNameKeys: Record<string, MessageKey> = {
  dark: 'settings.appearanceThemeNameDark',
  light: 'settings.appearanceThemeNameLight',
  contrast: 'settings.appearanceThemeNameContrast',
};

export function themeDisplayName(theme: NamedTheme): string {
  if (!theme.builtIn) return theme.name;
  const key = builtInThemeNameKeys[theme.id];
  return key ? msg(key) : theme.name;
}

let appliedTokenNames: string[] = [];
let currentTheme: Theme | null = null;
let personalColor = '';

export function applyPersonalAccent(color: string): void {
  if ((color && !validAccent(color)) || color === personalColor) return;
  personalColor = color;
  if (currentTheme) applyTheme(currentTheme);
}

export function personalThemeVars(theme: Theme): Record<string, string> {
  return { ...themeVars(theme), ...accentPalette(personalColor, theme.base, theme.tokens ?? {}) };
}

export function themeVars(theme: Theme): Record<string, string> {
  const tokens = theme.tokens ?? {};
  const vars: Record<string, string> = {};
  for (const name of Object.keys(tokens)) {
    if (name === '--ui-scale') continue;
    const value = tokens[name];
    if (value === undefined) continue;
    vars[name] = value;
  }
  return vars;
}

function styleElement(): HTMLStyleElement {
  const existing = document.getElementById(STYLE_ELEMENT_ID) as HTMLStyleElement | null;
  if (existing) return existing;
  const created = document.createElement('style');
  created.id = STYLE_ELEMENT_ID;
  document.head.appendChild(created);
  return created;
}

export function applyTheme(theme: Theme): void {
  currentTheme = theme;
  const vars = personalThemeVars(theme);
  const root = document.documentElement;
  for (const name of appliedTokenNames) {
    if (!(name in vars)) root.style.removeProperty(name);
  }
  for (const [name, value] of Object.entries(vars)) {
    root.style.setProperty(name, value, personalColor && name.startsWith('--accent') ? 'important' : '');
  }
  appliedTokenNames = Object.keys(vars);
  styleElement().textContent = theme.css ?? '';
  const themeAccent = typeof getComputedStyle === 'function'
    ? getComputedStyle(root).getPropertyValue('--accent').trim()
    : vars['--accent'];
  displayedAccent.set(personalColor || themeAccent || '#6673F2');
}

export function clearTheme(): void {
  currentTheme = null;
  const root = document.documentElement;
  for (const name of appliedTokenNames) {
    root.style.removeProperty(name);
  }
  appliedTokenNames = [];
  const existing = document.getElementById(STYLE_ELEMENT_ID) as HTMLStyleElement | null;
  if (existing) existing.textContent = '';
}
