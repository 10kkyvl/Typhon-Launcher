import { contrast } from '../theme/accent';
import { describe, expect, it } from 'vitest';
import { appearanceOf, appearancePalette, DEFAULT_APPEARANCE } from './appearance';

describe('profile appearance compatibility', () => {
  it('reads profiles created before customization and keeps explicit zero sliders', () => {
    expect(appearanceOf()).toEqual(DEFAULT_APPEARANCE);
    expect(appearanceOf({ coverDim: 0, coverPosition: 0 })).toMatchObject({ coverDim: 0, coverPosition: 0 });
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
