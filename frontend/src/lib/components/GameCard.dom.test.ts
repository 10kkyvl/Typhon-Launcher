import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
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

  it('says which game its play button starts', () => {
    renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay: vi.fn() });
    const play = screen.getByRole('button', { name: msg('ui.play') });
    expect(description(play)).toBe('Hades');
  });
});
