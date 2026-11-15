import { describe, expect, it } from 'vitest';
import { askMove, keyTarget, placeAt, pointerTarget, roomFor, settleMove, shownOrder, type MoveAsk, type RowBox } from './queueReorder';

describe('keyTarget: a key on the grip to the place it asks for', () => {
  it.each([
    ['ArrowUp', 2, 4, 1],
    ['ArrowDown', 2, 4, 3],
    ['Home', 2, 4, 0],
    ['End', 1, 4, 3],
    ['ArrowUp', 0, 4, 0],
    ['ArrowDown', 3, 4, 3],
    ['End', 3, 4, 3],
    ['Home', 0, 1, 0],
  ])('%s at %i of %i → %i', (key, index, count, target) => {
    expect(keyTarget(key, index, count)).toBe(target);
  });

  it.each(['Enter', ' ', 'ArrowLeft', 'PageDown', 'Tab'])('%j asks for nothing', (key) => {
    expect(keyTarget(key, 1, 4)).toBeNull();
  });
});

// Three rows of 50 px with 10 px between them: midpoints at 25, 85, 145.
const rows: RowBox[] = [
  { top: 0, height: 50 },
  { top: 60, height: 50 },
  { top: 120, height: 50 },
];

describe('pointerTarget: where a dragged row lands, from how far the pointer moved it', () => {
  it.each([
    [0, 0, 0],
    [0, 59, 0],
    [0, 61, 1],
    [0, 121, 2],
    [0, 500, 2],
    [2, -59, 2],
    [2, -61, 1],
    [2, -500, 0],
    [1, 0, 1],
    [1, -61, 0],
    [1, 61, 2],
  ])('row %i moved by %i px → %i', (from, dy, target) => {
    expect(pointerTarget(rows, from, dy)).toBe(target);
  });

  it('takes rows of different heights by their own midpoints', () => {
    const tall: RowBox[] = [
      { top: 0, height: 120 },
      { top: 130, height: 40 },
      { top: 180, height: 40 },
    ];
    // The short last row's middle is at 200; the tall first one's at 60.
    expect(pointerTarget(tall, 2, -139)).toBe(1);
    expect(pointerTarget(tall, 2, -141)).toBe(0);
  });
});

describe('roomFor: how far a row moves to make room for the dragged one', () => {
  it.each([
    [0, 1, 3, 60, 0],
    [2, 1, 3, 60, -60],
    [3, 1, 3, 60, -60],
    [4, 1, 3, 60, 0],
    [1, 1, 3, 60, 0],
    [0, 3, 0, 60, 60],
    [2, 3, 0, 60, 60],
    [3, 3, 0, 60, 0],
    [2, 2, 2, 60, 0],
  ])('row %i, dragged from %i to %i, step %i → %i', (index, from, to, step, shift) => {
    expect(roomFor(index, from, to, step)).toBe(shift);
  });
});

describe('placeAt and shownOrder: the order the list shows while moves are asked', () => {
  it('moves one id to a place, clamped to the list', () => {
    expect(placeAt(['a', 'b', 'c', 'd'], 'd', 0)).toEqual(['d', 'a', 'b', 'c']);
    expect(placeAt(['a', 'b', 'c', 'd'], 'a', 2)).toEqual(['b', 'c', 'a', 'd']);
    expect(placeAt(['a', 'b', 'c'], 'a', 9)).toEqual(['b', 'c', 'a']);
    expect(placeAt(['a', 'b', 'c'], 'x', 0)).toEqual(['a', 'b', 'c']);
  });

  it('applies the asked places in the order they were asked, and ignores an id that left', () => {
    const asked = new Map<string, number>([
      ['c', 0],
      ['gone', 1],
      ['a', 2],
    ]);
    expect(shownOrder(['a', 'b', 'c'], asked)).toEqual(['c', 'b', 'a']);
    expect(shownOrder(['a', 'b', 'c'], new Map())).toEqual(['a', 'b', 'c']);
  });
});

describe('askMove and settleMove: one move in flight per download, the last asked place wins', () => {
  type Step = ['ask', number] | ['settle', boolean];
  const table: [string, Step[], (number | null)[], MoveAsk | undefined][] = [
    ['a first ask is sent', [['ask', 2]], [2], { sent: 2, next: null }],
    ['asks while one is in flight are not sent; only the last is kept', [['ask', 2], ['ask', 1], ['ask', 0]], [2, null, null], { sent: 2, next: 0 }],
    ['the kept ask is sent when the one in flight succeeds', [['ask', 2], ['ask', 1], ['ask', 0], ['settle', true]], [2, null, null, 0], { sent: 0, next: null }],
    ['an ask back to the place in flight is not sent again', [['ask', 2], ['ask', 1], ['ask', 2], ['settle', true]], [2, null, null, null], undefined],
    ['a success with nothing kept ends the move', [['ask', 1], ['settle', true]], [1, null], undefined],
    ['a failure drops the kept ask: the list goes back to Go’s order', [['ask', 2], ['ask', 0], ['settle', false]], [2, null, null], undefined],
  ];

  it.each(table)('%s', (_name, steps, sends, end) => {
    let state: MoveAsk | undefined;
    const sent: (number | null)[] = [];
    for (const step of steps) {
      const result = step[0] === 'ask' ? askMove(state, step[1]) : settleMove(state as MoveAsk, step[1]);
      state = result.state;
      sent.push(result.send);
    }
    expect(sent).toEqual(sends);
    expect(state).toEqual(end);
  });
});
