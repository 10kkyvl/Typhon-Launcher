import { screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { msg } from '../../lib/i18n';
import { renderDom } from '../../lib/testing/dom';
import LayoutEditor from './LayoutEditor.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

// The sidebar list is the first list of the editor; its rows hold the item name and its buttons.
function sidebarRows(): HTMLElement[] {
  return within(screen.getAllByRole('list')[0]).getAllByRole('listitem');
}

const names = () => sidebarRows().map((row) => row.querySelector('.item-name')?.textContent);

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
