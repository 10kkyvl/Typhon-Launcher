import { get } from 'svelte/store';
import { describe, expect, it, vi } from 'vitest';
import { ambientState, claimAmbient } from '../stores/ambient';
import { renderDom } from '../testing/dom';
import HeroCarousel, { ADVANCE_MS, type HeroSlide } from './HeroCarousel.svelte';

// The real claim, counted: the backdrop must not be claimed again on every update of the page.
vi.mock('../stores/ambient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../stores/ambient')>();
  return { ...actual, claimAmbient: vi.fn(actual.claimAmbient) };
});

const slide = (id: string, patch: Partial<HeroSlide> = {}): HeroSlide => ({ id, title: `Game ${id}`, art: {}, ...patch });

describe('HeroCarousel: the backdrop claim', () => {
  it('claims once per slide shown, not on every update of the page, and lets go when turned off', async () => {
    vi.useFakeTimers();
    const claim = vi.mocked(claimAmbient);
    claim.mockClear();
    // claimAmbient(null) claims nothing (stores/ambient.ts): only the games claimed count.
    const claims = () => claim.mock.calls.filter(([id]) => id !== null);
    const props = { slides: [slide('a'), slide('b')], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn(), ambient: true };
    const { rerender } = renderDom(HeroCarousel, props);
    expect(claims()).toEqual([['a']]);

    await rerender({ ...props, slides: [slide('a', { line: 'Played now' }), slide('b')] });
    await rerender({ ...props, slides: [slide('a', { launching: true }), slide('b')] });
    expect(claims()).toEqual([['a']]);
    expect(get(ambientState).claim).toBe('a');

    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(claims()).toEqual([['a'], ['b']]);
    expect(get(ambientState).claim).toBe('b');

    await rerender({ ...props, ambient: false });
    expect(get(ambientState).claim).toBeNull();
    expect(claims()).toHaveLength(2);
  });
});
