import { accentPalette, validAccent } from '../theme/accent';
import { AUTO_SOURCES, AVATAR_FRAMES, NAME_STYLES, type ProfileAppearance } from '../services/account';
import { deriveSurfaces } from './appearanceColor';
import type { ArtPalette } from './artPalette';

export const DEFAULT_APPEARANCE: ProfileAppearance = {
  theme: 'midnight', accent: '#67d8ef', coverUrl: '', coverDim: 35, coverPosition: 50,
  customFrom: '#142235', customTo: '#111923', customAngle: 125, autoSource: 'playing',
  avatarFrame: 'none', nameStyle: 'plain', parallax: true,
};

export interface ProfileTheme { id: string; background: string; surface: string; banner: string }

export const PROFILE_THEMES = [
  { id: 'midnight', background: '#0e141e', surface: '#151d29', banner: 'linear-gradient(125deg, #142235, #29394d 55%, #111923)' },
  { id: 'black', background: '#000000', surface: '#0c0c0f', banner: 'linear-gradient(125deg, #000000, #17171c 55%, #000000)' },
  { id: 'orbital', background: '#0b161f', surface: '#12232e', banner: 'linear-gradient(125deg, #142530, #345d72 55%, #111e31)' },
  { id: 'forest', background: '#101b18', surface: '#192b24', banner: 'linear-gradient(125deg, #13271f, #3e6450 55%, #102b29)' },
  { id: 'neon', background: '#191223', surface: '#281c35', banner: 'linear-gradient(125deg, #31233d, #563971 55%, #232244)' },
  { id: 'mono', background: '#17191c', surface: '#25282c', banner: 'linear-gradient(125deg, #26292f, #53575d 55%, #202329)' },
  { id: 'solar', background: '#201711', surface: '#30231b', banner: 'linear-gradient(125deg, #372519, #805039 55%, #382226)' },
] as const satisfies readonly ProfileTheme[];
export const PROFILE_ACCENTS = ['#67d8ef', '#729bff', '#b193ff', '#f3b56b', '#ed7889', '#91c59c'];

export const CUSTOM_THEME = 'custom';
export const AUTO_THEME = 'auto';

function oneOf<T extends string>(list: readonly T[], value: unknown, fallback: T): T {
  return typeof value === 'string' && (list as readonly string[]).includes(value) ? (value as T) : fallback;
}
function hexOr(value: unknown, fallback: string): string {
  return typeof value === 'string' && validAccent(value) ? value : fallback;
}
function clamped(value: unknown, min: number, max: number, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback;
}

export function appearanceOf(value?: Partial<ProfileAppearance> | null): ProfileAppearance {
  const a: Partial<Record<keyof ProfileAppearance, unknown>> = { ...DEFAULT_APPEARANCE, ...value };
  const defaults = DEFAULT_APPEARANCE;
  return {
    theme: oneOf([...PROFILE_THEMES.map((t) => t.id), CUSTOM_THEME, AUTO_THEME], a.theme, defaults.theme),
    accent: hexOr(a.accent, defaults.accent),
    coverUrl: typeof a.coverUrl === 'string' ? a.coverUrl : '',
    coverDim: clamped(a.coverDim, 0, 100, defaults.coverDim),
    coverPosition: clamped(a.coverPosition, 0, 100, defaults.coverPosition),
    customFrom: hexOr(a.customFrom, defaults.customFrom),
    customTo: hexOr(a.customTo, defaults.customTo),
    customAngle: Math.round(clamped(a.customAngle, 0, 360, defaults.customAngle)),
    autoSource: oneOf(AUTO_SOURCES, a.autoSource, defaults.autoSource),
    avatarFrame: oneOf(AVATAR_FRAMES, a.avatarFrame, defaults.avatarFrame),
    nameStyle: oneOf(NAME_STYLES, a.nameStyle, defaults.nameStyle),
    parallax: typeof a.parallax === 'boolean' ? a.parallax : defaults.parallax,
  };
}

export function customBanner(value: Pick<ProfileAppearance, 'customFrom' | 'customTo' | 'customAngle'>): string {
  return `linear-gradient(${value.customAngle}deg, ${value.customFrom}, ${value.customTo})`;
}

export function themeOf(value: ProfileAppearance, art?: ArtPalette | null): ProfileTheme {
  if (value.theme === CUSTOM_THEME) {
    const { background, surface } = deriveSurfaces(value.customFrom, value.customTo);
    return { id: CUSTOM_THEME, background, surface, banner: customBanner(value) };
  }
  if (value.theme === AUTO_THEME && art) {
    return { id: AUTO_THEME, background: art.background, surface: art.surface, banner: `linear-gradient(125deg, ${art.surface}, ${art.background})` };
  }
  return PROFILE_THEMES.find((t) => t.id === value.theme) ?? PROFILE_THEMES[0];
}

export function appearancePalette(value: ProfileAppearance, art?: ArtPalette | null): Record<string, string> {
  const theme = themeOf(value, art);
  const accent = value.theme === AUTO_THEME && art ? art.accent : value.accent;
  return accentPalette(accent, 'dark', {
    '--bg': theme.background, '--surface': theme.background,
    '--surface-2': theme.surface, '--surface-3': theme.surface, '--surface-4': theme.surface, '--bg-sidebar': theme.background,
  });
}
export function appearanceAccentStyle(value: ProfileAppearance, art?: ArtPalette | null): string {
  return Object.entries(appearancePalette(value, art)).map(([key, color]) => `${key}:${color}`).join(';');
}
