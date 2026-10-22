import { describe, expect, it, vi } from 'vitest';
import { createReportCache } from './sysreqCache';
import type { SysreqReport } from '../services/sysreq';

const good: SysreqReport = { verdict: 'good', reason: '', checks: [] };
const unknown: SysreqReport = { verdict: 'unknown', reason: 'partial', checks: [] };

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
}

describe('report cache', () => {
  it('reuses a settled report', async () => {
    const cache = createReportCache(() => 0);
    const load = vi.fn(async () => good);
    await cache.fetch('a', load);
    await settle();
    await cache.fetch('a', load);
    expect(load).toHaveBeenCalledTimes(1);
  });

  it('does not keep an unknown verdict', async () => {
    const cache = createReportCache(() => 0);
    const load = vi.fn(async () => unknown);
    await cache.fetch('a', load);
    await settle();
    await cache.fetch('a', load);
    expect(load).toHaveBeenCalledTimes(2);
  });

  it('does not keep a failure', async () => {
    const cache = createReportCache(() => 0);
    const load = vi.fn(async () => {
      throw new Error('x');
    });
    await expect(cache.fetch('a', load)).rejects.toThrow();
    await settle();
    expect(cache.size).toBe(0);
  });

  it('drops an entry older than a minute', async () => {
    let t = 0;
    const cache = createReportCache(() => t);
    const load = vi.fn(async () => good);
    await cache.fetch('a', load);
    t = 60_000;
    await cache.fetch('a', load);
    expect(load).toHaveBeenCalledTimes(1);
    t = 120_001;
    await cache.fetch('a', load);
    expect(load).toHaveBeenCalledTimes(2);
  });

  it('refetches on demand', async () => {
    const cache = createReportCache(() => 0);
    const load = vi.fn(async () => good);
    await cache.fetch('a', load);
    await cache.fetch('a', load, true);
    expect(load).toHaveBeenCalledTimes(2);
  });

  it('keeps only the newest keys', async () => {
    const cache = createReportCache(() => 0, 60_000, 3);
    for (const key of ['a', 'b', 'c', 'd']) await cache.fetch(key, async () => good);
    expect(cache.size).toBe(3);
    const load = vi.fn(async () => good);
    await cache.fetch('a', load);
    expect(load).toHaveBeenCalledTimes(1);
  });
});
