import { describe, expect, it } from 'vitest';
import type { DownloadStatus } from '../services/downloads';
import { createSpeedHistory, SPEED_HOLD_LIMIT_SECONDS, SPEED_WINDOW_SECONDS, type SpeedHistory } from './speedHistory';

const T0 = Date.parse('2026-11-15T12:00:00.000Z');

/** Four ticks a second from `from` (ms) for `seconds`, as Go sends them while a download moves. */
function ticks(history: SpeedHistory, id: string, from: number, seconds: number, speed: number, status: DownloadStatus = 'downloading') {
  for (let s = 0; s < seconds; s++) {
    for (let q = 0; q < 4; q++) history.record(id, status, speed, from + s * 1000 + q * 250);
  }
  return from + seconds * 1000;
}

describe('the speed history when the clock steps back', () => {
  it('draws a pause after a step back as zero and then a gap, not as the last speed held', () => {
    const history = createSpeedHistory();
    const end = ticks(history, 'a', T0, 3, 8000);

    history.record('a', 'paused', 0, end - 2000);
    const series = history.series('a', end + 3000);

    expect(series.moving).toBe(false);
    expect(series.samples.slice(-8)).toEqual([8000, 8000, 0, null, null, null, null, null]);
  });

  it('keeps drawing a download that goes on after the clock stepped back an hour', () => {
    const history = createSpeedHistory();
    const end = ticks(history, 'a', T0, 3, 5000);

    const back = ticks(history, 'a', end - 3_600_000, 5, 7000);
    const series = history.series('a', back);

    expect(series.moving).toBe(true);
    expect(series.samples.slice(-6)).toEqual([7000, 7000, 7000, 7000, 7000, 7000]);
    expect(series.samples.slice(-8, -6)).toEqual([5000, 5000]);
  });
});

describe('the speed history across a sleep', () => {
  it('holds the speed through a silence up to the limit, as while Go sends no tick', () => {
    const history = createSpeedHistory();
    const end = ticks(history, 'a', T0, 3, 5000);

    const series = history.series('a', end + (SPEED_HOLD_LIMIT_SECONDS - 1) * 1000);

    expect(series.samples.slice(-SPEED_HOLD_LIMIT_SECONDS)).toEqual(new Array(SPEED_HOLD_LIMIT_SECONDS).fill(5000));
  });

  it('draws a gap for a silence longer than the limit, then the speed once ticks come back', () => {
    const history = createSpeedHistory();
    const end = ticks(history, 'a', T0, 3, 5000);

    const woke = ticks(history, 'a', end + 60_000, 3, 6000);
    const series = history.series('a', woke - 1);

    expect(series.moving).toBe(true);
    expect(series.samples.slice(-66)).toEqual([5000, 5000, 5000, ...new Array(60).fill(null), 6000, 6000, 6000]);
  });

  it('draws a gap when the reader comes back from a sleep before any tick does', () => {
    const history = createSpeedHistory();
    const end = ticks(history, 'a', T0, 3, 5000);
    history.series('a', end);

    const series = history.series('a', end + 90_000);

    expect(series.moving).toBe(true);
    expect(series.samples.slice(-90)).toEqual(new Array(90).fill(null));
  });
});

describe('the speed history stays bounded', () => {
  it('holds at most the window, plus the one sample before it, after 500 seconds of a download', () => {
    const history = createSpeedHistory();

    ticks(history, 'a', T0, 500, 4096);

    expect(history.size()).toEqual({ tracks: 1, samples: SPEED_WINDOW_SECONDS + 1 });
  });

  it('holds the window per download with many downloads moving at once', () => {
    const history = createSpeedHistory();

    for (let tick = 0; tick < 300 * 4; tick++) {
      for (let n = 0; n < 20; n++) history.record(`d${n}`, 'downloading', 100 * n, T0 + tick * 250);
    }

    expect(history.size().tracks).toBe(20);
    expect(history.size().samples).toBeLessThanOrEqual(20 * (SPEED_WINDOW_SECONDS + 1));
  });

  it('keeps no track for a download that never downloaded', () => {
    const history = createSpeedHistory();

    let now = T0;
    for (const status of ['queued', 'paused', 'metadata', 'verifying', 'failed', 'completed'] as const) {
      now = ticks(history, status, now, 30, 0, status);
    }

    expect(history.size()).toEqual({ tracks: 0, samples: 0 });
  });

  it('lets go of the tracks of downloads that are gone', () => {
    const history = createSpeedHistory();
    const end = ticks(history, 'a', T0, 5, 100);
    ticks(history, 'b', end, 5, 100);

    history.keep(new Set(['b']));

    expect(history.size()).toEqual({ tracks: 1, samples: 5 });
  });
});
