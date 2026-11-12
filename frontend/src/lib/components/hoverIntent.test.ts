import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createHoverIntent, HOVER_DELAY, LEAVE_GRACE, type HoverIntent } from './hoverIntent';

// Every hint of the window shares one rule (one shown at a time): each test lets go of the ones it made.
const made: HoverIntent[] = [];

function make() {
  const changes: boolean[] = [];
  const intent = createHoverIntent((open) => changes.push(open));
  made.push(intent);
  return { changes, intent };
}

afterEach(() => {
  for (const intent of made.splice(0)) intent.destroy();
});

describe('hover intent', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('opens only after the hover delay', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY - 1);
    expect(changes).toEqual([]);
    vi.advanceTimersByTime(1);
    expect(changes).toEqual([true]);
    expect(intent.open).toBe(true);
  });

  it('never opens when the pointer leaves early', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(100);
    intent.unhover();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([]);
  });

  it('closes when the pointer has been away for the leave grace', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE - 1);
    expect(changes).toEqual([true]);
    vi.advanceTimersByTime(1);
    expect(changes).toEqual([true, false]);
  });

  it('stays open while the pointer crosses from the control onto the hint', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE - 1);
    intent.hover();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([true]);
    intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE);
    expect(changes).toEqual([true, false]);
  });

  it('a press before the delay keeps it closed until the pointer leaves and comes back', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(100);
    intent.escape();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([]);
    intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE);
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    expect(changes).toEqual([true]);
  });

  it('a press before the delay keeps it closed when focus then moves off the control into what the press opened', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(100);
    intent.escape();
    intent.blur();
    vi.advanceTimersByTime(HOVER_DELAY * 3);
    expect(changes).toEqual([]);
    intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE);
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    expect(changes).toEqual([true]);
  });

  it('a press while open hides it, and coming back inside the grace does not show it again', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.escape();
    intent.unhover();
    intent.hover();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([true, false]);
  });

  it('a dismissal forgets focus: a control removed without a blur does not hold the next hint open', () => {
    const { intent, changes } = make();
    intent.focus();
    intent.dismiss();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE);
    expect(changes).toEqual([true, false, true, false]);
    expect(intent.open).toBe(false);
  });

  it('a pending leave is cancelled on destroy', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.unhover();
    intent.destroy();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([true]);
  });

  it('opens at once on keyboard focus and closes on blur', () => {
    const { intent, changes } = make();
    intent.focus();
    expect(changes).toEqual([true]);
    intent.blur();
    expect(changes).toEqual([true, false]);
  });

  it('closes on Escape and stays closed until the next focus', () => {
    const { intent, changes } = make();
    intent.focus();
    intent.escape();
    expect(changes).toEqual([true, false]);
    intent.focus();
    expect(changes).toEqual([true, false]);
    intent.blur();
    intent.focus();
    expect(changes).toEqual([true, false, true]);
  });

  it('stays open on focus when the pointer leaves', () => {
    const { intent, changes } = make();
    intent.focus();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE);
    expect(changes).toEqual([true]);
    expect(intent.open).toBe(true);
  });

  it('dismisses on scroll and can open again afterwards', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.dismiss();
    expect(changes).toEqual([true, false]);
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    expect(changes).toEqual([true, false, true]);
  });

  it('cancels the pending timer on destroy', () => {
    const { intent, changes } = make();
    intent.hover();
    intent.destroy();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([]);
  });
});

describe('one hint at a time', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const rest = (intent: HoverIntent) => {
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
  };

  it('the pointer resting on another control hides the keyboard hint, which comes back when the pointer leaves', () => {
    const keyboard = make();
    const pointer = make();
    keyboard.intent.focus();

    rest(pointer.intent);

    expect(pointer.changes).toEqual([true]);
    expect(keyboard.changes).toEqual([true, false]);

    pointer.intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE);

    expect(pointer.changes).toEqual([true, false]);
    expect(keyboard.changes).toEqual([true, false, true]);
  });

  it('the keyboard hint does not come back once focus has left its control', () => {
    const keyboard = make();
    const pointer = make();
    keyboard.intent.focus();
    rest(pointer.intent);
    keyboard.intent.blur();

    pointer.intent.unhover();
    vi.advanceTimersByTime(LEAVE_GRACE);

    expect(keyboard.changes).toEqual([true, false]);
  });

  it('keyboard focus moving on closes the pointer hint, which does not come back by itself', () => {
    const pointer = make();
    const keyboard = make();
    rest(pointer.intent);

    keyboard.intent.focus();
    expect(pointer.changes).toEqual([true, false]);
    keyboard.intent.blur();

    expect(keyboard.changes).toEqual([true, false]);
    expect(pointer.changes).toEqual([true, false]);
    rest(pointer.intent);
    expect(pointer.changes).toEqual([true, false, true]);
  });

  it('a third hint takes over from the second; the keyboard hint comes back only when the last one goes', () => {
    const keyboard = make();
    const first = make();
    const second = make();
    keyboard.intent.focus();
    rest(first.intent);
    // The second takes over while the first is still shown.
    rest(second.intent);

    expect(first.changes).toEqual([true, false]);
    expect(keyboard.changes).toEqual([true, false]);
    expect(second.changes).toEqual([true]);

    second.intent.dismiss();

    expect(keyboard.changes).toEqual([true, false, true]);
    expect(first.changes).toEqual([true, false]);
  });

  it('a keyboard hint closed with Escape stays closed when the pointer hint goes', () => {
    const keyboard = make();
    const pointer = make();
    keyboard.intent.focus();
    keyboard.intent.escape();
    rest(pointer.intent);
    pointer.intent.dismiss();

    expect(keyboard.changes).toEqual([true, false]);
  });

  it('a shown hint destroyed with its control gives the keyboard hint back', () => {
    const keyboard = make();
    const pointer = make();
    keyboard.intent.focus();
    rest(pointer.intent);

    pointer.intent.destroy();

    expect(keyboard.changes).toEqual([true, false, true]);
  });
});
