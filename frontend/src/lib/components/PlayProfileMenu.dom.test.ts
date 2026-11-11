import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import PlayProfileMenu from './PlayProfileMenu.svelte';

vi.mock('../services/backend', () => ({ inWails: false }));

const game = { profiles: [{ id: 'safe', name: 'Safe mode' }], lastProfile: 'safe', emulation: undefined } as never;

describe('PlayProfileMenu in a DOM', () => {
  it('lists the launch profiles as one choice with the last one checked, and runs the picked one', async () => {
    const onpick = vi.fn();
    const view = renderDom(PlayProfileMenu, { game, onpick });
    const chevron = view.getByRole('button', { name: msg('games.playProfileChoose') });

    await view.user.click(chevron);

    expect(view.getByRole('menu', { name: msg('games.playProfileChoose') })).toBeTruthy();
    const choices = view.getAllByRole('menuitemradio');
    expect(choices.map((el) => [el.textContent?.trim(), el.getAttribute('aria-checked')])).toEqual([
      [msg('games.profileDefault'), 'false'],
      ['Safe mode', 'true'],
    ]);
    await view.user.click(choices[0]);
    expect(onpick).toHaveBeenCalledExactlyOnceWith('', expect.objectContaining({ label: msg('games.profileDefault') }));
    expect(view.queryByRole('menu')).toBeNull();
    expect(document.activeElement).toBe(chevron);
  });

  it('a disabled chevron (the game cannot start now) opens nothing', async () => {
    const view = renderDom(PlayProfileMenu, { game, onpick: vi.fn(), disabled: true });

    await view.user.click(view.getByRole('button', { name: msg('games.playProfileChoose') }));

    expect(view.queryByRole('menu')).toBeNull();
  });
});
