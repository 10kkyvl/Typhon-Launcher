import { render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { libraryGames, libraryLoaded, libraryLoadError } from '../../stores/library';
import { getGames, getRunningGames } from '../../services/library';
import { igdbIdsOf } from '../../services/profile';
import { locale, msg } from '../../i18n';
import { calls, resetBindings } from '../../testing/fakeBinding';
import { renderDom, user } from '../../testing/dom';
import { libraryGame } from './blocksTesting';
import GamePicker from './GamePicker.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: true }));
vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));

const GAMES = [
  libraryGame('a', 'Elden Ring'),
  libraryGame('b', 'Hades'),
  libraryGame('c', 'Hollow Knight'),
  libraryGame('d', 'Unlisted Game'),
  libraryGame('e', 'Hidden Game', { hidden: true }),
  libraryGame('f', 'Local Game', { canonicalGameId: '' }),
];
const IDS = { 'canon-a': '1', 'canon-b': '2', 'canon-c': '3' };

function mount(props: { chosen?: number[]; disabled?: boolean } = {}) {
  const onpick = vi.fn();
  const rendered = renderDom(GamePicker, { chosen: props.chosen ?? [], disabled: props.disabled, onpick });
  return { ...rendered, onpick };
}

const rowNames = () => within(screen.getByRole('list')).getAllByRole('button').map((row) => row.querySelector('.title')?.textContent);

beforeEach(() => {
  locale.set('en');
  resetBindings();
  vi.mocked(igdbIdsOf).mockReset();
  vi.mocked(igdbIdsOf).mockResolvedValue(IDS);
  libraryGames.set(GAMES);
});

afterEach(() => {
  libraryGames.set([]);
  libraryLoaded.set(false);
  libraryLoadError.set(null);
  document.body.replaceChildren();
  expect(calls).toEqual([]);
});

describe('the game picker', () => {
  it('offers the library games that are in the catalog and not hidden, and asks for their catalog ids once', async () => {
    mount();
    await screen.findByRole('list');

    expect(rowNames()).toEqual(['Elden Ring', 'Hades', 'Hollow Knight', 'Unlisted Game']);
    expect(vi.mocked(igdbIdsOf).mock.calls).toEqual([[['canon-a', 'canon-b', 'canon-c', 'canon-d']]]);
  });

  it('narrows the rows to the titles that hold the typed words, whatever their case', async () => {
    const { user } = mount();
    await screen.findByRole('list');

    await user.type(screen.getByRole('textbox', { name: msg('profile.pickerSearch') }), 'ho');
    expect(rowNames()).toEqual(['Hollow Knight']);
  });

  it('says when the typed words match no game', async () => {
    const { user } = mount();
    await screen.findByRole('list');

    await user.type(screen.getByRole('textbox', { name: msg('profile.pickerSearch') }), 'zzz');
    expect(screen.getByText(msg('profile.pickerNoMatch'))).toBeTruthy();
    expect(screen.queryByRole('list')).toBeNull();
  });

  it('says when the library holds no game the catalog knows', async () => {
    libraryGames.set([libraryGame('f', 'Local Game', { canonicalGameId: '' })]);
    mount();

    expect(await screen.findByText(msg('profile.pickerEmpty'))).toBeTruthy();
  });

  it('shows at most forty rows', async () => {
    libraryGames.set(Array.from({ length: 45 }, (_, index) => libraryGame(`g${index}`, `Game ${index}`)));
    mount();
    await screen.findByRole('list');

    expect(rowNames()).toHaveLength(40);
  });

  it('hands the pressed game to the caller with its catalog id', async () => {
    const { user, onpick } = mount();

    await user.click(await screen.findByRole('button', { name: 'Hades' }));
    expect(onpick.mock.calls).toEqual([[{ igdbId: 2, game: expect.objectContaining({ id: 'b', title: 'Hades', canonicalGameId: 'canon-b' }) }]]);
  });

  it('picks with the keyboard: Tab from the search field reaches the first row, Enter picks it', async () => {
    const { user, onpick } = mount();
    await screen.findByRole('list');

    await user.click(screen.getByRole('textbox', { name: msg('profile.pickerSearch') }));
    await user.tab();
    expect(document.activeElement?.textContent).toContain('Elden Ring');
    await user.keyboard('{Enter}');
    expect(onpick.mock.calls.map(([pick]) => pick.igdbId)).toEqual([1]);
  });

  it('marks a game already chosen and one not in the catalog, and picks neither', async () => {
    const { user, onpick } = mount({ chosen: [2] });
    const taken = await screen.findByRole('button', { name: /Hades/ });
    const unlisted = screen.getByRole('button', { name: /Unlisted Game/ });

    expect(taken.textContent).toContain(msg('profile.pickerTaken'));
    expect(unlisted.textContent).toContain(msg('profile.pickerNoCatalog'));
    await user.click(taken);
    await user.click(unlisted);
    expect(onpick).not.toHaveBeenCalled();
  });

  it('picks nothing while the caller holds it', async () => {
    const { user, onpick } = mount({ disabled: true });

    await user.click(await screen.findByRole('button', { name: 'Hades' }));
    expect(onpick).not.toHaveBeenCalled();
  });

  it('says it is loading while the catalog ids are on their way', async () => {
    vi.mocked(igdbIdsOf).mockReturnValueOnce(new Promise(() => {}));
    mount();

    expect(screen.getByText(msg('social.loadingEllipsis'))).toBeTruthy();
  });

  // Every failed load on a page carries its Retry in the notice's action, outside the live region.
  it('keeps the retry of a failed load inside its notice, and the notice goes once it loads', async () => {
    vi.mocked(igdbIdsOf).mockRejectedValueOnce(new Error('offline'));
    const { user } = mount();

    const alert = await screen.findByRole('alert');
    const notice = alert.closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    expect(alert.contains(retry)).toBe(false);
    expect(alert.textContent).toContain(msg('profile.pickerFailed'));

    await user.click(retry);
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(igdbIdsOf).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('list')).toBeTruthy();
  });
});

describe('the game picker, when something goes wrong or goes away', () => {
  it('says that the library could not be read, in the picker, instead of an empty list, and reads it again on Retry', async () => {
    libraryGames.set([]);
    libraryLoadError.set(new Error('the library service is down'));
    vi.mocked(getGames).mockResolvedValueOnce(GAMES);
    vi.mocked(getRunningGames).mockResolvedValueOnce([]);
    const { user } = mount();

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('library.loadFailedText'));
    expect(screen.queryByText(msg('profile.pickerEmpty'))).toBeNull();

    await user.click(within(alert.closest('.notice') as HTMLElement).getByRole('button', { name: msg('common.retry') }));
    await screen.findByRole('list');
    expect(screen.queryByRole('alert')).toBeNull();
    expect(vi.mocked(getGames)).toHaveBeenCalledTimes(1);
  });

  it('keeps focus on a pressed game that turns taken, held and described as taken, and picks it once', async () => {
    let chosen: number[] = [];
    const onpick = vi.fn((pick: { igdbId: number }) => {
      chosen = [...chosen, pick.igdbId];
      void view.rerender({ chosen });
    });
    const view = renderDom(GamePicker, { chosen, onpick });
    const row = await screen.findByRole('button', { name: 'Hades' });
    row.focus();

    await view.user.keyboard('{Enter}');
    await vi.waitFor(() => expect(row.getAttribute('aria-disabled')).toBe('true'));
    expect(document.activeElement).toBe(row);
    expect(row.matches(':disabled')).toBe(false);
    expect(document.getElementById(row.getAttribute('aria-describedby') ?? '')?.textContent).toBe(msg('profile.pickerTaken'));
    await view.user.keyboard('{Enter}');
    await view.user.click(row);
    expect(onpick).toHaveBeenCalledTimes(1);
  });

  it('a held Enter picks once, whatever the page does with the pick', async () => {
    const { user, onpick } = mount();
    (await screen.findByRole('button', { name: 'Hades' })).focus();

    await user.keyboard('{Enter>4/}');
    expect(onpick).toHaveBeenCalledTimes(1);
  });

  it('hands focus back to the control that opened it when a pick closes it', async () => {
    const opener = document.createElement('button');
    opener.textContent = 'Choose a game';
    document.body.append(opener);
    const target = document.createElement('div');
    document.body.append(target);
    const view = render(GamePicker, { target, props: { chosen: [], onpick: () => view.unmount() } });
    const row = await screen.findByRole('button', { name: 'Hades' });
    row.focus();

    await user().keyboard('{Enter}');
    await vi.waitFor(() => expect(target.querySelector('.picker')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it('leaves focus alone when the pick that closed it was a press that held no focus', async () => {
    const opener = document.createElement('button');
    document.body.append(opener);
    const target = document.createElement('div');
    document.body.append(target);
    const view = render(GamePicker, { target, props: { chosen: [], onpick: () => view.unmount() } });
    const row = await screen.findByRole('button', { name: 'Hades' });

    expect(document.activeElement).toBe(document.body);
    row.click();
    await vi.waitFor(() => expect(target.querySelector('.picker')).toBeNull());
    await tick();
    expect(document.activeElement).toBe(document.body);
  });
});
