import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { openLayer } from '../layers';
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
    // Eight stops (the image, previous, next, zoom out, the slider, zoom in, reset, close): more than
    // one round, ending on reset.
    for (let step = 0; step < 14; step += 1) {
      await keys.tab();
      expect(viewer.contains(document.activeElement)).toBe(true);
    }

    // Tab ended on one of the viewer's icon buttons, whose label shows as a hint: the first Escape
    // hides the hint, the next one closes the viewer.
    expect(document.activeElement?.getAttribute('aria-label')).toBe(msg('ui.imageZoomReset'));
    expect(document.querySelector('[data-part="tooltip"]')).not.toBeNull();
    await keys.keyboard('{Escape}');
    expect(document.querySelector('[data-part="tooltip"]')).toBeNull();
    expect(screen.queryByRole('dialog')).not.toBeNull();
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

    expect(zOf(lightboxSource, '.backdrop')).toBe('var(--z-layer)');
    expect(zOf(modalSource, '.overlay')).toBe('var(--z-layer)');
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

describe('Lightbox semantics', () => {
  const three = [...items, { id: 'c', url: 'https://img.test/c.png' }];

  it('is one dialog that contains every control, and Tab never leaves it', async () => {
    render(LightboxHarness, { target: app, props: { items: three } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Screenshot' }));
    const dialog = screen.getByRole('dialog', { name: msg('ui.imageViewer') });
    expect(dialog.getAttribute('aria-modal')).toBe('true');

    for (const name of [msg('ui.previous'), msg('ui.next'), msg('ui.imageZoomIn'), msg('common.close')]) {
      expect(dialog.contains(screen.getByRole('button', { name })), name).toBe(true);
    }
    for (let step = 0; step < 10; step += 1) {
      await keys.tab();
      expect(document.activeElement?.closest('[role="dialog"]')).toBe(dialog);
    }
  });

  it('announces the position when the image changes, and Home/End jump to the first and last image', async () => {
    document.documentElement.classList.add('no-anim');
    render(LightboxHarness, { target: app, props: { items: three } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Screenshot' }));
    const status = screen.getByRole('status');
    const image = () => document.querySelector('img')?.getAttribute('src');
    expect(status.textContent?.trim()).toBe('1 / 3');

    await keys.keyboard('{ArrowRight}');
    expect(status.textContent?.trim()).toBe('2 / 3');
    await keys.keyboard('{End}');
    expect(image()).toBe(three[2].url);
    expect(status.textContent?.trim()).toBe('3 / 3');
    await keys.keyboard('{Home}');
    expect(image()).toBe(three[0].url);
  });
});

// A disabled button cannot hold focus: Chromium drops it to <body>, out of the dialog. At a limit
// the zoom buttons stay focusable, say they are unavailable and ignore the press.
describe('Lightbox zoom at its limits', () => {
  const zoomValue = () => document.querySelector('.zoom-value')?.textContent?.trim();

  it('Zoom out pressed down to 100% keeps focus, says it is unavailable and ignores further presses', async () => {
    const { keys, dialog } = await openViewer();
    const zoomIn = within(dialog).getByRole('button', { name: msg('ui.imageZoomIn') });
    const zoomOut = within(dialog).getByRole('button', { name: msg('ui.imageZoomOut') });
    zoomIn.focus();
    await keys.keyboard('{Enter}');
    expect(zoomValue()).toBe('125%');
    zoomOut.focus();

    await keys.keyboard('{Enter}');
    expect(zoomValue()).toBe('100%');
    expect((zoomOut as HTMLButtonElement).disabled).toBe(false);
    expect(zoomOut.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(zoomOut);

    await keys.keyboard('{Enter}');
    expect(zoomValue()).toBe('100%');
    expect(document.activeElement).toBe(zoomOut);
  });

  it('Zoom in pressed up to 400% keeps focus and ignores further presses; keys still zoom', async () => {
    const { keys, dialog } = await openViewer();
    const zoomIn = within(dialog).getByRole('button', { name: msg('ui.imageZoomIn') });
    zoomIn.focus();

    for (let press = 0; press < 12; press += 1) await keys.keyboard('{Enter}');

    expect(zoomValue()).toBe('400%');
    expect((zoomIn as HTMLButtonElement).disabled).toBe(false);
    expect(zoomIn.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(zoomIn);

    await keys.keyboard('-');
    expect(zoomValue()).toBe('375%');
    expect(zoomIn.hasAttribute('aria-disabled')).toBe(false);
    await keys.keyboard('0');
    expect(zoomValue()).toBe('100%');
    await keys.keyboard('+');
    expect(zoomValue()).toBe('125%');
  });
});

describe('Lightbox under a non-modal layer', () => {
  it('a tooltip open above the viewer does not take its keys', async () => {
    document.documentElement.classList.add('no-anim');
    const { keys } = await openViewer();
    const tip = document.createElement('div');
    tip.setAttribute('role', 'tooltip');
    tip.textContent = 'Zoom';
    document.body.append(tip);
    const close = openLayer(tip, { modal: false, dismiss: () => {} });

    await keys.keyboard('{ArrowRight}');

    expect(document.querySelector('img')?.getAttribute('src')).toBe(items[1].url);
    close();
    tip.remove();
  });
});
