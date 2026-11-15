import { describe, expect, it } from 'vitest';
import buttonSource from '../lib/components/Button.svelte?raw';
import cardSource from '../lib/components/GameCard.svelte?raw';
import graphSource from '../lib/components/SpeedGraph.svelte?raw';
import progressSource from '../lib/components/ProgressBar.svelte?raw';
import tabsSource from '../lib/components/Tabs.svelte?raw';
import { contrast, fromHex, gameToneOf, hslToRgb, toHex, toneVars, type RGB } from '../lib/art/tone';
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
    const tone = { fill: '#2f6f9f', onFill: '#ffffff', accent: '#7fb4dd', mark: '#5a9ccc' };
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
const BAR = styleOf(progressSource, 'ProgressBar.svelte');
const ENABLED = ":not(:disabled, [aria-disabled='true'])";

// What a launching control paints, at rest, hovered and pressed, with the properties in scope.
function button(tokens: Record<string, string>) {
  return {
    rest: computed(ruleBody(BUTTON, '.play').background, tokens),
    label: computed(ruleBody(BUTTON, '.play').color, tokens),
    hover: computed(ruleBody(BUTTON, `.play:hover${ENABLED}`).background, tokens),
    glow: ruleBody(BUTTON, `.play:hover${ENABLED}`)['box-shadow'].replace(/var\([\s\S]+\)$/, (v) => computed(v, tokens)),
    // Pressed under the pointer, and pressed from the keyboard with the pointer elsewhere.
    pressed: computed(ruleBody(BUTTON, `.play:active:hover${ENABLED}`).background, tokens),
    pressedAlone: computed(ruleBody(BUTTON, ".play:active:not(:hover, :disabled, [aria-disabled='true'])").background, tokens),
  };
}

// A progress bar of a tone: its fill and the streak that runs in it while data flows. The accent tone is the
// track's own rule; the streak's fallback is written where it paints.
function bar(tone: 'accent' | 'game', tokens: Record<string, string>) {
  const own = tone === 'accent' ? {} : ruleBody(BAR, `.track[data-tone='${tone}']`);
  const scope = { ...tokens, ...ruleBody(BAR, '.track'), ...own };
  const band = /color-mix\(in srgb, (var\([\s\S]+\)) 30%, transparent\)/.exec(ruleBody(BAR, '.fill.active::after').background)?.[1];
  if (!band) throw new Error('the streak has no colour');
  return { fill: computed(scope['--bar'], scope), streak: computed(band, scope) };
}

function disc(tokens: Record<string, string>) {
  return {
    rest: computed(ruleBody(CARD, '.play').background, tokens),
    glyph: computed(ruleBody(CARD, '.play').color, tokens),
    hover: computed(ruleBody(CARD, '.play:hover').background, tokens),
    pressed: computed(ruleBody(CARD, ".play:active:hover:not([aria-busy='true'])").background, tokens),
    pressedAlone: computed(ruleBody(CARD, ".play:active:not(:hover, [aria-busy='true'])").background, tokens),
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
          // Pressed under the pointer the primary button shows its hover fill; pressed with the pointer
          // elsewhere (Space held on a focused button), its rest fill.
          pressed: computed(hover.background, tokens),
          pressedAlone: computed(primary.background, tokens),
        });
      });

      it(`${name}: a card's round Play draws the accent fill and label, its hover fill when hovered or pressed under the pointer`, () => {
        expect(disc(tokens)).toEqual({
          rest: computed('var(--accent)', tokens),
          glyph: computed('var(--accent-on, var(--accent-on-default))', tokens),
          hover: computed('var(--accent-hover)', tokens),
          pressed: computed('var(--accent-hover)', tokens),
          pressedAlone: computed('var(--accent)', tokens),
        });
      });
    }
  }

  for (const { id, tokens } of PRESETS) {
    it(`${id}: a progress bar in the game tone is the accent bar it was`, () => {
      expect(bar('game', tokens)).toEqual(bar('accent', tokens));
      expect(bar('game', tokens).fill).toBe(tokens['--accent-bright']);
    });
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
        pressedAlone: v['--game-fill-active'],
      });
      expect(disc(tokens)).toEqual({
        rest: v['--game-fill'],
        glyph: v['--game-glyph'],
        hover: v['--game-disc-hover'],
        pressed: v['--game-disc-active'],
        pressedAlone: v['--game-disc-active'],
      });
    });

    it(`${id}: a progress bar in the game tone fills with the game's fill and runs its label colour as the streak`, () => {
      const v = toneVars(gameToneOf([200, 40, 30], { bg: preset['--bg'], text: preset['--text'] }));
      const tokens = { ...preset, ...v, ...ruleBody(css('./shell.css'), '.game-tone') };
      expect(bar('game', tokens)).toEqual({ fill: v['--game-fill'], streak: v['--game-on-fill'] });
      expect(bar('accent', tokens).fill).toBe(preset['--accent-bright']);
    });
  }
});

// Games of every hue, vivid and muted, light and dark: what the art can hand the tone.
const SWEEP: RGB[] = [];
for (let h = 0; h < 360; h += 5) for (const [s, l] of [[1, 0.5], [0.6, 0.5], [0.9, 0.3], [0.9, 0.75]]) SWEEP.push(hslToRgb(h, s, l));

const GRAPH = styleOf(graphSource, 'SpeedGraph.svelte');
const graph = (tokens: Record<string, string>) => ({
  line: computed(ruleBody(GRAPH, '.line').stroke, tokens),
  dot: computed(ruleBody(GRAPH, '.dot').background, tokens),
});

/**
 * The speed graph's trace is a graphic the player reads (WCAG 1.4.11: 3:1 to what is next to it), drawn
 * on the download's panel (--surface-2), not on --bg, against which the game's colours are clamped.
 */
describe("the speed graph's trace keeps 3:1 on the panel it is drawn on", () => {
  for (const { id, tokens: preset } of PRESETS) {
    const panel = fromHex(preset['--surface-2']);

    it(`${id}: outside a game the trace is the bright accent`, () => {
      expect(graph(preset)).toEqual({ line: preset['--accent-bright'], dot: preset['--accent-bright'] });
      expect(contrast(fromHex(preset['--accent-bright']), panel)).toBeGreaterThanOrEqual(3);
    });

    it(`${id}: in every game's colour the line and its dot hold 3:1 on --surface-2`, () => {
      const worst = SWEEP.map((color) => {
        const { line, dot } = graph({ ...preset, ...toneVars(gameToneOf(color, { bg: preset['--bg'], text: preset['--text'] })) });
        expect(dot).toBe(line);
        return { color: toHex(color), line, ratio: contrast(fromHex(line), panel) };
      }).reduce((a, b) => (b.ratio < a.ratio ? b : a));
      expect(worst.ratio, `${worst.color} draws ${worst.line}`).toBeGreaterThanOrEqual(3);
    });
  }
});
