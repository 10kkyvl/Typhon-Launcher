import { fireEvent, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import Artwork from './Artwork.svelte';

const picture = (root: ParentNode) => root.querySelector('img');
const generated = (root: ParentNode) => root.querySelector('svg.generated-art');

describe('Artwork: what shows when a picture is missing or fails', () => {
  it('draws the generated art of the game, with its letters on top, when there is no picture', () => {
    const { container } = renderDom(Artwork, { src: '', label: 'Hollow Knight', seed: 'g-hk' });

    expect(picture(container)).toBeNull();
    expect(generated(container)).not.toBeNull();
    expect(container.textContent?.trim()).toBe('HK');
  });

  it('tries the fallback picture before the generated art when the cover fails to load', async () => {
    const { container } = renderDom(Artwork, { src: 'meta-cover.png', fallback: 'game-cover.png', label: 'Hades', seed: 'g1' });
    expect(picture(container)?.getAttribute('src')).toBe('meta-cover.png');

    await fireEvent.error(picture(container) as HTMLImageElement);
    expect(picture(container)?.getAttribute('src')).toBe('game-cover.png');
    expect(generated(container)).toBeNull();

    await fireEvent.error(picture(container) as HTMLImageElement);
    expect(picture(container)).toBeNull();
    expect(generated(container)).not.toBeNull();
  });

  it('goes straight to the fallback picture when there is no cover, and skips a fallback equal to the failed cover', async () => {
    const first = renderDom(Artwork, { src: '', fallback: 'game-cover.png', label: 'Hades' });
    expect(picture(first.container)?.getAttribute('src')).toBe('game-cover.png');

    const same = renderDom(Artwork, { src: 'cover.png', fallback: 'cover.png', label: 'Hades', seed: 'g1' });
    await fireEvent.error(picture(same.container) as HTMLImageElement);
    expect(picture(same.container)).toBeNull();
    expect(generated(same.container)).not.toBeNull();
  });

  it('tries a new cover again after an earlier one failed', async () => {
    const { container, rerender } = renderDom(Artwork, { src: 'old.png', label: 'Hades' });
    await fireEvent.error(picture(container) as HTMLImageElement);
    expect(picture(container)).toBeNull();

    await rerender({ src: 'new.png', label: 'Hades' });

    expect(picture(container)?.getAttribute('src')).toBe('new.png');
  });

  it('draws the same art for the same game, seeded by its id over its name', () => {
    const shapes = (html: string) => html.replace(/id="[^"]*"|url\(#[^)]*\)/g, '');
    const a = renderDom(Artwork, { src: '', label: 'Same name', seed: 'g-1' }).container.innerHTML;
    const b = renderDom(Artwork, { src: '', label: 'Same name', seed: 'g-1' }).container.innerHTML;
    const c = renderDom(Artwork, { src: '', label: 'Same name', seed: 'g-2' }).container.innerHTML;
    expect(shapes(b)).toBe(shapes(a));
    expect(shapes(c)).not.toBe(shapes(a));
  });

  it('keeps its text alternative over the generated art: an image named alt, or nothing', () => {
    renderDom(Artwork, { src: '', alt: 'Celeste', seed: 'g-celeste' });
    expect(screen.getByRole('img', { name: 'Celeste' })).toBeTruthy();
  });
});

// Generated art stands for a game, and only a caller that knows which game gives the seed: a search
// hit or a screenshot that fails keeps the plain tile it always had.
describe('Artwork without a seed', () => {
  it('draws no generated art: the letters on a plain tile, or "?" with no name at all', () => {
    const named = renderDom(Artwork, { src: '', label: 'Hollow Knight' });
    expect(generated(named.container)).toBeNull();
    expect(named.container.textContent?.trim()).toBe('HK');

    const bare = renderDom(Artwork, { src: '', alt: '' });
    expect(generated(bare.container)).toBeNull();
    expect(bare.container.textContent?.trim()).toBe('?');
  });

  it('ends a failed cover and fallback on the plain tile, not on a landscape', async () => {
    const { container } = renderDom(Artwork, { src: 'shot.png', fallback: 'other.png', alt: '' });
    await fireEvent.error(picture(container) as HTMLImageElement);
    await fireEvent.error(picture(container) as HTMLImageElement);
    expect(picture(container)).toBeNull();
    expect(generated(container)).toBeNull();
    expect(container.textContent?.trim()).toBe('?');
  });

  it('treats an empty seed as none', () => {
    const { container } = renderDom(Artwork, { src: '', label: 'Hades', seed: '' });
    expect(generated(container)).toBeNull();
  });
});
