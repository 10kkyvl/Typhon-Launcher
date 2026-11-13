import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { claimLiveIsland, liveIsland, liveIslandCurrent, liveIslandHeld, liveIslandMessages, liveIslandOn } from './liveIsland';
import { QUEUE_STALE_MS, RESUME_FLOOR_MS } from './toasts';

const run = () => {};

beforeEach(() => {
  vi.useFakeTimers();
  liveIsland.clear();
});
afterEach(() => {
  liveIsland.clear();
  vi.useRealTimers();
});

const current = () => get(liveIslandCurrent)?.id;

describe('live island queue', () => {
  it('shows one message at a time: of equal rank the first, the next when it goes', () => {
    liveIsland.show({ id: 'a', text: 'A' });
    liveIsland.show({ id: 'b', text: 'B' });
    expect(current()).toBe('a');
    liveIsland.hide('a');
    expect(current()).toBe('b');
    liveIsland.hide('b');
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('replaces a message shown again under the same id and puts it at the back', () => {
    liveIsland.show({ id: 'a', text: 'A' });
    liveIsland.show({ id: 'b', text: 'B' });
    liveIsland.show({ id: 'a', text: 'A2' });
    expect(get(liveIslandMessages).map((m) => m.text)).toEqual(['B', 'A2']);
  });

  it('patches in place and ignores unknown ids', () => {
    liveIsland.show({ id: 'a', text: 'A' });
    liveIsland.update('a', { text: 'A!', count: 2 });
    liveIsland.update('nope', { text: 'x' });
    expect(get(liveIslandCurrent)).toMatchObject({ id: 'a', text: 'A!', count: 2 });
    expect(get(liveIslandMessages)).toHaveLength(1);
  });

  it('hides by ttl, and a new ttl restarts the timer', () => {
    liveIsland.show({ id: 'a', text: 'A', ttl: 1000 });
    vi.advanceTimersByTime(900);
    liveIsland.update('a', { ttl: 1000 });
    vi.advanceTimersByTime(900);
    expect(current()).toBe('a');
    vi.advanceTimersByTime(200);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('keeps a message without ttl until hidden', () => {
    liveIsland.show({ id: 'a', text: 'A' });
    vi.advanceTimersByTime(60_000);
    expect(current()).toBe('a');
  });

  it('does not let an old timer hide a re-shown message', () => {
    liveIsland.show({ id: 'a', text: 'A', ttl: 1000 });
    liveIsland.show({ id: 'a', text: 'A again' });
    vi.advanceTimersByTime(5000);
    expect(get(liveIslandCurrent)?.text).toBe('A again');
  });

  it('orders by rank: an error, then a friend, then a plain note', () => {
    liveIsland.show({ id: 'info', text: 'info' });
    liveIsland.show({ id: 'friend', text: 'friend', kind: 'friend' });
    liveIsland.show({ id: 'error', text: 'error', kind: 'error' });
    const seen: string[] = [];
    while (current()) {
      seen.push(current() as string);
      liveIsland.hide(current() as string);
    }
    expect(seen).toEqual(['error', 'friend', 'info']);
  });

  it('ranks a plain message in a danger or warning tone (a failed toast) as an error', () => {
    liveIsland.show({ id: 'friend', text: 'friend', kind: 'friend' });
    liveIsland.show({ id: 'toast', text: 'failed', tone: 'danger' });
    expect(current()).toBe('toast');
    liveIsland.show({ id: 'warn', text: 'careful', tone: 'warning' });
    liveIsland.hide('toast');
    expect(current()).toBe('warn');
  });

  it('a message that waited gets its full time, counted only while it is on screen', () => {
    liveIsland.show({ id: 'first', text: 'first', ttl: 1000 });
    liveIsland.show({ id: 'next', text: 'next', ttl: 1000 });
    vi.advanceTimersByTime(1000);
    expect(current()).toBe('next');
    vi.advanceTimersByTime(900);
    expect(current()).toBe('next');
    vi.advanceTimersByTime(100);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('a message replaced by a higher one comes back with the rest of its time', () => {
    liveIsland.show({ id: 'low', text: 'low', ttl: 1000 });
    vi.advanceTimersByTime(600);
    liveIsland.show({ id: 'high', text: 'high', kind: 'error' });
    expect(current()).toBe('high');
    vi.advanceTimersByTime(5000);
    expect(get(liveIslandMessages).map((m) => m.id)).toContain('low');
    liveIsland.hide('high');
    expect(current()).toBe('low');
    vi.advanceTimersByTime(399);
    expect(current()).toBe('low');
    vi.advanceTimersByTime(1);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('holds the clock while the reader looks, then gives a moment more', () => {
    liveIsland.show({ id: 'a', text: 'A', ttl: 1000 });
    liveIsland.hold(true);
    expect(get(liveIslandHeld)).toBe(true);
    vi.advanceTimersByTime(10_000);
    expect(current()).toBe('a');
    liveIsland.hold(false);
    vi.advanceTimersByTime(RESUME_FLOOR_MS - 100);
    expect(current()).toBe('a');
    vi.advanceTimersByTime(100);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('under a dialog a message with an action does not run out, and gets its full time when the dialog closes', () => {
    liveIsland.show({ id: 'offer', text: 'offer', ttl: 1000, actions: [{ label: 'Open', run }] });
    vi.advanceTimersByTime(700);
    liveIsland.dialog(true);
    vi.advanceTimersByTime(10_000);
    expect(current()).toBe('offer');
    liveIsland.dialog(false);
    vi.advanceTimersByTime(999);
    expect(current()).toBe('offer');
    vi.advanceTimersByTime(1);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('under a dialog a plain message runs out as usual', () => {
    liveIsland.dialog(true);
    liveIsland.show({ id: 'plain', text: 'plain', ttl: 1000 });
    vi.advanceTimersByTime(1000);
    expect(get(liveIslandCurrent)).toBeUndefined();
    liveIsland.dialog(false);
  });
});

// The stack's rule: a plain message that has waited too long for its turn is no longer news.
describe('a plain message that waited too long', () => {
  it('is dropped at its turn, through its own close when it has one', () => {
    const close = vi.fn(() => liveIsland.hide('old'));
    liveIsland.show({ id: 'first', text: 'first' });
    liveIsland.show({ id: 'old', text: 'old', close });
    vi.advanceTimersByTime(QUEUE_STALE_MS + 1);
    liveIsland.hide('first');
    expect(close).toHaveBeenCalledTimes(1);
    expect(get(liveIslandCurrent)).toBeUndefined();
  });

  it('is shown if its turn comes within the limit', () => {
    liveIsland.show({ id: 'first', text: 'first' });
    liveIsland.show({ id: 'next', text: 'next' });
    vi.advanceTimersByTime(QUEUE_STALE_MS);
    liveIsland.hide('first');
    expect(current()).toBe('next');
  });

  it('keeps an error and a message with an action however long they waited', () => {
    liveIsland.show({ id: 'first', text: 'first', kind: 'error' });
    liveIsland.show({ id: 'error', text: 'error', tone: 'danger' });
    liveIsland.show({ id: 'offer', text: 'offer', actions: [{ label: 'Undo', run }] });
    vi.advanceTimersByTime(QUEUE_STALE_MS * 5);
    liveIsland.hide('first');
    expect(current()).toBe('error');
    liveIsland.hide('error');
    expect(current()).toBe('offer');
  });

  it('is not dropped once it has been on screen, when it comes back after a higher one', () => {
    liveIsland.show({ id: 'note', text: 'note' });
    liveIsland.show({ id: 'error', text: 'error', kind: 'error' });
    vi.advanceTimersByTime(QUEUE_STALE_MS * 2);
    liveIsland.hide('error');
    expect(current()).toBe('note');
  });

  it('counts its wait from when it was raised again', () => {
    liveIsland.show({ id: 'first', text: 'first' });
    liveIsland.show({ id: 'again', text: 'again' });
    vi.advanceTimersByTime(QUEUE_STALE_MS);
    liveIsland.update('again', { count: 2 });
    vi.advanceTimersByTime(QUEUE_STALE_MS);
    liveIsland.hide('first');
    expect(current()).toBe('again');
  });
});

// The stack's rule: a toast held by a dialog takes no slot, so what the dialog raises is still shown.
describe('under a dialog, a message that waits it out', () => {
  it('lets a plain note raised after it go first', () => {
    liveIsland.dialog(true);
    liveIsland.show({ id: 'offer', text: 'Removed', actions: [{ label: 'Undo', run }] });
    liveIsland.show({ id: 'plain', text: 'Saved', ttl: 1000 });
    expect(current()).toBe('plain');
    vi.advanceTimersByTime(1000);
    expect(current()).toBe('offer');
    liveIsland.dialog(false);
  });

  it('lets an error raised after it go first, though it is an error itself', () => {
    liveIsland.dialog(true);
    liveIsland.show({ id: 'failed', text: 'Download failed', kind: 'error', ttl: 9000, actions: [{ label: 'Retry', run }] });
    liveIsland.show({ id: 'space', text: 'Not enough space', tone: 'danger' });
    expect(current()).toBe('space');
    liveIsland.hide('space');
    expect(current()).toBe('failed');
    liveIsland.dialog(false);
  });

  it('takes its place by rank again when the dialog closes', () => {
    liveIsland.dialog(true);
    liveIsland.show({ id: 'offer', text: 'Removed', kind: 'error', actions: [{ label: 'Undo', run }] });
    liveIsland.show({ id: 'plain', text: 'Saved' });
    expect(current()).toBe('plain');
    liveIsland.dialog(false);
    expect(current()).toBe('offer');
  });
});

describe('who shows the messages', () => {
  it('the island is on while one has claimed it, and off when it lets go', () => {
    expect(get(liveIslandOn)).toBe(false);
    const release = claimLiveIsland();
    expect(get(liveIslandOn)).toBe(true);
    release();
    expect(get(liveIslandOn)).toBe(false);
  });
});
