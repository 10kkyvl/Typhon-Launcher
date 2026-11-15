/**
 * The generated art of a game: a sky, a sun or stars, and two lands, seeded by the game's id, so the
 * same game always gets the same picture with no network and no theme. Ported from the 2.0
 * prototype's procedural key art (typhon-ui `identity/art`), whose canvas painter returned a JPEG
 * data URL; here the scene is plain data that GeneratedArt.svelte draws as SVG, so nothing is read
 * back from a canvas and a grid of covers costs a few paths each.
 *
 * The colours below are generator data (sky palettes, land shades), not UI styling: they are the
 * picture, the same in every theme.
 */

export type Sky = 'dusk' | 'dawn' | 'night' | 'day' | 'storm' | 'ember' | 'neon' | 'toxic' | 'space';
export type Land = 'sea' | 'peaks' | 'hills' | 'gothic' | 'city' | 'mesa' | 'dunes' | 'ruins' | 'forest';
export type ArtShape = 'cover' | 'wide';

export interface ArtSpec {
  sky: Sky;
  /** Hue shift in degrees applied to the sky palette. */
  hue: number;
  /** Far land first. */
  lands: [Land, Land];
  /** Horizontal sun position, 0..1. */
  sunX: number;
}

export interface ArtScene {
  width: number;
  height: number;
  /** Top to bottom. */
  sky: { offset: number; color: string }[];
  /** All stars as one path, drawn in white at `starOpacity`. */
  stars: string;
  starOpacity: number;
  sun: { x: number; y: number; r: number; color: string; glow: string; glowR: number } | null;
  /** Far to near. */
  lands: { d: string; fill: string }[];
}

type Rand = () => number;
type Hsl = [number, number, number];

/** FNV-1a, unsigned 32 bit: the prototype's seed hash. */
export function hash(text: string): number {
  let h = 2166136261;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

/** mulberry32: floats in [0, 1) from a 32 bit seed. */
export function rng(seed: number): Rand {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const SKIES: Sky[] = ['dusk', 'dawn', 'night', 'day', 'storm', 'ember', 'neon', 'toxic', 'space'];
const LANDS: Land[] = ['sea', 'peaks', 'hills', 'gothic', 'city', 'mesa', 'dunes', 'ruins', 'forest'];

const PALETTE: Record<Sky, [Hsl, Hsl, Hsl]> = {
  dusk: [[212, 40, 9], [206, 30, 24], [30, 70, 56]],
  night: [[228, 50, 6], [232, 40, 15], [222, 35, 27]],
  neon: [[208, 58, 7], [200, 58, 19], [188, 80, 40]],
  day: [[208, 62, 46], [202, 58, 68], [44, 70, 84]],
  space: [[214, 55, 4], [210, 42, 12], [196, 45, 24]],
  storm: [[214, 16, 11], [205, 14, 24], [38, 25, 44]],
  ember: [[8, 55, 6], [14, 65, 19], [30, 90, 48]],
  dawn: [[200, 38, 22], [24, 58, 56], [44, 88, 76]],
  toxic: [[80, 22, 8], [72, 28, 19], [56, 42, 42]],
};

const SIZES: Record<ArtShape, [number, number]> = { cover: [360, 480], wide: [1400, 500] };

const STARRY: Sky[] = ['night', 'space', 'neon', 'dusk', 'ember', 'toxic'];
const SUNNY: Sky[] = ['dusk', 'dawn', 'ember', 'day'];
const MAX_STARS = 60;

const clamp = (v: number) => Math.max(0, Math.min(100, v));
const hsla = (h: number, s: number, l: number, a = 1) =>
  `hsla(${Math.round(((h % 360) + 360) % 360)}, ${+clamp(s).toFixed(1)}%, ${+clamp(l).toFixed(1)}%, ${a})`;
const n = (v: number) => String(+v.toFixed(1));

function polygon(points: [number, number][]): string {
  return points.map(([x, y], i) => `${i ? 'L' : 'M'}${n(x)} ${n(y)}`).join('') + 'Z';
}

const box = (x: number, y: number, w: number, h: number) => polygon([[x, y], [x + w, y], [x + w, y + h], [x, y + h]]);

/** The spec of a game: what its picture shows, before any geometry. */
export function artSpec(seed: string): ArtSpec {
  const r = rng(hash(`${seed}:art`));
  const pick = <T>(list: T[]) => list[Math.floor(r() * list.length)];
  return { sky: pick(SKIES), hue: Math.round(r() * 120 - 60), lands: [pick(LANDS), pick(LANDS)], sunX: Math.round(r() * 100) / 100 };
}

function landShade(c3: Hsl, depth: number): string {
  return hsla(c3[0], c3[1] * 0.45 * (1 - depth * 0.6), Math.max(2.5, c3[2] * 0.62 * (1 - depth) + 3));
}

function landPath(r: Rand, w: number, h: number, type: Land, base: number, depth: number): string {
  const ground = box(0, base, w, h - base);
  switch (type) {
    case 'peaks': {
      const pts: [number, number][] = [[0, h]];
      let px = -w * 0.05;
      let last: [number, number] | null = null;
      while (px < w * 1.05) {
        const p: [number, number] = [px, base - h * (0.06 + r() * 0.26) * (1 - depth * 0.4)];
        if (last) pts.push([(last[0] + p[0]) / 2, Math.max(last[1], p[1]) + h * 0.05 * (0.5 + r())]);
        pts.push(p);
        last = p;
        px += w * (0.06 + r() * 0.14);
      }
      pts.push([w, h]);
      return polygon(pts);
    }
    case 'hills':
    case 'dunes': {
      const amp = type === 'dunes' ? 0.06 : 0.09;
      const f1 = 1 + r() * 2;
      const f2 = 3 + r() * 3;
      const ph = r() * 6;
      const pts: [number, number][] = [[0, h]];
      for (let i = 0; i <= 40; i++) {
        const u = i / 40;
        pts.push([u * w, base - h * amp * (Math.sin(u * f1 * Math.PI + ph) * 0.7 + Math.sin(u * f2 * Math.PI + ph * 2) * 0.3 + 0.8)]);
      }
      pts.push([w, h]);
      return polygon(pts);
    }
    case 'mesa': {
      const parts = [ground];
      let px = -w * 0.05;
      while (px < w) {
        const mw = w * (0.08 + r() * 0.22);
        const mh = h * (0.06 + r() * 0.2) * (1 - depth * 0.3);
        const sl = mw * 0.15;
        if (r() > 0.25) parts.push(polygon([[px, base], [px + sl, base - mh], [px + mw - sl, base - mh], [px + mw, base]]));
        px += mw + w * (0.02 + r() * 0.1);
      }
      return parts.join('');
    }
    case 'city':
    case 'gothic': {
      const parts = [ground];
      let px = -4;
      while (px < w) {
        const bw = type === 'city' ? w * (0.025 + r() * 0.06) : w * (0.014 + r() * 0.04);
        const top = type === 'city' ? base - h * (0.08 + r() * 0.42) * (1 - depth * 0.35) : base - r() * h * 0.25 * (1 - depth * 0.2);
        parts.push(box(px, top, bw + 1, base - top));
        if (type === 'gothic' && r() > 0.45) parts.push(polygon([[px, top], [px + bw / 2, top - bw * (1.4 + r() * 3.4)], [px + bw, top]]));
        px += bw + (r() < 0.25 ? w * 0.01 : 0);
      }
      return parts.join('');
    }
    case 'forest': {
      const parts = [ground];
      const S = Math.min(w, h);
      let px = -10;
      while (px < w) {
        const tw = S * (0.03 + r() * 0.05) * (0.6 + depth);
        const th = tw * (2.2 + r() * 1.6);
        const yb = base + r() * h * 0.02;
        parts.push(polygon([[px, yb], [px + tw / 2, yb - th], [px + tw, yb]]));
        px += tw * (0.35 + r() * 0.5);
      }
      return parts.join('');
    }
    case 'ruins': {
      const parts = [ground];
      const S = Math.min(w, h);
      let px = w * (0.02 + r() * 0.06);
      while (px < w) {
        const cw = S * (0.025 + r() * 0.025) * (0.7 + depth * 0.6);
        const ch = h * (0.1 + r() * 0.3) * (1 - depth * 0.3);
        if (r() > 0.4) {
          parts.push(box(px, base - ch, cw, ch));
          parts.push(box(px - cw * 0.25, base - ch - cw * 0.3, cw * 1.5, cw * 0.32));
        } else parts.push(polygon([[px, base], [px, base - ch * 0.6], [px + cw * 0.6, base - ch * 0.75], [px + cw, base - ch * 0.5], [px + cw, base]]));
        px += cw * (2 + r() * 3);
      }
      return parts.join('');
    }
    case 'sea': {
      const cx = w * (r() < 0.5 ? 0 : 0.7);
      return ground + polygon([[cx, base], [cx + w * 0.05, base - h * 0.12], [cx + w * 0.2, base - h * 0.09], [cx + w * 0.3, base]]);
    }
  }
}

/** The picture of a game in a box of `shape`: everything GeneratedArt.svelte draws. */
export function artScene(seed: string, shape: ArtShape): ArtScene {
  const spec = artSpec(seed);
  const [width, height] = SIZES[shape];
  const r = rng(hash(`${seed}|${shape}|${width}x${height}`));
  const [c1, c2, c3] = PALETTE[spec.sky];
  const t = spec.hue;
  const top: Hsl = [c1[0] + t, c1[1], c1[2]];
  const low: Hsl = [c3[0] + t, c3[1], c3[2]];
  const sky = [
    { offset: 0, color: hsla(top[0], top[1], top[2]) },
    { offset: 0.5, color: hsla(c2[0] + t, c2[1], c2[2]) },
    { offset: 0.78, color: hsla(low[0], low[1], low[2]) },
    { offset: 1, color: hsla(low[0], low[1], low[2] * 0.7) },
  ];

  let stars = '';
  if (STARRY.includes(spec.sky)) {
    const reach = spec.sky === 'dusk' ? 0.35 : 0.62;
    const parts: string[] = [];
    for (let i = 0; i < MAX_STARS; i++) {
      const s = (r() < 0.9 ? 1 : 1.8) * (width / 360);
      parts.push(box(r() * width, r() * height * reach, s, s));
    }
    stars = parts.join('');
  }

  const S = Math.min(width, height);
  let sun: ArtScene['sun'] = null;
  if (SUNNY.includes(spec.sky)) {
    const day = spec.sky === 'day';
    const radius = S * (day ? 0.07 : 0.13);
    sun = {
      x: width * spec.sunX,
      y: day ? height * 0.18 : height * 0.62,
      r: radius,
      color: hsla(low[0] + 10, 95, day ? 92 : 80),
      glow: hsla(low[0] + 8, 95, 72, 0.45),
      glowR: radius * (day ? 5 : 4.2),
    };
  }

  const bases = [0.68, 0.82];
  const lands = spec.lands.map((type, i) => {
    const depth = i === 0 ? 0.2 : 0.95;
    return { d: landPath(r, width, height, type, height * bases[i], depth), fill: landShade(low, depth) };
  });

  return { width, height, sky, stars, starOpacity: spec.sky === 'dusk' ? 0.35 : 0.6, sun, lands };
}
