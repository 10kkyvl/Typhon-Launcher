import { describe, expect, it } from 'vitest';
import {
  ART_BLACK,
  ART_WHITE,
  CONTRAST_CHECKS,
  CONTRAST_FALLBACKS,
  CONTRAST_TOKENS,
  DANGER_LABEL,
  DERIVED_COLORS,
  colorExpression,
  contrastReport,
  formatRatio,
  type RGBA,
} from './contrastReport';
import { tokenGroups } from './tokens';

const black: RGBA = [0, 0, 0, 1];
const white: RGBA = [255, 255, 255, 1];
const grey: RGBA = [119, 119, 119, 1];

function all(color: RGBA): Record<string, RGBA> {
  return Object.fromEntries(CONTRAST_TOKENS.map((name) => [name, color]));
}

const row = (colors: Record<string, RGBA | undefined>, id: string) => contrastReport(colors).find((r) => r.id === id)!;

describe('contrastReport', () => {
  it('measures black on white as 21:1', () => {
    const got = row({ ...all(white), '--text': black }, 'text');
    expect(got.ratio).toBeCloseTo(21, 5);
    expect(got.ok).toBe(true);
  });

  it.each([
    { name: '#777 on white is 4.48:1, below 4.5', fg: grey, min: 'text', ok: false },
    { name: '#777 on white passes the 3:1 secondary bar', fg: grey, min: 'text3', ok: true },
  ])('$name', ({ fg, min, ok }) => {
    const id = min;
    const token = CONTRAST_CHECKS.find((c) => c.id === id)!.fg;
    expect(row({ ...all(white), [token]: fg }, id).ok).toBe(ok);
  });

  it('reports the worst background of the pair', () => {
    const got = row({ ...all(black), '--text': white, '--surface-2': [200, 200, 200, 1] }, 'text');
    expect(got.ok).toBe(false);
    expect(got.against).toBe('--surface-2');
  });

  it('lays a translucent surface over --bg before measuring', () => {
    const colors = { ...all(black), '--text': white, '--surface': [255, 255, 255, 0.9] as RGBA };
    expect(row(colors, 'text').against).toBe('--surface');
    expect(row(colors, 'text').ok).toBe(false);
    expect(row({ ...colors, '--surface': [255, 255, 255, 0.05] }, 'text').ok).toBe(true);
  });

  it('warns when the sidebar island makes sidebar text unreadable, measured over the page', () => {
    const page = { ...all(black), '--text': white, '--text-2': white, '--bg-sidebar': black };
    expect(row({ ...page, '--island-fill': [255, 255, 255, 0.9] }, 'text')).toMatchObject({ ok: false, against: '--island-fill' });
    expect(row({ ...page, '--island-fill': [255, 255, 255, 0.9] }, 'text2')).toMatchObject({ ok: false, against: '--island-fill' });
    expect(row({ ...page, '--island-fill': [255, 255, 255, 0.05] }, 'text').ok).toBe(true);
  });

  it('says unknown, not pass, when a colour could not be read', () => {
    const colors: Record<string, RGBA | undefined> = { ...all(white), '--text': black, '--surface-3': undefined };
    expect(row(colors, 'text').ok).toBeNull();
    expect(row({ ...all(white), '--text': undefined }, 'text')).toMatchObject({ ok: null, ratio: null });
  });

  it('still flags a failing pair when another background is unknown', () => {
    const colors: Record<string, RGBA | undefined> = { ...all(white), '--text': [250, 250, 250, 1], '--surface-3': undefined };
    expect(row(colors, 'text').ok).toBe(false);
  });

  it('only names tokens the theme editor knows, or ones with a fallback', () => {
    const known = new Set([...tokenGroups.flatMap((group) => group.tokens), ...Object.keys(CONTRAST_FALLBACKS), ...Object.keys(DERIVED_COLORS)]);
    expect(CONTRAST_TOKENS.filter((name) => !known.has(name))).toEqual([]);
  });

  it('warns when the danger button label is unreadable on its tint, over any surface it stands on', () => {
    const dark = { ...all(black), '--text': white, '--danger': [239, 112, 112, 1] as RGBA, '--danger-subtle': [239, 112, 112, 0.13] as RGBA };
    expect(row({ ...dark, [DANGER_LABEL]: [250, 210, 210, 1] }, 'dangerButton').ok).toBe(true);
    expect(row({ ...dark, [DANGER_LABEL]: [120, 60, 60, 1] }, 'dangerButton')).toMatchObject({ ok: false, against: '--danger-subtle' });
    // Fine over the page, unreadable where the button stands on a light card.
    const card = { ...dark, [DANGER_LABEL]: [230, 230, 230, 1] as RGBA };
    expect(row(card, 'dangerButton').ok).toBe(true);
    expect(row({ ...card, '--surface-2': [235, 235, 235, 1] }, 'dangerButton').ok).toBe(false);
  });

  it('warns when the hovered danger button, the page colour on the full danger fill, is unreadable', () => {
    expect(row({ ...all(black), '--danger': [239, 112, 112, 1] }, 'dangerButtonHover').ok).toBe(true);
    expect(row({ ...all(black), '--danger': [40, 10, 10, 1] }, 'dangerButtonHover')).toMatchObject({ ok: false, against: '--danger' });
  });

  it('asks the browser for a derived colour by the expression the component writes, for a token by its variable', () => {
    expect(colorExpression(DANGER_LABEL)).toBe(DERIVED_COLORS[DANGER_LABEL]);
    expect(colorExpression('--text')).toBe('var(--text)');
    expect(colorExpression('--accent-on')).toBe(`var(--accent-on, ${CONTRAST_FALLBACKS['--accent-on']})`);
  });

  // Words on a game's art stand on the text plate (.stage-copy): --text over --scrim-copy, laid over the
  // two extremes a picture brings.
  describe('the text plate on game art', () => {
    const page = (bg: RGBA, text: RGBA, plate: RGBA): Record<string, RGBA> => ({
      ...all(black),
      '--bg': bg,
      '--text': text,
      '--scrim-copy': plate,
      [ART_BLACK]: black,
      [ART_WHITE]: white,
    });

    it('measures the plate over black and over white art, as the colours the window draws there', () => {
      expect(colorExpression(ART_BLACK)).toBe('#000000');
      expect(colorExpression(ART_WHITE)).toBe('#ffffff');
      const check = CONTRAST_CHECKS.find((c) => c.id === 'stagePlate');
      expect(check).toMatchObject({ fg: '--text', backgrounds: ['--scrim-copy'], grounds: [ART_BLACK, ART_WHITE], min: 4.5 });
    });

    it('warns when the plate is see-through: light words vanish on white art', () => {
      expect(row(page([14, 15, 17, 1], [237, 238, 240, 1], [0, 0, 0, 0]), 'stagePlate')).toMatchObject({ ok: false, against: '--scrim-copy' });
    });

    it('passes the dark preset with its plate', () => {
      expect(row(page([14, 15, 17, 1], [237, 238, 240, 1], [8, 9, 10, 0.66]), 'stagePlate').ok).toBe(true);
    });

    it('passes a dark theme on a #333 page with #ddd words on the dark preset\'s plate, and flags a plate as light as its page', () => {
      expect(row(page([51, 51, 51, 1], [221, 221, 221, 1], [8, 9, 10, 0.66]), 'stagePlate').ok).toBe(true);
      expect(row(page([51, 51, 51, 1], [221, 221, 221, 1], [51, 51, 51, 0.66]), 'stagePlate')).toMatchObject({ ok: false, against: '--scrim-copy' });
    });
  });

  it('rounds the ratio down so a near miss never reads as a pass', () => {
    expect(formatRatio(4.4999, 'ru')).toBe('4,4:1');
    expect(formatRatio(16.84, 'en')).toBe('16.8:1');
  });
});
