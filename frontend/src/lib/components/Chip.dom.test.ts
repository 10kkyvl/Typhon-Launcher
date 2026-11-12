import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import { user } from '../testing/dom';
import { description } from '../testing/hints';
import ChipHarness from '../testing/ChipHarness.svelte';
import Chip from './Chip.svelte';
import chipSource from './Chip.svelte?raw';

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

  it('the remove button shows its name as a hint, and is named once', async () => {
    const children = createRawSnippet(() => ({ render: () => '<span>rpg</span>' }));
    render(Chip, { target: app, props: { onremove: vi.fn(), removeLabel: 'Remove the tag rpg', children } });
    await user().tab();
    const remove = screen.getByRole('button', { name: 'Remove the tag rpg' });
    expect(document.activeElement).toBe(remove);
    const bubble = document.querySelector<HTMLElement>('[data-part="tooltip"]');
    expect(bubble?.textContent?.trim()).toBe('Remove the tag rpg');
    expect(bubble?.getAttribute('aria-hidden')).toBe('true');
    expect(remove.hasAttribute('title')).toBe(false);
    expect(remove.hasAttribute('aria-describedby')).toBe(false);
  });

  it('Backspace and Delete remove nothing: the remove button, pressed, is the keyboard path', async () => {
    const onremove = vi.fn();
    render(ChipHarness, { target: app, props: { onremove } });
    screen.getByRole('button', { name: 'Remove the tag rpg' }).focus();
    const keys = user();
    await keys.keyboard('{Backspace}{Delete}');
    expect(onremove).not.toHaveBeenCalled();
    await keys.keyboard(' ');
    expect(onremove).toHaveBeenCalledTimes(1);
  });
});

// A chip that cannot be pressed now, as Button: refused with a reason it keeps focus and says why;
// busy (its own request running) it keeps focus and says so; disabled alone it is out of reach.
describe('Chip that cannot be pressed now', () => {
  const props = (extra: Record<string, unknown>) => ({ 'aria-label': 'Helpful', onclick: vi.fn(), ...extra });

  async function pressAll(chip: HTMLElement) {
    const keys = user();
    await keys.click(chip);
    chip.focus();
    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');
  }

  it('disabled with a reason stays in the Tab order, says why and ignores a press, Enter and Space', async () => {
    const extra = props({ disabled: true, reason: 'Sign in to vote' });
    render(Chip, { target: app, props: extra });
    const chip = screen.getByRole('button', { name: 'Helpful' });

    await user().tab();
    expect(document.activeElement).toBe(chip);
    await pressAll(chip);

    expect(chip.hasAttribute('disabled')).toBe(false);
    expect(chip.getAttribute('aria-disabled')).toBe('true');
    expect(description(chip)).toBe('Sign in to vote');
    expect(extra.onclick).not.toHaveBeenCalled();
  });

  it('busy keeps focus, says it is busy and ignores a press, Enter and Space', async () => {
    const extra = props({ busy: true });
    render(Chip, { target: app, props: extra });
    const chip = screen.getByRole('button', { name: 'Helpful' });
    chip.focus();

    await pressAll(chip);

    expect(document.activeElement).toBe(chip);
    expect(chip.getAttribute('aria-busy')).toBe('true');
    expect(chip.getAttribute('aria-disabled')).toBe('true');
    expect(extra.onclick).not.toHaveBeenCalled();
  });

  it('disabled alone is out of reach', () => {
    render(Chip, { target: app, props: props({ disabled: true }) });
    expect((screen.getByRole('button', { name: 'Helpful' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('draws no hover, no press and no pointer while it cannot be pressed', () => {
    const css = styleOf(chipSource, 'Chip.svelte');
    const NOT = ":not(:disabled, [aria-disabled='true'])";
    for (const selector of [`button.chip:active${NOT}`, `button.outline:hover${NOT}`, `button.filled:hover${NOT}`]) expect(() => ruleBody(css, selector), selector).not.toThrow();
    expect(css).not.toMatch(/button\.(?:chip:active|outline:hover|filled:hover|quiet:hover)\s*[,{]/);
    expect(ruleBody(css, "button.chip:is(:disabled, [aria-disabled='true'])")).toMatchObject({ cursor: 'default' });
    expect(ruleBody(css, "button.chip[aria-busy='true']")).toMatchObject({ cursor: 'progress', opacity: '1' });
  });
});

describe('Chip given an address', () => {
  it('is a real link out with the chip look, its attributes passed on, and its mark hidden from the name', () => {
    render(ChipHarness, { target: app });
    const link = screen.getByRole('link', { name: 'Steam' });
    expect(link.tagName).toBe('A');
    expect(link.getAttribute('href')).toBe('https://store.steampowered.com/app/504230');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    expect(link.classList.contains('chip')).toBe(true);
    expect(link.classList.contains('outline')).toBe(true);
    expect(link.hasAttribute('aria-pressed')).toBe(false);
    const mark = link.querySelector('[data-part="chip-end"]');
    expect(mark?.getAttribute('aria-hidden')).toBe('true');
    expect(mark?.querySelector('svg')).toBeInstanceOf(SVGElement);
  });

  it('hovers as the button chip does and sizes its mark itself', () => {
    const css = styleOf(chipSource, 'Chip.svelte');
    expect(ruleBody(css, 'a.outline:hover')).toEqual(ruleBody(css, "button.outline:hover:not(:disabled, [aria-disabled='true'])"));
    expect(ruleBody(css, '.end :global(svg)')).toEqual({ width: 'var(--chip-mark)', height: 'var(--chip-mark)' });
  });
});
