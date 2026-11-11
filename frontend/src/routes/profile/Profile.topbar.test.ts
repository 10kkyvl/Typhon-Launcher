import { describe, expect, it, vi } from 'vitest';
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
import profile from './Profile.svelte?raw';
import canvas from '../../lib/components/ProfileCanvas.svelte?raw';
import { goSources } from '../../lib/testing/sources';
import { luminance, rgb } from '../../lib/theme/accent';
import { CUSTOM_THEME, appearanceOf, themeOf } from '../../lib/profile/appearance';
import { DARK_INK, LIGHT_INK, type CanvasInk } from '../../lib/profile/appearanceColor';

// The profile editor bar sticks over the profile canvas, whose surfaces and ink come from the profile's own
// theme, not the window's. Whatever the window theme, the bar must read in the canvas's ink.

type RGB = [number, number, number];
const ratio = (a: RGB, b: RGB) => {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
};

function preset(id: string): Record<string, string> {
  const source = goSources['internal/theme/presets.go'];
  const block = [...source.matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)].find((m) => new RegExp(`ID:\\s*"${id}"`).test(m[1]));
  expect(block, id).toBeDefined();
  return Object.fromEntries([...(block as RegExpMatchArray)[1].matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((t) => [t[1], t[2]]));
}

function rule(source: string, selector: string): string {
  const style = /<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';
  const body = new RegExp(`(?:^|\\n)\\s*${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`).exec(style)?.[1];
  expect(body, selector).toBeTypeOf('string');
  return body as string;
}

// What the canvas sets on its element for a profile theme, as ProfileCanvas writes it.
function canvasVars(theme: { background: string; surface: string }, ink: CanvasInk): Record<string, string> {
  const set = [...canvas.matchAll(/style:(--[\w-]+)=/g)].map((m) => m[1]);
  const values: Record<string, string> = {
    '--surface-2': theme.surface,
    '--surface': theme.background,
    '--text': ink.text,
    '--text-2': ink.text2,
    '--text-3': ink.text3,
  };
  return Object.fromEntries(Object.entries(values).filter(([name]) => set.includes(name)));
}

const darkProfile = themeOf(appearanceOf({ theme: 'midnight' }));
const lightProfile = themeOf(appearanceOf({ theme: CUSTOM_THEME, customPure: true, customFrom: '#f4efe6', customTo: '#e6edf5' }));

describe('the profile editor bar', () => {
  const bar = rule(profile, '.topbar');
  const sub = rule(profile, '.topbar-sub');
  const fill = /background\s*:\s*var\((--[\w-]+)\)/.exec(bar)?.[1];
  const subInk = /color\s*:\s*var\((--[\w-]+)\)/.exec(sub)?.[1];

  it('the profile themes used here cover both inks', () => {
    expect(darkProfile.ink ?? LIGHT_INK).toBe(LIGHT_INK);
    expect(lightProfile.ink).toBe(DARK_INK);
  });

  for (const windowTheme of ['dark', 'light']) {
    for (const [name, theme] of [
      ['dark profile', darkProfile],
      ['light profile', lightProfile],
    ] as const) {
      it(`reads on a ${windowTheme} window over a ${name}`, () => {
        expect(fill, 'the bar paints one token').toBeTypeOf('string');
        const window = preset(windowTheme);
        const vars = { ...window, ...canvasVars(theme, theme.ink ?? LIGHT_INK) };
        const page = rgb(theme.background, [0, 0, 0]);
        const surface = rgb(vars[fill as string] ?? '', page);
        expect(ratio(rgb(vars['--text'], surface), surface), '--text').toBeGreaterThanOrEqual(4.5);
        expect(ratio(rgb(vars[subInk as string], surface), surface), subInk).toBeGreaterThanOrEqual(4.5);
      });
    }
  }
});
