import { screen, within } from '@testing-library/svelte';
import { Heart } from '@lucide/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CompatInfo } from '../services/sources';
import { resetHistory, route } from '../stores/router';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import GameCard from './GameCard.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

afterEach(() => resetHistory());

describe('GameCard: a card that opens its game', () => {
  it('says the game once on its cover: the picture inside the named button adds no second name', () => {
    renderDom(GameCard, { id: 'g1', title: 'Hades', cover: 'cover.png' });
    const cover = screen.getAllByRole('button', { name: 'Hades' })[0];
    expect(within(cover).queryByRole('img')).toBeNull();
    expect(screen.queryByAltText('Hades')).toBeNull();
  });

  it('opens the game from its title with the keyboard', async () => {
    const { user } = renderDom(GameCard, { id: 'g1', title: 'Hades', meta: '12 h' });
    const info = screen.getByRole('button', { name: 'Hades 12 h' });
    info.focus();

    await user.keyboard('{Enter}');

    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
  });

  it('shows how the game starts for others in a status badge of its tone', () => {
    renderDom(GameCard, { id: 'g1', title: 'Hades', compat: { works: 1, total: 4 } satisfies CompatInfo });
    const label = msg('games.compatBadgeBroken', { works: '1', total: '4' });
    const badge = screen.getByText(label);
    expect(badge.classList.contains('badge')).toBe(true);
    expect(badge.classList.contains('danger')).toBe(true);
    expect(badge.closest('[title]')).toBeNull();
  });

  it('is one Tab stop that opens the game, beside its play and quick actions, each tied to the game', async () => {
    const compat = msg('games.compatBadgeWorks', { works: '3', total: '4' });
    const { user } = renderDom(GameCard, {
      id: 'g1',
      title: 'Hades',
      meta: '12 h',
      installed: true,
      onplay: vi.fn(),
      compat: { works: 3, total: 4 } satisfies CompatInfo,
      actions: [{ id: 'fav', label: 'Add to favourites', icon: Heart, onclick: vi.fn() }],
    });

    const stops: [string, string][] = [];
    for (let step = 0; step < 3; step += 1) {
      await user.tab();
      const at = document.activeElement as HTMLElement;
      stops.push([at.getAttribute('aria-label') ?? at.textContent?.replace(/\s+/g, ' ').trim() ?? '', description(at)]);
    }
    await user.tab();

    expect(stops).toEqual([
      [msg('ui.play'), 'Hades'],
      ['Add to favourites', 'Hades'],
      ['Hades 12 h', compat],
    ]);
    expect(document.activeElement).toBe(document.body);
    expect(screen.getAllByRole('button', { name: 'Hades' })[0].tabIndex).toBe(-1);
  });

  it('says which game its play button starts', () => {
    renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay: vi.fn() });
    const play = screen.getByRole('button', { name: msg('ui.play') });
    expect(description(play)).toBe('Hades');
  });
});
