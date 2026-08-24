import { describe, expect, it } from 'vitest';
import { centerOffset, clampOffset, clampZoom, coverScale, cropSource, zoomAround, type CropView } from './crop';

function view(patch: Partial<CropView> = {}): CropView {
  return { viewport: 300, width: 600, height: 300, zoom: 1, offsetX: -150, offsetY: 0, ...patch };
}

describe('coverScale', () => {
  it.each([
    [300, 600, 300, 1],
    [300, 300, 600, 1],
    [300, 1200, 600, 0.5],
    [300, 150, 150, 2],
  ])('viewport %i over %ix%i', (viewport, width, height, expected) => {
    expect(coverScale(viewport, width, height)).toBe(expected);
  });

  it.each([
    [0, 600, 300],
    [300, 0, 300],
    [300, 600, 0],
    [300, -1, 300],
  ])('refuses degenerate input %i %i %i', (viewport, width, height) => {
    expect(coverScale(viewport, width, height)).toBe(0);
  });
});

describe('clampOffset', () => {
  it('keeps the viewport covered', () => {
    expect(clampOffset(50, 600, 300)).toBe(0);
    expect(clampOffset(-400, 600, 300)).toBe(-300);
    expect(clampOffset(-100, 600, 300)).toBe(-100);
  });

  it('centers an image smaller than the viewport', () => {
    expect(clampOffset(-100, 200, 300)).toBe(centerOffset(200, 300));
  });
});

describe('clampZoom', () => {
  it.each([
    [0.2, 1],
    [1, 1],
    [2.5, 2.5],
    [9, 4],
    [Number.NaN, 1],
  ])('%o becomes %o', (zoom, expected) => {
    expect(clampZoom(zoom)).toBe(expected);
  });
});

describe('zoomAround', () => {
  it('keeps the anchor point in place', () => {
    const next = zoomAround(view(), 2, 150, 150);
    expect(next.zoom).toBe(2);
    expect(next.offsetX).toBe(-450);
    expect(next.offsetY).toBe(-150);
  });

  it('clamps the offset so no empty edge appears', () => {
    const next = zoomAround(view({ offsetX: 0, offsetY: 0 }), 2, 0, 0);
    expect(next.offsetX).toBe(0);
    expect(next.offsetY).toBe(0);
    expect(zoomAround(view(), 4, 150, 150).zoom).toBe(4);
    expect(zoomAround(view(), 40, 150, 150).zoom).toBe(4);
  });

  it('leaves a degenerate view untouched', () => {
    const degenerate = view({ width: 0 });
    expect(zoomAround(degenerate, 2, 0, 0)).toEqual(degenerate);
  });
});

describe('cropSource', () => {
  it('takes the centered square of a landscape image', () => {
    expect(cropSource(view())).toEqual({ sx: 150, sy: 0, size: 300 });
  });

  it('takes the centered square of a portrait image', () => {
    expect(cropSource(view({ width: 300, height: 900, offsetX: 0, offsetY: -300 }))).toEqual({
      sx: 0,
      sy: 300,
      size: 300,
    });
  });

  it('shrinks the source square as the zoom grows', () => {
    expect(cropSource(view({ zoom: 2, offsetX: -300, offsetY: -150 }))).toEqual({ sx: 150, sy: 75, size: 150 });
  });

  it('never reads outside the image', () => {
    const out = cropSource(view({ offsetX: 5000, offsetY: -5000 }));
    expect(out.sx).toBe(0);
    expect(out.sy).toBe(0);
    expect(out.size).toBe(300);
  });

  it('reports nothing for a degenerate view', () => {
    expect(cropSource(view({ height: 0 }))).toEqual({ sx: 0, sy: 0, size: 0 });
  });
});
