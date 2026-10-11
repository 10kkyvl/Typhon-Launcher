import { writable } from 'svelte/store';
import { hexToHsv } from '../theme/colorPicker';
import { darkenUntil, mixHex, PROFILE_TEXT_3, toHex } from './appearanceColor';

export interface ArtPalette {
  accent: string;
  background: string;
  surface: string;
}

export type ArtFailure = 'unavailable' | 'load' | 'tainted' | 'empty';
export type ArtResult = { ok: true; palette: ArtPalette } | { ok: false; reason: ArtFailure };

export const artStatus = writable<'idle' | 'ok' | 'failed'>('idle');

const SAMPLE_SIDE = 64;
const MIN_SATURATION = 0.28;
const MIN_VALUE = 0.3;

interface Bucket { n: number; r: number; g: number; b: number }

export function quantise(pixels: ArrayLike<number>): ArtPalette | null {
  const buckets = new Map<number, Bucket>();
  let total = 0, sumR = 0, sumG = 0, sumB = 0;
  for (let i = 0; i + 3 < pixels.length; i += 4) {
    if (pixels[i + 3] < 128) continue;
    const r = pixels[i], g = pixels[i + 1], b = pixels[i + 2];
    total++; sumR += r; sumG += g; sumB += b;
    const key = ((r >> 4) << 8) | ((g >> 4) << 4) | (b >> 4);
    const bucket = buckets.get(key);
    if (bucket) { bucket.n++; bucket.r += r; bucket.g += g; bucket.b += b; }
    else buckets.set(key, { n: 1, r, g, b });
  }
  if (total === 0) return null;

  const average = toHex([sumR / total, sumG / total, sumB / total]);
  let best: string | null = null;
  let bestScore = 0;
  for (const bucket of buckets.values()) {
    const color = toHex([bucket.r / bucket.n, bucket.g / bucket.n, bucket.b / bucket.n]);
    const { s, v } = hexToHsv(color);
    if (s < MIN_SATURATION || v < MIN_VALUE) continue;
    const score = bucket.n * s * s * v;
    if (score > bestScore) { bestScore = score; best = color; }
  }
  const accent = best ?? mixHex(average, '#ffffff', 0.35);
  const background = darkenUntil(mixHex(mixHex(average, accent, 0.4), '#000000', 0.8), PROFILE_TEXT_3, 4.6);
  const surface = darkenUntil(mixHex(background, accent, 0.12), PROFILE_TEXT_3, 4.6);
  return { accent, background, surface };
}

const cache = new Map<string, Promise<ArtResult>>();

function sample(image: HTMLImageElement): ArtResult {
  const width = image.naturalWidth, height = image.naturalHeight;
  if (!width || !height) return { ok: false, reason: 'empty' };
  const scale = Math.min(1, SAMPLE_SIDE / Math.max(width, height));
  const canvas = document.createElement('canvas');
  canvas.width = Math.max(1, Math.round(width * scale));
  canvas.height = Math.max(1, Math.round(height * scale));
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  if (!ctx) return { ok: false, reason: 'unavailable' };
  let data: Uint8ClampedArray;
  try {
    ctx.drawImage(image, 0, 0, canvas.width, canvas.height);
    data = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
  } catch {
    return { ok: false, reason: 'tainted' };
  }
  const palette = quantise(data);
  return palette ? { ok: true, palette } : { ok: false, reason: 'empty' };
}

function read(url: string): Promise<ArtResult> {
  if (typeof Image === 'undefined' || typeof document === 'undefined') {
    return Promise.resolve({ ok: false, reason: 'unavailable' });
  }
  return new Promise((resolve) => {
    const image = new Image();
    image.crossOrigin = 'anonymous';
    image.onload = () => resolve(sample(image));
    image.onerror = () => resolve({ ok: false, reason: 'load' });
    image.src = url;
  });
}

export function loadArtPalette(url: string): Promise<ArtResult> {
  const known = cache.get(url);
  if (known) return known;
  const pending = read(url).then((result) => {
    // A failed fetch can be transient; a tainted or empty image stays that way.
    if (!result.ok && (result.reason === 'load' || result.reason === 'unavailable')) cache.delete(url);
    return result;
  });
  cache.set(url, pending);
  return pending;
}

export function resetArtCache() {
  cache.clear();
}
