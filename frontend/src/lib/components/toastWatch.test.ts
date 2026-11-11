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
});
