import { screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { renderDom } from '../../lib/testing/dom';
import { SEPARATOR } from '../../lib/theme/layout';
import LayoutEditor from './LayoutEditor.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

// The sidebar list is the first list of the editor; its rows hold the item name and its buttons.
function sidebarRows(): HTMLElement[] {
  return within(screen.getAllByRole('list')[0]).getAllByRole('listitem');
}

const names = () => sidebarRows().map((row) => row.querySelector('.item-name')?.textContent);

describe('LayoutEditor: what it shows after an edit', () => {
  // The sidebar drops a separator that ends the list or follows another; the editor shows what the
  // sidebar will draw, so it drops it too.
  it('shows the sidebar as it will be drawn: a separator moved to the end is gone', async () => {
    const { user } = renderDom(LayoutEditor, {});
    await user.click(screen.getByRole('button', { name: msg('settings.layoutAddSeparator') }));
    const count = names().length;
    const at = names().lastIndexOf(msg('settings.layoutSeparator'));
    expect(at).toBe(count - 2);

    await user.click(within(sidebarRows()[at]).getByRole('button', { name: msg('settings.layoutMoveDown') }));

    expect(names()).toHaveLength(count - 1);
    expect(names().at(-1)).not.toBe(msg('settings.layoutSeparator'));
  });

  // The value the editor hands back is compared by value with what it last handed back: comparing it by
  // identity never matches the bound value, which Svelte wraps in a proxy, and Svelte warns.
  it('recognises its own value when it comes back through the binding', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { user } = renderDom(LayoutEditor, {});

    await user.click(within(sidebarRows()[0]).getByRole('button', { name: msg('settings.layoutHide') }));

    expect(warn.mock.calls.flat().join(' ')).not.toMatch(/state_proxy_equality_mismatch|proxies and the values they proxy/);
    warn.mockRestore();
  });
});

describe('LayoutEditor: focus when a moved separator is dropped', () => {
  it('lands on the row now at the end when a separator is moved past the last row', async () => {
    const { user } = renderDom(LayoutEditor, {});
    await user.click(screen.getByRole('button', { name: msg('settings.layoutAddSeparator') }));
    const at = names().lastIndexOf(msg('settings.layoutSeparator'));
    within(sidebarRows()[at]).getByRole('button', { name: msg('settings.layoutMoveDown') }).focus();

    await user.keyboard('{Enter}');

    const last = sidebarRows().at(-1) as HTMLElement;
    expect(document.activeElement).toBe(within(last).getByRole('button', { name: msg('settings.layoutMoveUp') }));
  });

  it('lands on the row now at that place when a separator is moved next to another', async () => {
    const layout = { sidebar: { order: ['library', SEPARATOR, 'catalog', SEPARATOR, 'installed'] } };
    const { user } = renderDom(LayoutEditor, { layout });
    expect(names().slice(0, 5)).toEqual([names()[0], msg('settings.layoutSeparator'), names()[2], msg('settings.layoutSeparator'), names()[4]]);
    const catalog = names()[2];
    within(sidebarRows()[3]).getByRole('button', { name: msg('settings.layoutMoveUp') }).focus();

    await user.keyboard('{Enter}');

    expect(names()[2]).toBe(catalog);
    expect(document.activeElement).toBe(within(sidebarRows()[2]).getByRole('button', { name: msg('settings.layoutMoveUp') }));
  });
});

describe('LayoutEditor: the row buttons', () => {
  it('are the base icon buttons, named by what they do, and move an item from the keyboard', async () => {
    const { user } = renderDom(LayoutEditor, {});
    const [first, second] = names();
    const row = sidebarRows()[0];
    const up = within(row).getByRole('button', { name: msg('settings.layoutMoveUp') });
    const down = within(row).getByRole('button', { name: msg('settings.layoutMoveDown') });
    expect(up.classList.contains('icon-btn')).toBe(true);
    expect(down.classList.contains('icon-btn')).toBe(true);
    expect((up as HTMLButtonElement).disabled).toBe(true);

    down.focus();
    await user.keyboard('{Enter}');

    expect(names().slice(0, 2)).toEqual([second, first]);
    // Focus follows the moved row, on the action that moved it.
    const moved = sidebarRows()[1];
    expect(document.activeElement).toBe(within(moved).getByRole('button', { name: msg('settings.layoutMoveDown') }));
    await user.keyboard('{Enter}');
    expect(names()[2]).toBe(first);
  });

  it('puts focus on the opposite move when the row reaches the end of its list', async () => {
    const { user } = renderDom(LayoutEditor, {});
    const rows = sidebarRows();
    const before = names();
    const last = before[before.length - 1];
    const next = before[before.length - 2];
    within(rows[rows.length - 2]).getByRole('button', { name: msg('settings.layoutMoveDown') }).focus();

    await user.keyboard('{Enter}');

    const end = sidebarRows()[rows.length - 1];
    expect(names().slice(-2)).toEqual([last, next]);
    expect(document.activeElement).toBe(within(end).getByRole('button', { name: msg('settings.layoutMoveUp') }));
  });

  it('keeps focus on a separator as it moves past the rows around it', async () => {
    const { user } = renderDom(LayoutEditor, {});
    await user.click(screen.getByRole('button', { name: msg('settings.layoutAddSeparator') }));
    const at = names().indexOf(msg('settings.layoutSeparator'));
    within(sidebarRows()[at]).getByRole('button', { name: msg('settings.layoutMoveUp') }).focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(names()[at - 2]).toBe(msg('settings.layoutSeparator'));
    expect(document.activeElement).toBe(within(sidebarRows()[at - 2]).getByRole('button', { name: msg('settings.layoutMoveUp') }));
  });

  // The sidebar starts with separators of its own; the first stands between rows on both sides.
  it('keeps focus on a separator moved down past the next row', async () => {
    const { user } = renderDom(LayoutEditor, {});
    const at = names().indexOf(msg('settings.layoutSeparator'));
    const next = names()[at + 1];
    within(sidebarRows()[at]).getByRole('button', { name: msg('settings.layoutMoveDown') }).focus();

    await user.keyboard('{Enter}');

    expect(names().slice(at, at + 2)).toEqual([next, msg('settings.layoutSeparator')]);
    expect(document.activeElement).toBe(within(sidebarRows()[at + 1]).getByRole('button', { name: msg('settings.layoutMoveDown') }));
  });

  it('puts focus on the row that takes a removed separator place', async () => {
    const { user } = renderDom(LayoutEditor, {});
    const at = names().indexOf(msg('settings.layoutSeparator'));
    const count = names().length;
    const after = names()[at + 1];
    within(sidebarRows()[at]).getByRole('button', { name: msg('settings.layoutRemoveSeparator') }).focus();

    await user.keyboard('{Enter}');

    expect(names()).toHaveLength(count - 1);
    expect(names()[at]).toBe(after);
    expect(document.activeElement).toBe(within(sidebarRows()[at]).getByRole('button', { name: msg('settings.layoutMoveUp') }));
  });

  it('keeps focus on a row moved to the other column of the game page', async () => {
    const { user } = renderDom(LayoutEditor, {});
    const main = screen.getByRole('list', { name: msg('settings.layoutMainColumn') });
    const side = screen.getByRole('list', { name: msg('settings.layoutSideColumn') });
    const row = within(main).getAllByRole('listitem')[0];
    const name = row.querySelector('.item-name')?.textContent;
    within(row).getByRole('button', { name: msg('settings.layoutToSide') }).focus();

    await user.keyboard('{Enter}');

    const arrived = within(side)
      .getAllByRole('listitem')
      .find((item) => item.querySelector('.item-name')?.textContent === name) as HTMLElement;
    expect(document.activeElement).toBe(within(arrived).getByRole('button', { name: msg('settings.layoutToMain') }));
  });

  // The name already says what a press does ("Hide" / "Show"); a pressed state on top of it would
  // read "Hide, pressed" for an item that is hidden.
  it('names the visibility button by its action and does not also say pressed', async () => {
    const { user } = renderDom(LayoutEditor, {});
    const row = sidebarRows()[0];
    const hide = within(row).getByRole('button', { name: msg('settings.layoutHide') });
    expect(hide.classList.contains('icon-btn')).toBe(true);
    expect(hide.hasAttribute('aria-pressed')).toBe(false);

    hide.focus();
    await user.keyboard(' ');

    const show = within(sidebarRows()[0]).getByRole('button', { name: msg('settings.layoutShow') });
    expect(show.hasAttribute('aria-pressed')).toBe(false);
    expect(document.activeElement).toBe(show);
    expect(sidebarRows()[0].classList.contains('hidden')).toBe(true);
  });

  it('keeps the support row visibility button on the base control', () => {
    renderDom(LayoutEditor, {});
    const support = screen.getByText(msg('tickets.nav')).closest('.support-row') as HTMLElement;
    const eye = within(support).getByRole('button', { name: msg('settings.layoutHide') });
    expect(eye.classList.contains('icon-btn')).toBe(true);
    expect(eye.hasAttribute('aria-pressed')).toBe(false);
  });
});
