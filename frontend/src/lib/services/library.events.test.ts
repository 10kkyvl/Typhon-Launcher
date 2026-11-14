import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { LibraryCollection, LibraryGame } from './library';

const runtime = vi.hoisted(() => ({
  handlers: new Map<string, (event: { data: unknown }) => void>(),
  offs: [] as Array<() => void>,
  wails: true,
}));

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      runtime.handlers.set(name, cb);
      const off = vi.fn();
      runtime.offs.push(off);
      return off;
    }),
  },
}));
vi.mock('./backend', () => ({
  get inWails() {
    return runtime.wails;
  },
}));

const { onCollectionsChanged, onLibraryUpdated } = await import('./library');

beforeEach(() => {
  runtime.handlers.clear();
  runtime.offs.length = 0;
  runtime.wails = true;
});

describe.each([
  ['onLibraryUpdated', 'library:updated', onLibraryUpdated as (handler: (list: unknown[]) => void) => () => void],
  ['onCollectionsChanged', 'library:collections', onCollectionsChanged as (handler: (list: unknown[]) => void) => () => void],
])('%s', (_name, event, subscribe) => {
  it('hands over the list the event carries', () => {
    const handler = vi.fn();
    subscribe(handler);
    const list = [{ id: 'a' } as LibraryGame & LibraryCollection];

    runtime.handlers.get(event)?.({ data: list });

    expect(handler).toHaveBeenCalledWith(list);
  });

  it.each([null, undefined, 'text', { id: 'a' }])('hands over an empty list for a payload that is not a list: %s', (data) => {
    const handler = vi.fn();
    subscribe(handler);

    runtime.handlers.get(event)?.({ data });

    expect(handler).toHaveBeenCalledWith([]);
  });

  it('answers the unsubscribe of the runtime', () => {
    const off = subscribe(vi.fn());

    off();

    expect(runtime.offs).toHaveLength(1);
    expect(runtime.offs[0]).toHaveBeenCalledTimes(1);
  });

  it('subscribes to nothing outside Wails', () => {
    runtime.wails = false;
    const off = subscribe(vi.fn());

    off();

    expect(runtime.handlers.size).toBe(0);
  });
});
