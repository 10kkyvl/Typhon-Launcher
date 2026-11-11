import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import Spinner from './Spinner.svelte';
import source from './Spinner.svelte?raw';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

describe('Spinner', () => {
  it('with a label is a status that says it', () => {
    render(Spinner, { target: app, props: { label: 'Loading' } });
    const status = screen.getByRole('status');
    expect(status.textContent?.trim()).toBe('Loading');
    expect(status.querySelector('.ring')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('without a label is decoration: no status, hidden from assistive technology', () => {
    render(Spinner, { target: app });
    expect(screen.queryByRole('status')).toBeNull();
    expect(app.querySelector('.spinner')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('takes a size, and the colour of the text around it inside a filled control', () => {
    render(Spinner, { target: app, props: { size: 'lg', tone: 'current' } });
    const spinner = app.querySelector('.spinner');
    expect(spinner?.classList.contains('lg')).toBe(true);
    expect(spinner?.classList.contains('current')).toBe(true);
    expect(source).toMatch(/\.spinner \{[^}]*color: var\(--accent-bright\);/);
    expect(source).toMatch(/\.current \{\s*color: inherit;/);
  });

  it('stands still when animations are off: the launcher setting and the system one', () => {
    expect(source).toMatch(/:global\(\.no-anim\) \.ring \{\s*animation: none;/);
    expect(source).toMatch(/@media \(prefers-reduced-motion: reduce\) \{\s*\.ring \{\s*animation: none;/);
  });

  it('turns at the shared spin duration and keeps its arc as a border, so the still ring shows it', () => {
    expect(source).toMatch(/animation: spinner-turn var\(--motion-spin\) linear infinite/);
    expect(source).toMatch(/border-top-color: currentColor/);
  });
});
