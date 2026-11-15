import type { DownloadStatus } from '../services/downloads';

/** How far back the speed graph looks, in seconds; one sample per second. */
export const SPEED_WINDOW_SECONDS = 120;

/**
 * The longest silence, in seconds, that a moving download's last speed is held across. Go sends no
 * tick while nothing changes and a reader asks every second, so a longer stretch with neither a tick
 * nor a reader's call means the webview was suspended (a sleep, a frozen window): those seconds are a
 * gap, not the last speed held. Go's speed is a 5-second average, so a real stall reaches its last
 * value well within the limit.
 */
export const SPEED_HOLD_LIMIT_SECONDS = 30;

export interface SpeedSeries {
  /**
   * Download speed in bytes/s per second of the window, oldest first, the current second last. A
   * second while the download stood still, or while the window slept, is null; the second it stopped
   * reads 0.
   */
  samples: (number | null)[];
  /** Whether the download is downloading now. */
  moving: boolean;
}

interface Sample {
  second: number;
  /** What this second draws: a speed, 0 for the second a download stopped, null for a gap. */
  speed: number | null;
  /** Whether the seconds after it, until the next sample, draw the same speed. */
  hold: boolean;
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
 * without one holds the previous sample while the download keeps downloading, up to
 * `SPEED_HOLD_LIMIT_SECONDS`. Memory is bounded: a track keeps the window plus the one sample before
 * it, and a download that never moved has none.
 *
 * Time is the wall clock the caller passes, made to never run back: when it steps back (an NTP
 * correction), the history's time stays at the last moment it saw and goes on from there, so a stop
 * still lands after the samples before it and a running download keeps drawing. A step forward reads
 * as a silence. The wall clock rather than `performance.now()`, because the hold limit needs the time
 * that passed during a system sleep, and whether `performance.now()` counts it differs between
 * WebView2 and WKWebView.
 */
export function createSpeedHistory() {
  const tracks = new Map<string, Track>();
  let shift = 0;
  let seen: number | undefined;

  function write(track: Track, sample: Sample) {
    const last = track.samples.at(-1);
    if (last && last.second === sample.second) track.samples[track.samples.length - 1] = sample;
    else if (!last || last.second < sample.second) track.samples.push(sample);
    while (track.samples.length > 1 && track.samples[1].second <= sample.second - SPEED_WINDOW_SECONDS) track.samples.shift();
  }

  /** The history's time for the wall clock `now` (ms); a silence over the limit becomes a gap. */
  function observe(now: number): number {
    let time = now + shift;
    if (seen !== undefined && time < seen) {
      shift += seen - time;
      time = seen;
    }
    if (seen !== undefined && time - seen > SPEED_HOLD_LIMIT_SECONDS * 1000) {
      const second = Math.floor(seen / 1000) + 1;
      for (const track of tracks.values()) if (track.moving) write(track, { second, speed: null, hold: false });
    }
    seen = time;
    return time;
  }

  return {
    /** Notes what a download does at `now` (ms). */
    record(id: string, status: DownloadStatus, speed: number, now: number) {
      const second = Math.floor(observe(now) / 1000);
      let track = tracks.get(id);
      if (status === 'downloading') {
        if (!track) {
          track = { samples: [], moving: false };
          tracks.set(id, track);
        }
        track.moving = true;
        write(track, { second, speed, hold: true });
        return;
      }
      if (track?.moving) {
        track.moving = false;
        write(track, { second, speed: 0, hold: false });
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

    /** The window ending at `now` (ms); a reader's call counts as a moment seen, like a tick. */
    series(id: string, now: number): SpeedSeries {
      const current = Math.floor(observe(now) / 1000);
      const out = emptySpeedSeries();
      const track = tracks.get(id);
      if (!track) return out;
      const first = current - SPEED_WINDOW_SECONDS + 1;
      let held: number | null = null;
      let next = 0;
      for (let slot = 0; slot < SPEED_WINDOW_SECONDS; slot++) {
        const second = first + slot;
        let value = held;
        while (next < track.samples.length && track.samples[next].second <= second) {
          const sample = track.samples[next++];
          held = sample.hold ? sample.speed : null;
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
