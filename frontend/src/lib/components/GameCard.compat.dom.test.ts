import { Heart } from '@lucide/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CompatInfo } from '../services/sources';
import { renderDom } from '../testing/dom';
import { applyStyles } from '../testing/styles';
import GameCard from './GameCard.svelte';
import source from './GameCard.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// A file of its own: jsdom's :focus-visible keeps the last element it matched across the tests of a
// file, so a Tab in an earlier test would change what the first Tab here matches. jsdom lays nothing
// out and has no :hover: the verdict stepping aside under the pointer is Chromium's to show.
describe('GameCard: the compat verdict on the cover', () => {
  it('lets its compat verdict step aside only while Play shows over the same corner', async () => {
    const { user } = renderDom(GameCard, {
      id: 'g1',
      title: 'Hades',
      installed: true,
      onplay: vi.fn(),
      compat: { works: 3, total: 4 } satisfies CompatInfo,
      actions: [{ id: 'fav', label: 'Add to favourites', icon: Heart, onclick: vi.fn() }],
    });
    const verdict = document.querySelector('.compat') as HTMLElement;
    applyStyles(verdict, source);
    const opacity = () => getComputedStyle(verdict).opacity;

    expect(opacity(), 'idle').toBe('1');
    await user.tab();
    expect(document.activeElement?.getAttribute('aria-label')).toBe(msg('ui.play'));
    expect(opacity(), 'on Play').toBe('0');
    await user.tab();
    expect(opacity(), 'on a quick action').toBe('1');
    await user.tab();
    expect(opacity(), 'on the title').toBe('1');
  });

  it('steps aside for a running game, whose Play stays shown', () => {
    renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, running: true, onplay: vi.fn(), compat: { works: 3, total: 4 } satisfies CompatInfo });
    const verdict = document.querySelector('.compat') as HTMLElement;
    applyStyles(verdict, source);

    expect(getComputedStyle(verdict).opacity).toBe('0');
  });

  it('stays inside the card, whatever the length of its label', () => {
    renderDom(GameCard, { id: 'g1', title: 'Hades', compat: { works: 12, total: 40 } satisfies CompatInfo });
    const verdict = document.querySelector('.compat') as HTMLElement;
    applyStyles(verdict, source);
    const style = getComputedStyle(verdict);

    // jsdom resolves the rem; the bound is the card's width less the badge's inset.
    expect(style.maxWidth).toMatch(/^calc\(100% - [\d.]+(px|rem)\)$/);
    expect(style.overflow).toBe('hidden');
  });
});
