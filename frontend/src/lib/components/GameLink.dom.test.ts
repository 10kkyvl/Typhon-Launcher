import { createRawSnippet } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import { resetHistory, route } from '../stores/router';
import GameLink from './GameLink.svelte';

const cover = createRawSnippet(() => ({ render: () => '<img class="cover-art" alt="" src="data:," />' }));
const at = () => ({ name: get(route).name, params: get(route).params });

afterEach(() => resetHistory());

describe("a game's title as a link to its page", () => {
  it('is a button named by the title that opens the game page on click', async () => {
    const { getByRole, user } = renderDom(GameLink, { id: 'hades', name: 'Hades' });
    const link = getByRole('button', { name: 'Hades' });

    expect(link.textContent).toBe('Hades');
    await user.click(link);
    expect(at()).toEqual({ name: 'game', params: { id: 'hades' } });
  });

  it('opens it from the keyboard: Tab to it, Enter', async () => {
    const { getByRole, user } = renderDom(GameLink, { id: 'hades', name: 'Hades' });
    await user.tab();
    expect(document.activeElement).toBe(getByRole('button', { name: 'Hades' }));
    await user.keyboard('{Enter}');
    expect(at()).toEqual({ name: 'game', params: { id: 'hades' } });
  });

  it('is the launcher’s text link, in the colour of the text it stands in', () => {
    const { getByRole } = renderDom(GameLink, { id: 'hades', name: 'Hades', truncate: true });
    const link = getByRole('button', { name: 'Hades' });
    expect(link.classList.contains('text-link')).toBe(true);
    expect(link.classList.contains('plain')).toBe(true);
    expect(link.classList.contains('truncate')).toBe(true);
  });

  it('is plain text when the game has no page to open', () => {
    const { container, queryByRole } = renderDom(GameLink, { id: '', name: 'Hades' });
    expect(queryByRole('button')).toBeNull();
    expect(container.textContent?.trim()).toBe('Hades');
  });
});

describe("a game's cover as a link to its page", () => {
  it('is a button named by the title, holding the picture, that opens the page', async () => {
    const { getByRole, user, container } = renderDom(GameLink, { id: 'hades', name: 'Hades', children: cover });
    const link = getByRole('button', { name: 'Hades' });

    expect(link.querySelector('.cover-art')).not.toBeNull();
    expect(link.tabIndex).toBe(0);
    await user.click(link);
    expect(at()).toEqual({ name: 'game', params: { id: 'hades' } });
    expect(container.querySelectorAll('button')).toHaveLength(1);
  });

  it('beside a title that already links there, it is for the pointer only: out of the Tab order and hidden', async () => {
    const { container, queryByRole, user } = renderDom(GameLink, { id: 'hades', name: 'Hades', children: cover, repeat: true });
    const link = container.querySelector('button') as HTMLButtonElement;

    expect(queryByRole('button')).toBeNull();
    expect(link.tabIndex).toBe(-1);
    expect(link.getAttribute('aria-hidden')).toBe('true');
    await user.click(link);
    expect(at()).toEqual({ name: 'game', params: { id: 'hades' } });
  });

  it('is the picture alone when the game has no page to open', () => {
    const { container } = renderDom(GameLink, { id: null, name: 'Hades', children: cover });
    expect(container.querySelector('button')).toBeNull();
    expect(container.querySelector('.cover-art')).not.toBeNull();
  });
});
