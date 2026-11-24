import { LOGO_ARCS, LOGO_VIEWBOX } from './logoArcs';

/**
 * The update show (UpdateShow.svelte) as a function of the wall clock. The five arcs of the logo lift
 * off it one by one and fly along a curve into the five rings of a funnel; the funnel turns and fills
 * while the update applies; once the new version runs a ring opens and the arcs fly back in reverse
 * order; when applying ends without one they go home from wherever they are.
 *
 * Every frame is derived from when the show began (`startedAt`) and when its last phase began
 * (`since`), never from what was drawn before, so a window that mounts late, or a second process that
 * takes the show over, draws the very frame of that moment without playing what came before it.
 */

export const FUNNEL_VIEW = { w: 260, h: 200 } as const;
/** The funnel's rings, widest (top) first, in FUNNEL_VIEW units: half-width and height of each. */
export const FUNNEL_LAYERS = [
  { rx: 112, y: -66 },
  { rx: 90, y: -33 },
  { rx: 68, y: 0 },
  { rx: 47, y: 33 },
  { rx: 28, y: 66 },
] as const;
/** Every ring is a circle seen from the side: squashed to this share of its height. */
export const FUNNEL_FLAT = 0.3;

/** How far an arc grows on its way, at most (its ring takes over at the end). */
const ARC_GROWTH = 4;
/** How far the path bends away from the straight line, as a share of its length. */
const BEND = 0.3;
/** Half a turn and a little more, a little more again for each inner arc. */
const TURN = 120;
const TURN_STEP = 20;
/** The share of the flight in which an arc lifts off the logo before it moves on. */
const LIFT = 0.2;
/** Where on its path an arc starts to fade under its ring. */
const FADE_FROM = 0.82;
/** The funnel fills on a curve towards this share while applying: the rest is the real completion. */
const FILL_TO = 0.92;
const FILL_RATE = 0.9;
/** The way back takes this share of the way out. */
const RETURN = 0.85;
/** The ring that opens at the finish: from and to this scale. */
const RING_FROM = 0.15;
const RING_TO = 1.9;
const RING_OPACITY = 0.8;

export interface ShowTiming {
  /** One arc's flight. */
  fly: number;
  /** Between two arcs leaving. */
  step: number;
  /** The scrim's fade, the pause before the finish. */
  slow: number;
  /** The short fades: the words, a ring going out. */
  base: number;
  /** One turn of the funnel's middle ring. */
  spin: number;
}

export interface ArcPose {
  x: number;
  y: number;
  rotate: number;
  sx: number;
  sy: number;
  opacity: number;
}

export const REST_POSE: Readonly<ArcPose> = { x: 0, y: 0, rotate: 0, sx: 1, sy: 1, opacity: 1 };

export interface ShowGeometry {
  /** The logo mark's box on screen, px. */
  logo: { left: number; top: number; size: number };
  /** The funnel's box on screen, px. */
  funnel: { left: number; top: number; width: number; height: number };
}

/** One arc's way from the logo to its ring: where it ends, how much it grows, and where it turns. */
export interface ArcFlight {
  dx: number;
  dy: number;
  sx: number;
  sy: number;
  turn: number;
  /** The point the arc turns and grows around, px from the logo box's top left. */
  ox: number;
  oy: number;
}

export type ShowPhase = 'run' | 'finish' | 'home';
export type ShowStage = 'fly' | 'hold' | 'finish' | 'home' | 'done';

export interface ShowClock {
  startedAt: number;
  phase: ShowPhase;
  since: number;
}

export interface ShowFrame {
  stage: ShowStage;
  /** The flying copies of the arcs, relative to the logo. */
  arcs: ArcPose[];
  /** The logo's own arcs that are away (hidden) at this moment. */
  away: boolean[];
  /** Each funnel ring's opacity. */
  layers: number[];
  /** How much of each ring is lit, 0..1, bottom first. */
  lit: number[];
  /** Each ring's turn, degrees. */
  turn: number[];
  scrim: number;
  words: number;
  ring: { scale: number; opacity: number };
}

const clamp01 = (value: number) => Math.min(1, Math.max(0, value));
const easeInOut = (p: number) => (p < 0.5 ? 4 * p ** 3 : 1 - (-2 * p + 2) ** 3 / 2);
const easeOut = (p: number) => 1 - (1 - p) ** 4;
const lerp = (from: number, to: number, p: number) => from + (to - from) * p;
const count = LOGO_ARCS.length;
const each = <T>(fn: (i: number) => T): T[] => Array.from({ length: count }, (_, i) => fn(i));

export const flightEnd = (t: ShowTiming) => (count - 1) * t.step + t.fly;

export function arcFlights(geo: ShowGeometry): ArcFlight[] {
  const { logo, funnel } = geo;
  const k = funnel.width / FUNNEL_VIEW.w;
  const scale = logo.size / LOGO_VIEWBOX;
  return LOGO_ARCS.map((arc, i) => {
    const [bx, by, bw, bh] = arc.box;
    const ox = (bx + bw / 2) * scale;
    const oy = (by + bh / 2) * scale;
    const layer = FUNNEL_LAYERS[i];
    const dx = funnel.left + funnel.width / 2 - (logo.left + ox);
    const dy = funnel.top + funnel.height / 2 + layer.y * k - (logo.top + oy);
    const sx = Math.min(ARC_GROWTH, (2 * layer.rx * k) / Math.max(1, bw * scale));
    return { dx, dy, sx, sy: sx * (FUNNEL_FLAT + 0.35), turn: -(TURN + i * TURN_STEP), ox, oy };
  });
}

/** An arc `p` (0..1) of the way along its path: a quadratic curve that bends the same way for every arc. */
export function poseAt(flight: ArcFlight, p: number): ArcPose {
  const { dx, dy, sx, sy } = flight;
  const cx = dx / 2 - dy * BEND;
  const cy = dy / 2 + dx * BEND;
  const u = 1 - p;
  const grow = p ** 0.8;
  const pop = 0.3 * Math.sin(Math.PI * Math.min(1, p / LIFT));
  const flat = p ** 2;
  return {
    x: 2 * u * p * cx + p * p * dx,
    y: 2 * u * p * cy + p * p * dy,
    rotate: flight.turn * p,
    sx: 1 + (sx - 1) * grow + pop,
    sy: 1 + (sy - 1) * flat + (sx - 1) * grow * (1 - flat) + pop,
    opacity: p < FADE_FROM ? 1 : clamp01(1 - (p - FADE_FROM) / (1 - FADE_FROM)),
  };
}

const fill = (elapsed: number) => FILL_TO * (1 - Math.exp((-FILL_RATE * Math.max(0, elapsed)) / 1000));
// Bottom ring first: the fill reads upwards.
const litBy = (value: number) => each((i) => clamp01(value * count - (count - 1 - i)));
// The lower the ring, the faster it turns.
const turnAt = (elapsed: number, t: ShowTiming) => each((i) => -((360 * elapsed) / (t.spin * (0.45 + 0.14 * i))) % 360);
const progressOf = (elapsed: number, i: number, t: ShowTiming) => clamp01((elapsed - i * t.step) / t.fly);
const HIDDEN_RING = { scale: RING_FROM, opacity: 0 };

function runFrame(elapsed: number, t: ShowTiming, flights: ArcFlight[]): ShowFrame {
  const end = flightEnd(t);
  return {
    stage: elapsed < end ? 'fly' : 'hold',
    arcs: flights.map((flight, i) => poseAt(flight, easeInOut(progressOf(elapsed, i, t)))),
    away: each(() => true),
    layers: each((i) => clamp01((elapsed - (i * t.step + 0.8 * t.fly)) / (0.3 * t.fly))),
    lit: litBy(fill(elapsed)),
    turn: turnAt(elapsed, t),
    scrim: clamp01(elapsed / t.slow),
    words: clamp01((elapsed - end) / t.slow),
    ring: HIDDEN_RING,
  };
}

function finishFrame(elapsed: number, sinceFinish: number, t: ShowTiming, flights: ArcFlight[]): ShowFrame {
  const hold = runFrame(elapsed - sinceFinish, t, flights);
  const s = sinceFinish - t.slow;
  const returns = flights.map((_, i) => {
    const delay = t.base + (count - 1 - i) * t.step;
    return clamp01((s - delay) / (RETURN * t.fly));
  });
  const ringAt = clamp01(s / t.fly);
  const done = s >= Math.max(t.base + (count - 1) * t.step + RETURN * t.fly, t.base + t.slow, t.fly);
  return {
    stage: done ? 'done' : 'finish',
    arcs: flights.map((flight, i) => (s < 0 ? poseAt(flight, 1) : poseAt(flight, 1 - easeInOut(returns[i])))),
    away: returns.map((q) => q < 1),
    layers: each((i) => (s < 0 ? 1 : 1 - clamp01((s - (t.base + (count - 1 - i) * t.step)) / t.base))),
    lit: litBy(lerp(fill(elapsed - sinceFinish), 1, clamp01(sinceFinish / t.slow))),
    turn: turnAt(elapsed, t),
    scrim: s < t.base ? hold.scrim : 1 - clamp01((s - t.base) / t.slow),
    words: s < 0 ? hold.words : hold.words * (1 - clamp01(s / t.base)),
    ring: s < 0 ? HIDDEN_RING : { scale: lerp(RING_FROM, RING_TO, easeOut(ringAt)), opacity: RING_OPACITY * (1 - easeOut(ringAt)) },
  };
}

function homeFrame(elapsed: number, sinceHome: number, t: ShowTiming, flights: ArcFlight[]): ShowFrame {
  const from = runFrame(elapsed - sinceHome, t, flights);
  const back = easeOut(clamp01(sinceHome / (2 * t.base)));
  const done = sinceHome >= Math.max(2 * t.base, t.slow);
  const fade = 1 - clamp01(sinceHome / t.base);
  return {
    stage: done ? 'done' : 'home',
    arcs: from.arcs.map((pose) => ({
      x: lerp(pose.x, REST_POSE.x, back),
      y: lerp(pose.y, REST_POSE.y, back),
      rotate: lerp(pose.rotate, REST_POSE.rotate, back),
      sx: lerp(pose.sx, REST_POSE.sx, back),
      sy: lerp(pose.sy, REST_POSE.sy, back),
      opacity: lerp(pose.opacity, REST_POSE.opacity, back),
    })),
    away: each(() => !done),
    layers: from.layers.map((value) => value * fade),
    lit: from.lit,
    turn: turnAt(elapsed, t),
    scrim: from.scrim * (1 - clamp01(sinceHome / t.slow)),
    words: from.words * fade,
    ring: HIDDEN_RING,
  };
}

/**
 * The frame of the show at `now`. A finish asked for while the arcs are still flying starts once
 * they have arrived; the way home starts at once, from where they were.
 */
export function showFrame(clock: ShowClock, now: number, t: ShowTiming, flights: ArcFlight[]): ShowFrame {
  const elapsed = now - clock.startedAt;
  switch (clock.phase) {
    case 'run':
      return runFrame(elapsed, t, flights);
    case 'finish': {
      const from = Math.max(clock.since, clock.startedAt + flightEnd(t));
      return now < from ? runFrame(elapsed, t, flights) : finishFrame(elapsed, now - from, t, flights);
    }
    case 'home':
      return homeFrame(elapsed, Math.max(0, now - clock.since), t, flights);
  }
}
