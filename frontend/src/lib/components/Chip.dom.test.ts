import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import ChipHarness from '../testing/ChipHarness.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

describe('Chip', () => {
  it('a toggle says whether it is on, and flips', async () => {
    render(ChipHarness, { target: app });
    const chip = screen.getByRole('button', { name: 'Installed' });
    expect(chip.getAttribute('aria-pressed')).toBe('false');

    await user().click(chip);

    expect(chip.getAttribute('aria-pressed')).toBe('true');
  });

  it('a menu trigger is not a toggle: it says it has a menu and whether it is open, even with a filter applied', async () => {
    render(ChipHarness, { target: app });
    const chip = screen.getByRole('button', { name: 'Sort' });
    expect(chip.hasAttribute('aria-pressed')).toBe(false);
    expect(chip.getAttribute('aria-haspopup')).toBe('menu');
    expect(chip.getAttribute('aria-expanded')).toBe('false');
    expect(chip.classList.contains('selected')).toBe(true);

    await user().click(chip);

    expect(chip.getAttribute('aria-expanded')).toBe('true');
    expect(chip.hasAttribute('aria-pressed')).toBe(false);
  });

  it('a plain chip is a button with no pressed state', async () => {
    render(ChipHarness, { target: app });
    const chip = screen.getByRole('button', { name: 'Plain 0' });
    expect(chip.hasAttribute('aria-pressed')).toBe(false);
    await user().click(chip);
    expect(screen.getByRole('button', { name: 'Plain 1' })).toBeTruthy();
  });

  it('a removable chip is its text and a named remove button, reached by Tab and pressed by Enter', async () => {
    const onremove = vi.fn();
    render(ChipHarness, { target: app, props: { onremove } });
    const remove = screen.getByRole('button', { name: 'Remove the tag rpg' });
    expect(screen.queryByRole('button', { name: 'rpg' })).toBeNull();
    expect(remove.closest('.chip')?.textContent).toContain('rpg');

    remove.focus();
    await user().keyboard('{Enter}');

    expect(onremove).toHaveBeenCalledExactlyOnceWith('rpg');
  });
});
