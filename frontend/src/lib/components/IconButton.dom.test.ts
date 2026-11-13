import { render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import { user } from '../testing/dom';
import { description } from '../testing/hints';
import buttonSource from './Button.svelte?raw';
import { HOVER_DELAY } from './hoverIntent';
import IconButton from './IconButton.svelte';
import source from './IconButton.svelte?raw';

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

const bubble = () => document.querySelector<HTMLElement>('[data-part="tooltip"]');

async function rest(element: HTMLElement) {
  await user().hover(element);
  vi.advanceTimersByTime(HOVER_DELAY);
  await tick();
}

describe('IconButton', () => {
  it('shows its label as a tooltip instead of a native title, and keeps the label as its name only', async () => {
    render(IconButton, { target: app, props: { label: 'Close' } });
    const button = screen.getByRole('button', { name: 'Close' });
    expect(button.hasAttribute('title')).toBe(false);
    expect(button.hasAttribute('aria-describedby')).toBe(false);

    await rest(button);
    expect(bubble()?.textContent?.trim()).toBe('Close');
    expect(bubble()?.getAttribute('aria-hidden')).toBe('true');
    expect(screen.getByRole('button', { name: 'Close' })).toBe(button);
  });

  it('keyboard focus shows the label', async () => {
    render(IconButton, { target: app, props: { label: 'Close' } });
    await user().tab();
    expect(bubble()?.textContent?.trim()).toBe('Close');
  });

  it('without a tooltip (the overlay window) leaves the label to the native title', async () => {
    render(IconButton, { target: app, props: { label: 'Close', tooltip: false } });
    const button = screen.getByRole('button', { name: 'Close' });
    expect(button.getAttribute('title')).toBe('Close');
    await rest(button);
    expect(bubble()).toBeNull();
  });

  it('a disabled button shows no tooltip; the native title tells a mouse user what the icon is', async () => {
    render(IconButton, { target: app, props: { label: 'Close', disabled: true } });
    const button = screen.getByRole('button', { name: 'Close' });
    expect(button.getAttribute('title')).toBe('Close');
    await rest(button);
    expect(bubble()).toBeNull();
  });

  it('disabled with a reason keeps focus, shows and is described by the reason, and ignores a press and Enter', async () => {
    const onclick = vi.fn();
    const { rerender } = render(IconButton, { target: app, props: { label: 'Back', reason: '', onclick } });
    const button = screen.getByRole('button', { name: 'Back' });
    button.focus();

    await rerender({ label: 'Back', reason: 'Nothing to go back to', disabled: true, onclick });
    const keys = user();
    await keys.keyboard('{Enter}');
    await keys.click(button);

    expect(screen.getByRole('button', { name: 'Back' })).toBe(button);
    expect(document.activeElement).toBe(button);
    expect(button.hasAttribute('disabled')).toBe(false);
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.hasAttribute('title')).toBe(false);
    expect(description(button)).toBe('Nothing to go back to');
    expect(onclick).not.toHaveBeenCalled();
  });

  it('the native title comes and goes with the disabled state, on the same button', async () => {
    const { rerender } = render(IconButton, { target: app, props: { label: 'Back', disabled: true } });
    const button = screen.getByRole('button', { name: 'Back' });
    await rerender({ label: 'Back', disabled: false });
    expect(screen.getByRole('button', { name: 'Back' })).toBe(button);
    expect(button.hasAttribute('title')).toBe(false);
    await rerender({ label: 'Back', disabled: true });
    expect(button.getAttribute('title')).toBe('Back');
  });

  it('the favourite button, whose name says its state, is lit without claiming a pressed state', async () => {
    const view = render(IconButton, { target: app, props: { label: 'Add to favourites', lit: false } });
    const button = screen.getByRole('button', { name: 'Add to favourites' });
    expect(button.classList.contains('lit')).toBe(false);

    await view.rerender({ label: 'Remove from favourites', lit: true });
    expect(screen.getByRole('button', { name: 'Remove from favourites' })).toBe(button);
    expect(button.classList.contains('lit')).toBe(true);
    expect(button.hasAttribute('aria-pressed')).toBe(false);
    expect(button.hasAttribute('lit')).toBe(false);
  });

  it('a lit button that is the current item, or the trigger of what it opened, says that instead', () => {
    render(IconButton, { target: app, props: { label: 'Settings of Pinned', lit: true, 'aria-current': 'true' } });
    render(IconButton, { target: app, props: { label: 'Open the browser', lit: true, 'aria-expanded': true } });
    const current = screen.getByRole('button', { name: 'Settings of Pinned' });
    const trigger = screen.getByRole('button', { name: 'Open the browser' });
    expect(current.getAttribute('aria-current')).toBe('true');
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    for (const button of [current, trigger]) {
      expect(button.classList.contains('lit')).toBe(true);
      expect(button.hasAttribute('aria-pressed')).toBe(false);
    }
  });

  // No icon button in the app is a fixed-name toggle today; the state stays for the first one.
  it('a toggle with a fixed name says whether it is on; a plain action says nothing of the kind', async () => {
    const view = render(IconButton, { target: app, props: { label: 'Mute the chat', pressed: false } });
    const button = screen.getByRole('button', { name: 'Mute the chat' });
    expect(button.getAttribute('aria-pressed')).toBe('false');
    await view.rerender({ label: 'Mute the chat', pressed: true });
    expect(button.getAttribute('aria-pressed')).toBe('true');

    render(IconButton, { target: app, props: { label: 'Close' } });
    expect(screen.getByRole('button', { name: 'Close' }).hasAttribute('aria-pressed')).toBe(false);
  });

  it('comes large and on a surface of its own when asked', () => {
    render(IconButton, { target: app, props: { label: 'Favourite', size: 'lg', variant: 'secondary' } });
    const button = screen.getByRole('button', { name: 'Favourite' });
    expect(button.classList.contains('lg')).toBe(true);
    expect(button.classList.contains('secondary')).toBe(true);
  });

  // The chat's send button and its floating opener: the one accent-filled action of their place.
  it('comes filled in the accent as the primary button does, with its hover, and stays filled while hovered', () => {
    render(IconButton, { target: app, props: { label: 'Send', variant: 'primary' } });
    const button = screen.getByRole('button', { name: 'Send' });
    expect(button.classList.contains('primary')).toBe(true);
    const css = styleOf(source, 'IconButton.svelte');
    const buttonCss = styleOf(buttonSource, 'Button.svelte');
    expect(ruleBody(css, '.icon-btn.primary')).toEqual(ruleBody(buttonCss, '.primary'));
    const hover = ruleBody(css, ".icon-btn.primary:hover:not(:disabled, [aria-disabled='true'])");
    expect(hover.background).toBe(ruleBody(buttonCss, ".primary:hover:not(:disabled, [aria-disabled='true'])").background);
    expect(hover.color).toBe(ruleBody(buttonCss, '.primary').color);
    // After the plain hover, at its specificity or more, so the fill is not swapped for the grey one.
    expect(css.indexOf(".icon-btn.primary:hover:not(:disabled, [aria-disabled='true'])")).toBeGreaterThan(css.indexOf(".icon-btn:hover:not(:disabled, [aria-disabled='true']),"));
  });

  it('with a hint shows the hint instead of its label, as its description; the name stays the label', async () => {
    render(IconButton, { target: app, props: { label: 'Attach an image', hint: 'Up to 4 images' } });
    const button = screen.getByRole('button', { name: 'Attach an image' });
    expect(description(button)).toBe('Up to 4 images');
    await rest(button);
    expect(bubble()?.textContent?.trim()).toBe('Up to 4 images');
    expect(document.querySelectorAll('[data-part="tooltip"]')).toHaveLength(1);
  });

  it('shows an open menu or panel on its trigger, and an "on" toggle, without a class from the caller', () => {
    expect(source).toMatch(/\.icon-btn\[aria-expanded='true'\] \{\s*background: var\(--hover-strong\);/);
    expect(source).toMatch(/\.icon-btn:is\(\.lit, \[aria-pressed='true'\]\) \{\s*background: var\(--accent-subtle\);\s*color: var\(--accent-text\);/);
    expect(source).not.toMatch(/class:active|\.active\b/);
  });

  // The overlay drew "on" as accent text on no fill, and its hover as any other hover (the text colour
  // on the hover fill): the pin takes the fill away and leaves the colour to the rules above.
  it('in the overlay a lit or open button keeps the look it had: no fill at rest, the plain hover', () => {
    const lit = ":is(.lit, [aria-pressed='true'], [aria-expanded='true'])";
    const pin = (tail: string) => new RegExp(`:global\\(\\[data-controls='1x'\\]\\) \\.icon-btn${lit.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}${tail} \\{([^}]*)\\}`).exec(source)?.[1].trim();
    expect(pin('')).toBe('background: transparent;');
    expect(pin(':hover:not\\(:disabled\\)')).toBe('background: var(--hover-strong);');
  });

  // The 1.x icon button had no border; a transparent one still shifts the anti-aliasing of the round
  // corners of the hover fill.
  it('in the overlay draws no border', () => {
    expect(source).toMatch(/:global\(\[data-controls='1x'\]\) \.icon-btn \{[^}]*border: 0;/);
  });
});

// The icon's place, so the spinner can be seen to stand in it.
const glyph = createRawSnippet(() => ({ render: () => '<svg class="glyph"></svg>' }));

describe('IconButton busy with its own action', () => {
  it('says it is busy, keeps its name and focus, shows the spinner in place of its icon, and ignores every activation', async () => {
    const onclick = vi.fn();
    const onkeydown = vi.fn();
    const onpointerdown = vi.fn();
    render(IconButton, { target: app, props: { label: 'Pause', busy: true, onclick, onkeydown, onpointerdown, children: glyph } });
    const button = screen.getByRole('button', { name: 'Pause' });
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.hasAttribute('disabled')).toBe(false);
    expect(button.querySelector('.spinner')?.getAttribute('aria-hidden')).toBe('true');
    expect(button.querySelector('.glyph')).toBeNull();

    const keys = user();
    await keys.click(button);
    button.focus();
    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');

    expect(onclick).not.toHaveBeenCalled();
    expect(onkeydown).not.toHaveBeenCalled();
    expect(onpointerdown).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(button);
  });

  it('stays the same element with focus on it as its action starts and ends, and works again after', async () => {
    const onclick = vi.fn();
    const { rerender } = render(IconButton, { target: app, props: { label: 'Pause', onclick, children: glyph } });
    const button = screen.getByRole('button', { name: 'Pause' });
    button.focus();

    await rerender({ label: 'Pause', busy: true, onclick, children: glyph });
    expect(screen.getByRole('button', { name: 'Pause' })).toBe(button);
    expect(document.activeElement).toBe(button);

    await rerender({ label: 'Pause', busy: false, onclick, children: glyph });
    expect(screen.getByRole('button', { name: 'Pause' })).toBe(button);
    expect(button.hasAttribute('aria-busy')).toBe(false);
    expect(button.hasAttribute('aria-disabled')).toBe(false);
    expect(button.querySelector('.spinner')).toBeNull();
    expect(button.querySelector('.glyph')).not.toBeNull();
    await user().click(button);
    expect(onclick).toHaveBeenCalledTimes(1);
  });

  it('lets Tab through while busy', async () => {
    render(IconButton, { target: app, props: { label: 'First' } });
    render(IconButton, { target: app, props: { label: 'Pause', busy: true } });
    screen.getByRole('button', { name: 'Pause' }).focus();
    await user().tab({ shift: true });
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'First' }));
  });

  it('keeps its label as the hint while busy, with no description and no native title: nothing flickers', async () => {
    const { rerender } = render(IconButton, { target: app, props: { label: 'Pause' } });
    const button = screen.getByRole('button', { name: 'Pause' });
    await rest(button);
    expect(bubble()?.textContent?.trim()).toBe('Pause');

    await rerender({ label: 'Pause', busy: true });
    expect(bubble()?.textContent?.trim()).toBe('Pause');
    expect(button.hasAttribute('aria-describedby')).toBe(false);
    expect(button.hasAttribute('title')).toBe(false);
  });

  it('busy and disabled with a reason: the reason is the hint and the description, both states are said, a press does nothing', async () => {
    const onclick = vi.fn();
    render(IconButton, { target: app, props: { label: 'Pause', busy: true, disabled: true, reason: 'The game is running', onclick } });
    const button = screen.getByRole('button', { name: 'Pause' });
    expect(button.hasAttribute('disabled')).toBe(false);
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(description(button)).toBe('The game is running');
    await user().click(button);
    expect(onclick).not.toHaveBeenCalled();
  });

  // The spinner stands in the icon's place: the box is the size's, busy or not, and busy keeps the
  // full colour (it is not unavailable), as Button does.
  it('keeps its box and its colour while busy', () => {
    const css = styleOf(source, 'IconButton.svelte');
    const busy = ruleBody(css, ".icon-btn[aria-busy='true']");
    expect(busy).toEqual(ruleBody(styleOf(buttonSource, 'Button.svelte'), ".btn[aria-busy='true']"));
    expect(Object.keys(busy).filter((name) => /width|height|padding|margin|font-size/.test(name))).toEqual([]);
    expect(css).not.toMatch(/aria-busy[^{]*\{[^}]*(width|height|padding)/);
    expect(source).toMatch(/\{#if busy\}<Spinner size="sm" tone="current" \/>\{:else\}\{@render children\?\.\(\)\}\{\/if\}/);
    expect(css.indexOf(".icon-btn[aria-busy='true']")).toBeGreaterThan(css.indexOf(".icon-btn[aria-disabled='true']"));
  });
});
