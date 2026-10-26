import { describe, expect, it } from 'vitest';
import { CONTRAST_CHECKS, CONTRAST_TOKENS, contrastReport, formatRatio, type RGBA } from './contrastReport';
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

  it('says unknown, not pass, when a colour could not be read', () => {
    const colors: Record<string, RGBA | undefined> = { ...all(white), '--text': black, '--surface-3': undefined };
    expect(row(colors, 'text').ok).toBeNull();
    expect(row({ ...all(white), '--text': undefined }, 'text')).toMatchObject({ ok: null, ratio: null });
  });

  it('still flags a failing pair when another background is unknown', () => {
    const colors: Record<string, RGBA | undefined> = { ...all(white), '--text': [250, 250, 250, 1], '--surface-3': undefined };
    expect(row(colors, 'text').ok).toBe(false);
  });

  it('only names tokens the theme editor knows', () => {
    const known = new Set(tokenGroups.flatMap((group) => group.tokens));
    expect(CONTRAST_TOKENS.filter((name) => !known.has(name))).toEqual([]);
  });

  it('rounds the ratio down so a near miss never reads as a pass', () => {
    expect(formatRatio(4.4999, 'ru')).toBe('4,4:1');
    expect(formatRatio(16.84, 'en')).toBe('16.8:1');
  });
});
