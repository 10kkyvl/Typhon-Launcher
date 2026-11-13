import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import GeneratedArt from './GeneratedArt.svelte';

afterEach(() => vi.unstubAllGlobals());

describe('GeneratedArt', () => {
  it('draws the same picture for the same game, and another for another game', () => {
    const first = renderDom(GeneratedArt, { seed: 'g-hades' }).container.innerHTML;
    const again = renderDom(GeneratedArt, { seed: 'g-hades' }).container.innerHTML;
    const other = renderDom(GeneratedArt, { seed: 'g-celeste' }).container.innerHTML;
    // Each drawing names its gradients by its own id; the shapes and colours are what must match.
    const shapes = (html: string) => html.replace(/id="[^"]*"|url\(#[^)]*\)/g, '');
    expect(shapes(again)).toBe(shapes(first));
    expect(shapes(other)).not.toBe(shapes(first));
  });

  it('is a decoration drawn in place: no request, no picture element, hidden from assistive technology', () => {
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    const { container } = renderDom(GeneratedArt, { seed: 'g-hades', shape: 'wide' });
    const svg = container.querySelector('svg') as SVGSVGElement;

    expect(svg.getAttribute('aria-hidden')).toBe('true');
    expect(svg.getAttribute('viewBox')).toMatch(/^0 0 \d+ \d+$/);
    expect(container.querySelector('img, image, canvas, foreignObject')).toBeNull();
    expect(container.innerHTML).not.toMatch(/https?:|data:|blob:/);
    expect(fetch).not.toHaveBeenCalled();
  });
});
