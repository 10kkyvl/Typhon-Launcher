import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ConfirmPrompt } from '../confirm/prompts';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import ConfirmModal from './ConfirmModal.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  app.remove();
  vi.restoreAllMocks();
});

const remove: ConfirmPrompt = { title: 'Remove game', text: 'Files stay on disk.', confirm: 'Remove', busy: 'Removing…' };

function deferred() {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function confirmDialog(props: { prompt?: ConfirmPrompt; onconfirm: () => void | Promise<void>; errorText?: (err: unknown) => string }) {
  const onclose = vi.fn();
  const view = render(ConfirmModal, { target: app, props: { prompt: remove, onclose, ...props } });
  const keys = user();
  const dialog = screen.getByRole('dialog', { name: (props.prompt ?? remove).title });
  const button = (name: string) => screen.getByRole('button', { name });
  return { ...view, keys, dialog, onclose, button };
}

describe('ConfirmModal in a DOM', () => {
  it('is described by its text and puts focus on the safe action of a destructive confirmation', () => {
    const { dialog, button } = confirmDialog({ onconfirm: vi.fn() });

    expect(document.getElementById(dialog.getAttribute('aria-describedby') ?? '')?.textContent).toBe('Files stay on disk.');
    expect(document.activeElement).toBe(button(msg('common.cancel')));
  });

  it('Enter on open answers the safe action only', async () => {
    const onconfirm = vi.fn();
    const { keys, onclose } = confirmDialog({ onconfirm });

    await keys.keyboard('{Enter}');

    expect(onconfirm).not.toHaveBeenCalled();
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('a primary confirmation starts on its confirm button, and Enter runs it once', async () => {
    const onconfirm = vi.fn();
    const { keys, onclose, button } = confirmDialog({ prompt: { ...remove, tone: 'primary', confirm: 'Move' }, onconfirm });
    expect(document.activeElement).toBe(button('Move'));

    await keys.keyboard('{Enter}');

    expect(onconfirm).toHaveBeenCalledTimes(1);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('closes once when the action resolves', async () => {
    const { keys, onclose, button } = confirmDialog({ onconfirm: async () => {} });

    await keys.click(button('Remove'));

    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('while the action runs it cannot be dismissed and shows it is busy', async () => {
    const action = deferred();
    const { keys, dialog, onclose, button } = confirmDialog({ onconfirm: () => action.promise });

    await keys.click(button('Remove'));

    const busy = button('Removing…');
    expect(dialog.getAttribute('aria-busy')).toBe('true');
    expect(busy).toHaveProperty('disabled', true);
    expect(button(msg('common.cancel'))).toHaveProperty('disabled', true);
    await keys.keyboard('{Escape}');
    await keys.click(button(msg('common.close')));
    await keys.click(dialog.parentElement as HTMLElement);
    expect(onclose).not.toHaveBeenCalled();

    action.resolve();
    await vi.waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
  });

  it('a failed action keeps the dialog open with the error text from the backend code, and retry runs it again', async () => {
    const onconfirm = vi
      .fn<() => Promise<void>>()
      .mockRejectedValueOnce(new Error('typhon:library.game_not_found: no row 42'))
      .mockResolvedValueOnce(undefined);
    const { keys, onclose, button } = confirmDialog({ onconfirm });

    await keys.click(button('Remove'));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('library.game_not_found'));
    expect(alert.textContent).not.toContain('typhon:');
    expect(onclose).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBeTruthy();

    const retry = button(msg('common.retry'));
    expect(document.activeElement).toBe(retry);
    await keys.click(retry);

    expect(onconfirm).toHaveBeenCalledTimes(2);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('a caller maps the failure with its own error helper; cancel still closes after a failure', async () => {
    const { keys, onclose, button } = confirmDialog({
      onconfirm: () => Promise.reject(new Error('typhon:sources.unknown_thing')),
      errorText: () => 'The source could not be removed.',
    });

    await keys.click(button('Remove'));
    expect((await screen.findByRole('alert')).textContent?.trim()).toBe('The source could not be removed.');

    await keys.click(button(msg('common.cancel')));
    expect(onclose).toHaveBeenCalledTimes(1);
  });
});
