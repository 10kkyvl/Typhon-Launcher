import { describe, expect, it } from 'vitest';
import { createLatest } from './latest';

describe('createLatest', () => {
  it('lets only the newest attempt apply its result', () => {
    const latest = createLatest();

    const first = latest.start();
    expect(first()).toBe(true);

    const second = latest.start();
    expect(first()).toBe(false);
    expect(second()).toBe(true);
  });

  it('keeps a single attempt current for as long as it runs', () => {
    const latest = createLatest();
    const only = latest.start();
    expect(only()).toBe(true);
    expect(only()).toBe(true);
  });

  it('tracks two consumers independently', () => {
    const a = createLatest();
    const b = createLatest();
    const first = a.start();
    b.start();
    expect(first()).toBe(true);
  });
});
