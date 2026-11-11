import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import LightboxHarness from '../testing/LightboxHarness.svelte';
import Lightbox from './Lightbox.svelte';

const items = [
  { id: 'a', url: 'https://img.test/a.png' },
  { id: 'b', url: 'https://img.test/b.png' },
];

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
});

// jsdom has no Web Animations; a test that needs Svelte's fades provides `animate` itself.
function spyOnAnimate() {
  const animate = vi.fn(() => ({ cancel: () => {}, onfinish: null }));
  Object.defineProperty(Element.prototype, 'animate', { value: animate, configurable: true, writable: true });
  return animate;
}

afterEach(() => {
  app.remove();
  Reflect.deleteProperty(Element.prototype, 'animate');
  document.documentElement.classList.remove('no-anim');
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function openViewer() {
  render(LightboxHarness, { target: app, props: { items } });
  const keys = user();
  const thumbnail = screen.getByRole('button', { name: 'Screenshot' });
  await keys.click(thumbnail);
  const dialog = screen.getByRole('dialog', { name: msg('ui.imageViewer') });
  return { keys, thumbnail, dialog };
}

// The user report behind this: an image opened from the chat could not be dismissed by a click
// around it, and its close button closed the launcher. The chat dock is `pointer-events: none`, so a
// viewer left inside it let every click through to the window chrome underneath.
describe('Lightbox placement', () => {
  it('leaves a click-through parent: the viewer is a child of the body, not of the chat dock', () => {
    const dock = document.createElement('div');
    dock.style.pointerEvents = 'none';
    app.append(dock);

    const view = render(Lightbox, { target: dock, props: { open: true, items } });

    const dialog = screen.getByRole('dialog');
    expect(dock.contains(dialog)).toBe(false);
    expect(dialog.parentElement?.parentElement).toBe(document.body);
    view.unmount();
  });
});

describe('Lightbox as a layer', () => {
  it('takes focus, keeps Tab inside, makes the page inert, and Escape returns focus to the thumbnail', async () => {
    const { keys, thumbnail, dialog } = await openViewer();
    const viewer = dialog.parentElement as HTMLElement;

    expect(viewer.contains(document.activeElement)).toBe(true);
    expect(thumbnail.closest('[inert]')).not.toBeNull();
    for (let step = 0; step < 12; step += 1) {
      await keys.tab();
      expect(viewer.contains(document.activeElement)).toBe(true);
    }

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(thumbnail.closest('[inert]')).toBeNull();
    expect(document.activeElement).toBe(thumbnail);
  });

  it('with animations off, the next image replaces the last one without a fade', async () => {
    const animate = spyOnAnimate();
    document.documentElement.classList.add('no-anim');
    const { keys } = await openViewer();

    await keys.keyboard('{ArrowRight}');

    expect(animate).not.toHaveBeenCalled();
    expect(document.querySelectorAll('img')).toHaveLength(1);
  });

  it('with animations on, the next image fades in (the check above can tell the two apart)', async () => {
    const animate = spyOnAnimate();
    const { keys } = await openViewer();

    await keys.keyboard('{ArrowRight}');

    expect(animate).toHaveBeenCalled();
  });

  it('an image that failed to load is tried again the next time the viewer opens', async () => {
    const { keys, thumbnail } = await openViewer();
    await fireEvent.error(document.querySelector('img') as HTMLImageElement);
    expect(screen.getByText(msg('ui.imageUnavailable'))).toBeTruthy();

    await keys.keyboard('{Escape}');
    await keys.click(thumbnail);

    expect(screen.queryByText(msg('ui.imageUnavailable'))).toBeNull();
    expect(document.querySelector('img')?.getAttribute('src')).toBe(items[0].url);
  });
});
