import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import OptionListHarness from '../testing/OptionListHarness.svelte';
import OptionList from './OptionList.svelte';
import type { OptionItem } from './optionList';

afterEach(() => {
  document.body.replaceChildren();
});

const LONG = 'C:/Users/me/AppData/Local/Supergiant Games/A folder whose name runs on for long enough to wrap/Hades/Profiles';

const FOLDERS: OptionItem[] = [
  { id: 'a', title: 'C:/Saves/Hades', note: 'Changed today', fact: '12 MB' },
  { id: 'b', title: LONG },
  { id: 'c', title: 'D:/Old/Hades', disabledReason: 'The drive is not connected' },
  { id: 'd', title: 'E:/Cloud/Hades', fact: '3 MB' },
];

const radio = (name: string) => screen.getByRole('radio', { name });
const checked = () => screen.queryAllByRole('radio').filter((row) => row.getAttribute('aria-checked') === 'true');
const before = () => screen.getByRole('button', { name: 'Before' });
const after = () => screen.getByRole('button', { name: 'After' });

describe('OptionList: roles and names', () => {
  it('is a named radio group of rows named by their titles and described by the rest', () => {
    renderDom(OptionListHarness, { options: FOLDERS, start: 'a' });
    expect(screen.getByRole('radiogroup', { name: 'Folder' })).toBeTruthy();
    expect(screen.getAllByRole('radio').map((row) => [row.getAttribute('aria-checked'), row.getAttribute('aria-disabled')])).toEqual([
      ['true', null],
      ['false', null],
      ['false', 'true'],
      ['false', null],
    ]);
    expect(description(radio('C:/Saves/Hades'))).toBe('Changed today 12 MB');
    expect(radio(LONG).hasAttribute('aria-describedby')).toBe(false);
    expect(description(radio('D:/Old/Hades'))).toBe('The drive is not connected');
    expect(screen.getByText('The drive is not connected')).toBeTruthy();
    expect(radio('D:/Old/Hades')).toHaveProperty('disabled', false);
  });

  it('takes its name from the words above it', () => {
    renderDom(OptionListHarness, { options: FOLDERS, labelled: true });
    expect(screen.getByRole('radiogroup', { name: 'Save folder' })).toBeTruthy();
  });

  it('a row drawn by the caller is named by what it draws, and still says why it is refused', () => {
    renderDom(OptionListHarness, { options: FOLDERS, custom: true });
    expect(document.querySelector('[data-custom="a"]')).not.toBeNull();
    expect(radio('C:/Saves/Hades 12 MB')).toBeTruthy();
    expect(radio('C:/Saves/Hades 12 MB').hasAttribute('aria-describedby')).toBe(false);
    expect(screen.queryByText('Changed today')).toBeNull();
    expect(description(radio('D:/Old/Hades'))).toBe('The drive is not connected');
  });

  it('draws nothing with no rows', () => {
    const { container } = renderDom(OptionListHarness, { options: [] });
    expect(screen.queryByRole('radiogroup')).toBeNull();
    expect(container.querySelector('.option-list')).toBeNull();
  });

  it('carries no native title', () => {
    renderDom(OptionListHarness, { options: FOLDERS });
    expect(document.querySelectorAll('[title]')).toHaveLength(0);
  });
});

describe('OptionList: the keyboard', () => {
  it('is one Tab stop: the chosen row, then the control after the list', async () => {
    const { user } = renderDom(OptionListHarness, { options: FOLDERS, start: 'd' });
    before().focus();
    await user.tab();
    expect(document.activeElement).toBe(radio('E:/Cloud/Hades'));
    await user.tab();
    expect(document.activeElement).toBe(after());
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(radio('E:/Cloud/Hades'));
  });

  it('with no choice, Tab reaches the first row that can be chosen', async () => {
    const refusedFirst = [{ id: 'x', title: 'X:/Gone', disabledReason: 'Gone' }, ...FOLDERS];
    const { user } = renderDom(OptionListHarness, { options: refusedFirst });
    before().focus();
    await user.tab();
    expect(document.activeElement).toBe(radio('C:/Saves/Hades'));
    expect(checked()).toEqual([]);
  });

  it('with every row refused, Tab still reaches the first, so its reason can be heard', async () => {
    const { user } = renderDom(OptionListHarness, { options: [{ id: 'x', title: 'X:/Gone', disabledReason: 'Gone' }] });
    before().focus();
    await user.tab();
    expect(document.activeElement).toBe(radio('X:/Gone'));
    await user.keyboard('{ArrowDown}{ }');
    expect(checked()).toEqual([]);
  });

  it('arrows move and choose, passing a refused row and wrapping at the ends; Home and End jump', async () => {
    const { user } = renderDom(OptionListHarness, { options: FOLDERS, start: 'b' });
    radio(LONG).focus();

    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(radio('E:/Cloud/Hades'));
    expect(checked()).toEqual([radio('E:/Cloud/Hades')]);
    expect(screen.getByRole('status').textContent).toBe('d');

    await user.keyboard('{ArrowDown}');
    expect(checked()).toEqual([radio('C:/Saves/Hades')]);
    await user.keyboard('{ArrowUp}');
    expect(checked()).toEqual([radio('E:/Cloud/Hades')]);
    await user.keyboard('{ArrowUp}');
    expect(checked()).toEqual([radio(LONG)]);
    await user.keyboard('{ArrowRight}');
    expect(checked()).toEqual([radio('E:/Cloud/Hades')]);
    await user.keyboard('{ArrowLeft}');
    expect(checked()).toEqual([radio(LONG)]);
    await user.keyboard('{End}');
    expect(document.activeElement).toBe(radio('E:/Cloud/Hades'));
    expect(checked()).toEqual([radio('E:/Cloud/Hades')]);
    await user.keyboard('{Home}');
    expect(document.activeElement).toBe(radio('C:/Saves/Hades'));
    expect(screen.getByRole('status').textContent).toBe('a');
  });

  it('brings the row the keys chose into view', async () => {
    const reveal = vi.fn();
    Element.prototype.scrollIntoView = reveal;
    try {
      const { user } = renderDom(OptionListHarness, { options: FOLDERS, start: 'a' });
      radio('C:/Saves/Hades').focus();
      await user.keyboard('{End}');
      expect(reveal).toHaveBeenCalledTimes(1);
      expect(reveal.mock.contexts[0]).toBe(radio('E:/Cloud/Hades'));
      expect(reveal).toHaveBeenCalledWith({ block: 'nearest' });
    } finally {
      delete (Element.prototype as Partial<Element>).scrollIntoView;
    }
  });

  it('a press chooses; a press on a refused row does not; Space chooses the focused row', async () => {
    const { user } = renderDom(OptionListHarness, { options: FOLDERS, start: 'a' });
    await user.click(radio(LONG));
    expect(checked()).toEqual([radio(LONG)]);
    await user.click(radio('D:/Old/Hades'));
    expect(checked()).toEqual([radio(LONG)]);
    expect(screen.getByRole('status').textContent).toBe('b');

    radio('E:/Cloud/Hades').focus();
    await user.keyboard(' ');
    expect(checked()).toEqual([radio('E:/Cloud/Hades')]);
  });
});

describe('OptionList: the value', () => {
  it('shows a value the caller sets, and none chosen for a value no row has', async () => {
    const { user } = renderDom(OptionListHarness, { options: FOLDERS, start: 'a' });
    await user.click(screen.getByRole('button', { name: 'Set' }));
    expect(checked()).toEqual([]);
    before().focus();
    await user.tab();
    expect(document.activeElement).toBe(radio('C:/Saves/Hades'));
  });

  it('with onchange, reports the choice and shows only what the caller holds', async () => {
    const asked: string[] = [];
    const { user } = renderDom(OptionListHarness, { options: FOLDERS, start: 'a', through: (id: string) => (asked.push(id), id !== 'd') });

    await user.click(radio(LONG));
    expect(checked()).toEqual([radio(LONG)]);
    await user.click(radio('E:/Cloud/Hades'));
    expect(checked()).toEqual([radio(LONG)]);
    await user.click(radio(LONG));
    expect(asked).toEqual(['b', 'd']);
  });

  it('never writes the value it is given when the caller holds it', async () => {
    const onchange = vi.fn();
    const { user } = renderDom(OptionList, { label: 'Folder', options: FOLDERS, value: 'a', onchange });
    await user.click(radio(LONG));
    expect(onchange).toHaveBeenCalledWith('b');
    expect(checked()).toEqual([radio('C:/Saves/Hades')]);
  });
});

describe('OptionList: held while the caller writes', () => {
  const group = () => screen.getByRole('radiogroup', { name: 'Folder' });
  const rowStates = () => screen.getAllByRole('radio').map((row) => row.getAttribute('aria-disabled'));

  it('says so on the group and on every row, and draws no reason of its own', () => {
    renderDom(OptionListHarness, { options: FOLDERS, start: 'a', disabled: true });
    expect(group().getAttribute('aria-disabled')).toBe('true');
    expect(rowStates()).toEqual(['true', 'true', 'true', 'true']);
    expect(screen.getAllByRole('radio').map((row) => (row as HTMLButtonElement).disabled)).toEqual([false, false, false, false]);
    expect(document.querySelectorAll('.reason')).toHaveLength(1);
    expect(description(radio('C:/Saves/Hades'))).toBe('Changed today 12 MB');
  });

  for (const held of ['bound', 'through onchange'] as const) {
    it(`no key and no press chooses, reports, writes or moves focus (${held})`, async () => {
      const asked: string[] = [];
      const through = held === 'bound' ? undefined : (id: string) => (asked.push(id), true);
      const { user } = renderDom(OptionListHarness, { options: FOLDERS, start: 'b', disabled: true, through });
      radio(LONG).focus();

      for (const key of ['{ArrowDown}', '{ArrowUp}', '{ArrowRight}', '{ArrowLeft}', '{Home}', '{End}', ' ', '{Enter}']) {
        await user.keyboard(key);
        expect(document.activeElement, key).toBe(radio(LONG));
      }
      await user.click(radio('E:/Cloud/Hades'));
      expect(document.activeElement).toBe(radio(LONG));

      expect(asked).toEqual([]);
      expect(screen.getByRole('status').textContent).toBe('b');
      expect(checked()).toEqual([radio(LONG)]);
    });
  }

  it('swallows the arrows it ignores, so the dialog body does not scroll under a held list', () => {
    renderDom(OptionListHarness, { options: FOLDERS, start: 'a', disabled: true });
    const row = radio('C:/Saves/Hades');
    row.focus();
    const event = new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true });
    row.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it('keeps the focused row focused and the Tab stop where it was when it turns on', async () => {
    const { user, rerender } = renderDom(OptionListHarness, { options: FOLDERS, start: 'd', disabled: false });
    radio('E:/Cloud/Hades').focus();

    await rerender({ options: FOLDERS, start: 'd', disabled: true });

    expect(document.activeElement).toBe(radio('E:/Cloud/Hades'));
    expect(radio('E:/Cloud/Hades')).toHaveProperty('disabled', false);
    expect(screen.getAllByRole('radio').map((row) => row.tabIndex)).toEqual([-1, -1, -1, 0]);
    await user.tab();
    expect(document.activeElement).toBe(after());
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(radio('E:/Cloud/Hades'));
  });

  it('gives everything back when it turns off', async () => {
    const { user, rerender } = renderDom(OptionListHarness, { options: FOLDERS, start: 'a', disabled: true });
    radio('C:/Saves/Hades').focus();

    await rerender({ options: FOLDERS, start: 'a', disabled: false });

    expect(group().hasAttribute('aria-disabled')).toBe(false);
    expect(rowStates()).toEqual([null, null, 'true', null]);
    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(radio(LONG));
    expect(screen.getByRole('status').textContent).toBe('b');
    await user.click(radio('E:/Cloud/Hades'));
    expect(screen.getByRole('status').textContent).toBe('d');
  });
});

describe('OptionList: rows that change under it', () => {
  it('keeps focus in the list when the focused row leaves: the chosen id is gone, so focus goes to the first row', async () => {
    const { rerender } = renderDom(OptionListHarness, { options: FOLDERS, start: 'b' });
    radio(LONG).focus();

    await rerender({ options: FOLDERS.filter((folder) => folder.id !== 'b'), start: 'b' });

    expect(checked()).toEqual([]);
    expect(document.activeElement).toBe(radio('C:/Saves/Hades'));
  });

  it('keeps focus on the row it is on when other rows change', async () => {
    const { rerender } = renderDom(OptionListHarness, { options: FOLDERS, start: 'd' });
    radio('E:/Cloud/Hades').focus();

    await rerender({ options: FOLDERS.filter((folder) => folder.id !== 'a'), start: 'd' });

    expect(document.activeElement).toBe(radio('E:/Cloud/Hades'));
    expect(checked()).toEqual([radio('E:/Cloud/Hades')]);
  });

  it('takes no focus when the rows change while focus is elsewhere', async () => {
    const { rerender } = renderDom(OptionListHarness, { options: FOLDERS, start: 'b' });
    after().focus();

    await rerender({ options: FOLDERS.filter((folder) => folder.id !== 'b'), start: 'b' });

    expect(document.activeElement).toBe(after());
  });
});

describe('OptionList: no warning', () => {
  it('mounts in every state without a Svelte warning', () => {
    const warn = vi.spyOn(console, 'warn');
    renderDom(OptionListHarness, { options: FOLDERS, start: 'c', custom: true });
    renderDom(OptionList, { labelledby: 'x', options: FOLDERS, value: 'zz', onchange: vi.fn() });
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});
