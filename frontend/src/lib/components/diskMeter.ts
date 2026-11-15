/**
 * What DiskMeter draws and says, from the numbers it is given. The truth rules live here: a total that
 * is zero, missing or not a number is unknown and nothing is drawn against it; segments that sum over
 * the total are clamped and the overflow is never shown as free space; a size the caller does not know
 * (the `sizeUnknown` flag of lib/diskusage: a lower bound when bytes > 0, unknown otherwise) is said as
 * such, never drawn as 0.
 */

/** The colours a segment can take; DiskMeter.svelte paints each from theme tokens. */
export const METER_TONES = ['game', 'game-2', 'game-3', 'data', 'reclaim', 'other'] as const;
export type MeterTone = (typeof METER_TONES)[number];

const GAME_STEPS = ['game', 'game-2', 'game-3'] as const satisfies readonly MeterTone[];

/** The tone of the n-th game in a row of game segments: neighbours never share a step. */
export function gameTone(index: number): MeterTone {
  const n = GAME_STEPS.length;
  return GAME_STEPS[((Math.trunc(index) % n) + n) % n];
}

export interface MeterSegment {
  key: string;
  label: string;
  bytes: number;
  tone: MeterTone;
  /** The size was not fully measured: `bytes` is a lower bound when above 0, nothing otherwise. */
  sizeUnknown?: boolean;
}

export type MeterSize = { kind: 'exact'; bytes: number } | { kind: 'atLeast'; bytes: number } | { kind: 'unknown' };

export interface MeterPart {
  key: string;
  label: string;
  tone: MeterTone;
  size: MeterSize;
  /** The part of the bar it takes, 0…1; 0 for a part that is not drawn. */
  share: number;
}

export interface MeterModel {
  /** The total, or null when it is unknown. */
  total: number | null;
  /** Every segment in the given order, for the legend and the text alternative. */
  parts: MeterPart[];
  /** The parts the bar draws: a known size above 0 against a known total, with room left in the bar. */
  drawn: MeterPart[];
  /** Free space to say, or null when there is none to say (unknown total, or not derivable). */
  free: MeterSize | null;
  /** The segments sum over the total. */
  overflow: boolean;
}

const measurable = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && value >= 0;

function sizeOf(segment: MeterSegment): MeterSize {
  if (!measurable(segment.bytes)) return { kind: 'unknown' };
  if (segment.sizeUnknown) return segment.bytes > 0 ? { kind: 'atLeast', bytes: segment.bytes } : { kind: 'unknown' };
  return { kind: 'exact', bytes: segment.bytes };
}

const bytesOf = (size: MeterSize) => (size.kind === 'unknown' ? 0 : size.bytes);

/**
 * `free`: the free space the caller measured (null when it could not); left out, it is derived as what
 * the segments leave of the total, and only when every size is exact.
 */
export function meter(segments: readonly MeterSegment[], total: number | null | undefined, free?: number | null): MeterModel {
  const known = measurable(total) && total > 0 ? total : null;
  let room = known ?? 0;
  let used = 0;
  const parts = segments.map((segment): MeterPart => {
    const size = sizeOf(segment);
    const bytes = bytesOf(size);
    used += bytes;
    let share = 0;
    if (known !== null && bytes > 0) {
      const drawn = Math.min(bytes, room);
      share = drawn / known;
      room -= drawn;
    }
    return { key: segment.key, label: segment.label, tone: segment.tone, size, share };
  });

  let freeSize: MeterSize | null;
  if (free !== undefined) freeSize = measurable(free) ? { kind: 'exact', bytes: free } : { kind: 'unknown' };
  else if (known !== null && parts.every((part) => part.size.kind === 'exact')) freeSize = { kind: 'exact', bytes: Math.max(0, known - used) };
  else freeSize = null;

  return {
    total: known,
    parts,
    drawn: parts.filter((part) => part.share > 0),
    free: freeSize,
    overflow: known !== null && used > known,
  };
}

/** A size in words: the formatted bytes, a lower bound, or that it is unknown. */
export function sizeWords(size: MeterSize, format: (bytes: number) => string, words: { atLeast: (size: string) => string; unknown: string }): string {
  if (size.kind === 'unknown') return words.unknown;
  return size.kind === 'atLeast' ? words.atLeast(format(size.bytes)) : format(size.bytes);
}
