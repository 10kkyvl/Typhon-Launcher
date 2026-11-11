import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { user } from '../testing/dom';
import { HOVER_DELAY } from './hoverIntent';
import IconButton from './IconButton.svelte';

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

  it('a disabled button shows no tooltip', async () => {
    render(IconButton, { target: app, props: { label: 'Close', disabled: true } });
    await rest(screen.getByRole('button', { name: 'Close' }));
    expect(bubble()).toBeNull();
  });
});
