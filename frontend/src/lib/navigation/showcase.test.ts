import { afterEach, describe, expect, it, vi } from 'vitest';

// The page itself is proven by Showcase.dom.test.ts; here it only has to be told apart from nothing.
const page = vi.hoisted(() => ({ default: () => {} }));
vi.mock('../../routes/showcase/Showcase.svelte', () => page);

// The module reads the mode once, as it loads: each case loads it again under its own mode.
async function gateIn(mode: string) {
  vi.stubEnv('MODE', mode);
  vi.resetModules();
  return import('./showcase');
}

afterEach(() => {
  vi.unstubAllEnvs();
  vi.resetModules();
});

describe('the showcase gate', () => {
  it('opens in a development build: the entry shows and the page loads', async () => {
    const { SHOWCASE, loadShowcase } = await gateIn('development');
    expect(SHOWCASE).toBe(true);
    expect(await loadShowcase()).toBe(page.default);
  });

  it.each(['production', 'test', ''])('stays shut in mode %j: no entry, and the route loads no page', async (mode) => {
    const { SHOWCASE, loadShowcase } = await gateIn(mode);
    expect(SHOWCASE).toBe(false);
    expect(await loadShowcase()).toBeNull();
  });
});
