import { describe, expect, it } from 'vitest';
import { contrast, fromHex, hslToRgb, toHex, toneVars, type GameTone } from './tone';

const hsl = (h: number, s: number, l: number) => toHex(hslToRgb(h, s, l));

const dark: GameTone = { fill: '#2f6f9f', onFill: '#ffffff', accent: '#7fb4dd', mark: '#5a9ccc' };
const light: GameTone = { fill: '#d9c35a', onFill: '#0e0f11', accent: '#e8d57a', mark: '#7a6a1c' };

describe('the custom properties a game paints its subtree with', () => {
  it('hands the whole colour to a subtree, and nothing for no game', () => {
    expect(toneVars(null)).toEqual({});
    expect(toneVars(dark)).toMatchObject({ '--game-fill': dark.fill, '--game-on-fill': '#ffffff', '--game-accent': dark.accent });
  });

  it('names only the properties shell.css maps for Play and the tab marker reads: no tint, no progress tone', () => {
    expect(Object.keys(toneVars(dark)).sort()).toEqual(
      ['--game-accent', '--game-disc-active', '--game-disc-hover', '--game-fill', '--game-fill-active', '--game-fill-hover', '--game-glow', '--game-glyph', '--game-on-fill'].sort(),
    );
  });

  it('hover and press move away from the label colour, so the label never loses contrast', () => {
    for (const tone of [dark, light]) {
      const v = toneVars(tone);
      const label = fromHex(tone.onFill);
      const base = contrast(label, fromHex(tone.fill));
      expect(contrast(label, fromHex(v['--game-fill-hover']))).toBeGreaterThan(base);
      expect(contrast(label, fromHex(v['--game-fill-active']))).toBeGreaterThan(contrast(label, fromHex(v['--game-fill-hover'])));
    }
  });

  it('the glow under a hovered Play is its own fill, not the theme ring', () => {
    const [r, g, b] = fromHex(dark.fill);
    expect(toneVars(dark)['--game-glow']).toBe(`rgba(${r}, ${g}, ${b}, 0.75)`);
  });

  // The round Play on a card is a glyph, not a label: it needs 3:1, so it can stay white on fills where a
  // label would have to turn black (a mid blue, a mid red).
  it("the round button's glyph is white wherever white keeps 3:1 on the fill, and the label colour elsewhere", () => {
    const midBlue: GameTone = { fill: '#4a90e2', onFill: '#0e0f11', accent: '#8fbdf0', mark: '#4a90e2' };
    expect(toneVars(midBlue)['--game-glyph']).toBe('#ffffff');
    expect(toneVars(dark)['--game-glyph']).toBe('#ffffff');
    expect(toneVars(light)['--game-glyph']).toBe(light.onFill);
  });

  it('the glyph keeps 3:1 on the disc at rest, on hover and on press, for any fill', () => {
    const white = fromHex('#ffffff');
    const ink = fromHex('#0e0f11');
    for (let h = 0; h < 360; h += 15) {
      for (const l of [0.3, 0.42, 0.5, 0.58, 0.7]) {
        const fill = hsl(h, 0.7, l);
        const onFill = contrast(fromHex(fill), white) >= contrast(fromHex(fill), ink) ? '#ffffff' : '#0e0f11';
        const v = toneVars({ fill, onFill, accent: '#ffffff', mark: '#ffffff' });
        const glyph = fromHex(v['--game-glyph']);
        for (const key of ['--game-fill', '--game-disc-hover', '--game-disc-active']) {
          expect(contrast(glyph, fromHex(v[key])), `${fill} ${key}`).toBeGreaterThanOrEqual(3);
        }
      }
    }
  });
});
