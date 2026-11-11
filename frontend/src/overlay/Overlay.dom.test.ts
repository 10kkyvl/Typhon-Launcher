import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../lib/testing/dom')).wailsRuntime());

import Overlay from './Overlay.svelte';
import { locale, msg } from '../lib/i18n';
import { user } from '../lib/testing/dom';

let app: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  locale.set('en');
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
  locale.set('ru');
});

describe('the overlay browser button', () => {
  it('opens the browser pane and then says the pane is open, lit as before, without claiming a pressed state', async () => {
    render(Overlay, { target: app });
    const globe = screen.getByRole('button', { name: msg('overlay.browserOpen') });
    expect(globe.getAttribute('aria-expanded')).toBe('false');
    expect(globe.classList.contains('lit')).toBe(false);
    expect(screen.queryByRole('region', { name: msg('overlay.browserLabel') })).toBeNull();

    await user().click(globe);

    expect(screen.getByRole('region', { name: msg('overlay.browserLabel') })).toBeTruthy();
    expect(globe.getAttribute('aria-expanded')).toBe('true');
    expect(globe.classList.contains('lit')).toBe(true);
    expect(globe.hasAttribute('aria-pressed')).toBe(false);
  });
});
