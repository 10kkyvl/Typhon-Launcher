import { contrast } from '../theme/accent';
import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import { appearanceOf, appearancePalette, customBanner, pickCustomColor, themeOf, DEFAULT_APPEARANCE, PROFILE_THEMES } from './appearance';
import { PROFILE_TEXT_3 } from './appearanceColor';

describe('profile appearance compatibility', () => {
  it('reads profiles created before customization and keeps explicit zero sliders', () => {
    expect(appearanceOf()).toEqual(DEFAULT_APPEARANCE);
    expect(appearanceOf({ coverDim: 0, coverPosition: 0 })).toMatchObject({ coverDim: 0, coverPosition: 0 });
  });
  it('keeps the black theme when loading a saved profile', () => {
    expect(appearanceOf({ theme: 'black' }).theme).toBe('black');
    expect(themeOf({ ...DEFAULT_APPEARANCE, theme: 'black' }).background).toBe('#000000');
  });
  it('falls back for unrecognized themes and invalid colors', () => {
    expect(appearanceOf({ theme: 'missing', accent: 'red;display:none', coverDim: NaN, coverPosition: 200 })).toMatchObject({
      theme: 'midnight', accent: '#67d8ef', coverDim: 35, coverPosition: 100,
    });
  });
  it('keeps button text legible with a custom dark or light accent', () => {
    for (const accent of ['#000000', '#67d8ef', '#ffffff', '#660044']) {
      const palette = appearancePalette({ ...DEFAULT_APPEARANCE, accent });
      expect(contrast(palette['--accent'], palette['--accent-on'])).toBeGreaterThanOrEqual(4.5);
      expect(contrast(palette['--accent-text'], '#151d29')).toBeGreaterThanOrEqual(4.5);
    }
  });
});

describe('appearance normalisation', () => {
  const cases: [string, Record<string, unknown>, Record<string, unknown>][] = [
    ['bad custom hex', { customFrom: 'red', customTo: '#12345' }, { customFrom: '#142235', customTo: '#111923' }],
    ['hex with injection', { customFrom: '#112233;display:none' }, { customFrom: '#142235' }],
    ['angle above range', { customAngle: 900 }, { customAngle: 360 }],
    ['angle below range', { customAngle: -20 }, { customAngle: 0 }],
    ['angle not a number', { customAngle: 'x' }, { customAngle: 125 }],
    ['angle fraction', { customAngle: 44.6 }, { customAngle: 45 }],
    ['unknown frame', { avatarFrame: 'rainbow' }, { avatarFrame: 'none' }],
    ['known frame', { avatarFrame: 'orbit' }, { avatarFrame: 'orbit' }],
    ['unknown name style', { nameStyle: 'comic' }, { nameStyle: 'plain' }],
    ['known name style', { nameStyle: 'glow' }, { nameStyle: 'glow' }],
    ['unknown auto source', { autoSource: 'random' }, { autoSource: 'playing' }],
    ['known auto source', { autoSource: 'most_played' }, { autoSource: 'most_played' }],
    ['unknown theme', { theme: 'rainbow' }, { theme: 'midnight' }],
    ['custom theme', { theme: 'custom' }, { theme: 'custom' }],
    ['auto theme', { theme: 'auto' }, { theme: 'auto' }],
    ['parallax off', { parallax: false }, { parallax: false }],
    ['parallax not a boolean', { parallax: 'no' }, { parallax: true }],
    ['valid name and frame colours', { nameColor: '#112233', frameColor: '#AABBCC' }, { nameColor: '#112233', frameColor: '#AABBCC' }],
    ['bad name colour', { nameColor: 'red' }, { nameColor: '' }],
    ['short frame colour', { frameColor: '#12345' }, { frameColor: '' }],
    ['injected frame colour', { frameColor: '#112233;display:none' }, { frameColor: '' }],
    ['non-string colours', { nameColor: 5, frameColor: null }, { nameColor: '', frameColor: '' }],
    ['pure on', { customPure: true }, { customPure: true }],
    ['pure not a boolean', { customPure: 'yes' }, { customPure: false }],
    ['null fields', { customFrom: null, avatarFrame: null, parallax: null }, { customFrom: '#142235', avatarFrame: 'none', parallax: true }],
  ];
  it.each(cases)('%s', (_name, input, expected) => {
    expect(appearanceOf(input as never)).toMatchObject(expected);
  });
  it('fills every field when the stored profile has none of the new ones', () => {
    expect(appearanceOf({ theme: 'forest', accent: '#112233', coverUrl: 'x', coverDim: 10, coverPosition: 20 } as never)).toEqual({
      ...DEFAULT_APPEARANCE, theme: 'forest', accent: '#112233', coverUrl: 'x', coverDim: 10, coverPosition: 20,
    });
  });
});

describe('profile themes', () => {
  const art = { accent: '#ff8800', background: '#1a0f08', surface: '#241812' };
  it('builds the custom gradient from the chosen colours and angle', () => {
    const a = appearanceOf({ theme: 'custom', customFrom: '#ff0000', customTo: '#0000ff', customAngle: 90 });
    expect(customBanner(a)).toBe('linear-gradient(90deg, #ff0000, #0000ff)');
    expect(themeOf(a).banner).toBe('linear-gradient(90deg, #ff0000, #0000ff)');
  });
  it('keeps muted text readable on surfaces derived from any custom colours', () => {
    for (const [from, to] of [['#ffffff', '#ffffff'], ['#000000', '#000000'], ['#ff00ff', '#00ffff'], ['#f3b56b', '#fff700']]) {
      const theme = themeOf(appearanceOf({ theme: 'custom', customFrom: from, customTo: to }));
      expect(contrast(PROFILE_TEXT_3, theme.background)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(PROFILE_TEXT_3, theme.surface)).toBeGreaterThanOrEqual(4.5);
    }
  });
  it('keeps the custom colours untouched when they are shown as is', () => {
    const light = themeOf(appearanceOf({ theme: 'custom', customFrom: '#ffffff', customTo: '#ffffff', customPure: true }));
    expect(light.background).toBe('#ffffff');
    expect(light.banner).toBe('linear-gradient(125deg, #ffffff, #ffffff)');
    expect(light.ink?.text).toBe('#121821');
    const plain = themeOf(appearanceOf({ theme: 'custom', customFrom: '#ffffff', customTo: '#ffffff' }));
    expect(plain.background).not.toBe('#ffffff');
    expect(plain.ink).toBeUndefined();
  });
  it('ignores the pure flag outside the custom theme', () => {
    expect(themeOf(appearanceOf({ theme: 'black', customPure: true }))).toEqual(themeOf(appearanceOf({ theme: 'black' })));
    expect(themeOf(appearanceOf({ theme: 'black', customPure: true })).ink).toBeUndefined();
  });
  it('keeps the accent readable on a light pure background', () => {
    for (const accent of ['#67d8ef', '#ffffff', '#000000', '#f3b56b']) {
      const a = appearanceOf({ theme: 'custom', customFrom: '#f4eedd', customTo: '#ffffff', customPure: true, accent });
      const palette = appearancePalette(a);
      expect(contrast(palette['--accent'], themeOf(a).background)).toBeGreaterThanOrEqual(3);
      expect(contrast(palette['--accent-text'], themeOf(a).background)).toBeGreaterThanOrEqual(4.5);
    }
  });
  it('builds a palette for every theme, with and without art', () => {
    for (const id of [...PROFILE_THEMES.map((t) => t.id), 'custom', 'auto']) {
      const a = appearanceOf({ theme: id });
      for (const palette of [appearancePalette(a), appearancePalette(a, art)]) {
        expect(palette['--accent']).toMatch(/^#[0-9a-f]{6}$/);
        expect(contrast(palette['--accent'], palette['--accent-on'])).toBeGreaterThanOrEqual(4.5);
      }
    }
  });
  it('takes the accent from the art only for the auto theme', () => {
    const auto = appearancePalette(appearanceOf({ theme: 'auto', accent: '#112233' }), art);
    const manual = appearancePalette(appearanceOf({ theme: 'midnight', accent: '#112233' }), art);
    expect(auto['--accent']).not.toBe(manual['--accent']);
    expect(themeOf(appearanceOf({ theme: 'auto' }), art).background).toBe('#1a0f08');
    expect(themeOf(appearanceOf({ theme: 'midnight' }), art).background).toBe('#0e141e');
  });
  it('falls back to the manual accent and default theme when the art gave nothing', () => {
    const a = appearanceOf({ theme: 'auto', accent: '#729bff' });
    expect(themeOf(a, null).background).toBe(themeOf(DEFAULT_APPEARANCE).background);
    expect(appearancePalette(a, null)).toEqual(appearancePalette({ ...a, theme: 'midnight' }));
  });
});

describe('picking a custom background colour', () => {
  it('switches any other theme to a flat custom colour shown as is', () => {
    for (const theme of ['midnight', 'black', 'auto']) {
      const start = appearanceOf({ theme, customFrom: '#112233', customTo: '#445566', customPure: false, accent: '#729bff' });
      const next = pickCustomColor(start, '#ff8800');
      expect(next).toEqual({ ...start, theme: 'custom', customFrom: '#ff8800', customTo: '#ff8800', customPure: true });
      expect(themeOf(next).background).toBe('#ff8800');
      expect(customBanner(next)).toBe('linear-gradient(125deg, #ff8800, #ff8800)');
    }
  });
  it('changes only the first colour while the custom theme is active', () => {
    const start = appearanceOf({ theme: 'custom', customFrom: '#112233', customTo: '#445566', customPure: false, customAngle: 40 });
    expect(pickCustomColor(start, '#ff8800')).toEqual({ ...start, customFrom: '#ff8800' });
  });
  it('does not mutate the appearance it was given', () => {
    const start = appearanceOf({ theme: 'forest' });
    const copy = { ...start };
    pickCustomColor(start, '#ff8800');
    expect(start).toEqual(copy);
  });
});
