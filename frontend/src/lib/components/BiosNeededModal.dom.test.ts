import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { biosFolder, listPlatforms, openBiosFolder, type PlatformStatus } from '../services/emulators';
import { askForBios, biosPrompt } from '../stores/biosPrompt';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import BiosNeededModal from './BiosNeededModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(),
  biosFolder: vi.fn(),
  openBiosFolder: vi.fn(),
}));

const ps2 = { id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2' } as PlatformStatus;
const ps1 = { id: 'ps1', name: 'PlayStation', emulator: 'DuckStation' } as PlatformStatus;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(listPlatforms).mockResolvedValue([ps2, ps1]);
  vi.mocked(biosFolder).mockImplementation(async (platform: string) => `/bios/${platform}`);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  get(biosPrompt)?.settle(false);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const toastTexts = () => get(toasts).map((item) => [item.kind, item.message]);

async function ask(platform = 'ps2', stillMissing = false) {
  const answer = askForBios(platform, stillMissing);
  const dialog = await screen.findByRole('dialog', { name: msg('emul.biosTitle') });
  return { answer, dialog };
}

const button = (dialog: HTMLElement, key: Parameters<typeof msg>[0]) => within(dialog).getByRole('button', { name: msg(key) });

describe('BiosNeededModal: the standing prompt', () => {
  it('says what is missing and where it goes, with Cancel focused first; Check again answers the launch with yes', async () => {
    render(BiosNeededModal, { target: app });
    const { answer, dialog } = await ask();
    const keys = user();

    expect(await within(dialog).findByText('/bios/ps2')).toBeTruthy();
    expect(dialog.textContent).toContain(msg('emul.biosText', { emulator: 'PCSX2', console: 'PlayStation 2' }));
    expect(document.activeElement).toBe(button(dialog, 'common.cancel'));

    await keys.click(button(dialog, 'emul.biosRetry'));
    await expect(answer).resolves.toBe(true);
  });

  it('opens the folder once per press, busy while it opens', async () => {
    const opening = held<void>();
    vi.mocked(openBiosFolder).mockReturnValueOnce(opening.promise);
    render(BiosNeededModal, { target: app });
    const { dialog } = await ask();
    const keys = user();
    await within(dialog).findByText('/bios/ps2');
    const open = button(dialog, 'emul.biosOpenFolder');
    open.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(open.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(open);
    expect(vi.mocked(openBiosFolder).mock.calls).toEqual([['ps2']]);
    opening.resolve();
  });

  it('holds Open folder with a reason while the folder is read: it stays reachable', async () => {
    vi.mocked(biosFolder).mockReturnValueOnce(new Promise(() => {}));
    render(BiosNeededModal, { target: app });
    const { dialog } = await ask();

    const open = button(dialog, 'emul.biosOpenFolder');
    expect(open).toHaveProperty('disabled', false);
    expect(open.getAttribute('aria-disabled')).toBe('true');
  });

  it('says a refused open that is a whole sentence alone', async () => {
    vi.mocked(openBiosFolder).mockRejectedValueOnce(new Error('typhon:emulators.bios_dir_unknown: no'));
    render(BiosNeededModal, { target: app });
    const { dialog } = await ask();
    const keys = user();
    await within(dialog).findByText('/bios/ps2');

    await keys.click(button(dialog, 'emul.biosOpenFolder'));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('emulators.bios_dir_unknown'));
    expect(toastTexts()).toEqual([]);
  });

  it('says a refused open by code, after what failed', async () => {
    vi.mocked(openBiosFolder).mockRejectedValueOnce(new Error('typhon:emulators.not_configured: none'));
    render(BiosNeededModal, { target: app });
    const { dialog } = await ask();
    const keys = user();
    await within(dialog).findByText('/bios/ps2');

    await keys.click(button(dialog, 'emul.biosOpenFolder'));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(
      msg('common.failedReason', { action: msg('emul.biosOpenFailed'), reason: msg('emulators.not_configured') }),
    );
  });

  it('says a folder that could not be found by code, after what failed', async () => {
    vi.mocked(biosFolder).mockRejectedValueOnce(new Error('typhon:emulators.unknown_platform: x'));
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    render(BiosNeededModal, { target: app });
    const { dialog } = await ask();

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(
      msg('common.failedReason', { action: msg('emul.biosOpenFailed'), reason: msg('emulators.unknown_platform') }),
    );
  });
});

describe('BiosNeededModal: replaced by another launch while the folder opens', () => {
  async function replaced() {
    const opening = held<void>();
    vi.mocked(openBiosFolder).mockReturnValueOnce(opening.promise);
    render(BiosNeededModal, { target: app });
    const first = await ask();
    const keys = user();
    await within(first.dialog).findByText('/bios/ps2');
    await keys.click(button(first.dialog, 'emul.biosOpenFolder'));
    const second = await ask('ps1');
    await within(second.dialog).findByText('/bios/ps1');
    return { opening, second };
  }

  it('a late refusal paints nothing in the new prompt and is said once in a toast naming the emulator, after what failed', async () => {
    const { opening, second } = await replaced();
    const open = button(second.dialog, 'emul.biosOpenFolder');
    expect(open.getAttribute('aria-busy')).toBeNull();

    opening.reject(new Error('typhon:emulators.not_configured: none'));

    await vi.waitFor(() =>
      expect(toastTexts()).toEqual([
        ['danger', msg('common.failedReason', { action: msg('emul.biosOpenFailedFor', { emulator: 'PCSX2' }), reason: msg('emulators.not_configured') })],
      ]),
    );
    expect(within(second.dialog).queryByRole('alert')).toBeNull();
    expect(open.getAttribute('aria-busy')).toBeNull();
  });

  it('a late refusal that is a whole sentence is said after the emulator it concerns', async () => {
    const { opening } = await replaced();

    opening.reject(new Error('typhon:emulators.bios_dir_unknown: no'));

    await vi.waitFor(() =>
      expect(toastTexts()).toEqual([['danger', 'Не удалось открыть папку BIOS для PCSX2. Не удалось определить папку BIOS этого эмулятора']]),
    );
  });
});
