import { describe, expect, it } from 'vitest';
import { createToastWatch } from './toastWatch';

const nobody = () => false;

describe('what the stack remembers about a toast', () => {
  it('is being read while the pointer is over it or focus is inside it, either one', () => {
    const watch = createToastWatch<string>();
    watch.listed([1], nobody);

    expect(watch.read(1, { hover: true })).toBe(true);
    expect(watch.read(1, { focus: true })).toBe(true);
    expect(watch.read(1, { hover: false })).toBe(true);
    expect(watch.read(1, { focus: false })).toBe(false);
  });

  it('keeps where focus came from while the toast is on the list', () => {
    const watch = createToastWatch<string>();
    watch.listed([1], nobody);
    watch.entered(1, 'search field');

    watch.listed([1, 2], nobody);

    expect(watch.origin(1)).toBe('search field');
  });

  it('forgets every toast that has left the list, however it left', () => {
    const watch = createToastWatch<string>();
    watch.listed([1, 2, 3], nobody);
    watch.read(1, { hover: true });
    watch.entered(2, 'button');
    watch.read(3, { focus: true });

    watch.listed([3], nobody);
    watch.listed([1, 2, 3], nobody);

    expect(watch.read(1, {})).toBe(false);
    expect(watch.origin(2)).toBeUndefined();
    expect(watch.read(3, {})).toBe(true);
  });

  it('gives back where focus came from for the toast that left while it held focus', () => {
    const watch = createToastWatch<string>();
    watch.listed([1, 2], nobody);
    watch.entered(1, 'search field');
    watch.entered(2, 'button');
    const asked: number[] = [];

    const home = watch.listed([2], (id) => asked.push(id) > 0 && id === 1);

    expect(home).toEqual({ origin: 'search field' });
    expect(asked).toEqual([1]);
    expect(watch.listed([], nobody)).toBeNull();
  });

  it('says that a toast left holding focus even when it does not know where focus came from', () => {
    const watch = createToastWatch<string>();
    watch.listed([1], nobody);

    expect(watch.listed([], () => true)).toEqual({ origin: undefined });
  });

  it('remembers nothing about a toast that is not on the list: a late event from a card already gone', () => {
    const watch = createToastWatch<string>();
    watch.listed([1], nobody);
    watch.listed([], nobody);

    expect(watch.read(1, { hover: true })).toBe(false);
    watch.entered(1, 'button');
    expect(watch.origin(1)).toBeUndefined();
  });
});
