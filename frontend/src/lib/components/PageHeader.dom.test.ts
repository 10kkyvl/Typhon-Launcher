import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import PageHeader from './PageHeader.svelte';
import source from './PageHeader.svelte?raw';

const actions = createRawSnippet(() => ({ render: () => '<button type="button">Pause all</button>' }));

describe('PageHeader in a DOM', () => {
  it('shows the title as the page heading, a focus target for script only', () => {
    const { getByRole } = renderDom(PageHeader, { title: 'Downloads' });
    const heading = getByRole('heading', { level: 1, name: 'Downloads' });

    expect(heading.getAttribute('tabindex')).toBe('-1');
  });

  it('keeps the title in the same block with and without a subtitle or actions', () => {
    const bare = renderDom(PageHeader, { title: 'Downloads' });
    const full = renderDom(PageHeader, { title: 'Sources', subtitle: 'Where releases come from', actions });

    for (const view of [bare, full]) {
      const heading = view.container.querySelector('h1');
      expect(heading?.parentElement?.className).toContain('heading');
      expect(heading?.parentElement?.parentElement?.className).toContain('text');
    }
    expect(full.container.querySelector('.actions button')?.textContent).toBe('Pause all');
    expect(bare.container.querySelector('.actions')).toBeNull();
  });
});

// jsdom lays nothing out: where the title sits is held on the stylesheet. The row is bottom-aligned
// for the action buttons; the text block must opt out, or a header whose buttons are taller than its
// title (actions, no subtitle) pushes the title down.
describe('PageHeader title height', () => {
  const css = source.slice(source.indexOf('<style>'));
  const rule = (selector: string) => new RegExp(`\\${selector}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';

  it('pins the text block to the top of the header row', () => {
    expect(rule('.text')).toMatch(/align-self:\s*flex-start/);
  });
});
