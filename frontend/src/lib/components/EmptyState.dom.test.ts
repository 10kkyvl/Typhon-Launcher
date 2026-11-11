import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import EmptyState from './EmptyState.svelte';

const actions = createRawSnippet(() => ({ render: () => '<button type="button">Add a source</button>' }));

describe('EmptyState in a DOM', () => {
  it('on a page its title is a heading, level 3 unless the page says otherwise', () => {
    const plain = renderDom(EmptyState, { title: 'No downloads yet', description: 'Add one from a source.' });
    expect(plain.container.querySelector('h3')?.textContent).toBe('No downloads yet');
    expect(plain.container.querySelector('p')?.textContent).toBe('Add one from a source.');

    const top = renderDom(EmptyState, { title: 'Nothing found', level: 2 });
    expect(top.container.querySelector('h2')?.textContent).toBe('Nothing found');
    expect(top.container.querySelector('h3')).toBeNull();
  });

  it('inline, in a dialog or a card, it is a quiet note and adds no heading', () => {
    const { container } = renderDom(EmptyState, { title: 'No tags yet', description: 'Type one above.', size: 'inline' });
    const root = container.querySelector('.empty');

    expect(root?.classList.contains('inline')).toBe(true);
    expect(container.querySelector('h1, h2, h3, h4, h5, h6')).toBeNull();
    expect(root?.querySelector('.title')?.textContent).toBe('No tags yet');
    expect(root?.textContent).toContain('Type one above.');
  });

  it('inline, reads as a quiet note unless its title leads a description', () => {
    const quiet = renderDom(EmptyState, { title: 'No tags yet', size: 'inline' });
    expect(quiet.container.querySelector('.empty')?.classList.contains('lead')).toBe(false);

    const lead = renderDom(EmptyState, { title: 'No backups', description: 'One is made before each launch.', size: 'inline', lead: true });
    const root = lead.container.querySelector('.empty');
    expect(root?.classList.contains('lead')).toBe(true);
    expect(root?.querySelector('.title')?.textContent).toBe('No backups');
    expect(root?.querySelector('.description')?.textContent).toBe('One is made before each launch.');
  });

  it('is left-aligned unless centred, and keeps its actions', () => {
    const left = renderDom(EmptyState, { title: 'No sources' });
    expect(left.container.querySelector('.empty')?.classList.contains('center')).toBe(false);

    const centred = renderDom(EmptyState, { title: 'No backups', size: 'inline', align: 'center', actions });
    const root = centred.container.querySelector('.empty');
    expect(root?.classList.contains('center')).toBe(true);
    expect(root?.querySelector('.actions button')?.textContent).toBe('Add a source');
  });
});
