import { afterEach, describe, expect, it, vi } from 'vitest';
import { contrast } from '../theme/accent';
import { hexToHsv } from '../theme/colorPicker';
import { PROFILE_TEXT_3 } from './appearanceColor';
import { loadArtPalette, quantise, resetArtCache } from './artPalette';

function pixels(colors: [number, number, number, number?][], each: number): number[] {
  return colors.flatMap(([r, g, b, a = 255]) => Array.from({ length: each }, () => [r, g, b, a]).flat());
}

describe('quantise', () => {
  it('returns null when nothing is opaque', () => {
    expect(quantise([])).toBeNull();
    expect(quantise(pixels([[255, 0, 0, 0]], 10))).toBeNull();
  });

  it('prefers a vibrant colour over a larger gray area', () => {
    const palette = quantise(pixels([[120, 120, 120], [220, 40, 30]], 40));
    expect(palette).not.toBeNull();
    const { h, s } = hexToHsv(palette!.accent);
    expect(s).toBeGreaterThan(0.6);
    expect(h < 20 || h > 340).toBe(true);
  });

  it('weights vibrant colours by how much of the image they cover', () => {
    const palette = quantise(pixels([[30, 90, 230], [230, 40, 30]], 1).concat(pixels([[30, 90, 230]], 50)));
    const { h } = hexToHsv(palette!.accent);
    expect(h).toBeGreaterThan(200);
    expect(h).toBeLessThan(240);
  });

  it('ignores transparent pixels', () => {
    const palette = quantise(pixels([[255, 0, 0, 0]], 100).concat(pixels([[20, 200, 90]], 3)));
    const { h } = hexToHsv(palette!.accent);
    expect(h).toBeGreaterThan(120);
    expect(h).toBeLessThan(160);
  });

  it('still returns a palette for a gray image', () => {
    const palette = quantise(pixels([[128, 128, 128]], 30));
    expect(palette).not.toBeNull();
    expect(palette!.accent).toMatch(/^#[0-9a-f]{6}$/);
  });

  it('gives a dark background and surface that keep muted text readable', () => {
    for (const color of [[255, 255, 255], [255, 220, 0], [0, 0, 0], [40, 120, 255]] as [number, number, number][]) {
      const palette = quantise(pixels([color], 20))!;
      expect(contrast(PROFILE_TEXT_3, palette.background)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(PROFILE_TEXT_3, palette.surface)).toBeGreaterThanOrEqual(4.5);
    }
  });
});

describe('loadArtPalette', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    resetArtCache();
  });

  function stubImage(behaviour: 'load' | 'error', size = { naturalWidth: 128, naturalHeight: 64 }) {
    let created = 0;
    class FakeImage {
      crossOrigin = '';
      naturalWidth = size.naturalWidth;
      naturalHeight = size.naturalHeight;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      set src(_value: string) {
        created++;
        queueMicrotask(() => (behaviour === 'load' ? this.onload?.() : this.onerror?.()));
      }
    }
    vi.stubGlobal('Image', FakeImage);
    return () => created;
  }

  function stubCanvas(read: () => Uint8ClampedArray) {
    vi.stubGlobal('document', {
      createElement: () => ({
        width: 0,
        height: 0,
        getContext: () => ({ drawImage() {}, getImageData: () => ({ data: read() }) }),
      }),
    });
  }

  it('reports unavailable when there is no image support', async () => {
    expect(await loadArtPalette('a.jpg')).toEqual({ ok: false, reason: 'unavailable' });
  });

  it('reports a load failure and retries the URL next time', async () => {
    const created = stubImage('error');
    stubCanvas(() => new Uint8ClampedArray());
    expect(await loadArtPalette('b.jpg')).toEqual({ ok: false, reason: 'load' });
    expect(await loadArtPalette('b.jpg')).toEqual({ ok: false, reason: 'load' });
    expect(created()).toBe(2);
  });

  it('reports a tainted canvas instead of a default palette', async () => {
    stubImage('load');
    stubCanvas(() => { throw new DOMException('tainted', 'SecurityError'); });
    expect(await loadArtPalette('c.jpg')).toEqual({ ok: false, reason: 'tainted' });
  });

  it('reports an empty image', async () => {
    stubImage('load', { naturalWidth: 0, naturalHeight: 0 });
    stubCanvas(() => new Uint8ClampedArray());
    expect(await loadArtPalette('d.jpg')).toEqual({ ok: false, reason: 'empty' });
  });

  it('extracts and caches a palette per URL', async () => {
    const created = stubImage('load');
    stubCanvas(() => new Uint8ClampedArray(pixels([[220, 40, 30]], 20)));
    const first = await loadArtPalette('e.jpg');
    const second = await loadArtPalette('e.jpg');
    expect(first.ok).toBe(true);
    expect(second).toBe(first);
    expect(created()).toBe(1);
  });
});
