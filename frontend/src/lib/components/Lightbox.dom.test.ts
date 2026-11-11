import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import LightboxHarness from '../testing/LightboxHarness.svelte';
import Lightbox from './Lightbox.svelte';
import lightboxSource from './Lightbox.svelte?raw';
import Modal from './Modal.svelte';
import modalSource from './Modal.svelte?raw';

// The portalled root of whatever layer holds `el`: the body's child.
function layerRoot(el: Element): HTMLElement {
  let node = el as HTMLElement;
  while (node.parentElement && node.parentElement !== document.body) node = node.parentElement;
  return node;
}

function zOf(source: string, selector: string): string | undefined {
  return new RegExp(`\\${selector} \\{[^}]*z-index: ([^;]+);`).exec(source)?.[1];
}

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
    expect(layerRoot(dialog).parentElement).toBe(document.body);
    view.unmount();
  });
});

describe('Lightbox as a layer', () => {
  it('takes focus, keeps Tab inside, makes the page inert, and Escape returns focus to the thumbnail', async () => {
    const { keys, thumbnail, dialog } = await openViewer();
    const viewer = layerRoot(dialog);

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

describe('Lightbox under a dialog that opens after it', () => {
  it('the later dialog paints above it and owns the keys: arrows leave the image, Escape closes the dialog, then the viewer', async () => {
    document.documentElement.classList.add('no-anim');
    const { keys, dialog } = await openViewer();
    const viewer = layerRoot(dialog);
    const image = () => document.querySelector('img')?.getAttribute('src');
    const shown = image();

    render(Modal, { target: app, props: { open: true, title: 'The game closed' } });
    const crash = layerRoot(screen.getByRole('dialog', { name: 'The game closed' }));

    expect(zOf(lightboxSource, '.backdrop')).toBe('var(--z-dialog)');
    expect(zOf(modalSource, '.overlay')).toBe('var(--z-dialog)');
    expect(viewer.compareDocumentPosition(crash) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(crash.contains(document.activeElement)).toBe(true);

    await keys.keyboard('{ArrowRight}');
    await keys.keyboard('{End}');
    expect(image()).toBe(shown);

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog', { name: 'The game closed' })).toBeNull();
    expect(screen.getByRole('dialog', { name: msg('ui.imageViewer') })).toBeTruthy();

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
