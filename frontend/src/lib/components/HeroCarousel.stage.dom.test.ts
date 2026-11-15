import { fireEvent, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { stageGame } from '../stores/stage';
import { renderDom } from '../testing/dom';
import HeroCarousel, { ADVANCE_MS, type HeroSlide } from './HeroCarousel.svelte';

const slide = (id: string, patch: Partial<HeroSlide> = {}): HeroSlide => ({ id, title: `Game ${id}`, art: {}, staged: true, ...patch });
const region = () => screen.getByRole('region', { name: msg('library.carousel.label') });
const pickers = () => within(screen.getByRole('group', { name: msg('library.carousel.picker') })).getAllByRole('button');
const staged = () => get(stageGame);
// A release is published after the task that made it.
const settle = async () => {
  await Promise.resolve();
  await tick();
};

function setup(patch: { slides?: HeroSlide[]; stage?: boolean; ambient?: boolean } = {}) {
  const props = { slides: [slide('a'), slide('b'), slide('c')], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn(), stage: true, ...patch };
  return { props, ...renderDom(HeroCarousel, props) };
}

/** The slides' own blocks: the words and, for a game with no wide art, its cover. */
const slideBlocks = () => [...region().querySelectorAll<HTMLElement>('[data-picture]')];
const blockOf = (title: string) => slideBlocks().find((block) => block.textContent?.includes(title)) as HTMLElement;

describe('HeroCarousel: the stage', () => {
  it('puts the game of the slide on screen on the stage by its id, moves it with the slide, and lets go once removed', async () => {
    vi.useFakeTimers();
    const { unmount } = setup();
    expect(staged()).toBe('a');

    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    await settle();
    expect(staged()).toBe('b');

    unmount();
    await settle();
    expect(staged()).toBeNull();
  });

  it('follows a slide the player picks', async () => {
    const { user } = setup();
    await user.click(pickers()[2]);
    await settle();
    expect(staged()).toBe('c');
  });

  it('keeps the slide on screen on the stage while a hold freezes the list', async () => {
    vi.useFakeTimers();
    const { props, rerender } = setup();
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    await fireEvent.pointerEnter(region());

    await rerender({ ...props, slides: [slide('x'), slide('c'), slide('a')] });
    await settle();
    expect(staged()).toBe('b');

    // Let go, the new list takes over; the slide's game left it, so the slide at its place is shown.
    await fireEvent.pointerLeave(region());
    await settle();
    expect(staged()).toBe('c');
  });

  it('claims nothing without `stage`, whatever else it feeds', async () => {
    vi.useFakeTimers();
    setup({ stage: false, ambient: true });
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    await settle();
    expect(staged()).toBeNull();
  });
});

describe('HeroCarousel: the block on the stage', () => {
  it('draws no picture of its own for a game the stage shows: its words alone', () => {
    setup({ slides: [slide('a', { art: { cover: 'a-cover.png' } }), slide('b')] });
    for (const block of slideBlocks()) {
      expect(block.dataset.picture).toBe('stage');
      expect(block.querySelector('img, svg.generated-art')).toBeNull();
    }
  });

  it('shows the cover of a game with no wide art in the block, as a poster', () => {
    setup({ slides: [slide('a', { staged: false, art: { cover: 'a-poster.png' } }), slide('b')] });
    const block = blockOf('Game a');
    expect(block.dataset.picture).toBe('cover');
    const img = block.querySelector('img') as HTMLImageElement;
    expect(img.getAttribute('src')).toBe('a-poster.png');
    expect(img.getAttribute('alt')).toBe('');
    expect((img.closest('.artwork') as HTMLElement).style.aspectRatio).toBe('3 / 4');
    expect(blockOf('Game b').querySelector('img')).toBeNull();
  });

  it("falls to the game's own cover, then to its generated art by the slide's seed", async () => {
    setup({ slides: [slide('a', { staged: false, artSeed: 'canon-a', art: { cover: 'meta.png', fallback: 'own.png' } })] });
    const img = () => blockOf('Game a').querySelector('img') as HTMLImageElement | null;

    await fireEvent.error(img() as HTMLImageElement);
    expect(img()?.getAttribute('src')).toBe('own.png');

    await fireEvent.error(img() as HTMLImageElement);
    expect(img()).toBeNull();
    expect(blockOf('Game a').querySelector('svg.generated-art')).not.toBeNull();
  });

  it('draws the generated art for a game with no cover at all', () => {
    setup({ slides: [slide('a', { staged: false })] });
    expect(blockOf('Game a').querySelector('svg.generated-art')).not.toBeNull();
  });

  it('stands its words on the stage plate, with the cover or without', () => {
    setup({ slides: [slide('a'), slide('b', { staged: false })] });
    for (const block of slideBlocks()) {
      const title = block.querySelector('.title') as HTMLElement;
      const plate = title.closest('.stage-copy') as HTMLElement;
      expect(plate, block.textContent ?? '').not.toBeNull();
      expect(plate.contains(block.querySelector('.cta'))).toBe(true);
      expect(plate.querySelector('.artwork')).toBeNull();
    }
  });
});
