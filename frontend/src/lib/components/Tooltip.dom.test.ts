import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import { description } from '../testing/hints';
import TooltipHarness from '../testing/TooltipHarness.svelte';
import { HOVER_DELAY, LEAVE_GRACE } from './hoverIntent';

const TEXT = 'Removes the cached covers';

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
  const view = render(TooltipHarness, { target: app, props });
  const keys = user();
  const control = () => screen.getByRole('button', { name: 'Clear' });
  const bubble = () => document.querySelector<HTMLElement>('[data-part="tooltip"]');
  const shown = () => bubble()?.textContent?.trim() === TEXT;
  const wait = async (ms: number) => {
    vi.advanceTimersByTime(ms);
    await tick();
    await tick();
  };
  // Keyboard focus, as Tab gives it (user-event dispatches focusin; jsdom's focus() does not).
  const focusByKeyboard = async () => {
    screen.getByRole('button', { name: 'Before' }).focus();
    await keys.tab();
  };
  return { keys, control, bubble, shown, wait, focusByKeyboard, rerender: view.rerender };
}

describe('Tooltip: pointer', () => {
  it('shows after the pointer rests on the control for the delay, not before', async () => {
    const { keys, control, shown, wait } = setup();
    await keys.hover(control());
    await wait(HOVER_DELAY - 50);
    expect(shown()).toBe(false);
    await wait(50);
    expect(shown()).toBe(true);
  });

  it('never shows when the pointer leaves before the delay', async () => {
    const { keys, control, shown, wait } = setup();
    await keys.hover(control());
    await wait(100);
    await keys.unhover(control());
    await wait(1000);
    expect(shown()).toBe(false);
  });

  it('stays while the pointer moves onto the hint, and goes once it leaves both', async () => {
    const { keys, control, bubble, shown, wait } = setup();
    await keys.hover(control());
    await wait(HOVER_DELAY);
    const tip = bubble() as HTMLElement;
    await keys.hover(tip);
    await wait(1000);
    expect(shown()).toBe(true);
    await keys.unhover(tip);
    await wait(LEAVE_GRACE);
    expect(shown()).toBe(false);
  });

  it('a press on the control hides it and it does not come back while the pointer stays', async () => {
    const { keys, control, shown, wait } = setup();
    await keys.hover(control());
    await wait(HOVER_DELAY);
    await keys.click(control());
    await wait(1000);
    expect(shown()).toBe(false);
  });

  it('a click focuses the control without showing the hint (pointer focus is not keyboard focus)', async () => {
    const { keys, control, shown, wait } = setup();
    await keys.click(control());
    await wait(1000);
    expect(document.activeElement).toBe(control());
    expect(shown()).toBe(false);
  });

  it('touch does not show it', async () => {
    const { keys, control, shown, wait } = setup();
    await keys.pointer([{ keys: '[TouchA>]', target: control() }]);
    await wait(1000);
    expect(shown()).toBe(false);
    await keys.pointer([{ keys: '[/TouchA]', target: control() }]);
    await wait(1000);
    expect(shown()).toBe(false);
  });
});

describe('Tooltip: keyboard', () => {
  it('keyboard focus shows it, blur hides it, and Tab never lands on it', async () => {
    const { keys, control, bubble, shown } = setup();
    await keys.tab();
    await keys.tab();
    expect(document.activeElement).toBe(control());
    expect(shown()).toBe(true);
    expect(bubble()?.hasAttribute('tabindex')).toBe(false);

    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
    expect(shown()).toBe(false);
  });

  it('Escape hides the hint and goes no further; focus stays on the control', async () => {
    const { keys, control, shown, wait, focusByKeyboard } = setup();
    await focusByKeyboard();
    expect(shown()).toBe(true);
    const page = vi.fn();
    window.addEventListener('keydown', page);
    try {
      await keys.keyboard('{Escape}');
      await wait(1000);
      expect(shown()).toBe(false);
      expect(document.activeElement).toBe(control());
      expect(page).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener('keydown', page);
    }
  });

  it('in a dialog the first Escape hides the hint, the second closes the dialog', async () => {
    const { keys, control, shown } = setup({ inDialog: true });
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
    await keys.tab();
    expect(document.activeElement).toBe(control());
    expect(shown()).toBe(true);

    await keys.keyboard('{Escape}');
    expect(shown()).toBe(false);
    expect(screen.queryByRole('dialog', { name: 'Settings' })).not.toBeNull();
    expect(document.activeElement).toBe(control());

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog', { name: 'Settings' })).toBeNull();
  });

  it('showing it never moves focus', async () => {
    const { keys, control, shown, wait } = setup();
    const before = screen.getByRole('button', { name: 'Before' });
    before.focus();
    await keys.hover(control());
    await wait(HOVER_DELAY);
    expect(shown()).toBe(true);
    expect(document.activeElement).toBe(before);
  });
});

describe('Tooltip: screen reader', () => {
  it('a description is the control description; the name stays the control own', async () => {
    const { control } = setup();
    expect(description(control())).toBe(`Takes a minute ${TEXT}`);
    expect(control().getAttribute('aria-label')).toBeNull();
    expect(control().textContent?.trim()).toBe('Clear');
  });

  it('a control that sets its own description keeps the hint in it as that changes', async () => {
    const { keys, control } = setup();
    expect(description(control())).toBe(`Takes a minute ${TEXT}`);
    await keys.click(screen.getByRole('button', { name: 'Change note' }));
    expect(control().getAttribute('aria-describedby')?.split(' ')[0]).toBe('note-b');
    expect(description(control())).toBe(`Cannot be undone ${TEXT}`);
  });

  it('a control created again inside the tooltip is described too', async () => {
    const { keys, control } = setup();
    const first = control();
    await keys.click(screen.getByRole('button', { name: 'Rebuild' }));
    expect(control()).not.toBe(first);
    expect(description(control())).toBe(`Takes a minute ${TEXT}`);
  });

  it('an empty text describes nothing and shows nothing', async () => {
    const { keys, control, bubble, wait } = setup({ text: '' });
    expect(description(control())).toBe('Takes a minute');
    await keys.hover(control());
    await wait(1000);
    expect(bubble()).toBeNull();
  });

  it('a visual label is not announced again as a description, and the bubble is hidden from the tree', async () => {
    const { keys, control, bubble, shown } = setup({ describe: false });
    expect(description(control())).toBe('Takes a minute');
    await keys.tab();
    await keys.tab();
    expect(shown()).toBe(true);
    expect(bubble()?.getAttribute('aria-hidden')).toBe('true');
  });
});

describe('Tooltip: placement and the stack', () => {
  it('is portalled out of the page, a theme part, placed fixed beside the control and flipped near the top', async () => {
    const { control, bubble, wait, focusByKeyboard } = setup({ placement: 'top' });
    control().getBoundingClientRect = () => DOMRect.fromRect({ x: 300, y: 2, width: 80, height: 32 });
    await focusByKeyboard();
    await wait(0);
    const tip = bubble() as HTMLElement;
    expect(app.contains(tip)).toBe(false);
    expect(tip.parentElement).toBe(document.body);
    expect(tip.style.position).toBe('fixed');
    expect(tip.dataset.placement).toBe('bottom');
  });

  it('appearing over an open menu leaves the menu open', async () => {
    const { keys, control, shown, wait } = setup();
    await keys.click(screen.getByRole('button', { name: 'Sort' }));
    expect(screen.getByRole('menu')).toBeTruthy();
    await keys.hover(control());
    await wait(HOVER_DELAY);
    expect(shown()).toBe(true);
    expect(screen.queryByRole('menu')).not.toBeNull();
  });

  it('a dialog opening hides it', async () => {
    const { shown, rerender, focusByKeyboard } = setup();
    await focusByKeyboard();
    expect(shown()).toBe(true);
    await rerender({ other: true });
    await tick();
    expect(screen.getByRole('dialog', { name: 'Other' })).toBeTruthy();
    expect(shown()).toBe(false);
  });
});

describe('Tooltip: going away', () => {
  async function showByHover() {
    const view = setup();
    await view.keys.hover(view.control());
    await view.wait(HOVER_DELAY);
    expect(view.shown()).toBe(true);
    return view;
  }

  it('a scroll of the page hides it', async () => {
    const { shown } = await showByHover();
    await fireEvent.scroll(document);
    expect(shown()).toBe(false);
  });

  it('a scroll elsewhere (a list the control is not in) or in the hint leaves it', async () => {
    const { shown, bubble } = await showByHover();
    await fireEvent.scroll(screen.getByTestId('unrelated'));
    await fireEvent.scroll(bubble() as HTMLElement);
    expect(shown()).toBe(true);
  });

  it('the window losing focus hides it', async () => {
    const { shown } = await showByHover();
    await fireEvent.blur(window);
    expect(shown()).toBe(false);
  });

  it('the control scrolled out of sight takes the hint with it', async () => {
    const { control, shown, wait } = await showByHover();
    control().getBoundingClientRect = () => DOMRect.fromRect({ x: 300, y: -500, width: 80, height: 32 });
    await fireEvent(window, new Event('resize'));
    await wait(50);
    expect(shown()).toBe(false);
  });

  it('the control removed hides it', async () => {
    const { shown, rerender, wait } = await showByHover();
    await rerender({ present: false });
    await wait(0);
    expect(shown()).toBe(false);
  });

  it('the control turning disabled hides it', async () => {
    const { shown, rerender, wait } = await showByHover();
    await rerender({ disabled: true });
    await wait(0);
    expect(shown()).toBe(false);
  });

  it('a disabled control shows no hint', async () => {
    const { keys, control, shown, wait } = setup({ disabled: true });
    await keys.hover(control());
    await wait(1000);
    expect(shown()).toBe(false);
  });
});
