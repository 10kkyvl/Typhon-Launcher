import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Lightbox from './Lightbox.svelte';

const items = [
  { id: 'a', url: 'https://img.test/a.png' },
  { id: 'b', url: 'https://img.test/b.png' },
];

beforeEach(() => {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// The user report behind this: an image opened from the chat could not be dismissed by a click
// around it, and its close button closed the launcher. The chat dock is `pointer-events: none`, so a
// viewer left inside it let every click through to the window chrome underneath.
describe('Lightbox placement', () => {
  it('leaves a click-through parent: the viewer is a child of the body, not of the chat dock', () => {
    const dock = document.createElement('div');
    dock.style.pointerEvents = 'none';
    document.body.append(dock);

    const view = render(Lightbox, { target: dock, props: { open: true, items } });

    const dialog = screen.getByRole('dialog');
    expect(dock.contains(dialog)).toBe(false);
    expect(dialog.parentElement?.parentElement).toBe(document.body);
    view.unmount();
    dock.remove();
  });
});
