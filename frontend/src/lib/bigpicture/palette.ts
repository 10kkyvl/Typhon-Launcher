import { accentPalette, validAccent } from '../theme/accent';
import { normalizeColor } from '../profile/colorNormalize';

export const bigPicturePalette: Readonly<Record<string, string>> = {
  '--bg': '#0e0f11',
  '--bg-sidebar': '#0b0c0e',
  '--surface': '#16181b',
  '--surface-2': '#1e2024',
  '--surface-3': '#25282c',
  '--surface-4': '#2c2f34',
  '--border': 'rgba(255, 255, 255, 0.07)',
  '--border-strong': 'rgba(255, 255, 255, 0.13)',
  '--hover': 'rgba(255, 255, 255, 0.045)',
  '--hover-strong': 'rgba(255, 255, 255, 0.08)',
  '--text': '#edeef0',
  '--text-2': '#b6b9bf',
  '--text-3': '#93979e',
};

// Big Picture paints the dark preset's surfaces whatever the theme, so every colour drawn on them
// comes from the dark preset as well; a light theme's dark status and accent text would vanish there.
const darkStatus: Readonly<Record<string, string>> = {
  '--success': '#55cf8d',
  '--success-subtle': 'rgba(85, 207, 141, 0.14)',
  '--warning': '#e0a85a',
  '--warning-subtle': 'rgba(224, 168, 90, 0.14)',
  '--danger': '#ef7070',
  '--danger-subtle': 'rgba(239, 112, 112, 0.13)',
};

const darkAccent: Readonly<Record<string, string>> = {
  '--accent': '#0b7285',
  '--accent-hover': '#0d7a8f',
  '--accent-subtle': 'rgba(44, 207, 232, 0.16)',
  '--accent-text': '#8be9f7',
  '--accent-ring': 'rgba(44, 207, 232, 0.75)',
  '--accent-bright': '#2ccfe8',
  '--accent-on': '#ffffff',
};

// The accent the window shows (a personal one, else the theme's) keeps its hue in Big Picture,
// re-derived for these surfaces; the dark preset's own accent, or one that cannot be read as a
// colour, takes the dark preset's family as designed.
export function bigPictureVars(accent: string): Record<string, string> {
  const seed = normalizeColor(accent) ?? '';
  const family = !validAccent(seed) || seed === darkAccent['--accent'] ? darkAccent : accentPalette(seed, 'dark', bigPicturePalette);
  return { ...bigPicturePalette, ...darkStatus, ...family };
}

export function bigPictureStyle(accent: string): string {
  return Object.entries(bigPictureVars(accent))
    .map(([name, value]) => `${name}: ${value}`)
    .join('; ');
}
