import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/emulators', async (original) => (await import('../../lib/testing/settingsDialogsTesting')).emulatorsService(await original()));
vi.mock('../../../bindings/typhon/internal/emulators', async (original) => (await import('../../lib/testing/settingsDialogsTesting')).fakeService(await original(), 'emulators'));

import { msg } from '../../lib/i18n';
import { libraryErrorText } from '../../lib/library/errors';
import { detectEmulators, listPlatforms, openOfficialPage, selectEmulatorFile, setEmulatorPath, type PlatformStatus } from '../../lib/services/emulators';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { description } from '../../lib/testing/hints';
import { deferred } from '../../lib/testing/settingsDialogsTesting';
import EmulatorsCard from './EmulatorsCard.svelte';

const PS2: PlatformStatus = {
  id: 'ps2',
  name: 'PlayStation 2',
  emulator: 'PCSX2',
  homepage: '',
  extensions: [],
  imageNames: [],
  path: '',
  status: 'not_set',
  reason: '',
  warnings: [],
};
const VITA: PlatformStatus = {
  ...PS2,
  id: 'vita',
  name: 'PlayStation Vita',
  emulator: 'Vita3K',
  path: '/Applications/Vita3K.app/Contents/MacOS/Vita3K',
  status: 'unavailable',
  reason: 'emul.vitaFirstLaunch',
  warnings: ['discord_exit_hang'],
};

const toastTexts = () => get(toasts).map((item) => item.message);

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listPlatforms).mockResolvedValue([PS2, VITA]);
});

afterEach(() => {
  document.body.replaceChildren();
  toasts.set([]);
  vi.restoreAllMocks();
  for (const mock of [listPlatforms, openOfficialPage, selectEmulatorFile, setEmulatorPath, detectEmulators]) vi.mocked(mock).mockReset();
  expect(calls).toEqual([]);
  resetBindings();
});

describe('EmulatorsCard: a platform', () => {
  it('shows its name and emulator, its path, its state and why it is unavailable', async () => {
    renderDom(EmulatorsCard, {});
    expect(await screen.findByText('PlayStation 2 · PCSX2')).toBeTruthy();
    expect(screen.getByText('PlayStation Vita · Vita3K')).toBeTruthy();
    expect(screen.getByText(VITA.path)).toBeTruthy();
    expect(screen.getByText(msg('emul.statusNotSet'))).toBeTruthy();
    expect(screen.getByText(msg('emul.statusUnavailable'))).toBeTruthy();
    expect(screen.getByText(msg('emul.vitaFirstLaunch'))).toBeTruthy();
    const hang = screen.getByText(msg('emul.discordExitHang'));
    expect(hang.closest('[data-tone]')?.getAttribute('data-tone')).toBe('warning');
  });

  it('names every button after its platform, starting with the words it shows', async () => {
    renderDom(EmulatorsCard, {});
    await screen.findByText('PlayStation 2 · PCSX2');
    for (const platform of ['PlayStation 2', 'PlayStation Vita']) {
      const official = screen.getByRole('button', { name: msg('settings.emulatorsOfficialFor', { platform }) });
      const choose = screen.getByRole('button', { name: msg('settings.emulatorsChooseFor', { platform }) });
      expect(official.textContent?.trim()).toBe(msg('emul.official'));
      expect(choose.textContent?.trim()).toBe(msg('emul.choose'));
      expect(msg('settings.emulatorsOfficialFor', { platform }).startsWith(msg('emul.official'))).toBe(true);
      expect(msg('settings.emulatorsChooseFor', { platform }).startsWith(msg('emul.choose'))).toBe(true);
    }
    // A reset only on a platform whose emulator is set.
    expect(screen.getByRole('button', { name: msg('settings.emulatorsResetFor', { platform: 'PlayStation Vita' }) }).textContent?.trim()).toBe(msg('emul.reset'));
    expect(msg('settings.emulatorsResetFor', { platform: 'PlayStation Vita' }).startsWith(msg('emul.reset'))).toBe(true);
    expect(screen.queryByRole('button', { name: msg('settings.emulatorsResetFor', { platform: 'PlayStation 2' }) })).toBeNull();
  });

  it('shows the path in full with no native tooltip', async () => {
    const { container } = renderDom(EmulatorsCard, {});
    await screen.findByText(VITA.path);
    expect(container.querySelectorAll('[title]')).toHaveLength(0);
  });

  it("opens the emulator's site, and says when it cannot", async () => {
    vi.mocked(openOfficialPage).mockResolvedValueOnce().mockRejectedValueOnce(new Error('no browser'));
    const { user } = renderDom(EmulatorsCard, {});
    await screen.findByText('PlayStation 2 · PCSX2');
    const ps2 = screen.getByRole('button', { name: msg('settings.emulatorsOfficialFor', { platform: 'PlayStation 2' }) });

    await user.click(ps2);
    expect(openOfficialPage).toHaveBeenLastCalledWith('ps2');
    await user.click(ps2);
    await waitFor(() => expect(toastTexts()).toContain(msg('emul.openSiteFailed')));
  });

  it('sets the picked program as the emulator', async () => {
    vi.mocked(selectEmulatorFile).mockResolvedValue('/apps/pcsx2');
    vi.mocked(setEmulatorPath).mockResolvedValue({ ...PS2, path: '/apps/pcsx2', status: 'ready' });
    const { user } = renderDom(EmulatorsCard, {});
    await screen.findByText('PlayStation 2 · PCSX2');

    await user.click(screen.getByRole('button', { name: msg('settings.emulatorsChooseFor', { platform: 'PlayStation 2' }) }));

    expect(selectEmulatorFile).toHaveBeenCalledExactlyOnceWith(msg('emul.pickDialog', { emulator: 'PCSX2' }));
    await waitFor(() => expect(setEmulatorPath).toHaveBeenCalledExactlyOnceWith('ps2', '/apps/pcsx2'));
    await waitFor(() => expect(toastTexts()).toContain(msg('emul.pathSaved', { emulator: 'PCSX2' })));
    expect(screen.getByText('/apps/pcsx2')).toBeTruthy();
    expect(screen.getByText(msg('emul.statusReady'))).toBeTruthy();
  });

  it('says a path that was not saved', async () => {
    vi.mocked(selectEmulatorFile).mockResolvedValue('/apps/pcsx2');
    vi.mocked(setEmulatorPath).mockRejectedValue(new Error('denied'));
    const { user } = renderDom(EmulatorsCard, {});
    await screen.findByText('PlayStation 2 · PCSX2');

    await user.click(screen.getByRole('button', { name: msg('settings.emulatorsChooseFor', { platform: 'PlayStation 2' }) }));

    await waitFor(() => expect(toastTexts()).toContain(msg('emul.saveFailed')));
  });

  it('resets a set emulator', async () => {
    vi.mocked(setEmulatorPath).mockResolvedValue({ ...VITA, path: '', status: 'not_set', reason: '' });
    const { user } = renderDom(EmulatorsCard, {});
    await screen.findByText('PlayStation 2 · PCSX2');

    await user.click(screen.getByRole('button', { name: msg('settings.emulatorsResetFor', { platform: 'PlayStation Vita' }) }));

    expect(setEmulatorPath).toHaveBeenCalledExactlyOnceWith('vita', '');
    await waitFor(() => expect(toastTexts()).toContain(msg('emul.pathReset', { emulator: 'Vita3K' })));
  });
});

const PLATFORM = PS2;

describe('EmulatorsCard', () => {
  it('says it is loading through a named spinner', async () => {
    const list = deferred<PlatformStatus[]>();
    vi.mocked(listPlatforms).mockReturnValue(list.promise);
    renderDom(EmulatorsCard, {});
    expect(screen.getByRole('status').textContent?.trim()).toBe(msg('emul.loading'));
    list.resolve([PLATFORM]);
    await waitFor(() => expect(screen.queryByRole('status')).toBeNull());
  });

  it('keeps the load error and focus on the busy Retry until the list arrives', async () => {
    vi.mocked(listPlatforms).mockRejectedValueOnce(new Error('down'));
    const { user } = renderDom(EmulatorsCard, {});
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('emul.loadFailed'));
    const retry = screen.getByRole('button', { name: msg('common.retry') });

    const list = deferred<PlatformStatus[]>();
    vi.mocked(listPlatforms).mockReturnValueOnce(list.promise);
    retry.focus();
    await user.keyboard('{Enter}');

    expect(retry.isConnected).toBe(true);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    expect(screen.getByRole('alert').textContent).toContain(msg('emul.loadFailed'));

    list.resolve([PLATFORM]);
    await screen.findByText(`${PLATFORM.name} · ${PLATFORM.emulator}`);
    expect(screen.queryByRole('alert')).toBeNull();
    // Focus moves once the rows that replaced the notice are drawn, to their first control.
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('settings.emulatorsOfficialFor', { platform: PLATFORM.name }) })));
    // The notice went before its retry answered: the card still learns the retry is over.
    expect((screen.getByRole('button', { name: msg('emul.detect') }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('shows a new error in place when the retry fails again', async () => {
    vi.mocked(listPlatforms).mockRejectedValue(new Error('down'));
    const { user } = renderDom(EmulatorsCard, {});
    const alert = await screen.findByRole('alert');
    const before = alert.firstElementChild;
    const retry = screen.getByRole('button', { name: msg('common.retry') });
    retry.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(retry.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    // Said again though the words are the same: they come again into the alert, as new nodes.
    await waitFor(() => expect(alert.firstElementChild).not.toBe(before));
    expect(alert.textContent).toContain(msg('emul.loadFailed'));
  });
});

const PSP: PlatformStatus = { ...PS2, id: 'psp', name: 'PlayStation Portable', emulator: 'PPSSPP' };

const FOUND = { found: [{ platform: 'ps2', path: '/apps/pcsx2' }, { platform: 'psp', path: '/apps/ppsspp' }], unreadable: 0 };

async function openFound(found = FOUND) {
  vi.mocked(listPlatforms).mockResolvedValue([PS2, PSP]);
  vi.mocked(detectEmulators).mockResolvedValueOnce(found);
  const rendered = renderDom(EmulatorsCard, {});
  await screen.findByText('PlayStation 2 · PCSX2');
  const detect = screen.getByRole('button', { name: msg('emul.detect') });
  detect.focus();
  await rendered.user.keyboard('{Enter}');
  const dialog = await screen.findByRole('dialog', { name: msg('emul.detectTitle') });
  return {
    ...rendered,
    detect,
    dialog,
    box: (name: string) => within(dialog).getByRole('checkbox', { name }),
    apply: () => within(dialog).getByRole('button', { name: msg('emul.detectApply') }),
    cancel: () => within(dialog).getByRole('button', { name: msg('common.cancel') }),
  };
}

describe('EmulatorsCard: the found emulators', () => {
  it('ticks each found emulator for a later Apply, as a checkbox described by its path, the first one focused', async () => {
    const { box } = await openFound();
    expect(box('PCSX2').getAttribute('aria-checked')).toBe('true');
    expect(box('PPSSPP').getAttribute('aria-checked')).toBe('true');
    expect(description(box('PCSX2'))).toBe('/apps/pcsx2');
    expect(document.activeElement).toBe(box('PCSX2'));
  });

  it('says a search that could not look everywhere', async () => {
    const { dialog } = await openFound({ ...FOUND, unreadable: 2 });
    expect(within(dialog).getByText(msg('emul.detectPartial'))).toBeTruthy();
  });

  it('holds Apply with its reason while nothing is ticked, saving nothing', async () => {
    const { box, apply, user } = await openFound();
    await user.click(box('PCSX2'));
    await user.click(box('PPSSPP'));

    expect(apply().getAttribute('aria-disabled')).toBe('true');
    expect(apply()).toHaveProperty('disabled', false);
    expect(description(apply())).toBe(msg('emul.detectPickOne'));
    await user.click(apply());
    expect(setEmulatorPath).not.toHaveBeenCalled();
  });

  it('saves each ticked path once however often Apply is pressed, Apply busy and focused, the rest held with the reason', async () => {
    const saving = deferred<PlatformStatus>();
    vi.mocked(setEmulatorPath).mockReturnValueOnce(saving.promise).mockResolvedValueOnce({ ...PSP, path: '/apps/ppsspp', status: 'ready' });
    const { dialog, box, apply, cancel, user } = await openFound();
    await user.click(box('PPSSPP'));
    apply().focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');
    await user.click(apply());

    expect(setEmulatorPath).toHaveBeenCalledExactlyOnceWith('ps2', '/apps/pcsx2');
    expect(apply().getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(apply());
    for (const held of [cancel(), box('PCSX2'), box('PPSSPP')]) {
      expect(held.getAttribute('aria-disabled')).toBe('true');
      expect(held).toHaveProperty('disabled', false);
      expect(description(held)).toContain(msg('emul.detectApplying'));
    }
    await user.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);

    saving.resolve({ ...PS2, path: '/apps/pcsx2', status: 'ready' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(setEmulatorPath).toHaveBeenCalledTimes(1);
  });

  it('closes once every ticked path is saved, says so, and gives the focus back to the search button', async () => {
    vi.mocked(setEmulatorPath).mockResolvedValueOnce({ ...PS2, path: '/apps/pcsx2', status: 'ready' }).mockResolvedValueOnce({ ...PSP, path: '/apps/ppsspp', status: 'ready' });
    const { apply, detect, user } = await openFound();

    await user.click(apply());

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(setEmulatorPath).mock.calls).toEqual([
      ['ps2', '/apps/pcsx2'],
      ['psp', '/apps/ppsspp'],
    ]);
    expect(toastTexts()).toEqual([msg('emul.detectApplied')]);
    expect(screen.getByText('/apps/pcsx2')).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(detect));
  });

  it('says the paths that were not saved in the dialog, which keeps them ticked for another Apply', async () => {
    const refused = new Error('typhon:library.invalid_path: nope');
    vi.mocked(setEmulatorPath)
      .mockResolvedValueOnce({ ...PS2, path: '/apps/pcsx2', status: 'ready' })
      .mockRejectedValueOnce(refused)
      .mockResolvedValueOnce({ ...PSP, path: '/apps/ppsspp', status: 'ready' });
    const { dialog, apply, user } = await openFound();
    apply().focus();

    await user.keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      `${msg('emul.detectApplyPartly')} ${msg('emul.detectApplyFailed', { emulator: 'PPSSPP', reason: libraryErrorText(refused, msg('emul.saveFailed')) })}`,
    );
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(dialog.textContent).not.toContain('nope');
    expect(within(dialog).queryByRole('checkbox', { name: 'PCSX2' })).toBeNull();
    expect(within(dialog).getByRole('checkbox', { name: 'PPSSPP' }).getAttribute('aria-checked')).toBe('true');
    expect(toastTexts()).toEqual([]);
    expect(document.activeElement).toBe(apply());
    expect(description(apply())).toContain(msg('emul.detectApplyPartly'));

    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(setEmulatorPath).mock.calls.map(([platform]) => platform)).toEqual(['ps2', 'psp', 'psp']);
    expect(toastTexts()).toEqual([msg('emul.detectApplied')]);
  });
});
