import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import ButtonHarness from '../testing/ButtonHarness.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

function setup(props: Record<string, unknown>) {
  const onclick = vi.fn();
  const view = render(ButtonHarness, { props: { variant: 'play', onclick, ...props } });
  return { onclick, button: () => screen.getByRole('button', { name: 'Save' }), keys: user(), rerender: view.rerender };
}

// The look of the button that launches a game: its colours are the game's inside a game's scope
// (styles/game-tone.test.ts); its behaviour is every other button's.
describe('Button that launches a game', () => {
  it('is drawn in the launching look, not the primary one', () => {
    const { button } = setup({});
    expect(button().classList.contains('play')).toBe(true);
    expect(button().classList.contains('primary')).toBe(false);
  });

  it('acts on a press like any button', async () => {
    const { button, keys, onclick } = setup({});
    await keys.click(button());
    expect(onclick).toHaveBeenCalledTimes(1);
  });

  it('busy, it keeps focus and its colour and ignores a press', async () => {
    const { button, keys, onclick } = setup({ busy: true });
    button().focus();
    await keys.click(button());
    expect(onclick).not.toHaveBeenCalled();
    expect(button().getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(button());
  });

  it('disabled with a reason, it stays in the Tab order and ignores a press', async () => {
    const { button, keys, onclick } = setup({ disabled: true, reason: 'The game is running' });
    expect(button().getAttribute('aria-disabled')).toBe('true');
    expect(button().hasAttribute('disabled')).toBe(false);
    await keys.click(button());
    expect(onclick).not.toHaveBeenCalled();
  });
});
