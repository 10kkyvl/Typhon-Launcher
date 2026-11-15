import { describe, expect, it } from 'vitest';
import type { DownloadStatus } from '../services/downloads';
import { createSpeedHistory, SPEED_WINDOW_SECONDS, type SpeedHistory } from './speedHistory';

const T0 = Date.parse('2026-11-15T12:00:00.000Z');

/** Four ticks a second from `from` (ms) for `seconds`, as Go sends them while a download moves. */
function ticks(history: SpeedHistory, id: string, from: number, seconds: number, speed: number, status: DownloadStatus = 'downloading') {
  for (let s = 0; s < seconds; s++) {
    for (let q = 0; q < 4; q++) history.record(id, status, speed, from + s * 1000 + q * 250);
  }
  return from + seconds * 1000;
}

describe('the speed history stays bounded', () => {
  it('holds at most the window, plus the one sample before it, after 500 seconds of a download', () => {
    const history = createSpeedHistory();

    ticks(history, 'a', T0, 500, 4096);

    expect(history.size()).toEqual({ tracks: 1, samples: SPEED_WINDOW_SECONDS + 1 });
  });

  it('holds the window per download with many downloads moving at once', () => {
    const history = createSpeedHistory();

    for (let n = 0; n < 20; n++) ticks(history, `d${n}`, T0, 300, 100 * n);

    expect(history.size().tracks).toBe(20);
    expect(history.size().samples).toBeLessThanOrEqual(20 * (SPEED_WINDOW_SECONDS + 1));
  });

  it('keeps no track for a download that never downloaded', () => {
    const history = createSpeedHistory();

    for (const status of ['queued', 'paused', 'metadata', 'verifying', 'failed', 'completed'] as const) {
      ticks(history, status, T0, 30, 0, status);
    }

    expect(history.size()).toEqual({ tracks: 0, samples: 0 });
  });

  it('lets go of the tracks of downloads that are gone', () => {
    const history = createSpeedHistory();
    ticks(history, 'a', T0, 5, 100);
    ticks(history, 'b', T0, 5, 100);

    history.keep(new Set(['b']));

    expect(history.size()).toEqual({ tracks: 1, samples: 5 });
  });
});
