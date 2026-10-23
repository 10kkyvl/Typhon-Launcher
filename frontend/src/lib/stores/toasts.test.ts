import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { dismissToast, toast, toasts } from './toasts';

beforeEach(() => {
  vi.useFakeTimers();
  toasts.set([]);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('toast', () => {
  it('shows a message as information unless it is told otherwise', () => {
    toast('saved');

    expect(get(toasts).map(({ message, kind }) => ({ message, kind }))).toEqual([{ message: 'saved', kind: 'info' }]);
  });

  it('keeps the kind the caller asked for', () => {
    toast('done', 'success');
    toast('broke', 'danger');

    expect(get(toasts).map((item) => item.kind)).toEqual(['success', 'danger']);
  });

  it('gives every toast its own id, in the order they were raised', () => {
    toast('first');
    toast('second');
    toast('third');

    const ids = get(toasts).map((item) => item.id);
    expect(new Set(ids).size).toBe(3);
    expect(ids).toEqual([...ids].sort((a, b) => a - b));
  });

  it('hides a toast by itself after three and a half seconds', () => {
    toast('saved');

    vi.advanceTimersByTime(3499);
    expect(get(toasts)).toHaveLength(1);

    vi.advanceTimersByTime(1);
    expect(get(toasts)).toHaveLength(0);
  });

  it('hides only the toast whose time is up, not the ones raised later', () => {
    toast('old');
    vi.advanceTimersByTime(2000);
    toast('new');

    vi.advanceTimersByTime(1500);
    expect(get(toasts).map((item) => item.message)).toEqual(['new']);

    vi.advanceTimersByTime(2000);
    expect(get(toasts)).toHaveLength(0);
  });

  it('shows the same message twice when it is raised twice', () => {
    toast('again');
    toast('again');

    expect(get(toasts).map((item) => item.message)).toEqual(['again', 'again']);
  });
});

describe('toast with an action', () => {
  it('carries the action and stays up longer than a plain toast', () => {
    const run = vi.fn();
    toast('mark it?', 'info', { label: 'Mark', run });

    expect(get(toasts)[0].action?.label).toBe('Mark');

    vi.advanceTimersByTime(3500);
    expect(get(toasts)).toHaveLength(1);

    vi.advanceTimersByTime(5500);
    expect(get(toasts)).toHaveLength(0);
    expect(run).not.toHaveBeenCalled();
  });

  it('leaves plain toasts without an action key', () => {
    toast('saved');

    expect('action' in get(toasts)[0]).toBe(false);
  });

  it('dismisses a single toast by id and keeps the others', () => {
    const first = toast('first');
    toast('second');

    dismissToast(first);

    expect(get(toasts).map((item) => item.message)).toEqual(['second']);
  });
});
