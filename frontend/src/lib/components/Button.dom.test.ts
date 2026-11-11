import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import ButtonHarness from '../testing/ButtonHarness.svelte';
import SplitButtonHarness from '../testing/SplitButtonHarness.svelte';
import { msg } from '../i18n';
import buttonSource from './Button.svelte?raw';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

function setup(props: Record<string, unknown>) {
  const handlers = { onclick: vi.fn(), onsubmit: vi.fn(), onkeydown: vi.fn(), onpointerdown: vi.fn() };
  const view = render(ButtonHarness, { target: app, props: { ...handlers, ...props } });
  const button = () => screen.getByRole('button', { name: 'Save' });
  return { ...handlers, button, keys: user(), rerender: view.rerender };
}

describe('Button disabled with a reason', () => {
  it('ignores a press, Enter and Space: no click handler, no submit, no handler spread onto it', async () => {
    const { keys, button, onclick, onsubmit, onkeydown, onpointerdown } = setup({ disabled: true, reason: 'The game is running' });
    expect(button().getAttribute('aria-disabled')).toBe('true');

    await keys.click(button());
    button().focus();
    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');

    expect(onclick).not.toHaveBeenCalled();
    expect(onsubmit).not.toHaveBeenCalled();
    expect(onkeydown).not.toHaveBeenCalled();
    expect(onpointerdown).not.toHaveBeenCalled();
  });

  it('Enter in a field of its form does not submit through it', async () => {
    const { keys, onsubmit } = setup({ disabled: true, reason: 'The game is running' });
    await keys.click(screen.getByRole('textbox', { name: 'Name' }));
    await keys.keyboard('{Enter}');
    expect(onsubmit).not.toHaveBeenCalled();
  });

  it('lets Tab and Escape through', async () => {
    const { keys, button } = setup({ disabled: true, reason: 'The game is running' });
    const page = vi.fn();
    document.addEventListener('keydown', page);
    try {
      button().focus();
      await keys.keyboard('{Escape}');
      expect(page).toHaveBeenCalledTimes(1);
    } finally {
      document.removeEventListener('keydown', page);
    }
  });

  it('stays the same element, focus kept, as the reason and the disabled state come and go', async () => {
    const { button, rerender, onclick, keys } = setup({ disabled: true, reason: 'The game is running' });
    const first = button();
    first.focus();

    await rerender({ disabled: false, reason: '' });
    await tick();
    expect(button()).toBe(first);
    expect(document.activeElement).toBe(first);
    expect(first.hasAttribute('aria-disabled')).toBe(false);
    await keys.click(first);
    expect(onclick).toHaveBeenCalledTimes(1);

    await rerender({ disabled: true, reason: 'The game is running' });
    await tick();
    expect(button()).toBe(first);
    expect(document.activeElement).toBe(first);
  });

  it('works as a plain button when enabled: the press, the submit and the spread handlers run', async () => {
    const { keys, button, onclick, onsubmit, onpointerdown } = setup({ disabled: false, reason: '' });
    await keys.click(button());
    expect(onclick).toHaveBeenCalledTimes(1);
    expect(onsubmit).toHaveBeenCalledTimes(1);
    expect(onpointerdown).toHaveBeenCalledTimes(1);
  });
});

describe('Button busy with its own action', () => {
  it('says it is busy, keeps its name and ignores every activation', async () => {
    const { keys, button, onclick, onsubmit, onkeydown, onpointerdown } = setup({ busy: true });
    expect(button().getAttribute('aria-busy')).toBe('true');
    expect(button().getAttribute('aria-disabled')).toBe('true');
    expect(button().hasAttribute('disabled')).toBe(false);
    expect(button().querySelector('.spinner')?.getAttribute('aria-hidden')).toBe('true');

    await keys.click(button());
    button().focus();
    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');

    expect(onclick).not.toHaveBeenCalled();
    expect(onsubmit).not.toHaveBeenCalled();
    expect(onkeydown).not.toHaveBeenCalled();
    expect(onpointerdown).not.toHaveBeenCalled();
  });

  it('stays the same element with focus on it as its action starts and ends, and works again after', async () => {
    const { keys, button, onclick, rerender } = setup({ busy: false });
    const first = button();
    first.focus();

    await rerender({ busy: true });
    await tick();
    expect(button()).toBe(first);
    expect(document.activeElement).toBe(first);

    await rerender({ busy: false });
    await tick();
    expect(button()).toBe(first);
    expect(first.hasAttribute('aria-busy')).toBe(false);
    expect(first.hasAttribute('aria-disabled')).toBe(false);
    expect(first.querySelector('.spinner')).toBeNull();
    await keys.click(first);
    expect(onclick).toHaveBeenCalledTimes(1);
  });

  it('lets Tab and Escape through while busy', async () => {
    const { keys, button } = setup({ busy: true });
    button().focus();
    await keys.tab({ shift: true });
    expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Name' }));
  });
});

describe.each([
  ['busy', { busy: true }],
  ['disabled with a reason', { disabled: true, reason: 'The game is running' }],
])('Button %s blocks activations only', (_, props) => {
  function listen(type: 'keydown' | 'keyup') {
    const seen: KeyboardEvent[] = [];
    const record = (event: Event) => {
      if (event instanceof KeyboardEvent) seen.push(event);
    };
    window.addEventListener(type, record);
    return { seen, stop: () => window.removeEventListener(type, record) };
  }

  it('a shortcut pressed on it reaches the window: Ctrl+K, the appearance reset', async () => {
    const { keys, button } = setup(props);
    const down = listen('keydown');
    try {
      button().focus();
      await keys.keyboard('{Control>}k{/Control}');
      await keys.keyboard('{Control>}{Shift>}{Alt>}T{/Alt}{/Shift}{/Control}');
      expect(down.seen.filter((e) => e.ctrlKey).map((e) => e.key)).toEqual(['Control', 'k', 'Control', 'Shift', 'Alt', 'T']);
      expect(down.seen.at(-1)?.code).toBe('KeyT');
      expect(down.seen.every((e) => !e.defaultPrevented)).toBe(true);
    } finally {
      down.stop();
    }
  });

  it('arrows and page keys pass untouched, so the page around it still scrolls and moves focus', async () => {
    const { keys, button } = setup(props);
    const down = listen('keydown');
    const up = listen('keyup');
    try {
      button().focus();
      await keys.keyboard('{ArrowDown}{ArrowUp}{PageDown}{PageUp}{Home}{End}');
      expect(down.seen.map((e) => e.key)).toEqual(['ArrowDown', 'ArrowUp', 'PageDown', 'PageUp', 'Home', 'End']);
      expect(down.seen.some((e) => e.defaultPrevented)).toBe(false);
      expect(up.seen.map((e) => e.key)).toEqual(['ArrowDown', 'ArrowUp', 'PageDown', 'PageUp', 'Home', 'End']);
    } finally {
      down.stop();
      up.stop();
    }
  });

  it('Enter and Space, held or tapped, still do nothing', async () => {
    const { keys, button, onclick, onsubmit, onkeydown } = setup(props);
    const down = listen('keydown');
    try {
      button().focus();
      await keys.keyboard('{Enter>3}{/Enter}');
      await keys.keyboard('{ >3}{/ }');
      expect(onclick).not.toHaveBeenCalled();
      expect(onsubmit).not.toHaveBeenCalled();
      expect(onkeydown).not.toHaveBeenCalled();
      expect(down.seen).toEqual([]);
    } finally {
      down.stop();
    }
  });
});

describe('Button shapes', () => {
  it('fills its row when asked', () => {
    const { button } = setup({ block: true });
    expect(button().classList.contains('block')).toBe(true);
    expect(buttonSource).toMatch(/\.block \{\s*width: 100%;/);
  });

  it('joins a neighbour on one side: no rounding there', () => {
    expect(buttonSource).toMatch(/\.join-end \{\s*border-top-right-radius: 0;\s*border-bottom-right-radius: 0;/);
    expect(buttonSource).toMatch(/\.join-start \{[^}]*border-top-left-radius: 0;\s*border-bottom-left-radius: 0;/);
  });
});

describe('a split button: an action with its chooser attached', () => {
  it('is a named group of two buttons, each with its own name, joined edge to edge and in Tab order', async () => {
    const onplay = vi.fn();
    render(SplitButtonHarness, { target: app, props: { onplay } });
    const group = screen.getByRole('group', { name: 'Play' });
    const play = screen.getByRole('button', { name: 'Play' });
    const chooser = screen.getByRole('button', { name: msg('games.playProfileChoose') });
    expect(group.contains(play) && group.contains(chooser)).toBe(true);
    expect(play.classList.contains('join-end')).toBe(true);
    expect(chooser.classList.contains('join-start')).toBe(true);
    expect(chooser.classList.contains('icon')).toBe(true);
    expect(chooser.getAttribute('aria-haspopup')).toBe('menu');

    const keys = user();
    screen.getByRole('button', { name: 'Before' }).focus();
    await keys.tab();
    expect(document.activeElement).toBe(play);
    await keys.tab();
    expect(document.activeElement).toBe(chooser);
    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));

    await keys.click(play);
    expect(onplay).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).toBeNull();
  });
});
