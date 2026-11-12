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
  // jsdom has no ResizeObserver; the corner measures the activity dock and the toast stack with one.
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
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
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
    // jsdom lays nothing out: the three boxes the shell measures are given here. A 1280 × 800 window,
    // the column down its right edge, one toast in the stack.
    const boxes = {
      lane: { left: 876, top: 48, width: 380, height: 728 },
      stack: { left: 876, top: 716, width: 380, height: 60 },
      panel: { left: 420, top: 250, width: 440, height: 300 },
    };
    const tall = { left: 340, top: 150, width: 600, height: 640 };
    const region = () => app.querySelector<HTMLElement>('.toast-region') as HTMLElement;

    beforeEach(() => {
      boxes.stack = { left: 876, top: 716, width: 380, height: 60 };
      boxes.panel = { left: 420, top: 250, width: 440, height: 300 };
      vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
        const box = this.matches('.toast-region')
          ? boxes.lane
          : this.matches('.toasts')
            ? boxes.stack
            : this.matches('[data-part="dialog"]')
              ? boxes.panel
              : { left: 0, top: 0, width: 0, height: 0 };
        return new DOMRect(box.left, box.top, box.width, box.height);
      });
    });

    afterEach(() => {
      vi.restoreAllMocks();
    });

    it('in the bottom right under a small dialog, kept live', async () => {
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();

      expect(screen.getByRole('dialog', { name: 'Install' })).toBeTruthy();
      expect(region().classList.contains('at-top')).toBe(false);
      expect(region().hasAttribute('data-layer-keep')).toBe(true);
    });

    it('at the top right while a tall dialog reaches into their corner, and back when it closes', async () => {
      boxes.panel = tall;
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      const keys = user();
      await tick();

      expect(region().classList.contains('at-top')).toBe(true);

      await keys.keyboard('{Escape}');
      await tick();

      expect(screen.queryByRole('dialog')).toBeNull();
      expect(region().classList.contains('at-top')).toBe(false);
    });

    it('are moved when the window is resized so that the dialog reaches their corner', async () => {
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(region().classList.contains('at-top')).toBe(false);

      boxes.panel = tall;
      window.dispatchEvent(new Event('resize'));
      await tick();

      expect(region().classList.contains('at-top')).toBe(true);
    });

    it('are placed by the dialog at rest, not by the smaller, lower box it enters in', async () => {
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(region().classList.contains('at-top')).toBe(false);

      boxes.panel = tall;
      screen.getByRole('dialog', { name: 'Install' }).dispatchEvent(new Event('animationend', { bubbles: true }));
      await tick();

      expect(region().classList.contains('at-top')).toBe(true);
    });

    it('are moved when the stack grows into a dialog that one toast cleared', async () => {
      boxes.panel = { left: 340, top: 200, width: 600, height: 450 };
      render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
      await tick();
      expect(region().classList.contains('at-top')).toBe(false);

      boxes.stack = { left: 876, top: 640, width: 380, height: 136 };
      resized(app.querySelector('.toasts') as Element);
      await tick();

      expect(region().classList.contains('at-top')).toBe(true);
    });

    it('stand by rules of the region alone: a column from under the title bar down to the corner, open to presses', () => {
      expect(/\.corner \{([^}]*)\}/.exec(appShellSource)?.[1]).toMatch(/pointer-events: none/);
      const rule = /\.toast-region \{([^}]*)\}/.exec(appShellSource)?.[1] ?? '';
      expect(rule).toMatch(/top: calc\(var\(--titlebar-h\) \+/);
      expect(rule).toMatch(/bottom: calc\(var\(--corner-bottom\) \+ var\(--dock-offset, 0px\)\)/);
      expect(rule).toMatch(/justify-content: flex-end/);
      expect(/\.toast-region\.at-top \{([^}]*)\}/.exec(appShellSource)?.[1]).toMatch(/justify-content: flex-start/);
    });
  });
});
