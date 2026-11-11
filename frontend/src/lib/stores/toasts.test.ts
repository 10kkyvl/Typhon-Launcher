import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  ACTION_MS,
  MAX_VISIBLE,
  PLAIN_MS,
  RESUME_FLOOR_MS,
  URGENT_MS,
  dismissToast,
  heldByDialog,
  holdToasts,
  pauseToast,
  queuedToasts,
  resumeToast,
  toast,
  toastDuration,
  toasts,
} from './toasts';

beforeEach(() => {
  vi.useFakeTimers();
  toasts.set([]);
  holdToasts(false);
});

afterEach(() => {
  toasts.set([]);
  holdToasts(false);
  vi.useRealTimers();
});

const shown = () => get(toasts).map((item) => item.message);
const waiting = () => queuedToasts().map((item) => item.message);

describe('toast', () => {
  it('shows a message as information unless it is told otherwise', () => {
    toast('saved');

    expect(get(toasts).map(({ message, kind }) => ({ message, kind }))).toEqual([{ message: 'saved', kind: 'info' }]);
  });

  it('keeps the kind the caller asked for, a warning included', () => {
    toast('done', 'success');
    toast('careful', 'warning');

    expect(get(toasts).map((item) => item.kind)).toEqual(['success', 'warning']);
  });

  it('gives every toast its own id, in the order they were raised', () => {
    const ids = [toast('first'), toast('second'), toast('third')];

    expect(new Set(ids).size).toBe(3);
    expect(ids).toEqual([...ids].sort((a, b) => a - b));
  });

  it('leaves plain toasts without an action key', () => {
    toast('saved');

    expect('action' in get(toasts)[0]).toBe(false);
  });

  it('dismisses a single toast by id and keeps the others', () => {
    const first = toast('first');
    toast('second');

    dismissToast(first);

    expect(shown()).toEqual(['second']);
  });
});

describe('how long a toast stays', () => {
  it('is four seconds for a plain one, six for a warning or an error, nine with an action', () => {
    expect(PLAIN_MS).toBe(4000);
    expect(URGENT_MS).toBe(6000);
    expect(ACTION_MS).toBe(9000);
    expect(toastDuration({ kind: 'info' })).toBe(PLAIN_MS);
    expect(toastDuration({ kind: 'success' })).toBe(PLAIN_MS);
    expect(toastDuration({ kind: 'warning' })).toBe(URGENT_MS);
    expect(toastDuration({ kind: 'danger' })).toBe(URGENT_MS);
    expect(toastDuration({ kind: 'danger', action: { label: 'Retry', run: () => {} } })).toBe(ACTION_MS);
  });

  it('hides a plain toast by itself when its time is up, not before', () => {
    toast('saved');

    vi.advanceTimersByTime(PLAIN_MS - 1);
    expect(get(toasts)).toHaveLength(1);

    vi.advanceTimersByTime(1);
    expect(get(toasts)).toHaveLength(0);
  });

  it('keeps an error on screen longer than a plain message', () => {
    toast('broke', 'danger');

    vi.advanceTimersByTime(PLAIN_MS);
    expect(shown()).toEqual(['broke']);

    vi.advanceTimersByTime(URGENT_MS - PLAIN_MS);
    expect(shown()).toEqual([]);
  });

  it('hides only the toast whose time is up, not the one raised later', () => {
    toast('old');
    vi.advanceTimersByTime(2500);
    toast('new');

    vi.advanceTimersByTime(1500);
    expect(shown()).toEqual(['new']);

    vi.advanceTimersByTime(2500);
    expect(shown()).toEqual([]);
  });

  it('carries the action and stays up longest, without running it', () => {
    const run = vi.fn();
    toast('mark it?', 'info', { label: 'Mark', run });

    expect(get(toasts)[0].action?.label).toBe('Mark');

    vi.advanceTimersByTime(ACTION_MS - 1);
    expect(get(toasts)).toHaveLength(1);

    vi.advanceTimersByTime(1);
    expect(get(toasts)).toHaveLength(0);
    expect(run).not.toHaveBeenCalled();
  });
});

describe('a toast being read', () => {
  it('stops its clock while paused and keeps what was left', () => {
    const id = toast('saved');
    vi.advanceTimersByTime(1000);

    pauseToast(id);
    vi.advanceTimersByTime(60_000);
    expect(shown()).toEqual(['saved']);

    resumeToast(id);
    vi.advanceTimersByTime(PLAIN_MS - 1000 - 1);
    expect(shown()).toEqual(['saved']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });

  it('gives at least a moment after the reader looks away', () => {
    const id = toast('saved');
    vi.advanceTimersByTime(PLAIN_MS - 100);

    pauseToast(id);
    resumeToast(id);

    vi.advanceTimersByTime(RESUME_FLOOR_MS - 1);
    expect(shown()).toEqual(['saved']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });

  it('takes a second pause or a second resume as one', () => {
    const id = toast('saved');

    pauseToast(id);
    pauseToast(id);
    vi.advanceTimersByTime(10_000);
    resumeToast(id);
    resumeToast(id);

    vi.advanceTimersByTime(PLAIN_MS - 1);
    expect(shown()).toEqual(['saved']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });

  it('ignores a pause or a resume for a toast that is gone', () => {
    const id = toast('saved');
    dismissToast(id);

    expect(() => {
      pauseToast(id);
      resumeToast(id);
    }).not.toThrow();
    expect(shown()).toEqual([]);
  });
});

describe('the same message raised again', () => {
  it('does not stack: the one on screen takes it, the same id comes back and its clock restarts', () => {
    const first = toast('again');
    vi.advanceTimersByTime(PLAIN_MS - 500);

    const second = toast('again');

    expect(second).toBe(first);
    expect(shown()).toEqual(['again']);
    vi.advanceTimersByTime(PLAIN_MS - 1);
    expect(shown()).toEqual(['again']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });

  it('is another toast when the kind differs, or the action', () => {
    toast('again');
    toast('again', 'danger');
    expect(get(toasts).map((item) => item.kind)).toEqual(['info', 'danger']);

    toasts.set([]);
    toast('again');
    toast('again', 'info', { label: 'Open', run: () => {} });
    expect(get(toasts).map((item) => item.action?.label)).toEqual([undefined, 'Open']);
  });

  it('folds into one that is still waiting, which keeps its place in the queue', () => {
    toast('one');
    toast('two');
    const third = toast('three');
    toast('four');

    expect(toast('three')).toBe(third);
    expect(waiting()).toEqual(['three', 'four']);
  });

  it('a paused one that takes a repeat stays paused, with its full time back', () => {
    const id = toast('again');
    vi.advanceTimersByTime(PLAIN_MS - 500);
    pauseToast(id);

    toast('again');
    vi.advanceTimersByTime(60_000);
    expect(shown()).toEqual(['again']);

    resumeToast(id);
    vi.advanceTimersByTime(PLAIN_MS - 1);
    expect(shown()).toEqual(['again']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });
});

describe('many toasts at once', () => {
  it('shows two and keeps the rest waiting in the order raised', () => {
    expect(MAX_VISIBLE).toBe(2);
    toast('one');
    toast('two');
    toast('three');
    toast('four');

    expect(shown()).toEqual(['one', 'two']);
    expect(waiting()).toEqual(['three', 'four']);
  });

  it('shows the next one when a visible one is dismissed, and starts its clock then', () => {
    const one = toast('one');
    toast('two');
    toast('three');
    vi.advanceTimersByTime(3000);

    dismissToast(one);

    expect(shown()).toEqual(['two', 'three']);
    vi.advanceTimersByTime(1000);
    expect(shown()).toEqual(['three']);
    vi.advanceTimersByTime(PLAIN_MS - 1000 - 1);
    expect(shown()).toEqual(['three']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });

  it('shows the next one when a visible one runs out', () => {
    toast('one');
    toast('two');
    toast('three');

    vi.advanceTimersByTime(PLAIN_MS);

    expect(shown()).toEqual(['three']);
    expect(waiting()).toEqual([]);
  });

  it('drops a waiting one that is dismissed before it was shown', () => {
    toast('one');
    toast('two');
    const three = toast('three');
    toast('four');

    dismissToast(three);

    expect(shown()).toEqual(['one', 'two']);
    expect(waiting()).toEqual(['four']);
  });

  it('emptying the list from outside drops the waiting ones and every clock too', () => {
    toast('one');
    toast('two');
    toast('three');

    toasts.set([]);
    vi.advanceTimersByTime(60_000);

    expect(shown()).toEqual([]);
    expect(waiting()).toEqual([]);
    toast('fresh');
    expect(shown()).toEqual(['fresh']);
  });
});

describe('a toast with an action while a dialog is open', () => {
  const action = { label: 'Retry', run: () => {} };

  it('is the only kind the dialog holds', () => {
    expect(heldByDialog({ action }, true)).toBe(true);
    expect(heldByDialog({ action }, false)).toBe(false);
    expect(heldByDialog({}, true)).toBe(false);
  });

  it('does not run out while the dialog is open: its clock starts when the dialog closes', () => {
    holdToasts(true);
    toast('failed', 'danger', action);

    vi.advanceTimersByTime(60_000);
    expect(shown()).toEqual(['failed']);

    holdToasts(false);
    vi.advanceTimersByTime(ACTION_MS - 1);
    expect(shown()).toEqual(['failed']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });

  it('one already on screen waits too, and gets its full time back after the dialog', () => {
    toast('failed', 'danger', action);
    vi.advanceTimersByTime(ACTION_MS - 1000);

    holdToasts(true);
    vi.advanceTimersByTime(60_000);
    expect(shown()).toEqual(['failed']);

    holdToasts(false);
    vi.advanceTimersByTime(ACTION_MS - 1);
    expect(shown()).toEqual(['failed']);
    vi.advanceTimersByTime(1);
    expect(shown()).toEqual([]);
  });

  it('a plain toast keeps its own clock while the dialog is open', () => {
    holdToasts(true);
    toast('saved');

    vi.advanceTimersByTime(PLAIN_MS);

    expect(shown()).toEqual([]);
  });

  it('stays while the reader is on it after the dialog has closed', () => {
    holdToasts(true);
    const id = toast('failed', 'danger', action);
    pauseToast(id);

    holdToasts(false);
    vi.advanceTimersByTime(60_000);
    expect(shown()).toEqual(['failed']);

    resumeToast(id);
    vi.advanceTimersByTime(ACTION_MS);
    expect(shown()).toEqual([]);
  });
});

describe('a dialog open with held toasts on screen', () => {
  const offer = (label: string) => ({ label, run: () => {} });

  it('held toasts take no slot: an error raised by the dialog shows at once and times out normally', () => {
    holdToasts(true);
    toast('Mark Hades as 100%?', 'info', offer('Mark'));
    toast('The emulator closed at once', 'info', offer('Settings'));

    toast('Install failed', 'danger');

    expect(shown()).toEqual(['Mark Hades as 100%?', 'The emulator closed at once', 'Install failed']);
    vi.advanceTimersByTime(URGENT_MS);
    expect(shown()).toEqual(['Mark Hades as 100%?', 'The emulator closed at once']);
  });

  it('toasts already on screen free their slots when the dialog opens over them', () => {
    toast('Mark Hades as 100%?', 'info', offer('Mark'));
    toast('The emulator closed at once', 'info', offer('Settings'));
    toast('Saved');
    expect(waiting()).toEqual(['Saved']);

    holdToasts(true);

    expect(shown()).toContain('Saved');
    expect(waiting()).toEqual([]);
  });

  it('after the dialog they count again: nothing new is shown until there is room', () => {
    holdToasts(true);
    toast('Mark Hades as 100%?', 'info', offer('Mark'));
    toast('The emulator closed at once', 'info', offer('Settings'));
    holdToasts(false);

    toast('Saved');

    expect(waiting()).toEqual(['Saved']);
  });
});
