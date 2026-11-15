import type { DownloadStatus } from '../services/downloads';

/** How far back the speed graph looks, in seconds; one sample per second. */
export const SPEED_WINDOW_SECONDS = 120;

export interface SpeedSeries {
  /**
   * Download speed in bytes/s per second of the window, oldest first, the current second last. A
   * second while the download stood still is null; the second it stopped reads 0.
   */
  samples: (number | null)[];
  /** Whether the download is downloading now. */
  moving: boolean;
}

interface Sample {
  second: number;
  speed: number;
  /** The download stopped at this second: the seconds after it are a gap, not this speed held. */
  stop: boolean;
}

interface Track {
  samples: Sample[];
  moving: boolean;
}

export function emptySpeedSeries(): SpeedSeries {
  return { samples: new Array<number | null>(SPEED_WINDOW_SECONDS).fill(null), moving: false };
}

/**
 * Per-download speed samples, one per second: the last value seen in that second. Go's
 * `downloadSpeed` is already a 5-second average (internal/download/speed.go), so a mean of the four
 * ticks of a second would only lag it further. Go sends no tick while nothing changes, so a second
 * without one holds the previous sample while the download keeps downloading. Memory is bounded: a
 * track keeps the window plus the one sample before it, and a download that never moved has none.
 */
export function createSpeedHistory() {
  const tracks = new Map<string, Track>();

  function write(track: Track, sample: Sample) {
    const last = track.samples.at(-1);
    if (last && last.second === sample.second) track.samples[track.samples.length - 1] = sample;
    else if (!last || last.second < sample.second) track.samples.push(sample);
    while (track.samples.length > 1 && track.samples[1].second <= sample.second - SPEED_WINDOW_SECONDS) track.samples.shift();
  }

  return {
    /** Notes what a download does at `now` (ms). */
    record(id: string, status: DownloadStatus, speed: number, now: number) {
      const second = Math.floor(now / 1000);
      let track = tracks.get(id);
      if (status === 'downloading') {
        if (!track) {
          track = { samples: [], moving: false };
          tracks.set(id, track);
        }
        track.moving = true;
        write(track, { second, speed, stop: false });
        return;
      }
      if (track?.moving) {
        track.moving = false;
        write(track, { second, speed: 0, stop: true });
      }
    },

    /** Drops every track whose download is not in `ids`. */
    keep(ids: ReadonlySet<string>) {
      for (const id of tracks.keys()) if (!ids.has(id)) tracks.delete(id);
    },

    /** How much it holds: the tracks and their samples together. */
    size(): { tracks: number; samples: number } {
      let samples = 0;
      for (const track of tracks.values()) samples += track.samples.length;
      return { tracks: tracks.size, samples };
    },

    series(id: string, now: number): SpeedSeries {
      const out = emptySpeedSeries();
      const track = tracks.get(id);
      if (!track) return out;
      const first = Math.floor(now / 1000) - SPEED_WINDOW_SECONDS + 1;
      let held: number | null = null;
      let next = 0;
      for (let slot = 0; slot < SPEED_WINDOW_SECONDS; slot++) {
        const second = first + slot;
        let value = held;
        while (next < track.samples.length && track.samples[next].second <= second) {
          const sample = track.samples[next++];
          held = sample.stop ? null : sample.speed;
          value = sample.second === second ? sample.speed : held;
        }
        out.samples[slot] = value;
      }
      out.moving = track.moving;
      return out;
    },
  };
}

export type SpeedHistory = ReturnType<typeof createSpeedHistory>;
