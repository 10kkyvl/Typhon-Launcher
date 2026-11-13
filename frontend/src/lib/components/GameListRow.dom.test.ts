import { screen, within } from '@testing-library/svelte';
import type { ComponentProps } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import GameListRow from './GameListRow.svelte';

type Props = ComponentProps<typeof GameListRow>;

function setup(patch: Partial<Props> = {}) {
  const props: Props = { id: 'g1', title: 'Hades', details: ['Played yesterday', '12 h'], size: '14.2 GB', onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn(), onmore: vi.fn(), ...patch };
  return { props, ...renderDom(GameListRow, props) };
}

const row = () => screen.getByRole('listitem');
const title = () => within(row()).getByRole('button', { name: 'Hades' });

describe('GameListRow: one game of a list', () => {
  it('is an item of a list whose main action opens the game, by key or by a press anywhere on the row', async () => {
    const { user, props } = setup();
    expect(row().tagName).toBe('LI');

    title().focus();
    await user.keyboard('{Enter}');
    await user.click(row().querySelector('.details') as HTMLElement);

    expect(props.onopen).toHaveBeenCalledTimes(2);
    expect(props.onopen).toHaveBeenNthCalledWith(1, 'g1');
    expect(props.onopen).toHaveBeenNthCalledWith(2, 'g1');
  });

  it('is reached by Tab: the title, then Play, then "more"; a press on those does not open the game too', async () => {
    const { user, props } = setup();
    const stops: string[] = [];
    for (let i = 0; i < 3; i += 1) {
      await user.tab();
      stops.push((document.activeElement as HTMLElement).getAttribute('aria-label') ?? document.activeElement?.textContent?.trim() ?? '');
    }
    expect(stops).toEqual(['Hades', msg('ui.play'), msg('library.row.more')]);

    await user.click(screen.getByRole('button', { name: msg('ui.play') }));
    expect(props.onplay).toHaveBeenCalledWith('g1');
    expect(props.onopen).not.toHaveBeenCalled();
  });

  it('shows the size once, outside the name of its main action', () => {
    setup();
    expect(row().textContent?.split('14.2 GB').length).toBe(2);
    expect(title().textContent?.trim()).toBe('Hades');
    expect(within(row()).getByText('Played yesterday')).toBeTruthy();
  });

  it('asks the page for the menu with the game id and the control to anchor it to', async () => {
    const { user, props } = setup();
    const more = screen.getByRole('button', { name: msg('library.row.more') });

    await user.click(more);

    expect(props.onmore).toHaveBeenCalledWith('g1', more);
    expect(props.onopen).not.toHaveBeenCalled();
  });
});

describe('GameListRow: play, stop and a launch in flight', () => {
  it('a running game says so in words and offers Stop', async () => {
    const { user, props } = setup({ running: true });

    expect(within(row()).getByText(msg('games.runningLabel'))).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('ui.play') })).toBeNull();
    await user.click(screen.getByRole('button', { name: msg('ui.stop') }));

    expect(props.onstop).toHaveBeenCalledWith('g1');
    expect(props.onplay).not.toHaveBeenCalled();
  });

  it('Play is busy while the launch is in flight: focused, not disabled, a second press sends nothing', async () => {
    const { user, props, rerender } = setup();
    const play = screen.getByRole('button', { name: msg('ui.play') });
    play.focus();
    await user.keyboard('{Enter}');
    expect(props.onplay).toHaveBeenCalledTimes(1);

    await rerender({ ...props, launching: true });

    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(play.hasAttribute('disabled')).toBe(false);
    expect(document.activeElement).toBe(play);
    await user.keyboard('{Enter}');
    await user.click(play);
    expect(props.onplay).toHaveBeenCalledTimes(1);

    await rerender({ ...props, launching: false, running: true });
    expect(screen.getByRole('button', { name: msg('ui.stop') })).toBe(play);
    expect(document.activeElement).toBe(play);
  });

  it('offers no Play where the page gives no way to launch', () => {
    setup({ onplay: undefined, onstop: undefined });
    expect(screen.queryByRole('button', { name: msg('ui.play') })).toBeNull();
    expect(screen.queryByRole('button', { name: msg('ui.stop') })).toBeNull();
  });

  it('shows a favourite mark the page switches on', () => {
    setup({ favorite: true });
    expect(within(row()).getByRole('img', { name: msg('library.row.favorite') })).toBeTruthy();
  });
});
