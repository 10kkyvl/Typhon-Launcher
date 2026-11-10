import { accentPalette, contrast, rgb, validAccent } from '../theme/accent';
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

const accentNames = ['--accent', '--accent-hover', '--accent-subtle', '--accent-text', '--accent-ring', '--accent-bright'];
const measurable = /^(?:#[0-9a-f]{6}|rgba?\([^()]*\))$/i;
const surfaces = ['--bg', '--bg-sidebar', '--surface', '--surface-2', '--surface-3', '--surface-4'];
const low = ['--bg', '--surface', '--surface-2'];
const hex = (c: number[]) => '#' + c.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');

// The pairs Big Picture draws with an accent family on its own surfaces.
function readsOnBigPicture(family: Record<string, string>): boolean {
  const bg = rgb(bigPicturePalette['--bg'], [0, 0, 0]);
  const ratio = (fg: string, under: number[]) => contrast(hex(rgb(fg, under as [number, number, number])), hex(under));
  const on = (name: string) => (name === '--bg' ? bg : rgb(bigPicturePalette[name], bg));
  return (
    low.every((s) => ratio(family['--accent-text'], on(s)) >= 4.5) &&
    surfaces.every((s) => ratio(family['--accent-bright'], on(s)) >= 3 && ratio(family['--accent-ring'], on(s)) >= 3) &&
    ['--accent', '--accent-hover'].every((fill) => contrast(family['--accent-on'], family[fill]) >= 4.5)
  );
}

// A dark theme's own accent family, when it is whole, plainly written and reads on these surfaces.
function ownFamily(tokens: Readonly<Record<string, string | undefined>>): Record<string, string> | null {
  const family: Record<string, string> = { '--accent-on': '#ffffff' };
  for (const name of accentNames) {
    const value = tokens[name]?.trim();
    if (!value || !measurable.test(value)) return null;
    family[name] = value;
  }
  return readsOnBigPicture(family) ? family : null;
}

export interface BigPictureAccent {
  // The accent the window shows: the personal one, else the theme's.
  shown: string;
  theme: { base: string; tokens?: Readonly<Record<string, string | undefined>> | null } | null;
  personal: string;
}

// With no personal accent, a dark theme keeps its own accent family in Big Picture when it reads
// there. Otherwise the shown accent keeps its hue, re-derived for these surfaces; the dark preset's
// own accent, or one that cannot be read as a colour, takes the dark preset's family as designed.
export function bigPictureVars({ shown, theme, personal }: BigPictureAccent): Record<string, string> {
  const own = !validAccent(personal) && theme?.base === 'dark' ? ownFamily(theme.tokens ?? {}) : null;
  const seed = normalizeColor(shown) ?? '';
  const family = own ?? (!validAccent(seed) || seed === darkAccent['--accent'] ? darkAccent : accentPalette(seed, 'dark', bigPicturePalette));
  return { ...bigPicturePalette, ...darkStatus, ...family };
}

export function bigPictureStyle(accent: BigPictureAccent): string {
  return Object.entries(bigPictureVars(accent))
    .map(([name, value]) => `${name}: ${value}`)
    .join('; ');
}
