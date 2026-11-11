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

beforeEach(() => {
  // jsdom has no ResizeObserver; the corner measures the activity dock with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
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

  it('stand at the top right under the title bar while the dialog is open, and back in the corner after', async () => {
    render(ToastHarness, { target: app, props: { onretry: vi.fn() } });
    const keys = user();
    const region = () => app.querySelector<HTMLElement>('.toast-region') as HTMLElement;
    await tick();

    expect(region().classList.contains('over-dialog')).toBe(true);
    expect(region().hasAttribute('data-layer-keep')).toBe(true);
    const rule = /\.toast-region\.over-dialog \{([^}]*)\}/.exec(appShellSource)?.[1] ?? '';
    expect(rule).toMatch(/top: calc\(var\(--titlebar-h\) \+/);
    expect(rule).toMatch(/bottom: auto/);

    await keys.keyboard('{Escape}');
    await tick();

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(region().classList.contains('over-dialog')).toBe(false);
  });
});
