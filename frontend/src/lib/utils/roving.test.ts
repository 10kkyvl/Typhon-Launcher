import { describe, expect, it } from 'vitest';
import { createTypeahead, firstEnabled, nextIndex } from './roving';

describe('nextIndex', () => {
  it('moves and wraps vertically', () => {
    expect(nextIndex(0, 3, 'ArrowDown')).toBe(1);
    expect(nextIndex(2, 3, 'ArrowDown')).toBe(0);
    expect(nextIndex(0, 3, 'ArrowUp')).toBe(2);
  });

  it('ignores keys of the other orientation', () => {
    expect(nextIndex(0, 3, 'ArrowRight')).toBeNull();
    expect(nextIndex(0, 3, 'ArrowDown', { orientation: 'horizontal' })).toBeNull();
    expect(nextIndex(0, 3, 'Enter')).toBeNull();
  });

  it('skips disabled items, including on Home and End', () => {
    const disabled = (i: number) => i === 1 || i === 3;
    expect(nextIndex(0, 4, 'ArrowDown', { isDisabled: disabled })).toBe(2);
    expect(nextIndex(2, 4, 'ArrowDown', { isDisabled: disabled })).toBe(0);
    expect(nextIndex(2, 4, 'End', { isDisabled: disabled })).toBe(2);
    expect(nextIndex(2, 4, 'Home', { isDisabled: (i) => i === 0 })).toBe(1);
  });

  it('enters from nothing focused', () => {
    expect(nextIndex(-1, 3, 'ArrowDown')).toBe(0);
    expect(nextIndex(-1, 3, 'ArrowUp')).toBe(2);
  });

  it('stops at the ends without loop', () => {
    expect(nextIndex(2, 3, 'ArrowRight', { orientation: 'horizontal', loop: false })).toBe(2);
  });
});

describe('createTypeahead', () => {
  const labels = ['Open', 'Install', 'Info', 'Move', 'Remove'];

  it('finds the next item starting with a letter and cycles on repeats', () => {
    let time = 0;
    const search = createTypeahead(500, () => time);
    expect(search('i', labels, 0)).toBe(1);
    time += 100;
    expect(search('i', labels, 1)).toBe(2);
    time += 100;
    expect(search('i', labels, 2)).toBe(1);
  });

  it('matches a typed prefix and resets after a pause', () => {
    let time = 0;
    const search = createTypeahead(500, () => time);
    expect(search('i', labels, 0)).toBe(1);
    time += 100;
    expect(search('n', labels, 1)).toBe(1);
    time += 100;
    expect(search('f', labels, 1)).toBe(2);
    time += 1000;
    expect(search('m', labels, 2)).toBe(3);
  });

  it('skips disabled items', () => {
    const search = createTypeahead();
    expect(search('i', labels, 0, (i) => i === 1)).toBe(2);
  });

  it('from nothing focused (the menu itself), starts at the first item', () => {
    const search = createTypeahead();
    expect(search('i', ['Install', 'Info'], -1)).toBe(0);
  });

  it('ignores case, the leading space of a label and a lone space', () => {
    const search = createTypeahead();
    expect(search('M', ['  open', 'move'], 0)).toBe(1);
    expect(createTypeahead()(' ', labels, 0)).toBeNull();
  });
});

describe('firstEnabled', () => {
  it('finds the first enabled index from a start, wrapping, or null when all are disabled', () => {
    expect(firstEnabled(3, (i) => i === 0)).toBe(1);
    expect(firstEnabled(3, (i) => i === 2, 2)).toBe(0);
    expect(firstEnabled(2, () => true)).toBeNull();
  });
});
