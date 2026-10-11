import type { GenreShare } from '../../services/social';

export interface FingerprintInput {
  genres: GenreShare[];
  hours: number;
  games: number;
  completed: number;
}

export interface Ring {
  r: number;
  width: number;
  opacity: number;
  sweep: number;
  arc: string;
}

export interface Satellite {
  x: number;
  y: number;
}

export interface Emblem {
  seed: number;
  rotation: number;
  core: number;
  rings: Ring[];
  satellites: Satellite[];
}

export const CENTER = 50;
const OUTER = 41;
const MAX_RINGS = 5;
const MAX_SATELLITES = 12;
const MIN_SWEEP = 10;
const MAX_SWEEP = 359;

function round(value: number): number {
  return Math.round(value * 100) / 100;
}

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value));
}

function finite(value: number): number {
  return Number.isFinite(value) && value > 0 ? value : 0;
}

export function seedOf(input: FingerprintInput): number {
  const text = [
    input.genres.map((genre) => `${genre.name}:${genre.share.toFixed(3)}`).join('|'),
    Math.round(finite(input.hours)),
    Math.round(finite(input.games)),
    Math.round(finite(input.completed)),
  ].join('#');
  let hash = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash >>> 0;
}

function generator(seed: number): () => number {
  let state = seed >>> 0;
  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function point(radius: number, degrees: number): { x: number; y: number } {
  const angle = ((degrees - 90) * Math.PI) / 180;
  return { x: round(CENTER + radius * Math.cos(angle)), y: round(CENTER + radius * Math.sin(angle)) };
}

export function arcPath(radius: number, start: number, sweep: number): string {
  const from = point(radius, start);
  const to = point(radius, start + sweep);
  const large = sweep > 180 ? 1 : 0;
  return `M ${from.x} ${from.y} A ${radius} ${radius} 0 ${large} 1 ${to.x} ${to.y}`;
}

export function emblemOf(input: FingerprintInput): Emblem {
  const seed = seedOf(input);
  const next = generator(seed);
  const hours = finite(input.hours);
  const games = finite(input.games);
  const completed = finite(input.completed);
  const top = input.genres.slice(0, MAX_RINGS);

  const core = round(clamp(4 + Math.log10(1 + games) * 2.5, 4, 9));
  const wanted = clamp(2.2 + Math.log10(1 + hours) * 1.3, 2.2, 6.5);
  const gap = clamp(2 + Math.log10(1 + games) * 0.9, 2, 4);
  const slots = Math.max(top.length, 1);
  const spacing = Math.min(wanted + gap, (OUTER - core - 2) / slots);
  const width = round(Math.min(wanted, spacing * 0.7));

  const rotation = round(next() * 360);
  const rings: Ring[] = top.map((genre, index) => {
    const r = round(OUTER - width / 2 - index * spacing);
    const sweep = round(clamp(genre.share * 360, MIN_SWEEP, MAX_SWEEP));
    const start = round(next() * 360);
    return { r, width, opacity: round(1 - index * 0.14), sweep, arc: arcPath(r, start, sweep) };
  });
  if (rings.length === 0) {
    rings.push({ r: round(OUTER - width / 2), width, opacity: 0.4, sweep: 0, arc: '' });
  }

  const satellites: Satellite[] = [];
  const count = Math.min(Math.round(completed), MAX_SATELLITES);
  for (let i = 0; i < count; i++) satellites.push(point(OUTER + 5, next() * 360));

  return { seed, rotation, core, rings, satellites };
}
