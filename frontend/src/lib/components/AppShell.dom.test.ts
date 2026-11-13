import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import ToastHarness from '../testing/ToastHarness.svelte';
import appShellSource from './AppShell.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// Read from disk: vitest serves `.css?raw` as an empty string, and under jsdom import.meta.url is not a file URL.
type Fs = { readFileSync(path: string, encoding: 'utf8'): string };
const node = (globalThis as unknown as { process: { cwd(): string; getBuiltinModule(id: 'node:fs'): Fs } }).process;
const shellCss = node.getBuiltinModule('node:fs').readFileSync(`${node.cwd()}/src/styles/shell.css`, 'utf8');
const z = (name: string) => Number(new RegExp(`--z-${name}:\\s*(-?\\d+);`).exec(shellCss)?.[1]);

let app: HTMLDivElement;

interface Observer {
  changed(): void;
  watches(element: Element): boolean;
}
const observers = new Set<Observer>();

/** What a browser does when `element` changes size: every observer watching it is called. */
function resized(element: Element): void {
  for (const observer of observers) if (observer.watches(element)) observer.changed();
}

beforeEach(() => {
  // jsdom has no ResizeObserver; the toast region measures the stack and the dialog's panel with one.
  observers.clear();
  vi.stubGlobal('ResizeObserver', class {
    #watched = new Set<Element>();
    constructor(readonly changed: () => void) {
      observers.add(this);
    }
    observe(element: Element) {
      this.#watched.add(element);
    }
    unobserve(element: Element) {
      this.#watched.delete(element);
    }
    disconnect() {
      this.#watched.clear();
    }
    watches(element: Element) {
      return this.#watched.has(element);
    }
  });
  // The toast stack is where toasts show with motion off; with it on the live island shows them
  // (LiveIsland.toasts.dom.test.ts).
  document.documentElement.classList.add('no-anim');
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.documentElement.classList.remove('no-anim');
  app.remove();
  toasts.set([]);
  vi.unstubAllGlobals();
});

describe('toasts raised from an open dialog', () => {
  it('are outside the inert app, on a layer above dialogs, and their action can be pressed', async () => {
    const onretry = vi.fn();
    render(ToastHarness, { target: app, props: { onretry } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: 'Install' });
    expect(app.querySelector('.shell')?.closest('[inert]')).not.toBeNull();

    await keys.click(screen.getByRole('button', { name: 'Install' }));

    const message = screen.getByText('Install failed');
    expect(message.closest('[inert]')).toBeNull();
    expect(dialog.contains(message)).toBe(false);
    expect(appShellSource).toMatch(/\.toast-region \{[^}]*z-index: var\(--z-toast\)/);
    expect(z('toast')).toBeGreaterThan(z('layer'));
    expect(z('toast')).toBeLessThan(z('titlebar'));

    await keys.click(screen.getByRole('button', { name: 'Retry' }));

    expect(onretry).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('dialog', { name: 'Install' })).toBeTruthy();
  });

  describe('where they stand', () => {
    // jsdom lays nothing out: the boxes the shell measures are given here. A 1280 × 800 window with a
    // 10px rem, the column down its right edge (it ends 24 from the right and the bottom), one toast
    // in the stack. A dialog's layer root covers the window.
    const WINDOW = { left: 0, top: 0, width: 1280, height: 800 };
    const SMALL = { left: 420, top: 250, width: 440, height: 300 };
    const TALL = { left: 340, top: 150, width: 600, height: 640 };
    // Both corners taken; 248 are left beside it, enough for a toast.
    const TALL_FROM_TOP = { left: 300, top: 56, width: 700, height: 734 };
    // Both corners taken and nothing beside it.
    const WIDE = { left: 40, top: 56, width: 1200, height: 734 };
    const ONE_TOAST = { left: 876, top: 716, width: 380, height: 60 };
    const boxes = { lane: { left: 876, top: 48, width: 380, height: 728 }, stack: ONE_TOAST, panel: SMALL, confirm: SMALL };
    const region = () => app.querySelector<HTMLElement>('.toast-region') as HTMLElement;
    const stack = () => app.querySelector('.toasts') as Element;
    const root = (name: string) => screen.getByRole('dialog', { name }).parentElement as HTMLElement;
    const roomOf = (name: string) => root(name).style.getPropertyValue('--toast-room');
    // What follows a change of size is done in the next frame.
    const frame = async () => {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      await tick();
    };

    beforeEach(() => {
      Object.assign(boxes, { stack: ONE_TOAST, panel: SMALL, confirm: SMALL });
      document.documentElement.style.fontSize = '10px';
      vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
        const panel = () => (this.querySelector('h3')?.textContent === 'Confirm' ? boxes.confirm : boxes.panel);
        const box = this.matches('.toast-region')
          ? boxes.lane
          : this.matches('.toasts')
            ? boxes.stack
            : this.matches('[data-part="dialog"]')
              ? panel()
              : this.matches('.overlay')
                ? WINDOW
                : { left: 0, top: 0, width: 0, height: 0 };
        return new DOMRect(box.left, box.top, box.width, box.height);
      });
    });

    afterEach(() => {
      vi.restoreAllMocks();
      document.documentElement.style.fontSize = '';
    });

    it('in the bottom right under a small dialog, kept live, as wide as they like and with nothing asked', async () => {
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();

      expect(region().classList.contains('at-top')).toBe(false);
      expect(region().hasAttribute('data-layer-keep')).toBe(true);
      expect(region().style.maxWidth).toBe('');
      expect(roomOf('Install')).toBe('');
    });

    it('at the top right while a tall dialog reaches into their corner, and back when it closes', async () => {
      boxes.panel = TALL;
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      const keys = user();
      await tick();

      expect(region().classList.contains('at-top')).toBe(true);

      await keys.keyboard('{Escape}');
      await tick();

      expect(screen.queryByRole('dialog')).toBeNull();
      expect(region().classList.contains('at-top')).toBe(false);
    });

    it('beside a dialog that takes both corners, narrowed to the gutter; as wide as they like again when it closes', async () => {
      boxes.panel = TALL_FROM_TOP;
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      const keys = user();
      await tick();

      expect(region().style.maxWidth).toBe('248px');
      expect(region().classList.contains('at-top')).toBe(false);
      expect(roomOf('Install')).toBe('');

      await keys.keyboard('{Escape}');
      await tick();

      expect(region().style.maxWidth).toBe('');
    });

    it('under a dialog with no clear spot and no gutter: the dialog on top is asked for the room the stack needs', async () => {
      boxes.panel = WIDE;
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();

      // From the window's bottom up to the stack's top (800 - 716) and a gap of 8.
      expect(roomOf('Install')).toBe('92px');
      expect(root('Install').classList.contains('overlay')).toBe(true);
      expect(region().style.maxWidth).toBe('');
      expect(region().classList.contains('at-top')).toBe(false);
    });

    it('the room follows the stack, is not given up because the dialog now ends above it, and goes with the last toast', async () => {
      boxes.panel = WIDE;
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(roomOf('Install')).toBe('92px');

      // The dialog made room: measured now, it is clear of the stack.
      boxes.panel = { ...WIDE, height: 652 };
      boxes.stack = { left: 876, top: 640, width: 380, height: 136 };
      resized(stack());
      await frame();
      expect(roomOf('Install')).toBe('168px');

      boxes.stack = { left: 1256, top: 776, width: 0, height: 0 };
      resized(stack());
      await frame();
      expect(roomOf('Install')).toBe('');
    });

    it('no room is asked before there is a toast', async () => {
      boxes.panel = WIDE;
      boxes.stack = { left: 1256, top: 776, width: 0, height: 0 };
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(roomOf('Install')).toBe('');

      boxes.stack = ONE_TOAST;
      resized(stack());
      await frame();

      expect(roomOf('Install')).toBe('92px');
    });

    it('are placed again when the window changes size, from the dialog at rest, and without a listener on the window', async () => {
      const listened = vi.spyOn(window, 'addEventListener');
      boxes.panel = WIDE;
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(roomOf('Install')).toBe('92px');

      // A larger window: the same dialog now leaves the bottom right clear.
      boxes.panel = SMALL;
      resized(document.documentElement);
      await frame();

      expect(roomOf('Install')).toBe('');
      expect(listened.mock.calls.map(([type]) => type)).not.toContain('resize');
    });

    it('are placed by the dialog at rest, not by the smaller, lower box it enters in', async () => {
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(region().classList.contains('at-top')).toBe(false);

      boxes.panel = TALL;
      screen.getByRole('dialog', { name: 'Install' }).dispatchEvent(new Event('animationend', { bubbles: true }));
      await tick();

      expect(region().classList.contains('at-top')).toBe(true);
    });

    it('room asked of a dialog while it entered is taken back if it rests clear; an animation inside it changes nothing', async () => {
      boxes.panel = WIDE;
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      const dialog = screen.getByRole('dialog', { name: 'Install' });
      expect(roomOf('Install')).toBe('92px');

      boxes.panel = SMALL;
      (dialog.querySelector('h3') as HTMLElement).dispatchEvent(new Event('animationend', { bubbles: true }));
      await tick();
      expect(roomOf('Install')).toBe('92px');

      dialog.dispatchEvent(new Event('animationend', { bubbles: true }));
      await tick();
      expect(roomOf('Install')).toBe('');
    });

    it('are moved when the stack grows into a dialog that one toast cleared', async () => {
      boxes.panel = { left: 340, top: 200, width: 600, height: 450 };
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(region().classList.contains('at-top')).toBe(false);

      boxes.stack = { left: 876, top: 640, width: 380, height: 136 };
      resized(stack());
      await frame();

      expect(region().classList.contains('at-top')).toBe(true);
    });

    it('only the dialog on top counts: a small one opened over a wide one takes the room back from it', async () => {
      boxes.panel = WIDE;
      const view = render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(roomOf('Install')).toBe('92px');

      await view.rerender({ onretry: vi.fn(), confirm: true });
      await tick();

      expect(roomOf('Install')).toBe('');
      expect(roomOf('Confirm')).toBe('');

      boxes.confirm = WIDE;
      screen.getByRole('dialog', { name: 'Confirm' }).dispatchEvent(new Event('animationend', { bubbles: true }));
      await tick();

      expect(roomOf('Confirm')).toBe('92px');
      expect(roomOf('Install')).toBe('');
    });

    it('stand by rules of the region alone: a column from under the title bar down to the corner, open to presses', () => {
      expect(/\.corner \{([^}]*)\}/.exec(appShellSource)?.[1]).toMatch(/pointer-events: none/);
      const rule = /\.toast-region \{([^}]*)\}/.exec(appShellSource)?.[1] ?? '';
      expect(rule).toMatch(/top: calc\(var\(--titlebar-h\) \+/);
      // Its foot is the corner's own: nothing else stands in the corner to make room for.
      expect(/\.corner \{([^}]*)\}/.exec(appShellSource)?.[1]).toMatch(/bottom: var\(--corner-bottom\)/);
      expect(rule).not.toMatch(/bottom:/);
      expect(appShellSource).not.toMatch(/--dock-offset/);
      expect(rule).toMatch(/justify-content: flex-end/);
      expect(/\.toast-region\.at-top \{([^}]*)\}/.exec(appShellSource)?.[1]).toMatch(/justify-content: flex-start/);
    });
  });
});
