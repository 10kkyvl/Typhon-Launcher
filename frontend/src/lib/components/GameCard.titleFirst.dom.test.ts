import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { focusableIn } from '../layers/focus';
import { renderDom } from '../testing/dom';
import GameCard from './GameCard.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

const names = (root: HTMLElement) => focusableIn(root).map((el) => el.getAttribute('aria-label') ?? el.textContent?.trim());

describe('GameCard: the Tab order of its controls', () => {
  it('is Play, then the title, unless the page asks otherwise', () => {
    const { container } = renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay: vi.fn() });

    expect(names(container)).toEqual([msg('ui.play'), 'Hades']);
  });

  it('starts at the title with `titleFirst`, Play after it', () => {
    const { container } = renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay: vi.fn(), titleFirst: true });

    expect(names(container)).toEqual(['Hades', msg('ui.play')]);
  });
});
