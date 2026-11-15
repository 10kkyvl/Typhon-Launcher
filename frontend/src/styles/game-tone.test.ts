import { describe, expect, it } from 'vitest';
import { toneVars } from '../lib/art/tone';
import { ruleBody } from '../lib/testing/css';
import { goSources } from '../lib/testing/sources';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const css = (file: string) => fs.readFileSync(new URL(file, import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');

function rootNames(file: string): string[] {
  const text = css(file);
  const start = text.indexOf(':root {');
  const body = text.slice(text.indexOf('{', start) + 1, text.indexOf('\n}', start));
  return [...body.matchAll(/(--[\w-]+)\s*:/g)].map((m) => m[1]);
}

/**
 * A game's colour is a per-game value, not a theme token: stores/gameTone.ts sets the `--game-*`
 * properties inline on a game's subtree with the `game-tone` class, and shell.css points the colours a
 * launching control reads (`--play-*`) at them there. Outside such a subtree nothing defines them, so a
 * control falls back, where it paints, to the theme's accent tokens of its own subtree (a subtree that
 * sets its own --accent, as the profile canvas does, keeps it).
 */
describe("the game's colour in the stylesheets", () => {
  const scope = ruleBody(css('./shell.css'), '.game-tone');

  it('the game-tone class points every colour Play reads at the game colour', () => {
    expect(scope).toEqual({
      '--play-bg': 'var(--game-fill)',
      '--play-bg-hover': 'var(--game-fill-hover)',
      '--play-bg-active': 'var(--game-fill-active)',
      '--play-text': 'var(--game-on-fill)',
      '--play-glow': 'var(--game-glow)',
      '--play-glyph': 'var(--game-glyph)',
      '--play-disc-hover': 'var(--game-disc-hover)',
      '--play-disc-active': 'var(--game-disc-active)',
    });
  });

  it('every property the scope sets is read: by the game-tone class, or the tab marker for the accent', () => {
    const tone = { fill: '#2f6f9f', onFill: '#ffffff', accent: '#7fb4dd' };
    const mapped = new Set(Object.values(scope).map((value) => /^var\((--[\w-]+)\)$/.exec(value)?.[1]));
    expect(Object.keys(toneVars(tone)).filter((name) => !mapped.has(name))).toEqual(['--game-accent']);
  });

  it('is no theme token and has no root value: a theme cannot set it, and a subtree accent is not shadowed', () => {
    for (const file of ['./tokens.css', './shell.css']) {
      expect(rootNames(file).filter((name) => /^--(play|game)-/.test(name)), file).toEqual([]);
    }
    for (const file of ['internal/theme/tokens.go', 'internal/theme/presets.go']) {
      const source = goSources[file];
      expect(source, file).toBeTypeOf('string');
      expect(/"--(play|game)-/.test(source), file).toBe(false);
    }
  });
});
