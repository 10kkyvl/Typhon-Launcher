import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import GameCard from './GameCard.svelte';
import GameListRow from './GameListRow.svelte';
import HeroCarousel from './HeroCarousel.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

/** What a generated picture shows of its game: the sky's colours and the lands' shades, whatever its shape. */
function scene(root: ParentNode): string[] {
  const svg = root.querySelector('svg.generated-art');
  expect(svg).not.toBeNull();
  const stops = [...(svg as Element).querySelectorAll('linearGradient stop')].map((stop) => stop.getAttribute('stop-color') ?? '');
  const lands = [...(svg as Element).querySelectorAll(':scope > path[fill]:not([fill="currentColor"])')].map((path) => path.getAttribute('fill') ?? '');
  return [...stops, ...lands];
}

const shapes = (html: string) => html.replace(/id="[^"]*"|url\(#[^)]*\)/g, '');

// The page passes the game's canonical id as the seed, so the grid, the list and the carousel agree
// on one picture per game whatever id each of them holds.
describe('one generated picture per game across the card, the row and the carousel', () => {
  it('draws the same scene for the same seed, though each part holds another id', () => {
    const card = renderDom(GameCard, { id: 'lib-1', title: 'Hades', artSeed: 'canon-hades' }).container;
    const row = renderDom(GameListRow, { id: 'dl-7', title: 'Hades', artSeed: 'canon-hades', onopen: vi.fn() }).container;
    const hero = renderDom(HeroCarousel, { slides: [{ id: 'lib-1', title: 'Hades', art: {}, artSeed: 'canon-hades' }], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn() }).container;

    expect(scene(row)).toEqual(scene(card));
    expect(scene(hero)).toEqual(scene(card));
    // Card and row draw the same cover shape: the very same picture.
    expect(shapes((row.querySelector('svg.generated-art') as Element).outerHTML)).toBe(shapes((card.querySelector('svg.generated-art') as Element).outerHTML));
  });

  it('seeds by the id each part already holds when the page gives no seed', () => {
    const card = renderDom(GameCard, { id: 'g-1', title: 'Hades' }).container;
    const other = renderDom(GameCard, { id: 'g-2', title: 'Hades' }).container;
    const row = renderDom(GameListRow, { id: 'g-1', title: 'Hades', onopen: vi.fn() }).container;
    const hero = renderDom(HeroCarousel, { slides: [{ id: 'g-1', title: 'Hades', art: {} }], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn() }).container;

    expect(scene(row)).toEqual(scene(card));
    expect(scene(hero)).toEqual(scene(card));
    expect(scene(other)).not.toEqual(scene(card));
  });
});
