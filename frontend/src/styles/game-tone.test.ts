import { describe, expect, it } from 'vitest';
import buttonSource from '../lib/components/Button.svelte?raw';
import cardSource from '../lib/components/GameCard.svelte?raw';
import tabsSource from '../lib/components/Tabs.svelte?raw';
import { gameToneOf, toneVars } from '../lib/art/tone';
import { ruleBody, styleOf } from '../lib/testing/css';
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
    expect(rootNames('./shell.css')).toContain('--accent-on-default');
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

// The built-in presets' tokens (internal/theme/presets.go) and the fixed label default of shell.css.
const PRESETS = [...(goSources['internal/theme/presets.go'] ?? '').matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)].map((m) => ({
  id: /\bID:\s*"([^"]+)"/.exec(m[1])?.[1] ?? '',
  tokens: { '--accent-on-default': '#fff', ...Object.fromEntries([...m[1].matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((t) => [t[1], t[2]])) } as Record<string, string>,
}));

/** What a declaration computes to with these custom properties in scope, var() fallbacks included. */
function computed(expr: string, tokens: Record<string, string>): string {
  const ref = /^var\((--[\w-]+)(?:\s*,\s*([\s\S]+))?\)$/.exec(expr.trim());
  if (!ref) return expr.trim();
  if (ref[1] in tokens) return computed(tokens[ref[1]], tokens);
  if (ref[2] === undefined) throw new Error(`${ref[1]} has no value and no fallback`);
  return computed(ref[2], tokens);
}

const BUTTON = styleOf(buttonSource, 'Button.svelte');
const CARD = styleOf(cardSource, 'GameCard.svelte');
const ENABLED = ":not(:disabled, [aria-disabled='true'])";

// What a launching control paints, at rest, hovered and pressed, with the properties in scope.
function button(tokens: Record<string, string>) {
  return {
    rest: computed(ruleBody(BUTTON, '.play').background, tokens),
    label: computed(ruleBody(BUTTON, '.play').color, tokens),
    hover: computed(ruleBody(BUTTON, `.play:hover${ENABLED}`).background, tokens),
    glow: ruleBody(BUTTON, `.play:hover${ENABLED}`)['box-shadow'].replace(/var\([\s\S]+\)$/, (v) => computed(v, tokens)),
    pressed: computed(ruleBody(BUTTON, `.play:active${ENABLED}`).background, tokens),
  };
}

function disc(tokens: Record<string, string>) {
  return {
    rest: computed(ruleBody(CARD, '.play').background, tokens),
    glyph: computed(ruleBody(CARD, '.play').color, tokens),
    hover: computed(ruleBody(CARD, '.play:hover').background, tokens),
    pressed: computed(ruleBody(CARD, ".play:active:not([aria-busy='true'])").background, tokens),
  };
}

describe('a launching control outside any game is the primary fill it was', () => {
  it('reads the four presets', () => {
    expect(PRESETS.map((p) => p.id)).toEqual(['dark', 'light', 'contrast', 'blush']);
  });

  it("the primary button's own rules are unchanged", () => {
    expect(ruleBody(BUTTON, '.primary')).toEqual({ background: 'var(--accent)', color: 'var(--accent-on, var(--accent-on-default))' });
    expect(ruleBody(BUTTON, `.primary:hover${ENABLED}`)).toEqual({
      background: 'var(--accent-hover)',
      'box-shadow': '0 0.8rem 2rem -0.8rem var(--accent-ring)',
      transform: 'translateY(-1px)',
    });
  });

  for (const accentOn of [undefined, '#000000']) {
    for (const { id, tokens: preset } of PRESETS) {
      const tokens = accentOn ? { ...preset, '--accent-on': accentOn } : preset;
      const name = `${id}${accentOn ? ' with a personal accent' : ''}`;

      it(`${name}: Play and Stop draw the primary fill, label, hover, glow and pressed fill`, () => {
        const primary = ruleBody(BUTTON, '.primary');
        const hover = ruleBody(BUTTON, `.primary:hover${ENABLED}`);
        expect(button(tokens)).toEqual({
          rest: computed(primary.background, tokens),
          label: computed(primary.color, tokens),
          hover: computed(hover.background, tokens),
          glow: hover['box-shadow'].replace(/var\([\s\S]+\)$/, (v) => computed(v, tokens)),
          // Pressed under the pointer the primary button shows its hover fill.
          pressed: computed(hover.background, tokens),
        });
      });

      it(`${name}: a card's round Play draws the accent fill and label, its hover fill when hovered or pressed`, () => {
        expect(disc(tokens)).toEqual({
          rest: computed('var(--accent)', tokens),
          glyph: computed('var(--accent-on, var(--accent-on-default))', tokens),
          hover: computed('var(--accent-hover)', tokens),
          pressed: computed('var(--accent-hover)', tokens),
        });
      });
    }
  }

  it("the tab marker is --accent-bright unless a page hands it another colour", () => {
    const marker = ruleBody(styleOf(tabsSource, 'Tabs.svelte'), '.underline .plate::after').background;
    expect(computed(marker, PRESETS[0].tokens)).toBe(PRESETS[0].tokens['--accent-bright']);
    expect(computed(marker, { ...PRESETS[0].tokens, '--tab-marker': '#123456' })).toBe('#123456');
  });
});

describe("a launching control inside a game's scope takes the game's colour", () => {
  for (const { id, tokens: preset } of PRESETS) {
    it(`${id}: Play's rest, label, hover, glow and pressed fill, and the card's disc, come from the game`, () => {
      const v = toneVars(gameToneOf([200, 40, 30], { bg: preset['--bg'], text: preset['--text'] }));
      const tokens = { ...preset, ...v, ...ruleBody(css('./shell.css'), '.game-tone') };
      expect(button(tokens)).toEqual({
        rest: v['--game-fill'],
        label: v['--game-on-fill'],
        hover: v['--game-fill-hover'],
        glow: `0 0.8rem 2rem -0.8rem ${v['--game-glow']}`,
        pressed: v['--game-fill-active'],
      });
      expect(disc(tokens)).toEqual({ rest: v['--game-fill'], glyph: v['--game-glyph'], hover: v['--game-disc-hover'], pressed: v['--game-disc-active'] });
    });
  }
});
