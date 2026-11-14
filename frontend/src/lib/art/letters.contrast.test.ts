import { describe, expect, it } from 'vitest';
import artworkSource from '../components/Artwork.svelte?raw';
import { ruleBody, styleOf } from '../testing/css';
import { luminance } from '../theme/accent';
import { artScene, artSpec, type ArtShape, type Sky } from './scene';

/**
 * Artwork draws a game's letters over its generated art. Whatever the palette (a night sky or a pale
 * day one), the letters must read against what is behind them: their plate painted over every colour
 * the scene draws. Large decorative text: 3:1.
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

type RGB = [number, number, number];
type RGBA = [number, number, number, number];

const SKIES: Sky[] = ['dusk', 'dawn', 'night', 'day', 'storm', 'ember', 'neon', 'toxic', 'space'];
const WHITE: RGB = [255, 255, 255];

function shellTokens(): Record<string, string> {
  const css = fs.readFileSync(new URL('../../styles/shell.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
  return Object.fromEntries([...css.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
}

/** A token the rule names (`var(--x)`), as shell.css writes it: #rgb, #rrggbb or rgba(). */
function tokenColour(expr: string, tokens: Record<string, string>): RGBA {
  const name = /^var\((--[\w-]+)\)$/.exec(expr.trim())?.[1];
  expect(name, `${expr} names a token`).toBeTypeOf('string');
  const value = tokens[name as string];
  expect(value, name).toBeTypeOf('string');
  const hex = /^#([\da-f]{3}|[\da-f]{6})$/i.exec(value)?.[1];
  if (hex) {
    const digits = hex.length === 3 ? [...hex].map((c) => c + c).join('') : hex;
    return [0, 2, 4].map((i) => parseInt(digits.slice(i, i + 2), 16)).concat(1) as RGBA;
  }
  const parts = /^rgba?\(([^)]*)\)$/.exec(value)?.[1].split(',').map(Number);
  expect(parts, value).toBeDefined();
  const [r, g, b, a = 1] = parts as number[];
  return [r, g, b, a];
}

/** `hsla(h, s%, l%, a)` as the scene writes it. */
function hsla(value: string): [RGB, number] {
  const m = /^hsla\(([\d.-]+), ([\d.]+)%, ([\d.]+)%, ([\d.]+)\)$/.exec(value);
  expect(m, value).not.toBeNull();
  const [h, s, l, a] = (m as RegExpExecArray).slice(1).map(Number);
  const c = (1 - Math.abs((2 * l) / 100 - 1)) * (s / 100);
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m0 = l / 100 - c / 2;
  const [r, g, b] = h < 60 ? [c, x, 0] : h < 120 ? [x, c, 0] : h < 180 ? [0, c, x] : h < 240 ? [0, x, c] : h < 300 ? [x, 0, c] : [c, 0, x];
  return [[(r + m0) * 255, (g + m0) * 255, (b + m0) * 255], a];
}

const over = ([r, g, b, a]: RGBA, below: RGB): RGB => [r * a + below[0] * (1 - a), g * a + below[1] * (1 - a), b * a + below[2] * (1 - a)];

function ratio(a: RGB, b: RGB): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

/** Every colour a scene puts on screen, each translucent one over the lightest thing under it. */
function sceneColours(seed: string, shape: ArtShape): RGB[] {
  const scene = artScene(seed, shape);
  const solid = [...scene.sky.map((stop) => stop.color), ...scene.lands.map((land) => land.fill), ...(scene.sun ? [scene.sun.color] : [])].map((value) => {
    const [colour, alpha] = hsla(value);
    return alpha === 1 ? colour : over([...colour, alpha], WHITE);
  });
  const glow = scene.sun ? [over([...hsla(scene.sun.glow)[0], hsla(scene.sun.glow)[1]], solid[0])] : [];
  // The stars are specks of a pixel or two, thinner than a letter's stroke: what a letter stands on
  // is the sky, the sun and the lands.
  return [...solid, ...glow];
}

describe("a game's letters over its generated art", () => {
  const letters = ruleBody(styleOf(artworkSource, 'Artwork.svelte'), '.letters');
  const tokens = shellTokens();
  const ink = tokenColour(letters.color, tokens);
  // No plate: the letters stand on the scene itself.
  const plate: RGBA = letters.background ? tokenColour(letters.background, tokens) : [0, 0, 0, 0];

  it('reads every palette of the generator', () => {
    const seen = new Set<Sky>();
    for (let i = 0; i < 400; i += 1) seen.add(artSpec(`game-${i}`).sky);
    expect([...seen].sort()).toEqual([...SKIES].sort());
  });

  it('read at 3:1 or more against what is behind them, on every palette and both shapes', () => {
    const worst = new Map<Sky, number>();
    for (let i = 0; i < 400; i += 1) {
      const seed = `game-${i}`;
      const sky = artSpec(seed).sky;
      for (const shape of ['cover', 'wide'] as const) {
        for (const behind of sceneColours(seed, shape)) {
          const ground = over(plate, behind);
          const r = ratio(over(ink, ground), ground);
          worst.set(sky, Math.min(worst.get(sky) ?? Infinity, r));
        }
      }
    }
    const failing = [...worst].filter(([, r]) => r < 3).map(([sky, r]) => `${sky}: ${r.toFixed(2)} < 3`);
    expect(worst.size).toBe(SKIES.length);
    expect(failing).toEqual([]);
  });
});
