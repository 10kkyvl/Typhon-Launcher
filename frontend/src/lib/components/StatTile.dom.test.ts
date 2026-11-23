import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import { applyStyles } from '../testing/styles';
import StatTile from './StatTile.svelte';
import source from './StatTile.svelte?raw';

const snippet = (html: string) => createRawSnippet(() => ({ render: () => html }));

/** The markup without Svelte's scoping classes and anchors: what a change of the tile would alter. */
const shape = (root: Element) =>
  root.innerHTML
    .replace(/ svelte-[a-z0-9]+/g, '')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/ class=""/g, '')
    .replace(/>\s+</g, '><');

describe('StatTile', () => {
  it('draws a value and its label, with an icon over them, as before', () => {
    const { container } = renderDom(StatTile, { value: '128 h', label: 'Played', icon: snippet('<svg></svg>') });
    expect(shape(container)).toBe(
      '<div class="tile"><span class="icon"><svg></svg></span><span class="value">128 h</span><span class="label">Played</span></div>',
    );
  });

  it('draws a number value and its label without an icon, as before', () => {
    const { container } = renderDom(StatTile, { value: 42, label: 'Games' });
    expect(shape(container)).toBe('<div class="tile"><span class="value">42</span><span class="label">Games</span></div>');
  });

  it('shows a mark where the value stands, and no value', () => {
    const { container, getByText } = renderDom(StatTile, { label: 'Hours', mark: snippet('<span class="probe">Hidden</span>') });

    const tile = container.querySelector('.tile') as HTMLElement;
    expect(tile.querySelector('.value')).toBeNull();
    const mark = getByText('Hidden');
    expect(mark.compareDocumentPosition(getByText('Hours')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(tile.contains(mark)).toBe(true);
  });

  it('draws the same markup with its parts at the start, named or left out', () => {
    const { container } = renderDom(StatTile, { value: 42, label: 'Games', align: 'start' });
    expect(shape(container)).toBe('<div class="tile"><span class="value">42</span><span class="label">Games</span></div>');
  });

  it('lines its parts up at the start unless asked, and centres them and their text when asked', () => {
    const start = renderDom(StatTile, { value: 42, label: 'Games' });
    const centre = renderDom(StatTile, { value: 7, label: 'Completed games of the year', align: 'center' });
    const plain = start.container.querySelector('.tile') as HTMLElement;
    const centred = centre.container.querySelector('.tile') as HTMLElement;
    applyStyles(plain, source);

    expect([getComputedStyle(plain).alignItems, getComputedStyle(plain).textAlign]).toEqual(['flex-start', 'start']);
    expect([getComputedStyle(centred).alignItems, getComputedStyle(centred).textAlign]).toEqual(['center', 'center']);
  });

  it('adds a second line under the label', () => {
    const { container } = renderDom(StatTile, { value: '1 h 10 min', label: 'Longest session', sub: 'Hades · 10 Nov' });

    const parts = [...(container.querySelector('.tile') as HTMLElement).children].map((child) => child.textContent?.trim());
    expect(parts).toEqual(['1 h 10 min', 'Longest session', 'Hades · 10 Nov']);
  });
});
