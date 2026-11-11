import { describe, expect, it } from 'vitest';
import { createToastWatch } from './toastWatch';

describe('what the stack remembers about a toast', () => {
  it('is being read while the pointer is over it or focus is inside it, either one', () => {
    const watch = createToastWatch<string>();

    expect(watch.read(1, { hover: true })).toBe(true);
    expect(watch.read(1, { focus: true })).toBe(true);
    expect(watch.read(1, { hover: false })).toBe(true);
    expect(watch.read(1, { focus: false })).toBe(false);
  });

  it('keeps where focus came from until the toast is forgotten', () => {
    const watch = createToastWatch<string>();
    watch.entered(1, 'search field');

    expect(watch.origin(1)).toBe('search field');
    watch.forget(1);
    expect(watch.origin(1)).toBeUndefined();
  });

  it('forgets every toast that has left the list, however it left, and says which', () => {
    const watch = createToastWatch<string>();
    watch.read(1, { hover: true });
    watch.entered(2, 'button');
    watch.read(3, { focus: true });

    expect(watch.prune([3]).sort()).toEqual([1, 2]);

    expect(watch.size).toBe(1);
    expect(watch.origin(2)).toBeUndefined();
    expect(watch.read(1, {})).toBe(false);
  });

  it('keeps nothing for a toast nobody reads, so a late event from a toast already gone leaves no entry', () => {
    const watch = createToastWatch<string>();
    watch.read(1, { hover: true });
    watch.read(1, { hover: false });
    watch.read(2, { focus: false });

    expect(watch.size).toBe(0);
  });
});
