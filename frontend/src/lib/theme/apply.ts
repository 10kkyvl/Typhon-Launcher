import type { Theme } from '../../../bindings/typhon/internal/theme';

const STYLE_ELEMENT_ID = 'typhon-theme';

let appliedTokenNames: string[] = [];

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
  const vars = themeVars(theme);
  const root = document.documentElement;
  for (const name of appliedTokenNames) {
    if (!(name in vars)) root.style.removeProperty(name);
  }
  for (const [name, value] of Object.entries(vars)) {
    root.style.setProperty(name, value);
  }
  appliedTokenNames = Object.keys(vars);
  styleElement().textContent = theme.css ?? '';
}

export function clearTheme(): void {
  const root = document.documentElement;
  for (const name of appliedTokenNames) {
    root.style.removeProperty(name);
  }
  appliedTokenNames = [];
  const existing = document.getElementById(STYLE_ELEMENT_ID) as HTMLStyleElement | null;
  if (existing) existing.textContent = '';
}
