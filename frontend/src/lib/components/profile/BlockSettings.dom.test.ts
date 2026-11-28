import { fireEvent, screen, within } from '@testing-library/svelte';
import { tick, type ComponentProps } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { libraryGames } from '../../stores/library';
import { igdbIdsOf } from '../../services/profile';
import { msg } from '../../i18n';
import { locale } from '../../i18n';
import { calls, resetBindings } from '../../testing/fakeBinding';
import { renderDom } from '../../testing/dom';
import type { ProfileBlock, ProfileLayout } from '../../services/account';
import { libraryGame } from './blocksTesting';
import BlockSettings from './BlockSettings.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: true }));
vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));

function block(type: string, config: Record<string, unknown> = {}, id = 'b1'): ProfileBlock {
  return { id, type, width: 'full', config };
}

const TITLES: Record<number, string> = { 11: 'Alpha', 12: 'Beta', 13: 'Gamma' };

function mount(target: ProfileBlock, patch: Partial<ComponentProps<typeof BlockSettings>> = {}, others: ProfileBlock[] = []) {
  const onconfig = vi.fn();
  const onremember = vi.fn();
  const onremove = vi.fn();
  const onclose = vi.fn();
  const layout: ProfileLayout = { version: 1, blocks: [target, ...others] };
  const props: ComponentProps<typeof BlockSettings> = {
    block: target,
    layout,
    titleOf: (id: number) => TITLES[id] ?? `#${id}`,
    onconfig,
    onremember,
    onremove,
    onclose,
    ...patch,
  };
  const rendered = renderDom(BlockSettings, props);
  return { ...rendered, onconfig, onremember, onremove, onclose };
}

beforeEach(() => {
  locale.set('en');
  resetBindings();
  vi.mocked(igdbIdsOf).mockReset();
  vi.mocked(igdbIdsOf).mockResolvedValue({ 'canon-a': '21', 'canon-b': '22' });
  libraryGames.set([libraryGame('a', 'Picked One'), libraryGame('b', 'Picked Two')]);
});

afterEach(() => {
  libraryGames.set([]);
  expect(calls).toEqual([]);
});

describe('the settings of a block', () => {
  it('are a region named by the block, headed by its name, with a close and a remove control', async () => {
    const { user, onclose, onremove } = mount(block('genres'));
    const region = screen.getByRole('region', { name: msg('profile.blockSettings', { name: msg('profile.blockGenres') }) });

    expect(within(region).getByRole('heading', { level: 2 }).textContent).toBe(msg('profile.blockGenres'));
    await user.click(within(region).getByRole('button', { name: msg('common.close') }));
    await user.click(within(region).getByRole('button', { name: msg('profile.removeBlock', { name: msg('profile.blockGenres') }) }));
    expect([onclose.mock.calls.length, onremove.mock.calls.length]).toEqual([1, 1]);
  });

  it('say that a block without settings has none, and the completed block who sees it', () => {
    mount(block('genres'));
    expect(screen.getByText(msg('profile.blockNoSettings'))).toBeTruthy();
    expect(screen.queryByText(msg('profile.completedVisibility'))).toBeNull();
  });

  it('say who sees the completed block', () => {
    mount(block('completed'));
    expect(screen.getByText(msg('profile.blockNoSettings'))).toBeTruthy();
    expect(screen.getByText(msg('profile.completedVisibility'))).toBeTruthy();
  });
});

describe('a pinned block', () => {
  it('names the pinned game, or says none is picked', () => {
    const { unmount } = mount(block('pinned', { igdbId: 12, caption: '' }));
    expect(screen.getByText('Beta')).toBeTruthy();
    unmount();
    mount(block('pinned', { igdbId: 0, caption: '' }));
    expect(screen.getByText(msg('profile.pinnedNone'))).toBeTruthy();
  });

  it('writes the typed caption to the draft, held to its limit', async () => {
    const { user, onconfig } = mount(block('pinned', { igdbId: 12, caption: 'Hi' }));
    const caption = screen.getByRole('textbox', { name: msg('profile.pinnedCaption') }) as HTMLTextAreaElement;

    expect(caption.value).toBe('Hi');
    expect(caption.maxLength).toBe(140);
    await user.type(caption, '!');
    expect(onconfig).toHaveBeenLastCalledWith({ caption: 'Hi!' });
  });

  it('pins the game pressed in the picker and remembers it', async () => {
    const { user, onconfig, onremember } = mount(block('pinned', { igdbId: 0, caption: '' }));

    await user.click(await screen.findByRole('button', { name: 'Picked Two' }));
    expect(onconfig).toHaveBeenCalledWith({ igdbId: 22 });
    expect(onremember).toHaveBeenCalledWith({ igdbId: 22, game: expect.objectContaining({ id: 'b', title: 'Picked Two' }) });
  });

  it('says a caption with characters that are not allowed is wrong, at the caption', () => {
    mount(block('pinned', { igdbId: 12, caption: 'a\u0007' }), { issue: { code: 'bad_chars', field: 'caption' } });
    const caption = screen.getByRole('textbox', { name: msg('profile.pinnedCaption') });

    expect(caption.getAttribute('aria-invalid')).toBe('true');
    const error = document.getElementById(caption.getAttribute('aria-describedby')?.split(' ').at(-1) ?? '');
    expect(error?.textContent).toContain('characters that are not allowed');
  });
});

describe('a collection block', () => {
  it('offers the sources no other collection takes, its own included', async () => {
    const other = block('collection', { source: 'favorites' }, 'b2');
    const { user } = mount(block('collection', { source: 'most_played' }), {}, [other]);

    await user.click(screen.getByRole('combobox', { name: msg('profile.collectionSource') }));
    const names = screen.getAllByRole('option').map((option) => option.textContent?.trim());
    expect(names).toContain(msg('profile.sourceManual'));
    expect(names).not.toContain(msg('social.favoriteGamesTitle'));
    expect(names.length).toBe(3);
  });

  it('starts an empty list of its own when switched to manual, and lets an automatic source fill itself', async () => {
    const { user, onconfig } = mount(block('collection', { source: 'most_played' }));
    expect(screen.getByText(msg('profile.collectionAutoHint'))).toBeTruthy();

    await user.click(screen.getByRole('combobox', { name: msg('profile.collectionSource') }));
    await user.click(screen.getByRole('option', { name: msg('profile.sourceManual') }));
    expect(onconfig).toHaveBeenLastCalledWith({ source: 'manual', title: '', igdbIds: [] });
  });

  it('switched from manual to an automatic source, drops its own title and games', async () => {
    const { user, onconfig } = mount(block('collection', { source: 'manual', title: 'Mine', igdbIds: [11] }));

    await user.click(screen.getByRole('combobox', { name: msg('profile.collectionSource') }));
    await user.click(screen.getByRole('option', { name: msg('social.favoriteGamesTitle') }));
    expect(onconfig).toHaveBeenLastCalledWith({ source: 'favorites', title: undefined, igdbIds: undefined });
  });

  it('writes its typed title to the draft, held to its limit', async () => {
    const { user, onconfig } = mount(block('collection', { source: 'manual', title: 'Mine', igdbIds: [11] }));
    const title = screen.getByRole('textbox', { name: msg('profile.collectionTitle') }) as HTMLInputElement;

    expect(title.value).toBe('Mine');
    expect(title.maxLength).toBe(40);
    await user.type(title, 's');
    expect(onconfig).toHaveBeenLastCalledWith({ title: 'Mines' });
  });

  it('asks for a first game while its list is empty', () => {
    mount(block('collection', { source: 'manual', title: '', igdbIds: [] }));
    expect(screen.getByText(msg('profile.collectionEmpty'))).toBeTruthy();
  });

  it('moves a game up or down and removes one, each written to the draft', async () => {
    const { user, onconfig } = mount(block('collection', { source: 'manual', title: '', igdbIds: [11, 12, 13] }));

    await user.click(screen.getByRole('button', { name: msg('social.moveUp', { title: 'Beta' }) }));
    expect(onconfig).toHaveBeenLastCalledWith({ igdbIds: [12, 11, 13] });
    await user.click(screen.getByRole('button', { name: msg('social.moveDown', { title: 'Beta' }) }));
    expect(onconfig).toHaveBeenLastCalledWith({ igdbIds: [11, 13, 12] });
    await user.click(screen.getByRole('button', { name: msg('profile.collectionRemoveGame', { title: 'Gamma' }) }));
    expect(onconfig).toHaveBeenLastCalledWith({ igdbIds: [11, 12] });
  });

  it('does not move the first game up or the last one down', async () => {
    const { user, onconfig } = mount(block('collection', { source: 'manual', title: '', igdbIds: [11, 12] }));

    await user.click(screen.getByRole('button', { name: msg('social.moveUp', { title: 'Alpha' }) }));
    await user.click(screen.getByRole('button', { name: msg('social.moveDown', { title: 'Beta' }) }));
    expect(onconfig).not.toHaveBeenCalled();
  });

  it('adds the game pressed in the picker at the end, and remembers it', async () => {
    const { user, onconfig, onremember } = mount(block('collection', { source: 'manual', title: '', igdbIds: [11] }));

    await user.click(await screen.findByRole('button', { name: 'Picked One' }));
    expect(onconfig).toHaveBeenCalledWith({ igdbIds: [11, 21] });
    expect(onremember).toHaveBeenCalledWith({ igdbId: 21, game: expect.objectContaining({ id: 'a' }) });
  });

  it('says it is full at its limit, and offers no picker then', () => {
    const ids = Array.from({ length: 12 }, (_, index) => 100 + index);
    mount(block('collection', { source: 'manual', title: '', igdbIds: ids }));

    expect(screen.getByText(msg('profile.collectionFull', { count: 12 }))).toBeTruthy();
    expect(screen.queryByRole('textbox', { name: msg('profile.pickerSearch') })).toBeNull();
  });
});

describe('a text block', () => {
  it('writes its typed title and text to the draft, each held to its limit', async () => {
    const { user, onconfig } = mount(block('text', { title: 'T', body: 'B' }));
    const title = screen.getByRole('textbox', { name: msg('profile.textTitle') }) as HTMLInputElement;
    const body = screen.getByRole('textbox', { name: msg('profile.textBody') }) as HTMLTextAreaElement;

    expect([title.value, title.maxLength, body.value, body.maxLength]).toEqual(['T', 40, 'B', 1000]);
    await user.type(title, 'i');
    expect(onconfig).toHaveBeenLastCalledWith({ title: 'Ti' });
    await user.type(body, 'e');
    expect(onconfig).toHaveBeenLastCalledWith({ body: 'Be' });
  });

  it('says a wrong character at the field it is in, and nowhere else', () => {
    mount(block('text', { title: 'T', body: 'a\tb' }), { issue: { code: 'bad_chars', field: 'body' } });
    const title = screen.getByRole('textbox', { name: msg('profile.textTitle') });
    const body = screen.getByRole('textbox', { name: msg('profile.textBody') });

    expect(title.hasAttribute('aria-invalid')).toBe(false);
    expect(body.getAttribute('aria-invalid')).toBe('true');
    expect(screen.getAllByText(/characters that are not allowed/)).toHaveLength(1);
  });
});

describe('while the draft is being saved', () => {
  it('nothing in the settings writes to the draft', async () => {
    const { user, onconfig, onremove } = mount(block('collection', { source: 'manual', title: 'Mine', igdbIds: [11, 12] }), { disabled: true });

    await user.type(screen.getByRole('textbox', { name: msg('profile.collectionTitle') }), 'x');
    await user.click(screen.getByRole('button', { name: msg('social.moveDown', { title: 'Alpha' }) }));
    await user.click(screen.getByRole('button', { name: msg('profile.collectionRemoveGame', { title: 'Beta' }) }));
    await user.click(screen.getByRole('button', { name: msg('profile.removeBlock', { name: msg('profile.blockCollection') }) }));
    await user.click(await screen.findByRole('button', { name: 'Picked One' }));
    expect([onconfig.mock.calls, onremove.mock.calls]).toEqual([[], []]);
  });
});

describe('the fields of the settings', () => {
  const described = (field: HTMLElement) =>
    (field.getAttribute('aria-describedby') ?? '').split(' ').filter(Boolean).map((id) => document.getElementById(id)?.textContent?.trim());

  it('count a caption and a text against their limits in the profile words, and follow the direction of the text typed', () => {
    const { unmount } = mount(block('pinned', { igdbId: 12, caption: 'Hello' }));
    const caption = screen.getByRole('textbox', { name: msg('profile.pinnedCaption') });

    expect(caption.closest('.text-area')?.querySelector('.counter')?.textContent).toBe(msg('profile.fieldCounter', { count: 5, max: 140 }));
    expect(caption.getAttribute('dir')).toBe('auto');
    unmount();

    mount(block('text', { title: '', body: 'Привет' }));
    const body = screen.getByRole('textbox', { name: msg('profile.textBody') });
    expect(body.closest('.text-area')?.querySelector('.counter')?.textContent).toBe(msg('profile.fieldCounter', { count: 6, max: 1000 }));
    expect(body.getAttribute('dir')).toBe('auto');
  });

  it('say once in the profile words that a caption has come near its limit', async () => {
    const { user } = mount(block('pinned', { igdbId: 12, caption: 'x'.repeat(125) }));
    const caption = screen.getByRole('textbox', { name: msg('profile.pinnedCaption') });

    await user.type(caption, 'y');
    const status = caption.closest('.text-area')?.querySelector('[role="status"]');
    expect(status?.textContent).toBe(msg('profile.fieldNear', { count: 140 - 126 }));
  });

  // A one-line field has no counter: the count stands beside its name, its description is its error.
  const titleCount = (title: HTMLElement) => title.closest('.field')?.querySelector('.counter')?.textContent;

  it('count a title against its limit beside its name, and describe it by its error alone', () => {
    mount(block('text', { title: 'Hi', body: 'a\tb' }), { issue: { code: 'bad_chars', field: 'title' } });
    const title = screen.getByRole('textbox', { name: msg('profile.textTitle') });

    expect(title.closest('.text-field')).not.toBeNull();
    expect(titleCount(title)).toBe(msg('profile.fieldCounter', { count: 2, max: 40 }));
    expect(described(title)).toEqual([expect.stringContaining('characters that are not allowed')]);
  });

  it('counts characters, not the halves of a letter outside the basic plane', () => {
    mount(block('collection', { source: 'manual', title: '😀😀', igdbIds: [11] }));
    const title = screen.getByRole('textbox', { name: msg('profile.collectionTitle') });

    expect(titleCount(title)).toBe(msg('profile.fieldCounter', { count: 2, max: 40 }));
  });
});

describe('when the save starts while focus is in the settings', () => {
  it('a field keeps its focus and refuses edits instead of turning disabled', async () => {
    const { user, rerender, onconfig } = mount(block('text', { title: 'T', body: 'B' }));
    const body = screen.getByRole('textbox', { name: msg('profile.textBody') }) as HTMLTextAreaElement;
    body.focus();

    await rerender({ disabled: true });
    expect(document.activeElement).toBe(body);
    expect([body.matches(':disabled'), body.readOnly]).toEqual([false, true]);
    await user.type(body, 'x');
    expect(onconfig).not.toHaveBeenCalled();
  });

  it('a button keeps its focus, held with the reason', async () => {
    const { rerender } = mount(block('collection', { source: 'manual', title: '', igdbIds: [11, 12] }));
    const remove = screen.getByRole('button', { name: msg('profile.collectionRemoveGame', { title: 'Alpha' }) }) as HTMLButtonElement;
    const removeBlock = screen.getByRole('button', { name: msg('profile.removeBlock', { name: msg('profile.blockCollection') }) }) as HTMLButtonElement;
    remove.focus();

    await rerender({ disabled: true });
    expect(document.activeElement).toBe(remove);
    for (const button of [remove, removeBlock]) {
      expect([button.matches(':disabled'), button.getAttribute('aria-disabled')]).toEqual([false, 'true']);
      expect(document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent).toBe(msg('social.saving'));
    }
  });
});

describe("focus in a collection's list of games", () => {
  /** The settings of a manual collection whose page applies every change, as the profile page does. */
  function live(ids: number[]) {
    let target = block('collection', { source: 'manual', title: '', igdbIds: ids });
    const view = mount(target, {
      onconfig: (patch) => {
        target = { ...target, config: { ...target.config, ...patch } };
        void view.rerender({ block: target, layout: { version: 1, blocks: [target] } });
      },
    });
    return view;
  }

  const button = (key: 'social.moveUp' | 'social.moveDown' | 'profile.collectionRemoveGame', title: string) =>
    screen.getByRole('button', { name: msg(key, { title }) }) as HTMLButtonElement;
  const games = () => [...document.querySelectorAll('.games > li .game')].map((item) => item.textContent);
  const reasonOf = (control: HTMLElement) => document.getElementById(control.getAttribute('aria-describedby') ?? '')?.textContent;

  it('stays on a move button that took its game to an end, held there with the reason', async () => {
    const { user } = live([11, 12, 13]);

    await user.click(button('social.moveUp', 'Beta'));
    await vi.waitFor(() => expect(games()).toEqual(['Beta', 'Alpha', 'Gamma']));
    const up = button('social.moveUp', 'Beta');
    expect(document.activeElement).toBe(up);
    expect([up.matches(':disabled'), up.getAttribute('aria-disabled'), reasonOf(up)]).toEqual([false, 'true', msg('profile.gameFirst')]);

    await user.click(button('social.moveDown', 'Alpha'));
    await user.click(button('social.moveDown', 'Alpha'));
    const down = button('social.moveDown', 'Alpha');
    expect(document.activeElement).toBe(down);
    expect([down.matches(':disabled'), reasonOf(down)]).toEqual([false, msg('profile.gameLast')]);
  });

  it('a removed game hands focus to the remove button of the game now in its place, else the one above, else the search for a game', async () => {
    const { user } = live([11, 12, 13]);

    await user.click(button('profile.collectionRemoveGame', 'Beta'));
    await vi.waitFor(() => expect(document.activeElement).toBe(button('profile.collectionRemoveGame', 'Gamma')));
    await user.click(button('profile.collectionRemoveGame', 'Gamma'));
    await vi.waitFor(() => expect(document.activeElement).toBe(button('profile.collectionRemoveGame', 'Alpha')));
    await user.click(button('profile.collectionRemoveGame', 'Alpha'));
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('textbox', { name: msg('profile.pickerSearch') })));
  });

  it('a removal pressed with no focus held leaves focus alone', async () => {
    live([11, 12]);

    // A pointer press in the macOS view focuses no button.
    await fireEvent.click(button('profile.collectionRemoveGame', 'Alpha'));
    await vi.waitFor(() => expect(games()).toHaveLength(1));
    await tick();
    expect(document.activeElement).toBe(document.body);
  });
});
