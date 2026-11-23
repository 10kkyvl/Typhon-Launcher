import { screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/emulators')>()),
  listPlatforms: vi.fn(),
  openOfficialPage: vi.fn(),
  selectEmulatorFile: vi.fn(),
  setEmulatorPath: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { listPlatforms, openOfficialPage, selectEmulatorFile, setEmulatorPath, type PlatformStatus } from '../../lib/services/emulators';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
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
  for (const mock of [listPlatforms, openOfficialPage, selectEmulatorFile, setEmulatorPath]) vi.mocked(mock).mockReset();
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
