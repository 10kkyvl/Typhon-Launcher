import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { installRedist, installedRuntimes, officialLinks, scanRedist, type RedistPackage, type RedistResult } from '../services/redist';
import { libraryGames } from '../stores/library';
import { closeRedist, openRedist } from '../stores/redist';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import RedistModal from './RedistModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/redist', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/redist')>()),
  scanRedist: vi.fn(),
  installedRuntimes: vi.fn(),
  officialLinks: vi.fn(),
  installRedist: vi.fn(),
}));

const vc: RedistPackage = { name: 'VC++ 2015', path: '/games/hades/_redist/vc.exe', kind: 'vcredist', arch: 'x64', version: '14.0', sizeBytes: 1024 } as RedistPackage;
const dx: RedistPackage = { name: 'DirectX June 2010', path: '/games/hades/_redist/dx.exe', kind: 'directx', arch: '', version: '', sizeBytes: 2048 } as RedistPackage;
const done: RedistResult = { exitCode: 0, rebootRequired: false, alreadyInstalled: false };

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
  vi.mocked(scanRedist).mockResolvedValue([vc, dx]);
  vi.mocked(installedRuntimes).mockResolvedValue([
    { name: 'Microsoft Visual C++ 2022 x64', version: '14.40', arch: 'x64' },
    { name: 'Microsoft Visual C++ 2022 x86', version: '14.40', arch: 'x86' },
  ]);
  vi.mocked(officialLinks).mockResolvedValue([]);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  closeRedist();
  libraryGames.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const toastTexts = () => get(toasts).map((item) => [item.kind, item.message]);

async function openDialog() {
  openRedist('g1');
  return screen.findByRole('dialog', { name: msg('redist.title', { title: 'Hades' }) });
}

const row = (dialog: HTMLElement, name: string) => within(dialog).getByText(name).closest('li') as HTMLElement;

describe('RedistModal: the bundled packages', () => {
  it('say a kind Go has no word for as "other", never its raw value', async () => {
    vi.mocked(scanRedist).mockResolvedValue([vc, { ...dx, kind: 'mystery' as RedistPackage['kind'] }]);
    render(RedistModal, { target: app });
    const dialog = await openDialog();

    const meta = (await within(dialog).findByText(dx.name)).nextElementSibling;
    expect(meta?.textContent).toContain(msg('redist.kindOther'));
    expect(meta?.textContent).not.toContain('mystery');
    expect(within(dialog).getByText(vc.name).nextElementSibling?.textContent).toContain(msg('redist.kindVcredist'));
  });

  it('hold the other Install with the reason while one installs (Go runs one at a time), and a press there sends nothing', async () => {
    const installing = held<RedistResult>();
    vi.mocked(installRedist).mockReturnValueOnce(installing.promise);
    render(RedistModal, { target: app });
    const dialog = await openDialog();
    const keys = user();
    await within(dialog).findByText(vc.name);

    await keys.click(within(row(dialog, vc.name)).getByRole('button', { name: msg('redist.install') }));
    const other = within(row(dialog, dx.name)).getByRole('button', { name: msg('redist.install') });
    await keys.click(other);

    expect(other).toHaveProperty('disabled', false);
    expect(other.getAttribute('aria-disabled')).toBe('true');
    expect(other.getAttribute('aria-describedby') ?? '').not.toBe('');
    expect(document.getElementById(other.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(msg('redist.busy'));
    expect(vi.mocked(installRedist).mock.calls).toEqual([['g1', vc.path]]);
    installing.resolve(done);
  });
});

describe('RedistModal: closed while an installer runs', () => {
  it('can be closed (Go goes on); opened again, it shows the installer still running and starts no second one', async () => {
    const installing = held<RedistResult>();
    vi.mocked(installRedist).mockReturnValueOnce(installing.promise);
    render(RedistModal, { target: app });
    const first = await openDialog();
    const keys = user();
    await within(first).findByText(vc.name);
    await keys.click(within(row(first, vc.name)).getByRole('button', { name: msg('redist.install') }));

    await keys.keyboard('{Escape}');
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    const dialog = await openDialog();
    await within(dialog).findByText(vc.name);

    const running = within(row(dialog, vc.name)).getByRole('button', { name: msg('redist.installing') });
    expect(running.getAttribute('aria-busy')).toBe('true');
    const other = within(row(dialog, dx.name)).getByRole('button', { name: msg('redist.install') });
    expect(other.getAttribute('aria-disabled')).toBe('true');
    await keys.click(other);
    await keys.click(running);
    expect(vi.mocked(installRedist)).toHaveBeenCalledTimes(1);
    installing.resolve(done);
  });

  it('says the late result once in a toast naming the package, and paints nothing in the dialog opened since', async () => {
    const installing = held<RedistResult>();
    vi.mocked(installRedist).mockReturnValueOnce(installing.promise);
    render(RedistModal, { target: app });
    const first = await openDialog();
    const keys = user();
    await within(first).findByText(vc.name);
    await keys.click(within(row(first, vc.name)).getByRole('button', { name: msg('redist.install') }));
    closeRedist();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    const dialog = await openDialog();
    await within(dialog).findByText(vc.name);

    installing.resolve(done);

    await vi.waitFor(() => expect(toastTexts()).toEqual([['success', msg('redist.resultOk', { name: vc.name })]]));
    expect(within(dialog).queryByText(msg('redist.resultOk', { name: vc.name }))).toBeNull();
    await vi.waitFor(() => expect(within(row(dialog, vc.name)).getByRole('button', { name: msg('redist.install') }).getAttribute('aria-busy')).toBeNull());
  });

  it('says a late refusal once in a danger toast naming the package with the reason by code', async () => {
    const installing = held<RedistResult>();
    vi.mocked(installRedist).mockReturnValueOnce(installing.promise);
    render(RedistModal, { target: app });
    const first = await openDialog();
    const keys = user();
    await within(first).findByText(vc.name);
    await keys.click(within(row(first, vc.name)).getByRole('button', { name: msg('redist.install') }));
    closeRedist();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

    installing.reject(new Error('typhon:redist.elevation_declined: no'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', msg('redist.lateFailed', { name: vc.name, reason: msg('redist.elevation_declined') })]]));
  });

  it('says a refusal while it is open in the row, by code, and raises no toast', async () => {
    vi.mocked(installRedist).mockRejectedValueOnce(new Error('typhon:redist.failed: exit 1603'));
    render(RedistModal, { target: app });
    const dialog = await openDialog();
    const keys = user();
    await within(dialog).findByText(vc.name);

    await keys.click(within(row(dialog, vc.name)).getByRole('button', { name: msg('redist.install') }));

    const alert = await within(row(dialog, vc.name)).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('redist.failed'));
    expect(toastTexts()).toEqual([]);
  });
});
