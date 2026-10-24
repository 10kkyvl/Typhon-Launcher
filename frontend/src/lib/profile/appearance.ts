import { accentPalette, validAccent } from '../theme/accent';
import { ACCENT_SOURCES, AUTO_SOURCES, NAME_STYLES, type ProfileAppearance } from '../services/account';
import { DARK_INK, darkenUntil, deriveSurfaces, mixHex, PROFILE_TEXT_3, pureSurfaces, READABLE, smoothStops, type CanvasInk } from './appearanceColor';
import type { ArtPalette } from './artPalette';
import { isCustomFrame, presetFrame } from './frameLook';

export const DEFAULT_APPEARANCE: ProfileAppearance = {
  theme: 'midnight', accent: '#67d8ef', coverUrl: '', coverDim: 35, coverPosition: 50,
  customFrom: '#142235', customTo: '#111923', customAngle: 125, autoSource: 'playing',
  avatarFrame: 'none', nameStyle: 'plain', parallax: true, nameColor: '', frameColor: '', customPure: false, accentSource: 'custom',
};

export interface ProfileTheme {
  id: string; background: string; surface: string; banner: string; ink?: CanvasInk;
  stops: string[]; angle: number; noise: boolean;
}
type ProfilePreset = Pick<ProfileTheme, 'id' | 'background' | 'surface' | 'banner'>;

export const PROFILE_THEMES = [
  { id: 'midnight', background: '#0e141e', surface: '#151d29', banner: 'linear-gradient(125deg, #142235, #29394d 55%, #111923)' },
  { id: 'black', background: '#000000', surface: '#0c0c0f', banner: 'linear-gradient(125deg, #000000, #17171c 55%, #000000)' },
  { id: 'orbital', background: '#0b161f', surface: '#12232e', banner: 'linear-gradient(125deg, #142530, #345d72 55%, #111e31)' },
  { id: 'forest', background: '#101b18', surface: '#192b24', banner: 'linear-gradient(125deg, #13271f, #3e6450 55%, #102b29)' },
  { id: 'neon', background: '#191223', surface: '#281c35', banner: 'linear-gradient(125deg, #31233d, #563971 55%, #232244)' },
  { id: 'mono', background: '#17191c', surface: '#25282c', banner: 'linear-gradient(125deg, #26292f, #53575d 55%, #202329)' },
  { id: 'solar', background: '#201711', surface: '#30231b', banner: 'linear-gradient(125deg, #372519, #805039 55%, #382226)' },
] as const satisfies readonly ProfilePreset[];
export const PROFILE_ACCENTS = [
  '#67d8ef', '#729bff', '#b193ff', '#ee7fc4', '#ed7889', '#f08f5a',
  '#f3b56b', '#f2d45c', '#a6d756', '#91c59c', '#5ad1a7', '#dfe5ee',
];

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
    avatarFrame: isCustomFrame(a.avatarFrame) ? a.avatarFrame : presetFrame(a.avatarFrame),
    nameStyle: oneOf(NAME_STYLES, a.nameStyle, defaults.nameStyle),
    parallax: typeof a.parallax === 'boolean' ? a.parallax : defaults.parallax,
    nameColor: hexOr(a.nameColor, ''),
    frameColor: hexOr(a.frameColor, ''),
    customPure: a.customPure === true,
    accentSource: oneOf(ACCENT_SOURCES, a.accentSource, defaults.accentSource),
  };
}

export function customBanner(value: Pick<ProfileAppearance, 'customFrom' | 'customTo' | 'customAngle'>): string {
  return `linear-gradient(${value.customAngle}deg, ${value.customFrom}, ${value.customTo})`;
}

export function pickCustomColor(value: ProfileAppearance, color: string): ProfileAppearance {
  if (value.theme === CUSTOM_THEME) return { ...value, customFrom: color };
  return { ...value, theme: CUSTOM_THEME, customFrom: color, customTo: color, customPure: true };
}

const PAGE_ANGLE = 125;

function readableStops(colors: string[]): string[] {
  return colors.map((color) => darkenUntil(color, PROFILE_TEXT_3, READABLE));
}

export function themeOf(value: ProfileAppearance, art?: ArtPalette | null): ProfileTheme {
  if (value.theme === CUSTOM_THEME && value.customPure) {
    const { background, surface, ink, stops } = pureSurfaces(value.customFrom, value.customTo);
    return {
      id: CUSTOM_THEME, background, surface, banner: customBanner(value), ink,
      stops, angle: value.customAngle, noise: false,
    };
  }
  if (value.theme === CUSTOM_THEME) {
    const { background, surface } = deriveSurfaces(value.customFrom, value.customTo);
    return {
      id: CUSTOM_THEME, background, surface, banner: customBanner(value),
      stops: readableStops([value.customFrom, value.customTo]), angle: value.customAngle, noise: true,
    };
  }
  if (value.theme === AUTO_THEME && art) {
    return {
      id: AUTO_THEME, background: art.background, surface: art.surface,
      banner: `linear-gradient(${PAGE_ANGLE}deg, ${art.surface}, ${art.background})`,
      stops: readableStops([mixHex(art.background, art.accent, 0.35), art.background]), angle: PAGE_ANGLE, noise: true,
    };
  }
  const preset = PROFILE_THEMES.find((t) => t.id === value.theme) ?? PROFILE_THEMES[0];
  return {
    ...preset, stops: readableStops(preset.banner.match(/#[0-9a-f]{6}/gi) ?? [preset.background]),
    angle: PAGE_ANGLE, noise: true,
  };
}

const NOISE = "url(\"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='160' height='160'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2' stitchTiles='stitch'/><feColorMatrix values='0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 .06 0'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>\")";

export { smoothStops };

export function pageGradient(theme: ProfileTheme): string {
  const stops = smoothStops(theme.stops);
  const list = stops.map((color, i) => `${color} ${Math.round((i / (stops.length - 1)) * 100)}%`);
  return `linear-gradient(${theme.angle}deg, ${list.join(', ')})`;
}

export function pageBackground(theme: ProfileTheme): string {
  const gradient = pageGradient(theme);
  return theme.noise ? `${NOISE}, ${gradient}` : gradient;
}

export function appearancePalette(value: ProfileAppearance, art?: ArtPalette | null): Record<string, string> {
  const theme = themeOf(value, art);
  const accent = value.theme === AUTO_THEME && art ? art.accent : value.accent;
  return accentPalette(accent, theme.ink === DARK_INK ? 'light' : 'dark', {
    '--bg': theme.background, '--surface': theme.background,
    '--surface-2': theme.surface, '--surface-3': theme.surface, '--surface-4': theme.surface, '--bg-sidebar': theme.background,
  });
}
export function appearanceAccentStyle(value: ProfileAppearance, art?: ArtPalette | null): string {
  return Object.entries(appearancePalette(value, art)).map(([key, color]) => `${key}:${color}`).join(';');
}
