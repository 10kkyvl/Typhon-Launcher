import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { biosFolder, closeEmulator, listPlatforms, selectEmulatorFile, setEmulatorPath, type PlatformStatus } from '../services/emulators';
import { askForBios, biosPrompt } from '../stores/biosPrompt';
import { askToCloseEmulator, emulatorBusyPrompt } from '../stores/emulatorBusyPrompt';
import { askForEmulator, emulatorPrompt } from '../stores/emulatorPrompt';
import { user } from '../testing/dom';
import BiosNeededModal from './BiosNeededModal.svelte';
import EmulatorBusyModal from './EmulatorBusyModal.svelte';
import EmulatorNeededModal from './EmulatorNeededModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(),
  biosFolder: vi.fn(),
  closeEmulator: vi.fn(),
  selectEmulatorFile: vi.fn(),
  setEmulatorPath: vi.fn(),
}));

const ps2 = { id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2' } as PlatformStatus;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(listPlatforms).mockResolvedValue([ps2]);
  vi.mocked(biosFolder).mockResolvedValue('/bios/ps2');
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.spyOn(console, 'warn').mockImplementation(() => {});
});

afterEach(() => {
  get(emulatorBusyPrompt)?.settle(false);
  get(biosPrompt)?.settle(false);
  get(emulatorPrompt)?.settle(false);
  vi.mocked(listPlatforms).mockReset();
  vi.mocked(biosFolder).mockReset();
  vi.mocked(closeEmulator).mockReset();
  vi.mocked(selectEmulatorFile).mockReset();
  vi.mocked(setEmulatorPath).mockReset();
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe('EmulatorBusyModal', () => {
  it('keeps the focus on "close and play", busy, while the emulator closes, and says a failure in a danger notice', async () => {
    const closing = deferred<void>();
    vi.mocked(closeEmulator).mockReturnValueOnce(closing.promise);
    render(EmulatorBusyModal, { target: app });
    void askToCloseEmulator('ps2');
    const dialog = await screen.findByRole('dialog', { name: msg('emul.busyTitle', { emulator: 'PCSX2' }) });
    const keys = user();
    const confirm = within(dialog).getByRole('button', { name: msg('emul.busyConfirm', { emulator: 'PCSX2' }) });
    confirm.focus();

    await keys.keyboard('{Enter}');

    expect(confirm.getAttribute('aria-busy')).toBe('true');
    expect(confirm).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(confirm);

    closing.reject(new Error('locked'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('emul.busyCloseFailed', { emulator: 'PCSX2' }));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

describe('BiosNeededModal', () => {
  it('says the BIOS is still missing in a danger notice, and a failed read in another', async () => {
    vi.mocked(biosFolder).mockRejectedValueOnce(new Error('disk'));
    render(BiosNeededModal, { target: app });
    void askForBios('ps2', true);
    const dialog = await screen.findByRole('dialog', { name: msg('emul.biosTitle') });

    const missing = within(dialog).getByText(msg('emul.biosStillMissing'));
    expect(missing.closest('[data-tone="danger"]')).not.toBeNull();
    const failed = await within(dialog).findByText(msg('emul.biosOpenFailed'));
    expect(failed.closest('[data-tone="danger"]')).not.toBeNull();
    expect(failed.closest('[role="alert"]')).not.toBeNull();
  });
});

describe('EmulatorNeededModal', () => {
  it('keeps the focus on "choose", busy, while the file is picked and saved, and says a failed save in a danger notice', async () => {
    const picked = deferred<string>();
    vi.mocked(selectEmulatorFile).mockReturnValueOnce(picked.promise);
    vi.mocked(setEmulatorPath).mockRejectedValueOnce(new Error('bad file'));
    render(EmulatorNeededModal, { target: app });
    void askForEmulator('ps2', 'emulators.not_configured');
    const dialog = await screen.findByRole('dialog', { name: msg('emul.neededTitle') });
    const keys = user();
    const choose = within(dialog).getByRole('button', { name: msg('emul.neededChoose') });
    choose.focus();

    await keys.keyboard('{Enter}');

    expect(choose.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(choose);

    picked.resolve('/apps/pcsx2');
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('emul.saveFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});
