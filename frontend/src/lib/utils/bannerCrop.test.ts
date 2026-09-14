import { describe, it, expect } from 'vitest';
import { bannerCrop, bannerScale, clampBanner, zoomBanner, type BannerView } from './bannerCrop';
const view = (width: number, height: number): BannerView => ({ width, height, viewport: 800, zoom: 1, offsetX: 0, offsetY: 0 });
describe('4:1 banner selection', () => {
  it.each([[1200, 1800], [4000, 500], [2560, 640]])('keeps crop in the %ix%i image with exact 4:1 output', (w, h) => {
    const v = clampBanner({ ...view(w,h), offsetX: -100000, offsetY: -100000 });
    const r = bannerCrop(v);
    expect(r.width / r.height).toBe(4); expect(r.outputWidth / r.outputHeight).toBe(4);
    expect(r.x).toBeGreaterThanOrEqual(0); expect(r.y).toBeGreaterThanOrEqual(0);
    expect(r.x + r.width).toBeLessThanOrEqual(w); expect(r.y + r.height).toBeLessThanOrEqual(h);
    expect(r.outputWidth).toBeLessThanOrEqual(2560);
  });
  it('selects different vertical regions of a portrait and preserves the center while zooming', () => {
    const v = view(1000,2000), scale = bannerScale(v);
    const centered = { ...v, offsetY: (200 - 2000 * scale) / 2 };
    const before = bannerCrop(centered), after = bannerCrop(zoomBanner(centered,2));
    expect(after.y + after.height / 2).toBeCloseTo(before.y + before.height / 2);
    expect(after.width).toBeCloseTo(before.width / 2);
    expect(bannerCrop(clampBanner({ ...v, offsetY: -10000 })).y).toBe(1750);
  });
  it('does not export an image until its dimensions are known', () => { expect(bannerCrop(view(0,0)).outputWidth).toBe(0); });
});
