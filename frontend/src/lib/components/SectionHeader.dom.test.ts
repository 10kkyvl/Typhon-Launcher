import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import { ruleBody, styleOf } from '../testing/css';
import SectionHeader from './SectionHeader.svelte';
import source from './SectionHeader.svelte?raw';

const clear = createRawSnippet(() => ({ render: () => '<button type="button">Clear</button>' }));

describe('a section heading', () => {
  it('is a level 2 heading named by its title and count, the way the downloads sections are read today', () => {
    const { getByRole } = renderDom(SectionHeader, { title: 'Queue', count: 3 });
    const heading = getByRole('heading', { level: 2, name: 'Queue 3' });
    expect(heading.querySelector('.count')?.textContent).toBe('3');
  });

  it('shows a count of zero, and no count when none is given', () => {
    expect(renderDom(SectionHeader, { title: 'Done', count: 0 }).getByRole('heading', { name: 'Done 0' })).toBeTruthy();
    const plain = renderDom(SectionHeader, { title: 'Installing' }).getByRole('heading', { name: 'Installing' });
    expect(plain.querySelector('.count')).toBeNull();
  });

  it('takes a lower level and the id a section is labelled by', () => {
    const { getByRole } = renderDom(SectionHeader, { title: 'Files', level: 3, id: 'files-title' });
    expect(getByRole('heading', { level: 3, name: 'Files' }).id).toBe('files-title');
  });

  it('gives its heading an id of its own when none is given', () => {
    const { getByRole } = renderDom(SectionHeader, { title: 'Queue' });
    expect(getByRole('heading', { name: 'Queue' }).id).not.toBe('');
  });

  it('puts the actions beside the heading, outside it', () => {
    const { getByRole } = renderDom(SectionHeader, { title: 'Completed', count: 4, actions: clear });
    const heading = getByRole('heading', { name: 'Completed 4' });
    const action = getByRole('button', { name: 'Clear' });
    expect(heading.contains(action)).toBe(false);
    expect(action.closest('.section-header')).toBe(heading.closest('.section-header'));
  });

  // The look of the heading routes/downloads/DownloadsSection.svelte draws today (2/int/5), which this one
  // is to replace: pinned here rather than read from that file, which goes when the screen is rebuilt.
  it('draws the heading and its count as the downloads sections do today', () => {
    const css = styleOf(source, 'SectionHeader.svelte');
    expect(ruleBody(css, '.title')).toEqual({
      display: 'flex',
      'align-items': 'baseline',
      gap: 'var(--space-2)',
      'font-size': 'var(--font-lg)',
    });
    expect(ruleBody(css, '.section-header')['margin-bottom']).toBe('var(--space-3)');
    expect(ruleBody(css, '.count')).toEqual({
      'font-size': 'var(--font-sm)',
      'font-weight': '500',
      color: 'var(--text-3)',
      'font-variant-numeric': 'tabular-nums',
    });
  });
});
