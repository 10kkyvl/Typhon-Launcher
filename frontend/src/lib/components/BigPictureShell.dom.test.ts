import { render, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { selectEmulatorFile, setEmulatorPath } from '../services/emulators';
import { emulatorPrompt } from '../stores/emulatorPrompt';
import { toast, toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import BigPictureHarness from '../testing/BigPictureHarness.svelte';
import modalSource from './Modal.svelte?raw';
import shellSource from './BigPictureShell.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => {
  const { askForEmulator } = await import('../stores/emulatorPrompt');
  return {
    ...(await importOriginal<typeof import('../services/library')>()),
    // What launchGame does for an emulated game with no emulator set: wait for the prompt's answer.
    launchGame: vi.fn(async () => askForEmulator('ps2', 'emulators.not_configured')),
  };
});
vi.mock('../services/emulators', () => ({
  listPlatforms: vi.fn(async () => [{ id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2', status: 'not_set' }]),
  openOfficialPage: vi.fn(async () => {}),
  selectEmulatorFile: vi.fn(async () => '/Applications/PCSX2.app'),
  setEmulatorPath: vi.fn(async () => {}),
}));
vi.mock('../bigpicture/components/HomePage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/LibraryPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/CatalogPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/GamePage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/DownloadsPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/SettingsPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/ProfilePage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/SocialPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));

// Read from disk: vitest serves `.css?raw` as an empty string, and under jsdom import.meta.url is not a file URL.
type Fs = { readFileSync(path: string, encoding: 'utf8'): string };
const node = (globalThis as unknown as { process: { cwd(): string; getBuiltinModule(id: 'node:fs'): Fs } }).process;
const shellCss = node.getBuiltinModule('node:fs').readFileSync(`${node.cwd()}/src/styles/shell.css`, 'utf8');
const z = (name: string) => Number(new RegExp(`--z-${name}:\\s*(-?\\d+);`).exec(shellCss)?.[1]);

let app: HTMLDivElement;
let pad: { id: string; index: number; mapping: string; connected: boolean; axes: number[]; buttons: { pressed: boolean; value: number }[] };

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  // Big Picture navigates only among controls that have a box; jsdom lays nothing out.
  vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(() => [{}] as unknown as DOMRectList);
  pad = { id: 'Xbox Wireless Controller', index: 0, mapping: 'standard', connected: true, axes: [0, 0, 0, 0], buttons: Array.from({ length: 16 }, () => ({ pressed: false, value: 0 })) };
  Object.defineProperty(navigator, 'getGamepads', { value: () => [pad], configurable: true });
});

afterEach(() => {
  get(emulatorPrompt)?.settle(false);
  toasts.set([]);
  app.remove();
  vi.restoreAllMocks();
  Reflect.deleteProperty(navigator, 'getGamepads');
});

async function press(button: number) {
  pad.buttons[button] = { pressed: true, value: 1 };
  await new Promise((resolve) => requestAnimationFrame(resolve));
  await new Promise((resolve) => requestAnimationFrame(resolve));
  pad.buttons[button] = { pressed: false, value: 0 };
  await new Promise((resolve) => requestAnimationFrame(resolve));
  await new Promise((resolve) => requestAnimationFrame(resolve));
}

async function launchToPrompt(how: 'keyboard' | 'gamepad') {
  render(BigPictureHarness, { target: app });
  const play = await screen.findByRole('button', { name: 'Play' });
  await vi.waitFor(() => expect(document.activeElement).toBe(play));
  const keys = user();
  if (how === 'keyboard') await keys.keyboard('{Enter}');
  else await press(0);
  const dialog = await screen.findByRole('dialog', { name: msg('emul.neededTitle') });
  return { keys, play, dialog, shell: document.querySelector('[data-big-picture]') as HTMLElement };
}

describe('a shared dialog over Big Picture', () => {
  it('sits on a layer above the shell', () => {
    expect(z('dialog')).toBeGreaterThan(z('bigpicture'));
    expect(modalSource).toMatch(/\.overlay \{[^}]*z-index: var\(--z-dialog\)/);
    expect(shellSource).toMatch(/\.big-picture \{[^}]*z-index: var\(--z-bigpicture\)/);
  });

  it('opened by a launch, it is outside the inert shell, takes focus, and Big Picture keys cannot move focus behind it', async () => {
    const { keys, dialog, shell } = await launchToPrompt('keyboard');

    expect(shell.contains(dialog)).toBe(false);
    expect(shell.hasAttribute('inert')).toBe(true);
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));

    for (const key of ['{ArrowDown}', '{ArrowUp}', '{ArrowLeft}', '{ArrowRight}', '{Tab}', '{q}', '{e}', '{F10}']) {
      await keys.keyboard(key);
      expect(dialog.contains(document.activeElement), key).toBe(true);
    }
  });

  it('keyboard: Escape answers the waiting launch with "no" and focus returns to Play', async () => {
    const { keys, play, shell } = await launchToPrompt('keyboard');

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(await screen.findByText('false')).toBeTruthy();
    expect(shell.hasAttribute('inert')).toBe(false);
    expect(document.activeElement).toBe(play);
  });

  it('keyboard: Enter on a button of the dialog presses it', async () => {
    const { keys } = await launchToPrompt('keyboard');
    const choose = screen.getByRole('button', { name: msg('emul.neededChoose') });
    choose.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(vi.mocked(setEmulatorPath)).toHaveBeenCalledWith('ps2', '/Applications/PCSX2.app'));
    expect(vi.mocked(selectEmulatorFile)).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('true')).toBeTruthy();
  });

  it('gamepad: B answers "no" without moving Big Picture behind the dialog', async () => {
    const { dialog } = await launchToPrompt('gamepad');
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));

    await press(1);

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(await screen.findByText('false')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Play' })).toBe(document.activeElement);
  });

  it('gamepad: the D-pad moves between the dialog buttons and A presses the focused one', async () => {
    const { dialog } = await launchToPrompt('gamepad');
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    const choose = screen.getByRole('button', { name: msg('emul.neededChoose') });

    for (let step = 0; step < 4 && document.activeElement !== choose; step += 1) await press(15);
    expect(document.activeElement).toBe(choose);
    await press(0);

    expect(await screen.findByText('true')).toBeTruthy();
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('a toast raised while the prompt is open is outside the inert shell, above the dialog, and its action works', async () => {
    const { keys, shell } = await launchToPrompt('keyboard');
    const run = vi.fn();

    toast('Save uploaded', 'info', { label: 'Open', run });
    const message = await screen.findByText('Save uploaded');

    expect(shell.contains(message)).toBe(false);
    expect(message.closest('[inert]')).toBeNull();
    expect(shellSource).toMatch(/\.toast-corner \{[^}]*z-index: var\(--z-toast\)/);
    expect(z('toast')).toBeGreaterThan(z('dialog'));
    await keys.click(screen.getByRole('button', { name: 'Open' }));
    expect(run).toHaveBeenCalledTimes(1);
  });
});
