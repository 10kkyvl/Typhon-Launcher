import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { installRedist, installedRuntimes, officialLinks, scanRedist, type RedistPackage, type RedistResult } from '../services/redist';
import { libraryGames } from '../stores/library';
import { closeRedist, openRedist } from '../stores/redist';
import { user } from '../testing/dom';
import RedistModal from './RedistModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/redist', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/redist')>()),
  scanRedist: vi.fn(),
  installedRuntimes: vi.fn(),
  officialLinks: vi.fn(),
  installRedist: vi.fn(),
}));

const vc: RedistPackage = { name: 'VC++ 2015', path: '/games/hades/_redist/vc.exe', kind: 'vcredist', arch: 'x64', version: '14.0', sizeBytes: 1024 } as RedistPackage;

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
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
  vi.mocked(installedRuntimes).mockResolvedValue([
    { name: 'Microsoft Visual C++ 2022 x64', version: '14.40', arch: 'x64' },
    { name: 'Microsoft Visual C++ 2022 x86', version: '14.40', arch: 'x86' },
  ]);
  vi.mocked(officialLinks).mockResolvedValue([]);
});

afterEach(() => {
  closeRedist();
  libraryGames.set([]);
  vi.mocked(scanRedist).mockReset();
  vi.mocked(installedRuntimes).mockReset();
  vi.mocked(officialLinks).mockReset();
  vi.mocked(installRedist).mockReset();
  document.body.replaceChildren();
});

async function openDialog() {
  render(RedistModal, { target: app });
  openRedist('g1');
  return screen.findByRole('dialog', { name: msg('redist.title', { title: 'Hades' }) });
}

describe('RedistModal: the game folder scan', () => {
  it('says it is scanning with a labelled spinner', async () => {
    const scan = deferred<RedistPackage[]>();
    vi.mocked(scanRedist).mockReturnValueOnce(scan.promise);
    const dialog = await openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('redist.scanning'));
    scan.resolve([]);
  });

  it('keeps a failed scan and the focus on its busy Retry until the answer, then says there is nothing bundled', async () => {
    const again = deferred<RedistPackage[]>();
    vi.mocked(scanRedist).mockRejectedValueOnce(new Error('disk')).mockReturnValueOnce(again.promise);
    const dialog = await openDialog();
    const keys = user();
    const retry = await within(dialog).findByRole('button', { name: msg('redist.retry') });
    const notice = retry.closest('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve([]);
    const empty = await within(dialog).findByText(msg('redist.bundledEmpty'));
    expect(empty.closest('.empty')).not.toBeNull();
    expect(notice.isConnected).toBe(false);
  });

  it('moves the focus from a Retry that worked to the first package it found', async () => {
    vi.mocked(scanRedist).mockRejectedValueOnce(new Error('disk')).mockResolvedValueOnce([vc]);
    const dialog = await openDialog();
    const keys = user();
    (await within(dialog).findByRole('button', { name: msg('redist.retry') })).focus();

    await keys.keyboard('{Enter}');

    const install = await within(dialog).findByRole('button', { name: msg('redist.install') });
    await vi.waitFor(() => expect(document.activeElement).not.toBe(document.body));
    expect(document.activeElement).toBe(install);
  });

  it('says a scan that fails again in the same words again, and keeps the focus on its Retry', async () => {
    vi.mocked(scanRedist).mockRejectedValue(new Error('disk'));
    const dialog = await openDialog();
    const keys = user();
    const region = await within(dialog).findByRole('alert');
    const before = region.firstElementChild;
    const retry = within(region.closest('.notice') as HTMLElement).getByRole('button', { name: msg('redist.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent?.trim()).toBe(before?.textContent?.trim());
    expect(document.activeElement).toBe(retry);
  });

  it('keeps the focus on the pressed install button, busy, while the package installs', async () => {
    const installing = deferred<RedistResult>();
    vi.mocked(scanRedist).mockResolvedValueOnce([vc]);
    vi.mocked(installRedist).mockReturnValueOnce(installing.promise);
    const dialog = await openDialog();
    const keys = user();
    const install = await within(dialog).findByRole('button', { name: msg('redist.install') });
    install.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('redist.installing') });
    expect(busy).toBe(install);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);
    installing.resolve({ exitCode: 0, rebootRequired: false, alreadyInstalled: false });
  });
});

describe('RedistModal: closed and opened again while the scan Retry runs', () => {
  it('opens scanning, without the old failure: no empty notice with a busy Retry', async () => {
    const never = new Promise<never>(() => {});
    vi.mocked(scanRedist).mockRejectedValueOnce(new Error('disk')).mockReturnValue(never);
    const first = await openDialog();
    const keys = user();
    await keys.click(await within(first).findByRole('button', { name: msg('redist.retry') }));

    closeRedist();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    openRedist('g1');

    const dialog = await screen.findByRole('dialog', { name: msg('redist.title', { title: 'Hades' }) });
    expect(dialog.querySelectorAll('.notice[data-tone="danger"]').length).toBe(0);
    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('redist.scanning'));
  });
});

describe('RedistModal: an old scan Retry answering after the dialog was opened again', () => {
  it('leaves the new Retry busy, and a second press sends nothing', async () => {
    const old = deferred<RedistPackage[]>();
    const never = new Promise<never>(() => {});
    vi.mocked(scanRedist).mockRejectedValueOnce(new Error('disk')).mockReturnValueOnce(old.promise).mockRejectedValueOnce(new Error('disk')).mockReturnValue(never);
    const first = await openDialog();
    const keys = user();
    await keys.click(await within(first).findByRole('button', { name: msg('redist.retry') }));
    closeRedist();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    openRedist('g1');
    const dialog = await screen.findByRole('dialog', { name: msg('redist.title', { title: 'Hades' }) });
    const retry = await within(dialog).findByRole('button', { name: msg('redist.retry') });
    await keys.click(retry);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    const sent = vi.mocked(scanRedist).mock.calls.length;

    old.resolve([]);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(retry.getAttribute('aria-busy')).toBe('true');
    await keys.click(retry);
    expect(vi.mocked(scanRedist).mock.calls.length).toBe(sent);
  });
});

describe('RedistModal: the runtimes of the system', () => {
  it('states a missing runtime in a warning notice that holds its download link', async () => {
    vi.mocked(scanRedist).mockResolvedValueOnce([]);
    vi.mocked(installedRuntimes).mockResolvedValue([{ name: 'Microsoft Visual C++ 2022 x64', version: '14.40', arch: 'x64' }]);
    vi.mocked(officialLinks).mockResolvedValue([{ name: 'VC x86', url: 'https://aka.ms/vs/17/release/vc_redist.x86.exe', arch: 'x86' }]);
    const dialog = await openDialog();

    const text = await within(dialog).findByText(msg('redist.missing', { arch: 'x86' }));
    const notice = text.closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
    const link = within(notice).getByRole('link', { name: msg('redist.download') });
    expect(link.getAttribute('href')).toBe('https://aka.ms/vs/17/release/vc_redist.x86.exe');
    expect(link.classList.contains('text-link')).toBe(true);
  });

  it('says a failed read of the runtimes in a danger notice', async () => {
    vi.mocked(scanRedist).mockResolvedValueOnce([]);
    vi.mocked(installedRuntimes).mockRejectedValue(new Error('registry'));
    const dialog = await openDialog();

    const alerts = await within(dialog).findAllByRole('alert');
    expect(alerts.some((alert) => alert.closest('[data-tone="danger"]'))).toBe(true);
  });
});
