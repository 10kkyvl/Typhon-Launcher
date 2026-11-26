import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { launchGame, type LibraryGame } from '../services/library';
import { collections } from '../stores/collections';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { sentToGo } from '../testing/gameDialogsTesting';
import RandomGameModal from './RandomGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
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
  const sent = sentToGo();
  resetBindings();
  libraryGames.set([]);
  collections.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
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

describe('RandomGameModal: playing the pick', () => {
  const hades = { id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame;

  async function picked() {
    libraryGames.set([hades]);
    const onclose = vi.fn();
    render(RandomGameModal, { target: app, props: { onclose } });
    const dialog = screen.getByRole('dialog', { name: msg('games.randomTitle') });
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSpin') }));
    const play = await within(dialog).findByRole('button', { name: msg('games.randomPlay') });
    return { onclose, dialog, keys, play };
  }

  it('closes once the game has started', async () => {
    vi.mocked(launchGame).mockResolvedValueOnce(true);
    const { onclose, keys, play } = await picked();

    await keys.click(play);

    expect(launchGame).toHaveBeenCalledExactlyOnceWith('g1');
    await vi.waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
  });

  it('starts one game: while the launch is on its way Play does nothing more, and Filters and Again are held with the reason', async () => {
    let started!: () => void;
    vi.mocked(launchGame).mockReturnValueOnce(new Promise<boolean>((resolve) => (started = () => resolve(true))));
    const { onclose, dialog, keys, play } = await picked();
    play.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    for (const name of [msg('games.randomFilters'), msg('games.randomAgain')]) {
      const button = within(dialog).getByRole('button', { name });
      expect(button).toHaveProperty('disabled', false);
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(msg('games.randomLaunching'));
      await keys.click(button);
    }
    await keys.keyboard('{Escape}');
    expect(within(dialog).getByRole('button', { name: msg('games.randomPlay') })).toBe(play);
    expect(launchGame).toHaveBeenCalledTimes(1);
    expect(onclose).not.toHaveBeenCalled();

    started();
    await vi.waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
  });

  it.each([
    ['a code with words', new Error('typhon:library.already_running: already running'), () => msg('library.already_running')],
    ['no code of its own', new Error('typhon:library.something_new: запуск не удался'), () => msg('games.errorPlayFailed')],
  ])('says a refused launch (%s) in the dialog, in the launcher\'s words, and moves the focus there', async (_case, err, words) => {
    vi.mocked(launchGame).mockRejectedValueOnce(err);
    const { onclose, dialog, keys, play } = await picked();
    play.focus();

    await keys.keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(words());
    expect(dialog.textContent).not.toContain(err.message.split(': ').at(-1));
    expect(get(toasts)).toEqual([]);
    expect(onclose).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true));
  });
});
