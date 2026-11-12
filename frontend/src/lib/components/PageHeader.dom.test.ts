import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import { applyStyles } from '../testing/styles';
import PageHeader from './PageHeader.svelte';
import source from './PageHeader.svelte?raw';

const actions = createRawSnippet(() => ({ render: () => '<button type="button">Pause all</button>' }));

describe('PageHeader in a DOM', () => {
  it('shows the title as the page heading, a focus target for script only', () => {
    const { getByRole } = renderDom(PageHeader, { title: 'Downloads' });
    const heading = getByRole('heading', { level: 1, name: 'Downloads' });

    expect(heading.getAttribute('tabindex')).toBe('-1');
  });

  // The row is bottom-aligned so the buttons sit on the subtitle's line; a header whose buttons are
  // taller than its title (actions, no subtitle: Downloads) would push the title down with them. The
  // text block opts out and starts at the top of the row. This is a tripwire over the declarations that
  // can move the title inside the header; where the title lands in pixels, and how the row wraps on a
  // narrow window, only a browser shows.
  it('tripwire: nothing in the header moves the title off the top of the row, whatever stands beside it', () => {
    const none = (value: string, ...still: string[]) => ['', ...still].includes(value);
    for (const props of [{ title: 'Downloads', actions }, { title: 'Sources', subtitle: 'Where releases come from', actions }, { title: 'Library' }]) {
      const { container } = renderDom(PageHeader, props);
      const header = container.querySelector('header') as HTMLElement;
      applyStyles(header, source);
      const text = header.querySelector('.text') as HTMLElement;
      const heading = text.firstElementChild as HTMLElement;
      const h1 = heading.querySelector('h1') as HTMLElement;
      const row = getComputedStyle(header);

      expect(header.firstElementChild, props.title).toBe(text);
      expect(h1.textContent, props.title).toBe(props.title);
      expect(row.alignItems, props.title).toBe('flex-end');
      expect(none(row.flexDirection, 'row'), `${props.title}: direction ${row.flexDirection}`).toBe(true);
      expect(row.flexWrap, props.title).toBe('wrap');
      expect(getComputedStyle(text).alignSelf, props.title).toBe('flex-start');
      expect(none(getComputedStyle(text).order, '0'), `${props.title}: order`).toBe(true);
      for (const element of [text, heading, h1]) {
        const style = getComputedStyle(element);
        const where = `${props.title} ${element.tagName.toLowerCase()}.${element.className.split(' ')[0]}`;
        expect(none(style.marginTop, '0', '0px'), `${where} margin-top ${style.marginTop}`).toBe(true);
        expect(none(style.paddingTop, '0', '0px'), `${where} padding-top ${style.paddingTop}`).toBe(true);
        expect(none(style.position, 'static'), `${where} position ${style.position}`).toBe(true);
        expect(none(style.top, 'auto'), `${where} top ${style.top}`).toBe(true);
        expect(none(style.transform, 'none'), `${where} transform ${style.transform}`).toBe(true);
      }
    }
  });
});
