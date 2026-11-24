import { describe, expect, it } from 'vitest';
import { LOGO_ARCS } from './logoArcs';
import { REST_POSE, arcFlights, flightEnd, showFrame, type ShowFrame, type ShowTiming } from './updateShowTimeline';

// The show is a function of the wall clock: these are the frames a person sees at given moments, and
// what makes a second process that knows only `startedAt` draw the same thing.

const T: ShowTiming = { fly: 1200, step: 120, slow: 450, base: 200, spin: 1400 };
const GEO = { logo: { left: 20, top: 30, size: 28 }, funnel: { left: 500, top: 300, width: 360, height: 277 } };
const FLIGHTS = arcFlights(GEO);
const START = 1_700_000_000_000;

const run = (elapsed: number) => showFrame({ startedAt: START, phase: 'run', since: START }, START + elapsed, T, FLIGHTS);
const moved = (frame: ShowFrame, i: number) => Math.hypot(frame.arcs[i].x, frame.arcs[i].y);
const isRest = (frame: ShowFrame) => frame.arcs.every((pose) => Object.entries(REST_POSE).every(([key, value]) => Math.abs(pose[key as keyof typeof pose] - value) < 1e-6));

describe('the flight', () => {
  it('starts with every arc on the logo, the page still undimmed and the funnel empty', () => {
    const frame = run(0);
    expect(frame.stage).toBe('fly');
    expect(isRest(frame)).toBe(true);
    expect(frame.away).toEqual([true, true, true, true, true]);
    expect(frame.scrim).toBe(0);
    expect(frame.words).toBe(0);
    expect(frame.layers).toEqual([0, 0, 0, 0, 0]);
  });

  it('mid-flight has the outer arcs on their way, the inner ones only lifting, and the page dimmed', () => {
    const frame = run(600);
    expect(frame.stage).toBe('fly');
    expect(moved(frame, 0)).toBeGreaterThan(50);
    expect(moved(frame, 0)).toBeGreaterThan(moved(frame, 4));
    expect(frame.arcs[0].rotate).toBeLessThan(0);
    expect(frame.arcs[0].opacity).toBe(1);
    expect(frame.scrim).toBe(1);
  });

  it('lands each arc on its funnel ring: the arc fades as its ring shows', () => {
    const frame = run(flightEnd(T));
    expect(frame.stage).toBe('hold');
    expect(frame.arcs.map((pose) => pose.opacity)).toEqual([0, 0, 0, 0, 0]);
    // The last ring finishes lighting up a moment after its arc arrives.
    expect(frame.layers.slice(0, 4)).toEqual([1, 1, 1, 1]);
    expect(run(flightEnd(T) + 0.1 * T.fly).layers).toEqual([1, 1, 1, 1, 1]);
    const end = FLIGHTS.map((flight) => [flight.dx, flight.dy]);
    frame.arcs.forEach((pose, i) => {
      expect(pose.x).toBeCloseTo(end[i][0], 6);
      expect(pose.y).toBeCloseTo(end[i][1], 6);
    });
  });

  it('holds while the update applies: the rings turn and fill from the bottom, the words are there', () => {
    const a = run(flightEnd(T) + T.slow);
    const b = run(flightEnd(T) + T.slow + 700);
    expect(a.stage).toBe('hold');
    expect(a.words).toBe(1);
    expect(b.turn.every((deg, i) => deg !== a.turn[i])).toBe(true);
    expect(b.lit[4]).toBeGreaterThanOrEqual(a.lit[4]);
    expect(b.lit[4]).toBeGreaterThan(b.lit[0]);
    expect(Math.max(...run(10 * 60_000).lit)).toBeLessThanOrEqual(1);
    expect(run(10 * 60_000).lit[0]).toBeLessThan(1);
  });

  it('draws the same frame for the same moment, whoever asks', () => {
    expect(run(777)).toEqual(showFrame({ startedAt: START, phase: 'run', since: START }, START + 777, T, arcFlights(GEO)));
  });

  it('flies every arc from its own place on the logo to its own ring', () => {
    expect(FLIGHTS).toHaveLength(LOGO_ARCS.length);
    expect(new Set(FLIGHTS.map((flight) => Math.round(flight.dy))).size).toBe(LOGO_ARCS.length);
    expect(FLIGHTS.every((flight) => flight.sx > 1 && flight.sx <= 4)).toBe(true);
  });
});

describe('the finish', () => {
  const finish = (since: number, at: number) => showFrame({ startedAt: START, phase: 'finish', since: START + since }, START + at, T, FLIGHTS);
  const LONG = 60_000;
  const end = T.slow + T.base + 4 * T.step + 0.85 * T.fly;

  it('waits for a flight still under way before it opens the ring', () => {
    const frame = finish(300, 900);
    expect(frame.stage).toBe('fly');
    expect(frame).toEqual(run(900));
  });

  it('starts from the hold frame and opens the ring after a pause', () => {
    const first = finish(LONG, LONG);
    expect(first.stage).toBe('finish');
    expect(first.arcs.map((pose) => pose.opacity)).toEqual([0, 0, 0, 0, 0]);
    expect(first.away).toEqual([true, true, true, true, true]);
    expect(first.ring.opacity).toBe(0);
    const ring = finish(LONG, LONG + T.slow + 100);
    expect(ring.ring.opacity).toBeGreaterThan(0);
    expect(ring.ring.scale).toBeGreaterThan(0.15);
  });

  it('brings the arcs back in reverse order: the innermost lands first', () => {
    const mid = finish(LONG, LONG + T.slow + T.base + 0.85 * T.fly + 10);
    expect(mid.away[4]).toBe(false);
    expect(mid.away[0]).toBe(true);
  });

  it('ends with the logo whole, the page clear and nothing left on it', () => {
    const last = finish(LONG, LONG + end);
    expect(last.stage).toBe('done');
    expect(isRest(last)).toBe(true);
    expect(last.away).toEqual([false, false, false, false, false]);
    expect(last.scrim).toBe(0);
    expect(last.words).toBe(0);
    expect(last.layers).toEqual([0, 0, 0, 0, 0]);
    expect(finish(LONG, LONG + end - 20).stage).toBe('finish');
  });
});

describe('the way home after a failure', () => {
  const home = (since: number, at: number) => showFrame({ startedAt: START, phase: 'home', since: START + since }, START + at, T, FLIGHTS);

  it('starts from where the arcs were when applying ended', () => {
    expect(home(600, 600).arcs).toEqual(run(600).arcs);
    expect(home(600, 600).stage).toBe('home');
  });

  it('brings every arc back to the logo, from mid-flight or from the funnel, and clears the page', () => {
    for (const since of [600, flightEnd(T) + 5000]) {
      const last = home(since, since + Math.max(2 * T.base, T.slow));
      expect(last.stage).toBe('done');
      expect(isRest(last)).toBe(true);
      expect(last.away).toEqual([false, false, false, false, false]);
      expect(last.scrim).toBe(0);
      expect(last.layers).toEqual([0, 0, 0, 0, 0]);
    }
    const halfway = home(flightEnd(T) + 5000, flightEnd(T) + 5000 + T.base);
    expect(halfway.arcs.every((pose) => pose.opacity > 0 && pose.opacity < 1)).toBe(true);
  });
});
