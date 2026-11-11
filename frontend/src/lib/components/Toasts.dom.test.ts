import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { Theme } from '../services/theme';
import { activeTheme, endTrial, themeTrial, tryTheme } from '../stores/theme';
import { ACTION_MS, PLAIN_MS, RESUME_FLOOR_MS, holdToasts, toast, toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import ToastsHarness from '../testing/ToastsHarness.svelte';
import { mountThemeGuard } from './ThemeGuard';
import source from './Toasts.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

const theme = (id: string): Theme => ({ id, name: id, base: 'dark', tokens: {}, builtIn: id === 'dark', updatedAt: '' });

let app: HTMLDivElement;

beforeEach(() => {
  vi.useFakeTimers();
  toasts.set([]);
  holdToasts(false);
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  toasts.set([]);
  holdToasts(false);
  document.body.replaceChildren();
});

function setup(props: { dialog?: boolean } = {}) {
  const view = render(ToastsHarness, { target: app, props });
  const keys = user();
  const region = () => screen.getByRole('region', { name: msg('ui.toastsLabel') });
  const urgent = () => within(region()).getByRole('alert');
  const polite = () => within(region()).getByRole('status');
  const shown = () => get(toasts).map((item) => item.message);
  const card = (text: string) => screen.getByText(text).closest<HTMLElement>('[data-part="toast"]') as HTMLElement;
  const close = (text: string) => within(card(text)).getByRole('button', { name: msg('ui.toastDismiss') });
  const pass = async (ms: number) => {
    vi.advanceTimersByTime(ms);
    await tick();
  };
  return { keys, region, urgent, polite, shown, card, close, pass, rerender: view.rerender };
}

describe('Toasts: what a screen reader is told', () => {
  it('has its regions before the first toast: a named region with an empty alert and an empty status', () => {
    const { region, urgent, polite } = setup();

    expect(region()).toBeTruthy();
    expect(urgent().textContent?.trim()).toBe('');
    expect(polite().textContent?.trim()).toBe('');
  });

  it('puts an error and a warning in the alert region, the rest in the status one', async () => {
    const { urgent, polite } = setup();

    toast('Disk is full', 'danger');
    toast('Saved');
    await tick();
    expect(within(urgent()).getByText('Disk is full')).toBeTruthy();
    expect(within(polite()).getByText('Saved')).toBeTruthy();
    expect(within(urgent()).queryByText('Saved')).toBeNull();

    toasts.set([]);
    toast('Almost out of space', 'warning');
    toast('Installed', 'success');
    await tick();
    expect(within(urgent()).getByText('Almost out of space')).toBeTruthy();
    expect(within(polite()).getByText('Installed')).toBeTruthy();
  });

  it('keeps the same regions as toasts come and go', async () => {
    const { urgent, polite, pass } = setup();
    const before = [urgent(), polite()];

    toast('Saved');
    await pass(PLAIN_MS);
    toast('Disk is full', 'danger');
    await tick();

    expect([urgent(), polite()]).toEqual(before);
  });

  it('says a new toast alone: the lanes are not atomic, so the toasts already there are not read again', () => {
    const { urgent, polite } = setup();

    expect(urgent().getAttribute('aria-atomic')).toBe('false');
    expect(polite().getAttribute('aria-atomic')).toBe('false');
  });

  it('gives the close button a name but no text of its own to be read out with the message', async () => {
    const { close } = setup();
    toast('Saved');
    await tick();

    expect(close('Saved').getAttribute('aria-label')).toBe(msg('ui.toastDismiss'));
    expect(close('Saved').textContent?.trim()).toBe('');
    expect(close('Saved').closest('[aria-hidden="true"]')).toBeNull();
  });

  it('marks each toast as a theme part and hides its icon from the tree', async () => {
    const { card } = setup();
    toast('Saved');
    await tick();

    expect(card('Saved').dataset.part).toBe('toast');
    expect(card('Saved').querySelector('svg')?.getAttribute('aria-hidden')).toBe('true');
  });
});

describe('Toasts: a message raised again', () => {
  it('shows how many times it happened, in a sign for the eye and in words for a screen reader', async () => {
    const { card } = setup();
    toast('Image is too large', 'danger');
    await tick();
    expect(card('Image is too large').querySelector('.times')).toBeNull();

    toast('Image is too large', 'danger');
    toast('Image is too large', 'danger');
    await tick();

    const message = card('Image is too large').querySelector('.message') as HTMLElement;
    const sign = message.querySelector('.times') as HTMLElement;
    expect(sign.textContent?.trim()).toBe('×3');
    expect(sign.getAttribute('aria-hidden')).toBe('true');
    expect(message.querySelector('.sr-only')?.textContent?.trim()).toBe(msg('ui.toastRepeated', { count: 3 }));
  });

  it('is said again whole when the count changes: the message is one atomic piece, without the buttons', async () => {
    const { card } = setup();
    toast('Install failed', 'danger');
    await tick();

    const message = card('Install failed').querySelector('.message') as HTMLElement;
    expect(message.getAttribute('aria-atomic')).toBe('true');
    expect(message.querySelector('button')).toBeNull();
  });
});

describe('Toasts: closing', () => {
  it('every toast has a named close button that closes it', async () => {
    const { keys, close, shown } = setup();
    toast('Saved');
    toast('Disk is full', 'danger');
    await tick();

    await keys.click(close('Saved'));

    expect(shown()).toEqual(['Disk is full']);
  });

  it('the action runs once and the toast goes', async () => {
    const { keys, card, shown } = setup();
    const run = vi.fn();
    toast('Install failed', 'danger', { label: 'Retry', run });
    await tick();

    await keys.click(within(card('Install failed')).getByRole('button', { name: 'Retry' }));

    expect(run).toHaveBeenCalledTimes(1);
    expect(shown()).toEqual([]);
  });

  it('reached by Tab, one Escape closes the toast and puts focus back where it was', async () => {
    const { keys, close, shown } = setup();
    const before = screen.getByRole('button', { name: 'Before' });
    toast('Saved');
    await tick();
    before.focus();
    await keys.tab();
    expect(document.activeElement).toBe(close('Saved'));

    await keys.keyboard('{Escape}');

    expect(shown()).toEqual([]);
    expect(document.activeElement).toBe(before);
  });

  it('shows no hint of its own on the close button: nothing stands between Escape and the toast', async () => {
    const { keys, close, pass } = setup();
    toast('Saved');
    await tick();
    screen.getByRole('button', { name: 'Before' }).focus();
    await keys.tab();
    await pass(1000);

    expect(document.querySelector('[data-part="tooltip"]')).toBeNull();
    expect(close('Saved').getAttribute('aria-label')).toBe(msg('ui.toastDismiss'));
  });

  it('keeps its Escape to itself: a theme being tried is not ended by closing a toast', async () => {
    const unmount = mountThemeGuard();
    activeTheme.set(theme('dark'));
    expect(tryTheme(theme('draft'))).toBeNull();
    try {
      const { keys, shown } = setup();
      toast('Saved');
      await tick();
      screen.getByRole('button', { name: 'Before' }).focus();
      await keys.tab();

      await keys.keyboard('{Escape}');

      expect(shown()).toEqual([]);
      expect(get(themeTrial)).not.toBeNull();
    } finally {
      endTrial();
      unmount();
      activeTheme.set(null);
    }
  });

  it('closing by its button from the keyboard puts focus back where it was too', async () => {
    const { keys, close, shown } = setup();
    const before = screen.getByRole('button', { name: 'Before' });
    toast('Saved');
    await tick();
    before.focus();
    close('Saved').focus();

    await keys.keyboard('{Enter}');

    expect(shown()).toEqual([]);
    expect(document.activeElement).toBe(before);
  });

  it('the stack is reached by Tab after the page', async () => {
    const { keys, card } = setup();
    toast('Saved');
    await tick();
    screen.getByRole('button', { name: 'Before' }).focus();

    await keys.tab();

    expect(card('Saved').contains(document.activeElement)).toBe(true);
  });
});

describe('Toasts: being read', () => {
  it('the pointer over a toast stops its clock; leaving gives it a moment and lets it go', async () => {
    const { card, shown, pass } = setup();
    toast('Saved');
    await pass(PLAIN_MS - 100);

    await fireEvent.pointerEnter(card('Saved'));
    await pass(60_000);
    expect(shown()).toEqual(['Saved']);

    await fireEvent.pointerLeave(card('Saved'));
    await pass(RESUME_FLOOR_MS - 1);
    expect(shown()).toEqual(['Saved']);
    await pass(1);
    expect(shown()).toEqual([]);
  });

  it('focus inside a toast stops its clock; focus leaving lets it go', async () => {
    const { close, shown, pass } = setup();
    toast('Saved');
    await tick();

    close('Saved').focus();
    await pass(60_000);
    expect(shown()).toEqual(['Saved']);

    screen.getByRole('button', { name: 'Before' }).focus();
    await pass(PLAIN_MS);
    expect(shown()).toEqual([]);
  });

  it('focus moving between the buttons of one toast does not start the clock', async () => {
    const { card, close, shown, pass } = setup();
    toast('Install failed', 'danger', { label: 'Retry', run: () => {} });
    await tick();

    within(card('Install failed')).getByRole('button', { name: 'Retry' }).focus();
    close('Install failed').focus();
    await pass(60_000);

    expect(shown()).toEqual(['Install failed']);
  });

  it('stays while the pointer is still on it though focus has left', async () => {
    const { card, close, shown, pass } = setup();
    toast('Saved');
    await tick();
    await fireEvent.pointerEnter(card('Saved'));
    close('Saved').focus();

    screen.getByRole('button', { name: 'Before' }).focus();
    await pass(60_000);

    expect(shown()).toEqual(['Saved']);
  });
});

describe('Toasts: while a dialog is open', () => {
  it('a toast with an action waits for the dialog to close, then has its full time', async () => {
    const { shown, pass, rerender } = setup({ dialog: true });
    await tick();
    toast('Install failed', 'danger', { label: 'Retry', run: () => {} });

    await pass(60_000);
    expect(shown()).toEqual(['Install failed']);

    await rerender({ dialog: false });
    await tick();
    await pass(ACTION_MS - 1);
    expect(shown()).toEqual(['Install failed']);
    await pass(1);
    expect(shown()).toEqual([]);
  });

  it('is outside the inert page and can still be pressed', async () => {
    const { keys, close, shown } = setup({ dialog: true });
    await tick();
    toast('Saved');
    await tick();

    expect(close('Saved').closest('[inert]')).toBeNull();
    await keys.click(close('Saved'));

    expect(shown()).toEqual([]);
  });
});

describe('Toasts: a long message', () => {
  it('wraps anywhere inside a toast that cannot grow past the window', () => {
    const message = /\.message\s*\{([^}]*)\}/.exec(source)?.[1] ?? '';
    const box = /\.toast\s*\{([^}]*)\}/.exec(source)?.[1] ?? '';

    expect(message).toMatch(/overflow-wrap:\s*anywhere/);
    expect(message).toMatch(/min-width:\s*0/);
    expect(box).toMatch(/max-width:\s*min\(/);
  });
});
