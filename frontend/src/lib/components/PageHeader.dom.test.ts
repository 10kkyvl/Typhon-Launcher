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
  // text block opts out and starts at the top of the row. Where the title then lands, in pixels, and
  // how the row wraps on a narrow window, only a browser shows.
  it('starts the title at the top of the row whatever stands beside it', () => {
    for (const props of [{ title: 'Downloads', actions }, { title: 'Sources', subtitle: 'Where releases come from', actions }, { title: 'Library' }]) {
      const { container } = renderDom(PageHeader, props);
      const header = container.querySelector('header') as HTMLElement;
      applyStyles(header, source);
      const text = header.querySelector('.text') as HTMLElement;

      expect(text.firstElementChild?.querySelector('h1')?.textContent, props.title).toBe(props.title);
      expect(getComputedStyle(header).alignItems, props.title).toBe('flex-end');
      expect(getComputedStyle(text).alignSelf, props.title).toBe('flex-start');
    }
  });
});
