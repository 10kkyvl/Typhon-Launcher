import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { launchGame, type LibraryGame } from '../services/library';
import { collections } from '../stores/collections';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import RandomGameModal from './RandomGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  launchGame: vi.fn(),
}));
vi.mock('../game/randomPick', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../game/randomPick')>()),
  reducedMotion: () => true,
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  libraryGames.set([]);
  collections.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

function openDialog() {
  render(RandomGameModal, { target: app, props: { onclose: vi.fn() } });
  return screen.getByRole('dialog', { name: msg('games.randomTitle') });
}

describe('RandomGameModal', () => {
  it('says that nothing matches the filters in a warning notice that announces', async () => {
    const dialog = openDialog();
    const keys = user();

    await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSpin') }));

    const notice = within(dialog).getByText(msg('games.randomEmpty')).closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"]')).not.toBeNull();
  });

  it('says there are no collections to pick from with an inline empty state', async () => {
    const dialog = openDialog();
    const keys = user();

    await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSourceCollection') }));

    expect(within(dialog).getByText(msg('games.randomNoCollections')).closest('.empty')).not.toBeNull();
  });

  it('keeps the focus on "play", busy, while the game starts', async () => {
    let started!: () => void;
    vi.mocked(launchGame).mockReturnValueOnce(new Promise<boolean>((resolve) => (started = () => resolve(true))));
    libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame]);
    const dialog = openDialog();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSpin') }));
    const play = await within(dialog).findByRole('button', { name: msg('games.randomPlay') });
    play.focus();

    await keys.keyboard('{Enter}');

    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(play).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(play);
    started();
  });
});
