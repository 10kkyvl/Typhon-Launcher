import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: false }));

import { locale } from '../../i18n';
import type { FeedEvent } from '../../services/social';
import { user } from '../../testing/dom';
import FeedRow from '../FeedRow.svelte';

// A feed row of a friend's event with one reaction already on it (fire, the user's own): the "+" offers
// the seven others.
const event = {
  id: 7,
  user: { id: 'u2', username: 'neo', displayName: 'Neo', avatarUrl: '' },
  kind: 'completed',
  game: { igdbId: 1, title: 'Hades', coverUrl: '', artworkUrl: '' },
  createdAt: new Date().toISOString(),
  seconds: 0,
  achievements: [],
  review: null,
  reactions: [{ emoji: 'fire', count: 2 }],
  mine: ['fire'],
  note: '',
} as unknown as FeedEvent;

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

function setup() {
  const onreact = vi.fn();
  render(FeedRow, { target: app, props: { event, onreact } });
  const keys = user();
  const add = () => screen.getByRole('button', { name: 'Add a reaction' });
  const picker = () => screen.queryByRole('dialog', { name: 'Add a reaction' });
  return { keys, onreact, add, picker };
}

describe('feed reaction picker', () => {
  it('the "+" opens a picker of the reactions not on the event yet, each a button with its name', async () => {
    const { keys, add, picker } = setup();
    expect(add().getAttribute('aria-haspopup')).toBe('dialog');
    expect(add().getAttribute('aria-expanded')).toBe('false');

    await keys.click(add());

    const opened = picker() as HTMLElement;
    expect(opened).not.toBeNull();
    expect(add().getAttribute('aria-expanded')).toBe('true');
    expect(add().getAttribute('aria-controls')).toBe(opened.id);
    expect(app.contains(opened)).toBe(false);
    expect(within(opened).getAllByRole('button').map((button) => button.getAttribute('aria-label'))).toEqual([
      'Salute',
      'Heart',
      'Applause',
      'Skull',
      'Party',
      'Eyes',
      'Laugh',
    ]);
  });

  it('picking a reaction adds it, closes the picker and puts focus back on the "+"', async () => {
    const { keys, onreact, add, picker } = setup();
    await keys.click(add());

    await keys.click(within(picker() as HTMLElement).getByRole('button', { name: 'Party' }));

    expect(onreact).toHaveBeenCalledExactlyOnceWith('party');
    expect(picker()).toBeNull();
    expect(document.activeElement).toBe(add());
  });

  it('Enter on the "+" opens it on the first reaction; Escape closes it, focus on the "+"', async () => {
    const { keys, onreact, add, picker } = setup();
    add().focus();

    await keys.keyboard('{Enter}');
    expect(document.activeElement).toBe(within(picker() as HTMLElement).getByRole('button', { name: 'Salute' }));

    await keys.keyboard('{Escape}');
    expect(picker()).toBeNull();
    expect(document.activeElement).toBe(add());
    expect(onreact).not.toHaveBeenCalled();
  });

  it('a press outside closes it', async () => {
    const { keys, onreact, add, picker } = setup();
    await keys.click(add());
    expect(picker()).not.toBeNull();

    await keys.click(document.body);

    expect(picker()).toBeNull();
    expect(onreact).not.toHaveBeenCalled();
  });

  it('a reaction already on the event stays in the bar and toggles from there', async () => {
    const { keys, onreact } = setup();

    await keys.click(screen.getByRole('button', { name: 'Fire, 2' }));

    expect(onreact).toHaveBeenCalledExactlyOnceWith('fire');
  });
});

// The base control's own class is what draws it; a hand-made button of the same name would not have it.
const drawnBy = (element: HTMLElement, base: 'chip' | 'icon-btn') => element.classList.contains(base);

describe('the reaction controls are the base ones', () => {
  it('a reaction on the event is a chip, pressed while it is the user\'s; Enter and Space toggle it', async () => {
    const { keys, onreact } = setup();
    const fire = screen.getByRole('button', { name: 'Fire, 2' });
    expect(drawnBy(fire, 'chip')).toBe(true);
    expect(fire.getAttribute('aria-pressed')).toBe('true');
    expect(fire.dataset.reaction).toBe('fire');
    fire.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');

    expect(onreact.mock.calls).toEqual([['fire'], ['fire']]);
  });

  it('the "+" is a chip and each reaction in its picker an icon button that is not pressed', async () => {
    const { keys, add, picker } = setup();
    expect(drawnBy(add(), 'chip')).toBe(true);
    expect(add().hasAttribute('aria-pressed')).toBe(false);

    await keys.click(add());

    const choices = within(picker() as HTMLElement).getAllByRole('button');
    expect(choices.every((choice) => drawnBy(choice, 'icon-btn'))).toBe(true);
    expect(choices.map((choice) => choice.getAttribute('aria-pressed'))).toEqual(Array(7).fill('false'));
  });
});
