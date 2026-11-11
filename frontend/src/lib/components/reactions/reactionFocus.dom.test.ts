import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: false }));
vi.mock('../../services/social', async (actual) => ({
  ...(await actual<typeof import('../../services/social')>()),
  react: vi.fn(),
  unreact: vi.fn(),
}));

import { locale } from '../../i18n';
import { react, unreact, type FeedEvent } from '../../services/social';
import { feedEvents } from '../../stores/feed';
import { user } from '../../testing/dom';
import FeedHarness from '../../testing/FeedHarness.svelte';

// The feed row as the activity page has it, on the real feed store; the requests are held open so the
// pending state can be seen, then let go.
function feedEvent(reactions: { emoji: string; count: number }[], mine: string[]): FeedEvent {
  return {
    id: 7,
    user: { id: 'u2', username: 'neo', displayName: 'Neo', avatarUrl: '' },
    kind: 'completed',
    game: { igdbId: 1, title: 'Hades', coverUrl: '', artworkUrl: '' },
    createdAt: new Date().toISOString(),
    seconds: 0,
    achievements: [],
    review: null,
    reactions,
    mine,
    note: '',
  } as unknown as FeedEvent;
}

let app: HTMLDivElement;
let release: () => void = () => {};
let fail: (reason: Error) => void = () => {};

beforeEach(() => {
  locale.set('en');
  for (const call of [react, unreact]) {
    vi.mocked(call).mockReset();
    vi.mocked(call).mockImplementation(() => new Promise<void>((resolve, reject) => {
      release = resolve;
      fail = reject;
    }));
  }
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  release();
  feedEvents.set([]);
  document.body.replaceChildren();
});

function setup(event: FeedEvent) {
  feedEvents.set([event]);
  render(FeedHarness, { target: app });
  const keys = user();
  const add = () => screen.queryByRole('button', { name: 'Add a reaction' });
  const chip = (name: string) => screen.getByRole('button', { name });
  const picker = () => screen.queryByRole('dialog', { name: 'Add a reaction' });
  return { keys, add, chip, picker };
}

// Where a keyboard user is: a control that can hold focus (a disabled one cannot, browsers drop it).
function focusedControl(): HTMLButtonElement {
  const active = document.activeElement;
  expect(active).toBeInstanceOf(HTMLButtonElement);
  expect((active as HTMLButtonElement).disabled).toBe(false);
  return active as HTMLButtonElement;
}

async function settle() {
  release();
  await Promise.resolve();
  await tick();
}

describe('feed reactions keep the keyboard user in place', () => {
  it('adding the last reaction left in the picker puts focus on its new chip, through the request', async () => {
    const all = ['fire', 'salute', 'heart', 'clap', 'skull', 'party', 'eyes'].map((emoji) => ({ emoji, count: 1 }));
    const { keys, add, chip, picker } = setup(feedEvent(all, []));
    (add() as HTMLElement).focus();
    await keys.keyboard('{Enter}');

    await keys.keyboard('{Enter}');

    expect(react).toHaveBeenCalledExactlyOnceWith('7', 'joy');
    expect(picker()).toBeNull();
    expect(add()).toBeNull();
    expect(focusedControl()).toBe(chip('Laugh, 1'));
    expect(chip('Laugh, 1').getAttribute('aria-disabled')).toBe('true');

    await settle();
    expect(focusedControl()).toBe(chip('Laugh, 1'));
    expect(chip('Laugh, 1').hasAttribute('aria-disabled')).toBe(false);
  });

  it('adding a reaction that leaves the "+" puts focus on its new chip, which holds it while the request runs', async () => {
    const { keys, add, chip } = setup(feedEvent([{ emoji: 'fire', count: 2 }], ['fire']));
    (add() as HTMLElement).focus();
    await keys.keyboard('{Enter}');
    await keys.tab();

    await keys.keyboard('{Enter}');

    expect(react).toHaveBeenCalledExactlyOnceWith('7', 'heart');
    expect(focusedControl()).toBe(chip('Heart, 1'));
    expect(add()?.getAttribute('aria-disabled')).toBe('true');

    await settle();
    expect(focusedControl()).toBe(chip('Heart, 1'));
  });

  it('while the request runs, the bar takes no second reaction and the "+" opens nothing', async () => {
    const { keys, add, chip, picker } = setup(feedEvent([{ emoji: 'fire', count: 2 }], ['fire']));
    await keys.click(add() as HTMLElement);
    await keys.click(within(picker() as HTMLElement).getByRole('button', { name: 'Party' }));
    expect(react).toHaveBeenCalledOnce();

    await keys.click(chip('Fire, 2'));
    await keys.click(add() as HTMLElement);

    expect(unreact).not.toHaveBeenCalled();
    expect(react).toHaveBeenCalledOnce();
    expect(picker()).toBeNull();
  });

  it('taking back a reaction only the user gave moves focus from its chip, which goes, to the "+"', async () => {
    const { keys, add, chip } = setup(feedEvent([{ emoji: 'fire', count: 1 }, { emoji: 'heart', count: 3 }], ['fire']));
    chip('Fire, 1').focus();

    await keys.keyboard('{Enter}');

    expect(unreact).toHaveBeenCalledExactlyOnceWith('7', 'fire');
    expect(screen.queryByRole('button', { name: /^Fire/ })).toBeNull();
    expect(focusedControl()).toBe(add());
  });

  it('a reaction the server refuses takes its new chip away and leaves focus on the "+"', async () => {
    const { keys, add, chip } = setup(feedEvent([{ emoji: 'fire', count: 2 }], ['fire']));
    (add() as HTMLElement).focus();
    await keys.keyboard('{Enter}');
    await keys.tab();
    await keys.keyboard('{Enter}');
    expect(focusedControl()).toBe(chip('Heart, 1'));

    fail(new Error('offline'));
    await Promise.resolve();
    await tick();
    await tick();

    expect(screen.queryByRole('button', { name: /^Heart/ })).toBeNull();
    expect(focusedControl()).toBe(add());
  });

  it('a chip that goes while focus is elsewhere does not pull focus into the bar', async () => {
    const { keys, add, chip } = setup(feedEvent([{ emoji: 'fire', count: 2 }], ['fire']));
    const outside = document.createElement('button');
    document.body.append(outside);
    (add() as HTMLElement).focus();
    await keys.keyboard('{Enter}');
    await keys.tab();
    await keys.keyboard('{Enter}');
    expect(focusedControl()).toBe(chip('Heart, 1'));
    outside.focus();

    fail(new Error('offline'));
    await Promise.resolve();
    await tick();
    await tick();

    expect(document.activeElement).toBe(outside);
  });
});
