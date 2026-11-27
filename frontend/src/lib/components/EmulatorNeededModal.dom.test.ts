import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { listPlatforms, openOfficialPage, selectEmulatorFile, setEmulatorPath, type PlatformStatus } from '../services/emulators';
import { askForEmulator, emulatorPrompt } from '../stores/emulatorPrompt';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import EmulatorNeededModal from './EmulatorNeededModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(),
  openOfficialPage: vi.fn(),
  selectEmulatorFile: vi.fn(),
  setEmulatorPath: vi.fn(),
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
  get(emulatorPrompt)?.settle(false);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const toastTexts = () => get(toasts).map((item) => [item.kind, item.message]);

async function ask(platform = 'ps2') {
  const answer = askForEmulator(platform, 'emulators.not_configured');
  const dialog = await screen.findByRole('dialog', { name: msg('emul.neededTitle') });
  return { answer, dialog };
}

const button = (dialog: HTMLElement, key: Parameters<typeof msg>[0]) => within(dialog).getByRole('button', { name: msg(key) });

describe('EmulatorNeededModal: the standing prompt', () => {
  it('says which emulator the console needs, with Cancel focused first', async () => {
    render(EmulatorNeededModal, { target: app });
    const { dialog } = await ask();

    expect(await within(dialog).findByText(msg('emul.neededText', { console: 'PlayStation 2', emulator: 'PCSX2' }))).toBeTruthy();
    expect(document.activeElement).toBe(button(dialog, 'common.cancel'));
  });

  it('saves the chosen file for its console once and answers the launch with yes', async () => {
    const picking = held<string>();
    vi.mocked(selectEmulatorFile).mockReturnValueOnce(picking.promise);
    vi.mocked(setEmulatorPath).mockResolvedValueOnce(ps2);
    render(EmulatorNeededModal, { target: app });
    const { answer, dialog } = await ask();
    const keys = user();
    const choose = button(dialog, 'emul.neededChoose');
    choose.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    picking.resolve('/apps/pcsx2');

    await expect(answer).resolves.toBe(true);
    expect(vi.mocked(selectEmulatorFile)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(setEmulatorPath).mock.calls).toEqual([['ps2', '/apps/pcsx2']]);
  });

  it('opens the official site once per press, busy while it opens', async () => {
    const opening = held<void>();
    vi.mocked(openOfficialPage).mockReturnValueOnce(opening.promise);
    render(EmulatorNeededModal, { target: app });
    const { dialog } = await ask();
    const keys = user();
    const site = await vi.waitFor(() => {
      const found = button(dialog, 'emul.neededOpenSite');
      expect(found.getAttribute('aria-disabled')).toBeNull();
      return found;
    });
    site.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(site.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(site);
    expect(vi.mocked(openOfficialPage).mock.calls).toEqual([['ps2']]);
    opening.resolve();
  });

  it('holds Cancel and the site with a reason while the file is chosen: both stay reachable and do nothing', async () => {
    const picking = held<string>();
    vi.mocked(selectEmulatorFile).mockReturnValueOnce(picking.promise);
    render(EmulatorNeededModal, { target: app });
    const { answer, dialog } = await ask();
    const keys = user();
    const settled = vi.fn();
    void answer.then(settled);
    await vi.waitFor(() => expect(button(dialog, 'emul.neededOpenSite').getAttribute('aria-disabled')).toBeNull());
    await keys.click(button(dialog, 'emul.neededChoose'));

    for (const key of ['common.cancel', 'emul.neededOpenSite'] as const) {
      const held = button(dialog, key);
      expect(held).toHaveProperty('disabled', false);
      expect(held.getAttribute('aria-disabled')).toBe('true');
      await keys.click(held);
    }
    await Promise.resolve();
    expect(settled).not.toHaveBeenCalled();
    expect(vi.mocked(openOfficialPage)).not.toHaveBeenCalled();
    picking.resolve('');
  });

  it('holds the site with the reason while the list of emulators is read', async () => {
    vi.mocked(listPlatforms).mockReturnValueOnce(new Promise(() => {}));
    render(EmulatorNeededModal, { target: app });
    const { dialog } = await ask();

    const site = button(dialog, 'emul.neededOpenSite');
    expect(site).toHaveProperty('disabled', false);
    expect(site.getAttribute('aria-disabled')).toBe('true');
  });

  it('cannot be dismissed while the file is chosen, and a refusal shows in the prompt', async () => {
    const choosing = held<string>();
    vi.mocked(selectEmulatorFile).mockReturnValueOnce(choosing.promise);
    render(EmulatorNeededModal, { target: app });
    const { answer, dialog } = await ask();
    const settled = vi.fn();
    void answer.then(settled);
    const keys = user();
    await keys.click(button(dialog, 'emul.neededChoose'));

    await keys.keyboard('{Escape}');
    await Promise.resolve();
    expect(settled).not.toHaveBeenCalled();

    choosing.reject(new Error('typhon:emulators.not_found: gone'));
    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('emul.saveFailed'));
    expect(dialog.isConnected).toBe(true);
  });

  it('says a refused file in the prompt by code', async () => {
    vi.mocked(selectEmulatorFile).mockResolvedValueOnce('/apps/pcsx2');
    vi.mocked(setEmulatorPath).mockRejectedValueOnce(new Error('typhon:emulators.not_exe: no'));
    render(EmulatorNeededModal, { target: app });
    const { dialog } = await ask();
    const keys = user();

    await keys.click(button(dialog, 'emul.neededChoose'));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('emulators.not_exe'));
    expect(toastTexts()).toEqual([]);
  });
});

describe('EmulatorNeededModal: replaced by another launch while the file is saved', () => {
  async function replaced() {
    const saving = held<PlatformStatus>();
    vi.mocked(selectEmulatorFile).mockResolvedValueOnce('/apps/pcsx2');
    vi.mocked(setEmulatorPath).mockReturnValueOnce(saving.promise);
    render(EmulatorNeededModal, { target: app });
    const first = await ask();
    const keys = user();
    await keys.click(button(first.dialog, 'emul.neededChoose'));
    await vi.waitFor(() => expect(vi.mocked(setEmulatorPath)).toHaveBeenCalled());
    const second = await ask('ps3');
    await within(second.dialog).findByText(msg('emul.neededText', { console: 'PlayStation 3', emulator: 'RPCS3' }));
    return { saving, first, second };
  }

  it('a late refusal paints nothing in the new prompt and is said once in a toast naming the emulator', async () => {
    const { saving, second } = await replaced();
    const choose = button(second.dialog, 'emul.neededChoose');
    expect(choose.getAttribute('aria-busy')).toBeNull();

    saving.reject(new Error('typhon:emulators.not_exe: no'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', msg('emul.lateFailed', { emulator: 'PCSX2', reason: msg('emulators.not_exe') })]]));
    expect(within(second.dialog).queryByRole('alert')).toBeNull();
    expect(choose.getAttribute('aria-busy')).toBeNull();
  });

  it('a late success is said once in a toast naming the emulator', async () => {
    const { saving, first } = await replaced();
    await expect(first.answer).resolves.toBe(false);

    saving.resolve(ps2);

    await vi.waitFor(() => expect(toastTexts()).toEqual([['success', msg('emul.pathSaved', { emulator: 'PCSX2' })]]));
  });
});
