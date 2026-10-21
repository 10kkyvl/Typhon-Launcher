import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));
vi.mock('../services/backend', () => ({ inWails: true }));

import { clearDelayMs, clearSourceProgress, initSourceProgress, sourceProgress } from './sourceProgress';

function send(name: string, data: unknown) {
  handlers[name]({ data });
}

describe('sourceProgress store', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    initSourceProgress();
  });

  afterEach(() => {
    clearSourceProgress('a');
    clearSourceProgress('b');
    vi.useRealTimers();
  });

  it('tracks sources independently when events interleave', () => {
    send('sources:progress', { sourceId: 'a', stage: 'parse', done: 500, total: 2000 });
    send('sources:progress', { sourceId: 'b', stage: 'fetch', done: 0, total: 1 });
    send('sources:progress', { sourceId: 'a', stage: 'parse', done: 1000, total: 2000 });
    const map = get(sourceProgress);
    expect(map.a).toMatchObject({ stage: 'parse', done: 1000, total: 2000, sawMatch: false });
    expect(map.b).toMatchObject({ stage: 'fetch', done: 0, total: 1 });
  });

  it('keeps startedAt across events of one source', () => {
    send('sources:progress', { sourceId: 'a', stage: 'fetch', done: 0, total: 1 });
    const first = get(sourceProgress).a.startedAt;
    vi.advanceTimersByTime(500);
    send('sources:progress', { sourceId: 'a', stage: 'parse', done: 0, total: 10 });
    expect(get(sourceProgress).a.startedAt).toBe(first);
  });

  it('remembers that the match stage was seen', () => {
    send('sources:progress', { sourceId: 'a', stage: 'match', done: 0, total: 3 });
    send('sources:progress', { sourceId: 'a', stage: 'apply', done: 0, total: 0 });
    expect(get(sourceProgress).a.sawMatch).toBe(true);
  });

  it('ignores unknown stages and empty ids', () => {
    send('sources:progress', { sourceId: 'a', stage: 'bogus', done: 1, total: 1 });
    send('sources:progress', { sourceId: '', stage: 'fetch', done: 1, total: 1 });
    expect(get(sourceProgress)).toEqual({});
  });

  it('clears the entry when the source stops updating', () => {
    send('sources:progress', { sourceId: 'a', stage: 'save', done: 0, total: 1 });
    send('source:updated', { id: 'a', name: 'A', status: 'updating' });
    expect(get(sourceProgress).a).toBeDefined();
    send('source:updated', { id: 'a', name: 'A', status: 'idle' });
    expect(get(sourceProgress).a).toBeUndefined();
  });

  it('clears the entry on error and on removal', () => {
    send('sources:progress', { sourceId: 'a', stage: 'parse', done: 1, total: 2 });
    send('sources:progress', { sourceId: 'b', stage: 'parse', done: 1, total: 2 });
    send('source:error', { sourceId: 'a' });
    send('source:updated', { id: 'b' });
    expect(get(sourceProgress)).toEqual({});
  });

  it('clears the entry after the delay once save is complete', () => {
    send('sources:progress', { sourceId: 'a', stage: 'save', done: 1, total: 1 });
    vi.advanceTimersByTime(clearDelayMs - 1);
    expect(get(sourceProgress).a).toBeDefined();
    vi.advanceTimersByTime(1);
    expect(get(sourceProgress).a).toBeUndefined();
  });

  it('does not clear a source that started a new refresh', () => {
    send('sources:progress', { sourceId: 'a', stage: 'save', done: 1, total: 1 });
    vi.advanceTimersByTime(clearDelayMs - 1);
    send('sources:progress', { sourceId: 'a', stage: 'fetch', done: 0, total: 1 });
    vi.advanceTimersByTime(clearDelayMs);
    expect(get(sourceProgress).a).toBeDefined();
  });
});
