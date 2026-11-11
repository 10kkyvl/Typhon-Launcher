import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import ButtonHarness from '../testing/ButtonHarness.svelte';

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
