/**
 * What DiskMeter draws and says, from the numbers it is given. The truth rules live here: a total that
 * is zero, missing or not a number is unknown and nothing is drawn against it; segments that sum over
 * the total are drawn in proportion to each other and a measured free space, and only a measured free
 * space is said; a size the caller does not know (the `sizeUnknown` flag of lib/diskusage: a lower
 * bound when bytes > 0, unknown otherwise) is said as such, never drawn as 0; the bare track is free
 * space and nothing else, so space the meter cannot call free is a part of its own.
 */

/** The colours a segment can take; DiskMeter.svelte paints each from theme tokens. */
export const METER_TONES = ['game', 'game-2', 'game-3', 'data', 'reclaim', 'other', 'more'] as const;
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

/** A size measured, only a lower bound (a segment not fully measured), only an upper bound (the space
 *  left beside such a segment), or not known at all. */
export type MeterSize = { kind: 'exact'; bytes: number } | { kind: 'atLeast'; bytes: number } | { kind: 'atMost'; bytes: number } | { kind: 'unknown' };

/** The key of the part the meter adds for space no segment accounts for. */
export const METER_OTHER = 'meter:other';
/** The key of the part the meter adds for the space after the segments when it is not known to be
 *  free: other files and free space together. */
export const METER_REST = 'meter:rest';
/** The key of the part the segments too small to draw on their own are folded into. */
export const METER_MORE = 'meter:more';

/** The narrowest track each form of the meter is laid out for, in rem. */
export const METER_WIDTH = { full: 32, compact: 16 } as const;
/** The width a part above 0 bytes is drawn at however small, the gap of track between parts and the
 *  track's padding around them, in rem. */
export const METER_HAIRLINE = 0.2;
export const METER_GAP = 0.2;
export const METER_PAD = 0.2;

export interface MeterPart {
  /** A caller's segment, or a part the meter adds: `other` for space no segment and no measured free
   *  space accounts for, `rest` for the space after the segments when no free space was measured and
   *  none can be derived, `more` for the segments folded together. */
  kind: 'segment' | 'other' | 'rest' | 'more';
  /** Unique in the model, for keyed lists: the key, made unique when two segments share one. */
  id: string;
  /** The caller's key, reported by a pick; METER_OTHER, METER_REST or METER_MORE for a part the meter
   *  adds. */
  key: string;
  /** The caller's label; empty for a part the meter adds, which DiskMeter names from the catalogue. */
  label: string;
  /** Null for `rest`: it is neither a fill nor the track, and DiskMeter outlines it. */
  tone: MeterTone | null;
  size: MeterSize;
  /** The part of the bar it takes, 0…1; 0 for a part that is not drawn. */
  share: number;
  /** For `more`: the segments folded into it, in the caller's order, each with its own id. */
  folded?: MeterPart[];
}

export interface MeterModel {
  /** The total, or null when it is unknown. */
  total: number | null;
  /** Every segment in the given order, then the parts the meter adds: the legend and the text alternative. */
  parts: MeterPart[];
  /** The parts the bar draws: a known size above 0 against a known total, with room left in the bar. */
  drawn: MeterPart[];
  /** Free space to say, or null when there is none to say apart (a `rest` part holds it, or an overflow
   *  without a measured one). */
  free: MeterSize | null;
  /** The segments, with the measured free space, sum over the total: drawn in proportion to each other
   *  and the measured free space, which alone is said. */
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
 *
 * The bare track is free space. Measured, the free space is the track and what neither it nor the
 * segments account for is other files; derived, it is the track. Otherwise (not measured, or not
 * derivable beside a size not fully measured) the space after the segments is one `rest` part, other
 * files and free space together, exact when every size is, else at most that space; the free space is
 * then not said apart from it.
 *
 * Overflow: the segments, with the measured free space, add up to more than the total. Which number is
 * wrong is not the meter's to know, so each part is drawn against the others and the measured free
 * space over the whole bar (its width agrees with its size beside its neighbours, every part above 0
 * shows, the free space is the track they leave); a measured free space is said, as it is what the
 * player acts on, and a derived or unmeasurable one is neither drawn nor said. The caller explains
 * the cause.
 */
export function meter(segments: readonly MeterSegment[], total: number | null | undefined, free?: number | null, width: number = METER_WIDTH.full): MeterModel {
  const known = measurable(total) && total > 0 ? total : null;
  const sizes = segments.map(sizeOf);
  const used = sizes.reduce((sum, size) => sum + bytesOf(size), 0);
  const measured = free !== undefined && measurable(free);
  const measuredFree = measured ? free : 0;
  const overflow = known !== null && used + measuredFree > known;
  const against = overflow ? used + measuredFree : (known ?? 0);

  const parts = segments.map((segment, index): MeterPart => {
    const size = sizes[index];
    const bytes = bytesOf(size);
    const share = known !== null && bytes > 0 ? bytes / against : 0;
    return { kind: 'segment', id: segment.key, key: segment.key, label: segment.label, tone: segment.tone, size, share };
  });

  // The track is the free space: what neither the segments nor the measured free space account for
  // is a part of its own, or it would be drawn as free. Its size is exact only when every segment's
  // is; beside a size not fully measured, that segment may take some of it: at most the gap. Without
  // a measured free space the whole gap is that part, unless the free space is derived (every size
  // exact, free left out), which makes the gap free.
  const exact = sizes.every((size) => size.kind === 'exact');
  const gap = known !== null && !overflow ? known - used - measuredFree : 0;
  const rest = gap > 0 && !measured && (free !== undefined || !exact);
  if (gap > 0 && (measured || rest)) {
    const size: MeterSize = { kind: exact ? 'exact' : 'atMost', bytes: gap };
    if (measured) parts.push({ kind: 'other', id: METER_OTHER, key: METER_OTHER, label: '', tone: 'other', size, share: gap / against });
    else parts.push({ kind: 'rest', id: METER_REST, key: METER_REST, label: '', tone: null, size, share: gap / against });
  }

  let freeSize: MeterSize | null;
  if (overflow) freeSize = measured ? { kind: 'exact', bytes: measuredFree } : null;
  else if (rest) freeSize = null;
  else if (free !== undefined) freeSize = measurable(free) ? { kind: 'exact', bytes: free } : { kind: 'unknown' };
  else if (known !== null && parts.every((part) => part.size.kind === 'exact')) freeSize = { kind: 'exact', bytes: known - used };
  else freeSize = null;

  // Ids are made unique before the fold, so a folded segment keeps its own, and again after it, for a
  // caller's key that is the fold's.
  const listed = unique(folded(unique(parts), width));
  return { total: known, parts: listed, drawn: listed.filter((part) => part.share > 0), free: freeSize, overflow };
}

// Rem sums of tenths are not exact in binary: a part that takes exactly a hairline must count as one.
const EPSILON = 1e-9;

/**
 * The parts with those the bar cannot draw truthfully folded into one `more` part, placed after the
 * caller's segments, before the parts the meter adds. The parts are laid out in the track's content
 * box (the width less its padding on both sides); with k parts drawn they share that box less k − 1
 * gaps (the worst case: a full bar, whose flex bases add up to the box and shrink by the gaps in
 * proportion). A segment is drawn on its own only when it takes a hairline of that room or more, so
 * its width agrees with its size; one part may stay under that and be drawn as a hairline: the last
 * segment when it alone is left, else the fold. The largest set of segments that meets this, with
 * the k hairlines and k − 1 gaps inside the box, is drawn; other files and the rest are never folded
 * in with them. The fold never takes every segment (it would name none): when no segment meets the
 * rule, the largest stays, a hairline beside the fold. A fold of one is never made: that segment is
 * drawn as a hairline itself.
 */
function folded(parts: MeterPart[], width: number): MeterPart[] {
  const candidates = parts.filter((part) => part.kind === 'segment' && part.share > 0).sort((a, b) => b.share - a.share);
  const fixed = parts.filter((part) => part.kind !== 'segment' && part.share > 0).length;
  if (candidates.length < 2) return parts;

  const box = width - 2 * METER_PAD;
  let keep = 0;
  for (let own = candidates.length - 1; own >= 0; own -= 1) {
    const k = own + 1 + fixed;
    const room = box - (k - 1) * METER_GAP;
    const fits = k * METER_HAIRLINE + (k - 1) * METER_GAP <= box + EPSILON;
    if (fits && candidates.slice(0, own).every((part) => part.share * room >= METER_HAIRLINE - EPSILON)) {
      keep = own;
      break;
    }
  }
  // The fold never takes every segment: with none drawn at its size, the largest stays as a hairline.
  keep = Math.max(keep, 1);
  const rest = new Set(candidates.slice(keep));
  if (rest.size < 2) return parts;

  const inFold = parts.filter((part) => rest.has(part));
  const more: MeterPart = {
    kind: 'more',
    id: METER_MORE,
    key: METER_MORE,
    label: '',
    tone: 'more',
    size: { kind: inFold.some((part) => part.size.kind !== 'exact') ? 'atLeast' : 'exact', bytes: inFold.reduce((sum, part) => sum + bytesOf(part.size), 0) },
    share: inFold.reduce((sum, part) => sum + part.share, 0),
    folded: inFold,
  };
  const segments = parts.filter((part) => part.kind === 'segment' && !rest.has(part));
  return [...segments, more, ...parts.filter((part) => part.kind !== 'segment')];
}

// Two segments under one key would break a keyed list: bad input is shown as given, not thrown, so a
// repeated id gets the count of its earlier uses (the key a pick reports stays the caller's).
function unique(parts: MeterPart[]): MeterPart[] {
  const taken = new Set<string>();
  return parts.map((part) => {
    let id = part.id;
    for (let uses = 1; taken.has(id); uses += 1) id = `${part.id}#${uses}`;
    taken.add(id);
    return id === part.id ? part : { ...part, id };
  });
}

/** A size in words: the formatted bytes, a lower or an upper bound, or that it is unknown. */
export function sizeWords(
  size: MeterSize,
  format: (bytes: number) => string,
  words: { atLeast: (size: string) => string; atMost: (size: string) => string; unknown: string },
): string {
  if (size.kind === 'unknown') return words.unknown;
  if (size.kind === 'atLeast') return words.atLeast(format(size.bytes));
  return size.kind === 'atMost' ? words.atMost(format(size.bytes)) : format(size.bytes);
}
