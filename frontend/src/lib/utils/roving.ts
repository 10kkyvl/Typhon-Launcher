/**
 * Keyboard helpers for composite widgets (menus, listboxes, tabs, radio groups).
 *
 * An option of a radio group that cannot be chosen is `aria-disabled`, never `disabled`. When the
 * option shows why on itself (the remove dialog's tile says why the game cannot be uninstalled), the
 * arrows skip it: pass `isDisabled`. When the reason is only its hint (the release match dialog's
 * game that is not in the catalog), the arrows stop on it so the hint can be read, and a press or an
 * arrow onto it does not choose it.
 */

export type Orientation = 'horizontal' | 'vertical' | 'both';

export interface NextIndexOptions {
  orientation?: Orientation;
  /** Wrap from the last item to the first and back. Default true. */
  loop?: boolean;
  isDisabled?: (index: number) => boolean;
}

/**
 * The index an arrow, Home or End key moves to, skipping disabled items.
 * Returns null when the key is not a navigation key for this orientation or
 * no enabled item exists.
 */
export function nextIndex(current: number, count: number, key: string, options: NextIndexOptions = {}): number | null {
  const { orientation = 'vertical', loop = true, isDisabled = () => false } = options;
  if (count <= 0) return null;
  const vertical = orientation !== 'horizontal';
  const horizontal = orientation !== 'vertical';

  let start: number;
  let step: number;
  switch (key) {
    case 'Home':
      start = 0;
      step = 1;
      break;
    case 'End':
      start = count - 1;
      step = -1;
      break;
    case 'ArrowDown':
      if (!vertical) return null;
      start = current + 1;
      step = 1;
      break;
    case 'ArrowUp':
      if (!vertical) return null;
      start = current < 0 ? count - 1 : current - 1;
      step = -1;
      break;
    case 'ArrowRight':
      if (!horizontal) return null;
      start = current + 1;
      step = 1;
      break;
    case 'ArrowLeft':
      if (!horizontal) return null;
      start = current < 0 ? count - 1 : current - 1;
      step = -1;
      break;
    default:
      return null;
  }

  for (let i = 0, index = start; i < count; i++, index += step) {
    if (index >= count || index < 0) {
      if (!loop) return current >= 0 && !isDisabled(current) ? current : null;
      index = (index + count) % count;
    }
    if (!isDisabled(index)) return index;
  }
  return null;
}

/** First enabled index at or after `from` (wrapping), or null. */
export function firstEnabled(count: number, isDisabled: (index: number) => boolean = () => false, from = 0) {
  for (let i = 0; i < count; i++) {
    const index = (from + i) % count;
    if (!isDisabled(index)) return index;
  }
  return null;
}

/**
 * Typeahead per the ARIA practices: typed characters build a short buffer;
 * the next enabled item whose label starts with it gets focus. Repeating one
 * character cycles through items starting with it.
 */
export function createTypeahead(timeout = 500, now: () => number = () => Date.now()) {
  let buffer = '';
  let last = 0;

  return function search(
    char: string,
    labels: readonly string[],
    current: number,
    isDisabled: (index: number) => boolean = () => false,
  ): number | null {
    const time = now();
    if (time - last > timeout) buffer = '';
    if (char.length !== 1 || (char === ' ' && buffer === '')) return null;
    last = time;
    buffer += char.toLocaleLowerCase();

    const repeated = buffer.length > 1 && buffer.split('').every((c) => c === buffer[0]);
    const needle = repeated ? buffer[0] : buffer;
    // A fresh or repeated search starts after the current item; a longer word keeps the current one in
    // play. With nothing focused it starts at the first item.
    const start = current < 0 ? 0 : current + (needle.length === 1 ? 1 : 0);
    const count = labels.length;
    for (let i = 0; i < count; i++) {
      const index = (start + i) % count;
      if (isDisabled(index)) continue;
      if (labels[index].trim().toLocaleLowerCase().startsWith(needle)) return index;
    }
    return null;
  };
}

/** True for a printable single character typed without modifiers (typeahead input). */
export function isTypeaheadKey(event: KeyboardEvent) {
  return event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey;
}
