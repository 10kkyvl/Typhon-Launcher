import { Service } from '../../../bindings/typhon/internal/theme';
import type { Theme as BindingTheme } from '../../../bindings/typhon/internal/theme';
import { inWails } from './backend';

export type { BindingTheme };

export type ThemeBase = 'dark' | 'light';

export interface Theme {
  id: string;
  name: string;
  base: ThemeBase;
  tokens: Record<string, string>;
  css?: string;
  builtIn: boolean;
  updatedAt: string;
}

const unavailable = () => new Error('unavailable in browser');

function cleanTokens(tokens: BindingTheme['tokens']): Record<string, string> {
  const clean: Record<string, string> = {};
  for (const [name, value] of Object.entries(tokens ?? {})) {
    if (name === '--ui-scale') continue;
    if (value !== undefined) clean[name] = value;
  }
  return clean;
}

export function toTheme(raw: BindingTheme): Theme {
  return {
    id: raw.id,
    name: raw.name,
    base: raw.base === 'light' ? 'light' : 'dark',
    tokens: cleanTokens(raw.tokens),
    css: raw.css,
    builtIn: raw.builtIn,
    updatedAt: raw.updatedAt,
  };
}

function fromTheme(theme: Theme): BindingTheme {
  return {
    id: theme.id,
    name: theme.name,
    base: theme.base,
    tokens: theme.tokens,
    css: theme.css,
    builtIn: theme.builtIn,
    updatedAt: theme.updatedAt,
  };
}

function fallbackTheme(): Theme {
  return {
    id: 'dark',
    name: 'Тёмная',
    base: 'dark',
    tokens: {},
    css: '',
    builtIn: true,
    updatedAt: new Date(0).toISOString(),
  };
}

export async function listThemes(): Promise<Theme[]> {
  if (!inWails) return [];
  const list = (await Service.List()) ?? [];
  return list.map(toTheme);
}

export async function getTheme(id: string): Promise<Theme> {
  if (!inWails) throw unavailable();
  return toTheme(await Service.Get(id));
}

export async function saveTheme(theme: Theme): Promise<Theme> {
  if (!inWails) throw unavailable();
  return toTheme(await Service.Save(fromTheme(theme)));
}

export async function deleteTheme(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Service.Delete(id);
}

export async function applyTheme(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Service.Apply(id);
}

export async function confirmTheme(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Service.Confirm(id);
}

export async function activeTheme(): Promise<Theme> {
  if (!inWails) return fallbackTheme();
  return toTheme(await Service.Active());
}

export async function importTheme(path: string): Promise<Theme> {
  if (!inWails) throw unavailable();
  return toTheme(await Service.Import(path));
}

export async function exportTheme(id: string, path: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Service.Export(id, path);
}

export async function resetTheme(): Promise<void> {
  if (!inWails) throw unavailable();
  await Service.Reset();
}

export async function selectThemeFile(): Promise<string> {
  if (!inWails) return '';
  return await Service.SelectThemeFile();
}

export async function selectExportPath(): Promise<string> {
  if (!inWails) return '';
  return await Service.SelectExportPath();
}
