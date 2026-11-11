import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import PlayerCardHarness from '../testing/PlayerCardHarness.svelte';
import { HOVER_DELAY, LEAVE_GRACE } from './hoverIntent';

let app: HTMLDivElement;

beforeEach(() => {
  vi.useFakeTimers();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

function setup(props: Record<string, unknown> = {}) {
  const view = render(PlayerCardHarness, { target: app, props });
  const keys = user();
  const anchor = () => screen.getByRole('button', { name: 'Ann Lee' });
  const card = () => document.querySelector<HTMLElement>('[role="tooltip"]');
  const wait = async (ms: number) => {
    vi.advanceTimersByTime(ms);
    await tick();
    await tick();
  };
  const hover = async () => {
    await keys.hover(anchor());
    await wait(HOVER_DELAY);
    expect(card()).not.toBeNull();
  };
  return { keys, anchor, card, wait, hover, rerender: view.rerender };
}

describe('PlayerCardPopover', () => {
  it('shows after the pointer rests on the name, as its description, out of the page and placed beside it', async () => {
    const { keys, anchor, card, wait } = setup();
    anchor().getBoundingClientRect = () => DOMRect.fromRect({ x: 900, y: 200, width: 120, height: 40 });
    await keys.hover(anchor());
    await wait(HOVER_DELAY - 50);
    expect(card()).toBeNull();
    await wait(50);
    const shown = card() as HTMLElement;
    expect(shown.textContent).toContain('@ann');
    expect(anchor().getAttribute('aria-describedby')).toBe(shown.id);
    expect(app.contains(shown)).toBe(false);
    expect(shown.style.position).toBe('fixed');
    // Near the right edge of a 1024px window there is no room on the right: it opens on the left.
    expect(shown.dataset.placement?.startsWith('left')).toBe(true);
  });

  it('keyboard focus shows it and blur hides it; it never takes focus', async () => {
    const { keys, anchor, card } = setup();
    await keys.tab();
    await keys.tab();
    expect(document.activeElement).toBe(anchor());
    expect(card()).not.toBeNull();
    expect(card()?.hasAttribute('tabindex')).toBe(false);
    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
    expect(card()).toBeNull();
  });

  it('stays while the pointer moves onto it, and goes once the pointer leaves both', async () => {
    const { keys, card, wait, hover } = setup();
    await hover();
    const shown = card() as HTMLElement;
    await keys.hover(shown);
    await wait(1000);
    expect(card()).not.toBeNull();
    await keys.unhover(shown);
    await wait(LEAVE_GRACE);
    expect(card()).toBeNull();
  });

  it('Escape hides it through the layer stack and goes no further', async () => {
    const { keys, card, hover } = setup();
    await hover();
    const page = vi.fn();
    window.addEventListener('keydown', page);
    try {
      await keys.keyboard('{Escape}');
      expect(card()).toBeNull();
      expect(page).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener('keydown', page);
    }
  });

  it('a dialog opening hides it', async () => {
    const { card, hover, rerender } = setup();
    await hover();
    await rerender({ other: true });
    await tick();
    expect(screen.getByRole('dialog', { name: 'Other' })).toBeTruthy();
    expect(card()).toBeNull();
  });

  it('a scroll of the page hides it, a scroll of a list it is not in does not', async () => {
    const { card, hover } = setup();
    await hover();
    await fireEvent.scroll(screen.getByTestId('unrelated'));
    expect(card()).not.toBeNull();
    await fireEvent.scroll(document);
    expect(card()).toBeNull();
  });

  it('the window losing focus hides it', async () => {
    const { card, hover } = setup();
    await hover();
    await fireEvent.blur(window);
    expect(card()).toBeNull();
  });
});
