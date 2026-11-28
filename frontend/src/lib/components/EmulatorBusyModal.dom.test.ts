import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { closeEmulator, listPlatforms, type PlatformStatus } from '../services/emulators';
import { askToCloseEmulator, emulatorBusyPrompt } from '../stores/emulatorBusyPrompt';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import EmulatorBusyModal from './EmulatorBusyModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(),
  closeEmulator: vi.fn(),
}));

const ps2 = { id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2' } as PlatformStatus;
const ps3 = { id: 'ps3', name: 'PlayStation 3', emulator: 'RPCS3' } as PlatformStatus;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(listPlatforms).mockResolvedValue([ps2, ps3]);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  get(emulatorBusyPrompt)?.settle(false);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const toastTexts = () => get(toasts).map((item) => [item.kind, item.message]);

async function ask(platform = 'ps2', emulator = 'PCSX2') {
  const answer = askToCloseEmulator(platform);
  const dialog = await screen.findByRole('dialog', { name: msg('emul.busyTitle', { emulator }) });
  return { answer, dialog };
}

describe('EmulatorBusyModal: the question', () => {
  it('says the emulator is asked to close and its process is ended after a wait, with Cancel, the safe answer, focused', async () => {
    render(EmulatorBusyModal, { target: app });
    const { dialog } = await ask();

    expect(dialog.textContent).toContain(msg('emul.busyText', { emulator: 'PCSX2' }));
    expect(dialog.textContent).toContain(msg('emul.busyHint'));
    await vi.waitFor(() => expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: msg('common.cancel') })));
  });

  it('closes the emulator of its console once per press and answers the launch with yes', async () => {
    const closing = held<void>();
    vi.mocked(closeEmulator).mockReturnValueOnce(closing.promise);
    render(EmulatorBusyModal, { target: app });
    const { answer, dialog } = await ask();
    const keys = user();
    const confirm = within(dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'PCSX2' }) });
    confirm.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(closeEmulator).mock.calls).toEqual([['ps2']]);
    closing.resolve();
    await expect(answer).resolves.toBe(true);
  });

  it('holds Cancel with a reason while the emulator closes: it stays reachable and answers nothing', async () => {
    const closing = held<void>();
    vi.mocked(closeEmulator).mockReturnValueOnce(closing.promise);
    render(EmulatorBusyModal, { target: app });
    const { answer, dialog } = await ask();
    const keys = user();
    const settled = vi.fn();
    void answer.then(settled);
    await keys.click(within(dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'PCSX2' }) }));

    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel).toHaveProperty('disabled', false);
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    await keys.click(cancel);
    await Promise.resolve();
    expect(settled).not.toHaveBeenCalled();
    closing.resolve();
  });

  // Go is ending the process: answering the launch "no" now would leave the player without the game
  // and the emulator both.
  it('cannot be dismissed while the emulator closes: Escape and the close button answer nothing', async () => {
    const closing = held<void>();
    vi.mocked(closeEmulator).mockReturnValueOnce(closing.promise);
    render(EmulatorBusyModal, { target: app });
    const { answer, dialog } = await ask();
    const keys = user();
    const settled = vi.fn();
    void answer.then(settled);
    await keys.click(within(dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'PCSX2' }) }));

    await keys.keyboard('{Escape}');
    const close = within(dialog).getByRole('button', { name: msg('common.close') });
    expect(close).toHaveProperty('disabled', true);
    await keys.click(close);
    await Promise.resolve();

    expect(dialog.isConnected).toBe(true);
    expect(settled).not.toHaveBeenCalled();
    closing.resolve();
    await expect(answer).resolves.toBe(true);
  });

  it('says a refusal in the question by code, after what failed', async () => {
    vi.mocked(closeEmulator).mockRejectedValueOnce(new Error('typhon:emulators.not_configured: none'));
    render(EmulatorBusyModal, { target: app });
    const { dialog } = await ask();
    const keys = user();

    await keys.click(within(dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'PCSX2' }) }));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(
      msg('common.failedReason', { action: msg('emul.busyCloseFailed', { emulator: 'PCSX2' }), reason: msg('emulators.not_configured') }),
    );
    expect(toastTexts()).toEqual([]);
  });

  it('says a refusal in the question that is a whole sentence alone', async () => {
    vi.mocked(closeEmulator).mockRejectedValueOnce(new Error('typhon:emulators.already_running: open'));
    render(EmulatorBusyModal, { target: app });
    const { dialog } = await ask();
    const keys = user();

    await keys.click(within(dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'PCSX2' }) }));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('emulators.already_running'));
    expect(toastTexts()).toEqual([]);
  });
});

describe('EmulatorBusyModal: replaced by another launch while the emulator closes', () => {
  async function replaced() {
    const closing = held<void>();
    vi.mocked(closeEmulator).mockReturnValueOnce(closing.promise);
    render(EmulatorBusyModal, { target: app });
    const first = await ask();
    const keys = user();
    await keys.click(within(first.dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'PCSX2' }) }));
    const second = await ask('ps3', 'RPCS3');
    return { closing, first, second };
  }

  it('a late refusal paints nothing in the new question and is said once in a toast naming the emulator', async () => {
    const { closing, second } = await replaced();
    const confirm = within(second.dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'RPCS3' }) });
    expect(confirm.getAttribute('aria-busy')).toBeNull();

    closing.reject(new Error('typhon:emulators.not_configured: none'));

    await vi.waitFor(() =>
      expect(toastTexts()).toEqual([
        ['danger', msg('common.failedReason', { action: msg('emul.busyCloseFailed', { emulator: 'PCSX2' }), reason: msg('emulators.not_configured') })],
      ]),
    );
    expect(within(second.dialog).queryByRole('alert')).toBeNull();
    expect(confirm.getAttribute('aria-busy')).toBeNull();
  });

  it('a late refusal that is a whole sentence is said after the emulator it concerns', async () => {
    const { closing } = await replaced();

    closing.reject(new Error('typhon:emulators.already_running: open'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', 'Не удалось закрыть PCSX2. Эмулятор уже открыт: закройте его, чтобы запустить игру']]));
  });

  it('a late success is said once in a toast naming the emulator; the first launch was already answered no', async () => {
    const { closing, first } = await replaced();
    await expect(first.answer).resolves.toBe(false);

    closing.resolve();

    await vi.waitFor(() => expect(toastTexts()).toEqual([['success', msg('emul.busyClosedLate', { emulator: 'PCSX2' })]]));
  });
});
