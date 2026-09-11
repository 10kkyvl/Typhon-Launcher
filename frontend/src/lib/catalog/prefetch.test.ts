import { describe, expect, it, vi } from 'vitest';
import { createPagePrefetch } from './prefetch';
describe('catalog prefetch', () => {
  it('reuses one next-page request', async () => {
    const p = createPagePrefetch<number>(); const load = vi.fn(async () => 2);
    p.warm('page2-rev1', load); p.warm('page2-rev1', load);
    expect(await p.take('page2-rev1', load)).toBe(2); expect(load).toHaveBeenCalledTimes(1);
  });
  it('does not mix filters or revisions and retries a failed warm request', async () => {
    const p = createPagePrefetch<number>(); p.warm('old', async () => 1);
    expect(await p.take('new', async () => 3)).toBe(3);
    p.warm('failed', async () => { throw new Error('offline'); });
    expect(await p.take('failed', async () => 4)).toBe(4);
  });
});
