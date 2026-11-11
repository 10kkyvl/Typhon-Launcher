import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { enableWrapper } from '../services/wrappers';
import { closeGameProperties, openGameProperties } from '../stores/gameProperties';
import { libraryGames } from '../stores/library';
import { user } from '../testing/dom';
import GamePropertiesModal from './GamePropertiesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/wrappers', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/wrappers')>()),
  inspectWrapper: vi.fn(async () => ({ supported: true, arch: 'x86', imports: ['d3d9.dll'], suggested: 'dgvoodoo', active: null, problem: null })),
  enableWrapper: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame]);
});

afterEach(() => {
  closeGameProperties();
  libraryGames.set([]);
  app.remove();
});

describe('GamePropertiesModal while compatibility mode downloads', () => {
  it('can be closed mid-download (nothing to cancel, it may take minutes); reopened, it shows the work still running and starts nothing new', async () => {
    let finish!: () => void;
    vi.mocked(enableWrapper).mockReturnValueOnce(new Promise<void>((resolve) => (finish = resolve)));
    render(GamePropertiesModal, { target: app });
    const keys = user();
    openGameProperties('g1');
    const toggle = await screen.findByRole('switch', { name: msg('modals.compatToggle') });

    await keys.click(toggle);
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();

    openGameProperties('g1');
    const again = await screen.findByRole('switch', { name: msg('modals.compatToggle') });
    expect(again).toHaveProperty('disabled', true);
    await keys.click(again);
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);

    finish();
    await vi.waitFor(() => expect(screen.getByRole('switch', { name: msg('modals.compatToggle') })).toHaveProperty('disabled', false));
  });
});
