import { describe, expect, it } from 'vitest';
import { goSources } from '../testing/sources';
import { clampFill, clampTone, contrast, dominant, fromHex, gameToneOf, hslToRgb, rgbToHsl, toHex, toneVars, type RGB, type ToneTheme } from './tone';

// The four built-in presets as the window paints them (internal/theme/presets.go): every clamp is measured
// against each one's own --bg and --text, the high-contrast preset included.
const PRESETS: { id: string; theme: ToneTheme }[] = [...(goSources['internal/theme/presets.go'] ?? '').matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)].map((m) => {
  const token = (name: string) => new RegExp(`"${name}":\\s*"([^"]*)"`).exec(m[1])?.[1] ?? '';
  return { id: /\bID:\s*"([^"]+)"/.exec(m[1])?.[1] ?? '', theme: { bg: token('--bg'), text: token('--text') } };
});

/** `rgba(r, g, b, a)` of a preset's token, as the window resolves it. */
function rgba(value: string): [number, number, number, number] {
  const m = /^rgba\(\s*(\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\s*\)$/.exec(value);
  if (!m) throw new Error(`not an rgba() colour: ${value}`);
  return [Number(m[1]), Number(m[2]), Number(m[3]), Number(m[4])];
}

// The same presets with the grounds a mark is drawn on: the panel (--surface-2) and the progress track
// (--hover-strong), which is laid over the panel and over the page.
const MARK_PRESETS: { id: string; theme: ToneTheme }[] = [...(goSources['internal/theme/presets.go'] ?? '').matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)].map((m) => {
  const token = (name: string) => new RegExp(`"${name}":\\s*"([^"]*)"`).exec(m[1])?.[1] ?? '';
  return {
    id: /\bID:\s*"([^"]+)"/.exec(m[1])?.[1] ?? '',
    theme: { bg: token('--bg'), text: token('--text'), panel: token('--surface-2'), track: rgba(token('--hover-strong')) },
  };
});

/** The four grounds of a theme as the window paints them: page, panel, and the track over each, in whole channels. */
function grounds(theme: ToneTheme): RGB[] {
  const page = fromHex(theme.bg);
  const panel = fromHex(theme.panel ?? theme.bg);
  const [r, g, b, a] = theme.track ?? [0, 0, 0, 0];
  const over = (below: RGB) => below.map((v, i) => Math.round([r, g, b][i] * a + v * (1 - a))) as RGB;
  return [page, panel, over(page), over(panel)];
}

const pixels = (...colors: RGB[]) => Uint8ClampedArray.from(colors.flatMap(([r, g, b]) => [r, g, b, 255]));

// Every hue in 15° steps, at a dull, a mid and a full saturation, from near black to near white.
const SAMPLES: RGB[] = [];
for (let h = 0; h < 360; h += 15) for (const s of [0.1, 0.5, 1]) for (const l of [0.08, 0.3, 0.5, 0.7, 0.95]) SAMPLES.push(hslToRgb(h, s, l));
const EXTREMES: RGB[] = [[255, 255, 255], [0, 0, 0], [128, 128, 128], [250, 220, 90], [20, 40, 160]];

describe('the colour of a picture', () => {
  it('finds the coloured pixel among dark sky and white', () => {
    const sun: RGB = [230, 140, 60];
    const found = dominant(pixels([5, 8, 12], [5, 8, 12], [5, 8, 12], [250, 250, 250], sun, sun)) as RGB;
    const [h] = rgbToHsl(found);
    expect(h).toBeGreaterThan(15);
    expect(h).toBeLessThan(40);
  });

  it('keeps the one vivid hue of a two-colour cover instead of averaging them into mud', () => {
    // A gold emblem over teal fog: the mean of the two is an olive that is in neither.
    const gold: RGB = [222, 168, 48];
    const teal: RGB = [38, 92, 96];
    const data = pixels(...Array.from({ length: 12 }, () => gold), ...Array.from({ length: 88 }, () => teal));
    const [h] = rgbToHsl(dominant(data) as RGB);
    expect(h).toBeGreaterThan(34);
    expect(h).toBeLessThan(50);
  });

  it('has none for flat grey and for a transparent picture', () => {
    expect(dominant(pixels([120, 120, 120], [121, 121, 121]))).toBeNull();
    expect(dominant(Uint8ClampedArray.from([200, 30, 30, 0, 200, 30, 30, 0]))).toBeNull();
    expect(dominant([])).toBeNull();
  });

  it('round-trips hex', () => {
    expect(toHex(fromHex('#0a70c8'))).toBe('#0a70c8');
  });
});

describe('the game colour clamped into each built-in preset', () => {
  it('reads the four presets', () => {
    expect(PRESETS.map((p) => p.id)).toEqual(['dark', 'light', 'contrast', 'blush']);
    for (const { theme } of PRESETS) {
      expect(theme.bg).toMatch(/^#[0-9a-f]{6}$/i);
      expect(theme.text).toMatch(/^#[0-9a-f]{6}$/i);
    }
  });

  for (const { id, theme } of PRESETS) {
    it(`${id}: Play stands out on the page and its label reads at rest, hovered and pressed, for any picture colour`, () => {
      const page = fromHex(theme.bg);
      const failures: string[] = [];
      for (const c of [...SAMPLES, ...EXTREMES]) {
        const v = toneVars(gameToneOf(c, theme));
        const label = fromHex(v['--game-on-fill']);
        const fill = fromHex(v['--game-fill']);
        if (contrast(fill, page) < 3) failures.push(`${toHex(c)} fill on page ${contrast(fill, page).toFixed(2)}`);
        for (const key of ['--game-fill', '--game-fill-hover', '--game-fill-active']) {
          const ratio = contrast(label, fromHex(v[key]));
          if (ratio < 4.5) failures.push(`${toHex(c)} label on ${key} ${ratio.toFixed(2)}`);
        }
      }
      expect(failures).toEqual([]);
    });

    it(`${id}: the round Play's glyph keeps 3:1 on its disc at rest, hovered and pressed`, () => {
      const failures: string[] = [];
      for (const c of [...SAMPLES, ...EXTREMES]) {
        const v = toneVars(gameToneOf(c, theme));
        const glyph = fromHex(v['--game-glyph']);
        for (const key of ['--game-fill', '--game-disc-hover', '--game-disc-active']) {
          const ratio = contrast(glyph, fromHex(v[key]));
          if (ratio < 3) failures.push(`${toHex(c)} glyph on ${key} ${ratio.toFixed(2)}`);
        }
      }
      expect(failures).toEqual([]);
    });

    it(`${id}: the accent (the game page's tab marker) keeps 4.5:1 on the page`, () => {
      const page = fromHex(theme.bg);
      const failures = [...SAMPLES, ...EXTREMES]
        .map((c) => ({ c, ratio: contrast(fromHex(clampTone(c, theme).accent), page) }))
        .filter(({ ratio }) => ratio < 4.5)
        .map(({ c, ratio }) => `${toHex(c)} ${ratio.toFixed(2)}`);
      expect(failures).toEqual([]);
    });
  }

  it('keeps the hue of the art', () => {
    for (const { theme } of PRESETS) {
      const { fill } = clampFill([200, 40, 30], theme);
      expect(Math.round(rgbToHsl(fromHex(fill))[0] / 10)).toBe(Math.round(rgbToHsl([200, 40, 30])[0] / 10));
    }
  });
});

/** The lowest ratio of a colour against the grounds, with the ground it is measured on. */
function worstOn(color: string, on: RGB[]): { ratio: number; ground: string } {
  return on.map((g) => ({ ratio: contrast(fromHex(color), g), ground: toHex(g) })).reduce((a, b) => (b.ratio < a.ratio ? b : a));
}

// A mark (the progress fill, the speed graph's line) is a graphic: WCAG 1.4.11 asks 3:1 against what it is
// drawn on, and the Downloads screen draws it on the page, the panel, and the progress track over either.
describe("the game colour as a mark on the page, the panel and the progress track", () => {
  it('reads the four presets with their panel and track', () => {
    expect(MARK_PRESETS.map((p) => p.id)).toEqual(['dark', 'light', 'contrast', 'blush']);
    for (const { theme } of MARK_PRESETS) {
      expect(theme.panel).toMatch(/^#[0-9a-f]{6}$/i);
      expect(theme.track?.[3]).toBeGreaterThan(0);
    }
  });

  for (const { id, theme } of MARK_PRESETS) {
    it(`${id}: the mark keeps 3:1 against every ground, for any picture colour`, () => {
      const on = grounds(theme);
      const failures = [...SAMPLES, ...EXTREMES]
        .map((c) => ({ c, mark: gameToneOf(c, theme).mark }))
        .map(({ c, mark }) => ({ c, mark, worst: worstOn(mark, on) }))
        .filter(({ worst }) => !(worst.ratio >= 3))
        .map(({ c, mark, worst }) => `${toHex(c)} -> ${mark} on ${worst.ground} ${worst.ratio.toFixed(2)}`);
      expect(failures).toEqual([]);
    });

    it(`${id}: Play's fill and label and the tab marker are what they were: the panel and the track do not move them`, () => {
      for (const c of [...SAMPLES, ...EXTREMES]) {
        const page = { bg: theme.bg, text: theme.text };
        const { fill, onFill, accent } = gameToneOf(c, theme);
        expect({ fill, onFill, accent }, toHex(c)).toEqual({ ...clampFill(c, page), ...clampTone(c, page) });
      }
    });
  }

  it('keeps the hue of the art where the grounds leave it room', () => {
    for (const { theme } of MARK_PRESETS) {
      const { mark } = gameToneOf([200, 40, 30], theme);
      expect(Math.round(rgbToHsl(fromHex(mark))[0] / 10)).toBe(Math.round(rgbToHsl([200, 40, 30])[0] / 10));
    }
  });

  it("a user's theme with a mid-grey page and a lighter panel: the mark still keeps 3:1 on all four grounds", () => {
    const odd: ToneTheme = { bg: '#3a3a3a', text: '#f0f0f0', panel: '#5a5a5a', track: [255, 255, 255, 0.18] };
    const on = grounds(odd);
    const failures = [...SAMPLES, ...EXTREMES]
      .map((c) => ({ c, worst: worstOn(gameToneOf(c, odd).mark, on) }))
      .filter(({ worst }) => !(worst.ratio >= 3))
      .map(({ c, worst }) => `${toHex(c)} on ${worst.ground} ${worst.ratio.toFixed(2)}`);
    expect(failures).toEqual([]);
  });

  it('a theme that names only its page clamps the mark against the page', () => {
    const page = MARK_PRESETS[2].theme;
    for (const c of [...SAMPLES, ...EXTREMES]) {
      expect(worstOn(gameToneOf(c, { bg: page.bg, text: page.text }).mark, [fromHex(page.bg)]).ratio).toBeGreaterThanOrEqual(3);
    }
  });

  it('where no colour holds 3:1 against every ground, the mark is the end that comes nearest', () => {
    // Black page, light-grey panel: white reaches 2.85 on the panel, black 1 on the page.
    const split: ToneTheme = { bg: '#000000', text: '#ffffff', panel: '#999999' };
    expect(gameToneOf([30, 90, 220], split).mark).toBe('#ffffff');
  });
});
