import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import ModalHarness from '../testing/ModalHarness.svelte';
import modalSource from './Modal.svelte?raw';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  app.remove();
});

function page(props: { dismissible?: boolean; onclose?: () => void; oninnerclose?: () => void } = {}) {
  const view = render(ModalHarness, { target: app, props });
  const keys = user();
  const opener = screen.getByRole('button', { name: 'Open settings' });
  async function openDialog() {
    await keys.click(opener);
    return screen.getByRole('dialog', { name: 'Settings' });
  }
  return { ...view, keys, opener, openDialog };
}

describe('Modal in a DOM', () => {
  it('is a modal dialog labelled by its visible title', async () => {
    const { openDialog } = page();
    const dialog = await openDialog();

    expect(dialog.getAttribute('aria-modal')).toBe('true');
    const label = dialog.getAttribute('aria-labelledby');
    expect(label).toBeTruthy();
    expect(document.getElementById(label as string)?.textContent).toBe('Settings');
  });

  it('moves focus to its first control on open; Tab and Shift+Tab cycle inside it', async () => {
    const { keys, openDialog } = page();
    const dialog = await openDialog();
    const name = screen.getByRole('textbox', { name: 'Name' });
    expect(document.activeElement).toBe(name);

    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'More' }));
    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Save' }));
    await keys.tab();
    expect(dialog.contains(document.activeElement)).toBe(true);
    await keys.tab({ shift: true });
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Save' }));
  });

  it('Escape closes it once and returns focus to the button that opened it', async () => {
    const onclose = vi.fn();
    const { keys, opener, openDialog } = page({ onclose });
    const dialog = await openDialog();
    expect(dialog.contains(document.activeElement)).toBe(true);

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(onclose).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(opener);
  });

  it('nested: Escape closes only the top dialog and focus goes back into the one under it', async () => {
    const onclose = vi.fn();
    const oninnerclose = vi.fn();
    const { keys, openDialog } = page({ onclose, oninnerclose });
    await openDialog();
    const more = screen.getByRole('button', { name: 'More' });
    await keys.click(more);
    expect(screen.getByRole('dialog', { name: 'More settings' }).contains(document.activeElement)).toBe(true);

    await keys.keyboard('{Escape}');

    expect(oninnerclose).toHaveBeenCalledTimes(1);
    expect(onclose).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
    expect(document.activeElement).toBe(more);
  });

  it('a click on the scrim closes it; a press inside released on the scrim does not', async () => {
    const onclose = vi.fn();
    const { keys, openDialog } = page({ onclose });
    const dialog = await openDialog();
    const scrim = dialog.parentElement as HTMLElement;

    await keys.pointer([{ keys: '[MouseLeft>]', target: screen.getByRole('textbox', { name: 'Name' }) }, { keys: '[/MouseLeft]', target: scrim }]);
    expect(screen.queryByRole('dialog')).not.toBeNull();

    await keys.click(scrim);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('dismissible={false}: Escape, the scrim and the close button do nothing', async () => {
    const onclose = vi.fn();
    const { keys, openDialog } = page({ dismissible: false, onclose });
    const dialog = await openDialog();
    const close = screen.getByRole('button', { name: msg('common.close') });

    await keys.keyboard('{Escape}');
    await keys.click(dialog.parentElement as HTMLElement);
    await keys.click(close);

    expect(close).toHaveProperty('disabled', true);
    expect(screen.queryByRole('dialog')).not.toBeNull();
    expect(onclose).not.toHaveBeenCalled();
  });

  it('the close button closes it', async () => {
    const onclose = vi.fn();
    const { keys, openDialog } = page({ onclose });
    await openDialog();

    await keys.click(screen.getByRole('button', { name: msg('common.close') }));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('makes the page behind inert while open, and only while open', async () => {
    const { keys, opener, openDialog } = page();
    await openDialog();
    expect(opener.closest('[inert]')).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Elsewhere', hidden: true }).closest('[inert]')).not.toBeNull();

    await keys.click(screen.getByRole('button', { name: 'Save' }));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(opener.closest('[inert]')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  // jsdom lays nothing out, so this holds the rules and the element they read the room from; that
  // the footer really ends above a toast is checked in the running app.
  it('leaves the room a toast stack asks for: its layer root reads --toast-room and the panel ends that far above the bottom', async () => {
    const { openDialog } = page();
    const dialog = await openDialog();
    const root = dialog.parentElement as HTMLElement;
    const overlay = /\.overlay \{([^}]*)\}/.exec(modalSource)?.[1] ?? '';
    const panel = /\.modal \{([^}]*)\}/.exec(modalSource)?.[1] ?? '';

    expect(root.classList.contains('overlay')).toBe(true);
    // Nothing asked: the panel keeps its usual distance from the bottom and the scrim no padding.
    expect(overlay).toMatch(/--toast-room: 0px;/);
    expect(overlay).toMatch(/--dialog-clear: max\(var\(--dialog-edge\), var\(--toast-room\)\);/);
    expect(overlay).toMatch(/padding-bottom: calc\(var\(--dialog-clear\) - var\(--dialog-edge\)\);/);
    expect(panel).toMatch(/max-height: calc\(100vh - var\(--titlebar-h\) - var\(--dialog-edge\) - var\(--dialog-clear\)\);/);
  });
});
