import { fireEvent } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import HeroCarousel, { type HeroSlide } from './HeroCarousel.svelte';

function render(art: HeroSlide['art']) {
  const { container } = renderDom(HeroCarousel, { slides: [{ id: 'g1', title: 'Hades', art }], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn() });
  const slide = () => container.querySelector('[data-picture]') as HTMLElement;
  return {
    kind: () => slide().dataset.picture,
    wide: () => slide().querySelector('img.wide') as HTMLImageElement | null,
    poster: () => slide().querySelector('img.poster') as HTMLImageElement | null,
    generated: () => slide().querySelector('svg.generated-art'),
  };
}

describe('HeroCarousel: the picture of a slide', () => {
  it('draws the hero as a wide picture', () => {
    const view = render({ hero: 'hero.png', cover: 'cover.png' });
    expect(view.kind()).toBe('wide');
    expect(view.wide()?.getAttribute('src')).toBe('hero.png');
    expect(view.poster()).toBeNull();
  });

  it('draws a hero that is a poster as a poster, never stretched across', () => {
    const view = render({ hero: 'poster.png', heroIsPoster: true });
    expect(view.kind()).toBe('poster');
    expect(view.poster()?.getAttribute('src')).toBe('poster.png');
    expect(view.wide()).toBeNull();
  });

  it('draws the generated art of the game when there is no picture', () => {
    const view = render({});
    expect(view.kind()).toBe('generated');
    expect(view.generated()).not.toBeNull();
    expect(view.wide()).toBeNull();
    expect(view.poster()).toBeNull();
  });

  it('falls to the next picture when one fails to load: the hero, the cover as a poster, the generated art', async () => {
    const view = render({ hero: 'hero.png', cover: 'cover.png' });

    await fireEvent.error(view.wide() as HTMLImageElement);
    expect(view.kind()).toBe('poster');
    expect(view.poster()?.getAttribute('src')).toBe('cover.png');

    await fireEvent.error(view.poster() as HTMLImageElement);
    expect(view.kind()).toBe('generated');
    expect(view.generated()).not.toBeNull();
  });

  it('keeps every picture out of the accessibility tree: the title names the slide', () => {
    const view = render({ hero: 'hero.png' });
    expect(view.wide()?.getAttribute('alt')).toBe('');
  });
});
