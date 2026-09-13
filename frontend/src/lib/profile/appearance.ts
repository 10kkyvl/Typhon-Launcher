import { accentPalette } from '../theme/accent';
import type { ProfileAppearance } from '../services/account';

export const DEFAULT_APPEARANCE: ProfileAppearance = {
  theme: 'midnight', accent: '#67d8ef', coverUrl: '', coverDim: 35, coverPosition: 50,
};
export const PROFILE_THEMES = [
  { id: 'midnight', background: '#0e141e', surface: '#151d29', banner: 'linear-gradient(125deg, #142235, #29394d 55%, #111923)' },
  { id: 'orbital', background: '#0b161f', surface: '#12232e', banner: 'linear-gradient(125deg, #142530, #345d72 55%, #111e31)' },
  { id: 'forest', background: '#101b18', surface: '#192b24', banner: 'linear-gradient(125deg, #13271f, #3e6450 55%, #102b29)' },
  { id: 'neon', background: '#191223', surface: '#281c35', banner: 'linear-gradient(125deg, #31233d, #563971 55%, #232244)' },
  { id: 'mono', background: '#17191c', surface: '#25282c', banner: 'linear-gradient(125deg, #26292f, #53575d 55%, #202329)' },
  { id: 'solar', background: '#201711', surface: '#30231b', banner: 'linear-gradient(125deg, #372519, #805039 55%, #382226)' },
] as const;
export const PROFILE_ACCENTS = ['#67d8ef', '#729bff', '#b193ff', '#f3b56b', '#ed7889', '#91c59c'];

export function appearanceOf(value?: Partial<ProfileAppearance> | null): ProfileAppearance {
  const a = { ...DEFAULT_APPEARANCE, ...value };
  return {
    theme: PROFILE_THEMES.some((t) => t.id === a.theme) ? a.theme : DEFAULT_APPEARANCE.theme,
    accent: /^#[0-9a-f]{6}$/i.test(a.accent) ? a.accent : DEFAULT_APPEARANCE.accent,
    coverUrl: typeof a.coverUrl === 'string' ? a.coverUrl : '',
    coverDim: Number.isFinite(a.coverDim) ? Math.min(100, Math.max(0, a.coverDim)) : 35,
    coverPosition: Number.isFinite(a.coverPosition) ? Math.min(100, Math.max(0, a.coverPosition)) : 50,
  };
}

export function themeOf(value: ProfileAppearance) {
  return PROFILE_THEMES.find((t) => t.id === value.theme) ?? PROFILE_THEMES[0];
}

export function appearancePalette(value: ProfileAppearance): Record<string, string> {
  const theme = themeOf(value);
  return accentPalette(value.accent, 'dark', {
    '--bg': theme.background, '--surface': theme.background,
    '--surface-2': theme.surface, '--surface-3': theme.surface, '--surface-4': theme.surface, '--bg-sidebar': theme.background,
  });
}
export function appearanceAccentStyle(value: ProfileAppearance): string {
  return Object.entries(appearancePalette(value)).map(([key, color]) => `${key}:${color}`).join(';');
}
