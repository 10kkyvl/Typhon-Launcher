import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { executableCandidates } from '../services/install';
import { getLaunchArgsLine, setExecutable, setLaunchArgsLine, type LibraryGame } from '../services/library';
import { inspectWrapper, type WrapperInfo } from '../services/wrappers';
import { closeGameProperties, openGameProperties } from '../stores/gameProperties';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import GamePropertiesModal from './GamePropertiesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/install')>()),
  executableCandidates: vi.fn(),
}));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  getLaunchArgsLine: vi.fn(),
  setLaunchArgsLine: vi.fn(),
  setExecutable: vi.fn(),
  getProfiles: vi.fn(async () => []),
}));
vi.mock('../services/wrappers', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/wrappers')>()),
  inspectWrapper: vi.fn(),
}));

function wrapper(patch: Partial<WrapperInfo> = {}): WrapperInfo {
  return { supported: true, arch: 'x86', imports: ['d3d9.dll'], suggested: 'dgvoodoo', active: null, problem: null, ...patch } as WrapperInfo;
}

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
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: '/games/hades/Hades.exe' } as LibraryGame]);
  vi.mocked(executableCandidates).mockResolvedValue([
    { path: '/games/hades/Hades.exe', score: 1 },
    { path: '/games/hades/x64/Hades.exe', score: 0.5 },
  ]);
  vi.mocked(getLaunchArgsLine).mockResolvedValue('');
  vi.mocked(inspectWrapper).mockResolvedValue(wrapper());
});

afterEach(() => {
  closeGameProperties();
  libraryGames.set([]);
  toasts.set([]);
  vi.mocked(executableCandidates).mockReset();
  vi.mocked(getLaunchArgsLine).mockReset();
  vi.mocked(setLaunchArgsLine).mockReset();
  vi.mocked(setExecutable).mockReset();
  vi.mocked(inspectWrapper).mockReset();
  document.body.replaceChildren();
});

async function openDialog() {
  render(GamePropertiesModal, { target: app });
  openGameProperties('g1');
  return screen.findByRole('dialog', { name: msg('modals.propertiesTitle', { title: 'Hades' }) });
}

describe('GamePropertiesModal: the found executables', () => {
  it('say which one is current, and the pressed one keeps the focus while it is set', async () => {
    const saved = deferred<LibraryGame>();
    vi.mocked(setExecutable).mockReturnValueOnce(saved.promise);
    const dialog = await openDialog();
    const keys = user();
    const current = await within(dialog).findByRole('button', { name: '/games/hades/Hades.exe' });
    const other = within(dialog).getByRole('button', { name: '/games/hades/x64/Hades.exe' });
    expect(current.getAttribute('aria-current')).toBe('true');
    expect(other.getAttribute('aria-current')).toBeNull();
    other.focus();

    await keys.keyboard('{Enter}');

    expect(vi.mocked(setExecutable)).toHaveBeenCalledWith('g1', '/games/hades/x64/Hades.exe');
    expect(other).toHaveProperty('disabled', false);
    expect(other.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(other);
    saved.resolve({} as LibraryGame);
  });

  it('say a failed load in a danger notice', async () => {
    vi.mocked(executableCandidates).mockRejectedValue(new Error('disk'));
    const dialog = await openDialog();

    const text = await within(dialog).findByText(msg('modals.propertiesExeLoadFailed'));
    expect(text.closest('[data-tone="danger"]')).not.toBeNull();
    expect(text.closest('[role="alert"]')).not.toBeNull();
  });
});

describe('GamePropertiesModal: the launch arguments', () => {
  it('are a field named by their heading; a failed save says so in a danger notice, the save button stays focused and busy while it runs', async () => {
    const saved = deferred<LibraryGame>();
    vi.mocked(setLaunchArgsLine).mockReturnValueOnce(saved.promise);
    const dialog = await openDialog();
    const keys = user();
    const field = await within(dialog).findByRole('textbox', { name: msg('modals.propertiesArgsTitle') });
    await vi.waitFor(() => expect(field).toHaveProperty('disabled', false));
    await keys.type(field, '-windowed');
    const save = within(dialog).getByRole('button', { name: msg('modals.propertiesArgsSave') });
    save.focus();

    await keys.keyboard('{Enter}');

    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(save);

    saved.reject(new Error('disk'));
    const text = await within(dialog).findByText(msg('modals.propertiesArgsSaveFailed'), { selector: '[data-tone] *' });
    expect(text.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

describe('GamePropertiesModal: compatibility mode', () => {
  it('says it is reading the game with a labelled spinner', async () => {
    const read = deferred<WrapperInfo>();
    vi.mocked(inspectWrapper).mockReturnValueOnce(read.promise);
    const dialog = await openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('modals.compatLoading'));
    read.resolve(wrapper());
  });

  it('offers to force it with a disclosure that says whether the choice of wrapper is shown', async () => {
    vi.mocked(inspectWrapper).mockResolvedValue(wrapper({ suggested: '' }));
    const dialog = await openDialog();
    const keys = user();
    const force = await within(dialog).findByRole('button', { name: msg('modals.compatForce') });
    expect(force.getAttribute('aria-expanded')).toBe('false');
    const controls = force.getAttribute('aria-controls') ?? '';
    expect(document.getElementById(controls)).not.toBeNull();

    await keys.click(force);

    expect(force.getAttribute('aria-expanded')).toBe('true');
    const shown = document.getElementById(controls) as HTMLElement;
    expect(within(shown).getByRole('combobox', { name: msg('modals.compatWrapper') })).toBeTruthy();
  });

  it('states a problem in a titled danger notice that holds its actions', async () => {
    vi.mocked(inspectWrapper).mockResolvedValue(
      wrapper({ active: { wrapper: 'dgvoodoo', version: '2.8', preset: 'default', files: ['d3d9.dll'] }, problem: { code: 'wrappers.tampered', file: 'd3d9.dll' } } as Partial<WrapperInfo>),
    );
    const dialog = await openDialog();

    const title = await within(dialog).findByText(msg('modals.compatProblemTitle'));
    const notice = title.closest('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(within(notice).getByRole('button', { name: msg('modals.compatRevert') })).toBeTruthy();
    expect(within(notice).getByRole('button', { name: msg('modals.compatKeepChanged') })).toBeTruthy();
  });
});
