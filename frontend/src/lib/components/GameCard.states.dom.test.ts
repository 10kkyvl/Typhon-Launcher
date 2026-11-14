import { fireEvent, screen } from '@testing-library/svelte';
import { Heart } from '@lucide/svelte';
import type { ComponentProps } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { resetHistory, route } from '../stores/router';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import GameCard, { type GameCardAction } from './GameCard.svelte';

type Props = ComponentProps<typeof GameCard>;

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

afterEach(() => resetHistory());

const play = () => screen.getByRole('button', { name: msg('ui.play') });
const title = () => screen.getByRole('button', { name: 'Hades' });

describe('GameCard: a launch in flight', () => {
  it('shows Play busy and a second press, by pointer or key, sends nothing', async () => {
    const onplay = vi.fn();
    const { user } = renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay, launching: true });

    expect(play().getAttribute('aria-busy')).toBe('true');
    await user.click(play());
    play().focus();
    await user.keyboard('{Enter}');

    expect(onplay).not.toHaveBeenCalled();
  });

  it('keeps Play the focused element, never disabled, while the launch comes and goes', async () => {
    const onplay = vi.fn();
    const props: Props = { id: 'g1', title: 'Hades', installed: true, onplay };
    const { user, rerender } = renderDom(GameCard, props);
    const button = play();
    button.focus();
    await user.keyboard('{Enter}');
    expect(onplay).toHaveBeenCalledTimes(1);

    await rerender({ id: 'g1', title: 'Hades', installed: true, onplay, launching: true });
    expect(document.activeElement).toBe(button);
    expect(button.hasAttribute('disabled')).toBe(false);
    await user.keyboard('{Enter}');
    expect(onplay).toHaveBeenCalledTimes(1);

    await rerender({ id: 'g1', title: 'Hades', installed: true, onplay, launching: false, running: true });
    const stop = screen.getByRole('button', { name: msg('ui.stop') });
    expect(stop).toBe(button);
    expect(document.activeElement).toBe(button);
    expect(button.hasAttribute('aria-busy')).toBe(false);
  });
});

describe('GameCard: a quick action that cannot act', () => {
  it('stays focusable and focused, says it is unavailable and why, and ignores a press', async () => {
    const onclick = vi.fn();
    const action: GameCardAction = { id: 'fav', label: 'Add to favourites', icon: Heart, onclick };
    const props: Props = { id: 'g1', title: 'Hades', actions: [action] };
    const { user, rerender } = renderDom(GameCard, props);
    const button = screen.getByRole('button', { name: 'Add to favourites' });
    button.focus();

    await rerender({ id: 'g1', title: 'Hades', actions: [{ ...action, disabled: true, reason: 'The favourites are full' }] });
    expect(document.activeElement).toBe(button);
    expect(button.hasAttribute('disabled')).toBe(false);
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(description(button)).toContain('The favourites are full');

    await user.keyboard('{Enter}');
    await user.click(button);
    expect(onclick).not.toHaveBeenCalled();
  });
});

describe('GameCard: a quick action whose request runs', () => {
  it('says it is busy, not unavailable, keeps focus and ignores a press until the request answers', async () => {
    const onclick = vi.fn();
    const action: GameCardAction = { id: 'dismiss', label: 'Not interested', icon: Heart, onclick };
    const props: Props = { id: 'g1', title: 'Hades', actions: [action] };
    const { user, rerender } = renderDom(GameCard, props);
    const button = screen.getByRole('button', { name: 'Not interested' });
    button.focus();

    await rerender({ id: 'g1', title: 'Hades', actions: [{ ...action, busy: true }] });
    expect(document.activeElement).toBe(button);
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.hasAttribute('aria-disabled')).toBe(false);
    expect(button.hasAttribute('disabled')).toBe(false);
    await user.keyboard('{Enter}');
    await user.click(button);
    expect(onclick).not.toHaveBeenCalled();

    await rerender({ id: 'g1', title: 'Hades', actions: [action] });
    expect(button.hasAttribute('aria-busy')).toBe(false);
    await user.click(button);
    expect(onclick).toHaveBeenCalledOnce();
  });
});

describe('GameCard: keys on a focused card', () => {
  it('Enter opens the game, Ctrl+Enter launches it and opens nothing', async () => {
    const onplay = vi.fn();
    const { user } = renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay });
    title().focus();

    await user.keyboard('{Control>}{Enter}{/Control}');
    expect(onplay).toHaveBeenCalledTimes(1);
    expect(get(route).name).not.toBe('game');

    await user.keyboard('{Enter}');
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
    expect(onplay).toHaveBeenCalledTimes(1);
  });

  it('Ctrl+Enter does nothing while the launch is in flight, while the game runs, or with nothing to launch', async () => {
    const onplay = vi.fn();
    const first = renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay, launching: true });
    title().focus();
    await first.user.keyboard('{Control>}{Enter}{/Control}');
    first.unmount();

    const running = renderDom(GameCard, { id: 'g1', title: 'Hades', installed: true, onplay, running: true });
    title().focus();
    await running.user.keyboard('{Control>}{Enter}{/Control}');
    running.unmount();

    const idle = renderDom(GameCard, { id: 'g1', title: 'Hades' });
    title().focus();
    await idle.user.keyboard('{Control>}{Enter}{/Control}');

    expect(onplay).not.toHaveBeenCalled();
    expect(get(route).name).not.toBe('game');
  });

  // The catalogue passes `onplay` to every card it knows in the library, installed or not.
  it('Ctrl+Enter and Cmd+Enter launch nothing for a game that is not installed, though the page gave onplay', async () => {
    const onplay = vi.fn();
    const { user } = renderDom(GameCard, { id: 'g1', title: 'Hades', installed: false, onplay });
    title().focus();

    await user.keyboard('{Control>}{Enter}{/Control}');
    await user.keyboard('{Meta>}{Enter}{/Meta}');

    expect(onplay).not.toHaveBeenCalled();
  });
});

describe('GameCard: marks and pictures the page passes in', () => {
  it('shows a favourite mark only when the page switches it on, and says it', () => {
    const props: Props = { id: 'g1', title: 'Hades' };
    const { rerender } = renderDom(GameCard, props);
    expect(screen.queryByRole('img', { name: msg('library.row.favorite') })).toBeNull();

    return rerender({ id: 'g1', title: 'Hades', favorite: true }).then(() => {
      expect(screen.getByRole('img', { name: msg('library.row.favorite') })).toBeTruthy();
    });
  });

  it("falls back to the game's own cover when the metadata cover fails", async () => {
    const { container } = renderDom(GameCard, { id: 'g1', title: 'Hades', cover: 'meta.png', coverFallback: 'own.png' });
    await fireEvent.error(container.querySelector('img') as HTMLImageElement);
    expect(container.querySelector('img')?.getAttribute('src')).toBe('own.png');
  });
});
