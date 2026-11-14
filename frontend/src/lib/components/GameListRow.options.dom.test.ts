import { fireEvent, screen, within } from '@testing-library/svelte';
import { BellOff } from '@lucide/svelte';
import { get } from 'svelte/store';
import type { ComponentProps } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { ambientState, HOVER_REST_MS } from '../stores/ambient';
import { renderDom } from '../testing/dom';
import GameListRow from './GameListRow.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

type Props = ComponentProps<typeof GameListRow>;

function setup(patch: Partial<Props> = {}) {
  const props: Props = { id: 'g1', title: 'Hades', onopen: vi.fn(), ...patch };
  return { props, ...renderDom(GameListRow, props) };
}

const row = () => screen.getByRole('listitem');
const title = () => within(row()).getByRole('button', { name: 'Hades' });

afterEach(() => {
  vi.useRealTimers();
  document.body.replaceChildren();
});

describe('GameListRow: what the page may ask of a row', () => {
  it('draws the title in the muted tone unless the page asks for the plain one', () => {
    setup();
    expect(title().classList.contains('muted')).toBe(true);
    document.body.replaceChildren();

    setup({ titleTone: 'plain' });
    expect(title().classList.contains('plain')).toBe(true);
    expect(title().classList.contains('muted')).toBe(false);
  });

  it('lets the window backdrop follow its game under the pointer when `ambient`, and only then', async () => {
    vi.useFakeTimers();
    setup();
    await fireEvent.pointerEnter(row(), { pointerType: 'mouse' });
    vi.advanceTimersByTime(HOVER_REST_MS);
    expect(get(ambientState).hover).not.toBe('g1');
    await fireEvent.pointerLeave(row());
    document.body.replaceChildren();

    setup({ ambient: true });
    await fireEvent.pointerEnter(row(), { pointerType: 'mouse' });
    vi.advanceTimersByTime(HOVER_REST_MS);
    expect(get(ambientState).hover).toBe('g1');
    await fireEvent.pointerLeave(row());
  });

  it('shows the page\'s own actions after Play, each busy while its request runs', async () => {
    const unwatch = vi.fn();
    const { user, props, rerender } = setup({ actions: [{ id: 'unwatch', label: msg('games.unwatchAction'), icon: BellOff, onclick: unwatch }] });
    const action = within(row()).getByRole('button', { name: msg('games.unwatchAction') });
    action.focus();

    await user.keyboard('{Enter}');
    expect(unwatch).toHaveBeenCalledTimes(1);
    expect(props.onopen).not.toHaveBeenCalled();

    await rerender({ ...props, actions: [{ id: 'unwatch', label: msg('games.unwatchAction'), icon: BellOff, busy: true, onclick: unwatch }] });
    expect(action.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(action);
    await user.keyboard('{Enter}');
    expect(unwatch).toHaveBeenCalledTimes(1);
  });
});
