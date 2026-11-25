/** Where a text stands against its limit: the counter speaks only when this changes to one of the last three. */
export type CounterStage = 'under' | 'near' | 'at' | 'over';

/** A counter of a TextArea: the limit it is shown against, and the caller's words for it. */
export interface TextAreaCounter {
  max: number;
  /** The length as the caller's limit counts it; characters (code points) by default. */
  measure?: (value: string) => number;
  /** The count as shown, in the caller's words ("120 of 5000"). */
  text: (count: number, max: number) => string;
  /** What is said near, at and past the limit, in the caller's words. */
  limitText: (count: number, max: number) => string;
}

/** Near: the last tenth of the limit, at least the last character. */
export function counterStage(count: number, max: number): CounterStage {
  if (count > max) return 'over';
  if (count === max) return 'at';
  return max - count <= Math.max(1, Math.ceil(max / 10)) ? 'near' : 'under';
}

/** Characters as a reader counts them: a code point each, so an emoji or a letter outside the BMP is one. */
export function codePoints(text: string): number {
  return Array.from(text).length;
}

export interface FieldBox {
  /** The content's height with the padding (`scrollHeight`), measured with the height let go. */
  scrollHeight: number;
  lineHeight: number;
  /** Top and bottom padding together. */
  padding: number;
  /** Top and bottom border together. */
  border: number;
  rows: number;
  grow: number;
}

/**
 * The border-box height a growing field takes: its content's, from `rows` lines up to `grow` lines,
 * and whether the content scrolls past that. Null when a measure is not a number (no layout: jsdom).
 */
export function grownHeight(box: FieldBox): { height: number; scrolls: boolean } | null {
  const { scrollHeight, lineHeight, padding, border, rows, grow } = box;
  if (![scrollHeight, lineHeight, padding, border].every(Number.isFinite) || lineHeight <= 0) return null;
  const frame = padding + border;
  const min = lineHeight * rows + frame;
  const max = lineHeight * Math.max(rows, grow) + frame;
  const content = scrollHeight + border;
  return { height: Math.min(Math.max(content, min), max), scrolls: content > max };
}
