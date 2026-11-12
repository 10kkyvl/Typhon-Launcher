import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import StoreLinks from './StoreLinks.svelte';

describe('StoreLinks: a store page', () => {
  it('is a chip that is a real link to the page, opened by the launcher, its external mark out of the name', async () => {
    const onopen = vi.fn();
    const { user } = renderDom(StoreLinks, { links: { steam: 'https://store.steampowered.com/app/1145360' }, onopen });
    const link = screen.getByRole('link', { name: msg('games.storeSteam') });
    expect(link.classList.contains('chip')).toBe(true);
    expect(link.getAttribute('href')).toBe('https://store.steampowered.com/app/1145360');
    expect(link.querySelector('.end')?.getAttribute('aria-hidden')).toBe('true');
    let followed = true;
    document.addEventListener('click', (event) => (followed = !event.defaultPrevented), { once: true });
    link.focus();

    await user.keyboard('{Enter}');

    expect(onopen).toHaveBeenCalledWith('steam');
    expect(followed).toBe(false);
  });

  it('opens through the launcher on a middle click too, never in a window of its own', () => {
    const onopen = vi.fn();
    renderDom(StoreLinks, { links: { steam: 'https://store.steampowered.com/app/1145360' }, onopen });
    const link = screen.getByRole('link', { name: msg('games.storeSteam') });

    const middle = new MouseEvent('auxclick', { button: 1, bubbles: true, cancelable: true });
    link.dispatchEvent(middle);

    expect(middle.defaultPrevented).toBe(true);
    expect(onopen).toHaveBeenCalledWith('steam');
  });

  it('is a chip button, not a link, when the address is not a web page', async () => {
    const onopen = vi.fn();
    const { user } = renderDom(StoreLinks, { links: { gog: 'javascript:alert(1)' }, onopen });
    expect(screen.queryByRole('link')).toBeNull();
    const chip = screen.getByRole('button', { name: msg('games.storeGog') });
    expect(chip.classList.contains('chip')).toBe(true);
    chip.focus();

    await user.keyboard('{Enter}');

    expect(onopen).toHaveBeenCalledWith('gog');
  });
});
