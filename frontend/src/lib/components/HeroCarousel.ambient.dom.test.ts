import { fireEvent, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { ambientState, claimAmbient, restAmbient } from '../stores/ambient';
import { renderDom } from '../testing/dom';
import HeroCarousel, { ADVANCE_MS, type HeroSlide } from './HeroCarousel.svelte';

// The real claims, counted: the backdrop must not be claimed again on every update of the page.
vi.mock('../stores/ambient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../stores/ambient')>();
  return { ...actual, claimAmbient: vi.fn(actual.claimAmbient), restAmbient: vi.fn(actual.restAmbient) };
});

const slide = (id: string, patch: Partial<HeroSlide> = {}): HeroSlide => ({ id, title: `Game ${id}`, art: {}, ...patch });
const region = () => screen.getByRole('region', { name: msg('library.carousel.label') });
const pickers = () => within(screen.getByRole('group', { name: msg('library.carousel.picker') })).getAllByRole('button');
const resting = () => get(ambientState).resting;
// A release is published after the task that made it.
const settle = async () => {
  await Promise.resolve();
  await tick();
};

function setup(ambient = true, slides = [slide('a'), slide('b'), slide('c')]) {
  vi.mocked(claimAmbient).mockClear();
  vi.mocked(restAmbient).mockClear();
  const props = { slides, onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn(), ambient };
  return { props, ...renderDom(HeroCarousel, props) };
}

// restAmbient(null) claims nothing (stores/ambient.ts): only the games claimed count.
const claims = () => vi.mocked(restAmbient).mock.calls.filter(([id]) => id !== null);

describe('HeroCarousel: the backdrop', () => {
  it('claims once per slide shown, not on every update of the page, and lets go when turned off', async () => {
    vi.useFakeTimers();
    const { props, rerender } = setup(true, [slide('a'), slide('b')]);
    expect(claims()).toEqual([['a']]);

    await rerender({ ...props, slides: [slide('a', { line: 'Played now' }), slide('b')] });
    await rerender({ ...props, slides: [slide('a', { launching: true }), slide('b')] });
    expect(claims()).toEqual([['a']]);
    expect(resting()).toBe('a');

    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(claims()).toEqual([['a'], ['b']]);
    expect(resting()).toBe('b');

    await rerender({ ...props, ambient: false });
    await settle();
    expect(resting()).toBeNull();
    expect(claims()).toHaveLength(2);
  });

  it('is the page\'s resting picture, never a screen claim: a rested card can override it', () => {
    setup();
    expect(resting()).toBe('a');
    expect(get(ambientState).claim).toBeNull();
    expect(vi.mocked(claimAmbient).mock.calls.filter(([id]) => id !== null)).toEqual([]);
  });

  it('follows a slide the player picks', async () => {
    const { user } = setup();
    await user.click(pickers()[2]);
    expect(resting()).toBe('c');
  });

  it('claims nothing without `ambient`', async () => {
    vi.useFakeTimers();
    setup(false);
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(claims()).toEqual([]);
    expect(resting()).toBeNull();
  });

  it('lets go once it is removed', async () => {
    const { unmount } = setup();
    expect(resting()).toBe('a');
    unmount();
    await settle();
    expect(resting()).toBeNull();
  });

  it('keeps the slide on screen while a hold freezes the list, and with it the window', async () => {
    vi.useFakeTimers();
    const { props, rerender } = setup();
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(resting()).toBe('b');
    await fireEvent.pointerEnter(region());

    await rerender({ ...props, slides: [slide('x'), slide('c'), slide('a')] });
    await settle();
    expect(resting()).toBe('b');
    expect(claims()).toEqual([['a'], ['b']]);

    // Let go, the new list takes over; the slide's game left it, so the slide at its place is shown.
    await fireEvent.pointerLeave(region());
    await settle();
    expect(resting()).toBe('c');
  });
});
