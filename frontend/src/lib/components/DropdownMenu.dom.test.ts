import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { user } from '../testing/dom';
import MenuHarness from '../testing/MenuHarness.svelte';
import Modal from './Modal.svelte';
import type { MenuItem } from './menu';

const SORT: MenuItem[] = [
  { id: 'alpha', label: 'Alphabet', checked: true },
  { id: 'recent', label: 'Recently played', checked: false },
  { id: 'size', label: 'Size', checked: false, disabled: true },
  { id: 'rating', label: 'Rating', checked: false },
];

const ACTIONS: MenuItem[] = [
  { id: 'gone', label: 'Archived', disabled: true },
  { id: 'open', label: 'Open folder' },
  { id: 'move', label: 'Move' },
  { id: 'mute', label: 'Mute', checked: false, independent: true },
  { id: 'confirm', label: 'Remove', danger: true, separator: true },
];

const ROLES = ['menuitem', 'menuitemradio', 'menuitemcheckbox'] as const;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

function setup(props: { items?: MenuItem[]; inDialog?: boolean; align?: 'left' | 'right'; placement?: 'down' | 'up'; onselect?: (id: string, item: MenuItem) => void } = {}) {
  const onselect = props.onselect ?? vi.fn();
  const view = render(MenuHarness, { target: app, props: { items: ACTIONS, ...props, onselect } });
  const keys = user();
  const trigger = () => screen.getByRole('button', { name: 'Sort' });
  const menu = () => screen.queryByRole('menu');
  const item = (name: string): HTMLElement => {
    const found = ROLES.map((role) => screen.queryByRole(role, { name })).find((el) => el !== null);
    if (!found) throw new Error(`no menu item named ${name}`);
    return found;
  };
  return { keys, onselect, trigger, menu, item, rerender: view.rerender };
}

describe('DropdownMenu: opening', () => {
  it('a click opens the menu out of the page, named by its trigger, which says it is expanded and controls it', async () => {
    const { keys, trigger, menu } = setup();
    expect(trigger().getAttribute('aria-haspopup')).toBe('menu');
    expect(trigger().getAttribute('aria-expanded')).toBe('false');

    await keys.click(trigger());

    const opened = screen.getByRole('menu', { name: 'Sort' });
    expect(trigger().getAttribute('aria-expanded')).toBe('true');
    expect(trigger().getAttribute('aria-controls')).toBe(opened.id);
    expect(app.contains(opened)).toBe(false);
    expect(opened.contains(document.activeElement)).toBe(true);
    expect(menu()).toBe(opened);
  });

  it.each([
    ['Enter', 'first', '{Enter}', 'Open folder'],
    ['Space', 'first', ' ', 'Open folder'],
    ['ArrowDown', 'first', '{ArrowDown}', 'Open folder'],
    ['ArrowUp', 'last', '{ArrowUp}', 'Remove'],
  ])('%s on the trigger opens the menu with focus on the %s enabled item', async (_key, _which, press, focused) => {
    const { keys, trigger, item } = setup();
    trigger().focus();

    await keys.keyboard(press);

    expect(document.activeElement).toBe(item(focused));
  });

  it('places the menu fixed against the trigger edge that align names', async () => {
    const { keys, trigger, menu } = setup({ align: 'left' });
    await keys.click(trigger());
    expect(menu()?.style.position).toBe('fixed');
    expect(menu()?.dataset.placement).toBe('bottom-start');
  });

  it('opens upwards when asked to and aligns to the right edge by default', async () => {
    const { keys, trigger, menu } = setup({ placement: 'up' });
    // jsdom has no layout: put the trigger low in the window so there is room above it.
    trigger().getBoundingClientRect = () => DOMRect.fromRect({ x: 300, y: 600, width: 80, height: 32 });
    await keys.click(trigger());
    expect(menu()?.dataset.placement).toBe('top-end');
  });
});

describe('DropdownMenu: keyboard inside', () => {
  it('arrows move with wrap and skip disabled items; Home and End jump', async () => {
    const { keys, trigger, item } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(item('Open folder'));

    await keys.keyboard('{ArrowUp}');
    expect(document.activeElement).toBe(item('Remove'));
    await keys.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(item('Open folder'));
    await keys.keyboard('{End}');
    expect(document.activeElement).toBe(item('Remove'));
    await keys.keyboard('{Home}');
    expect(document.activeElement).toBe(item('Open folder'));
  });

  it('a typed letter jumps to the next item starting with it, also right after a pointer open', async () => {
    const { keys, trigger, item } = setup();
    await keys.click(trigger());

    await keys.keyboard('m');
    expect(document.activeElement).toBe(item('Move'));
    await keys.keyboard('m');
    expect(document.activeElement).toBe(item('Mute'));
    await keys.keyboard('a');
    expect(document.activeElement).toBe(item('Mute'));
  });

  it.each([
    ['Enter', '{Enter}'],
    ['Space', ' '],
  ])('%s runs the focused item, closes the menu and puts focus back on the trigger', async (_key, press) => {
    const { keys, onselect, trigger, menu } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');

    await keys.keyboard(press);

    expect(onselect).toHaveBeenCalledExactlyOnceWith('move', expect.objectContaining({ id: 'move' }));
    expect(menu()).toBeNull();
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(trigger());
  });

  it('a disabled item does nothing when pressed', async () => {
    const { keys, onselect, trigger, item, menu } = setup();
    await keys.click(trigger());

    await keys.click(item('Archived'));

    expect(onselect).not.toHaveBeenCalled();
    expect(menu()).not.toBeNull();
  });

  it('Escape closes the menu and focus is on the trigger', async () => {
    const { keys, trigger, menu } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}');
    expect(menu()).not.toBeNull();

    await keys.keyboard('{Escape}');

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('Tab closes the menu and focus moves on from the trigger', async () => {
    const { keys, trigger, menu } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}');
    expect(menu()).not.toBeNull();

    await keys.tab();

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });

  it('Shift+Tab closes the menu and focus moves back from the trigger', async () => {
    const { keys, trigger, menu } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}');
    expect(menu()).not.toBeNull();

    await keys.tab({ shift: true });

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Before' }));
  });
});

describe('DropdownMenu: held keys', () => {
  it('Enter held on the trigger opens the menu once and runs nothing', async () => {
    const { keys, onselect, trigger, item } = setup();
    trigger().focus();

    await keys.keyboard('{Enter>7}');
    await keys.keyboard('{/Enter}');

    expect(onselect).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(item('Open folder'));
  });

  it('Space held on the trigger opens the menu once; releasing it on the item runs nothing', async () => {
    const { keys, onselect, trigger, item } = setup();
    trigger().focus();

    await keys.keyboard('[Space>7]');
    await keys.keyboard('[/Space]');

    expect(onselect).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(item('Open folder'));
  });

  it.each([
    ['Enter', '{Enter>6}', '{/Enter}'],
    ['Space', '[Space>6]', '[/Space]'],
  ])('%s held on an item runs it once and does not reopen the menu', async (_key, hold, release) => {
    const { keys, onselect, trigger, menu } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');

    await keys.keyboard(hold);
    await keys.keyboard(release);

    expect(onselect).toHaveBeenCalledExactlyOnceWith('move', expect.objectContaining({ id: 'move' }));
    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });
});

describe('DropdownMenu: items changing while it is open', () => {
  it('when the focused item goes away, focus moves to the item now in its place and the keys keep working', async () => {
    const { keys, trigger, item, rerender } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}{ArrowDown}');
    expect(document.activeElement).toBe(item('Move'));

    await rerender({ items: ACTIONS.filter((entry) => entry.id !== 'move') });

    expect(document.activeElement).toBe(item('Mute'));
    await keys.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(item('Remove'));
  });

  it('when the focused last item goes away, focus moves to the enabled item above it', async () => {
    const { keys, trigger, item, rerender } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowUp}');
    expect(document.activeElement).toBe(item('Remove'));

    await rerender({ items: ACTIONS.filter((entry) => entry.id !== 'confirm') });

    expect(document.activeElement).toBe(item('Mute'));
  });

  it('when no enabled item is left, focus stays on the menu itself', async () => {
    const { keys, trigger, menu, rerender } = setup();
    trigger().focus();
    await keys.keyboard('{ArrowDown}');

    await rerender({ items: ACTIONS.filter((entry) => entry.disabled) });

    expect(document.activeElement).toBe(menu());
  });
});

describe('DropdownMenu: its trigger scrolled away', () => {
  it('closes when the trigger leaves the window, and focus goes back to it without scrolling there', async () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal('cancelAnimationFrame', () => {});
    try {
      const { keys, trigger, menu } = setup();
      let top = 300;
      trigger().getBoundingClientRect = () => DOMRect.fromRect({ x: 200, y: top, width: 80, height: 32 });
      const focus = vi.spyOn(trigger(), 'focus');
      await keys.click(trigger());
      expect(menu()).not.toBeNull();

      top = 120;
      document.dispatchEvent(new Event('scroll'));
      for (const frame of frames.splice(0)) frame(0);
      expect(menu()).not.toBeNull();

      top = -60;
      document.dispatchEvent(new Event('scroll'));
      for (const frame of frames.splice(0)) frame(0);
      await tick();

      expect(menu()).toBeNull();
      expect(trigger().getAttribute('aria-expanded')).toBe('false');
      expect(focus).toHaveBeenLastCalledWith({ preventScroll: true });
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('a keyboard open on a trigger scrolled out of sight brings it back, so the menu opens', async () => {
    const { keys, trigger, menu } = setup();
    let top = -60;
    trigger().getBoundingClientRect = () => DOMRect.fromRect({ x: 200, y: top, width: 80, height: 32 });
    const reveal = vi.fn(() => {
      top = 300;
    });
    trigger().scrollIntoView = reveal;
    trigger().focus();

    await keys.keyboard('{Enter}');

    expect(reveal).toHaveBeenCalledExactlyOnceWith({ block: 'nearest' });
    expect(menu()).not.toBeNull();
    expect(trigger().getAttribute('aria-expanded')).toBe('true');
  });

  it('opening on a trigger in sight scrolls nothing', async () => {
    const { keys, trigger, menu } = setup();
    trigger().getBoundingClientRect = () => DOMRect.fromRect({ x: 200, y: 300, width: 80, height: 32 });
    const reveal = vi.fn();
    trigger().scrollIntoView = reveal;
    trigger().focus();

    await keys.keyboard('{Enter}');

    expect(reveal).not.toHaveBeenCalled();
    expect(menu()).not.toBeNull();
  });
});

describe('DropdownMenu: pointer', () => {
  it('a press on an item runs it, closes the menu and leaves focus on the trigger', async () => {
    const { keys, onselect, trigger, item, menu } = setup();
    await keys.click(trigger());

    await keys.click(item('Open folder'));

    expect(onselect).toHaveBeenCalledExactlyOnceWith('open', expect.objectContaining({ label: 'Open folder' }));
    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('a press outside closes the menu and focus goes where the press put it, not back to the trigger', async () => {
    const { keys, trigger, menu } = setup();
    await keys.click(trigger());
    expect(menu()).not.toBeNull();
    const before = screen.getByRole('button', { name: 'Before' });

    await keys.click(before);

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(before);
  });

  it('a press on the trigger closes the menu and it stays closed', async () => {
    const { keys, trigger, menu } = setup();
    await keys.click(trigger());

    await keys.click(trigger());

    expect(menu()).toBeNull();
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
  });

  it('a press on an icon (an <svg> inside a button) outside closes the menu', async () => {
    const { keys, trigger, menu } = setup();
    await keys.click(trigger());
    expect(menu()).not.toBeNull();
    const icon = screen.getByRole('button', { name: 'Help' }).querySelector('path') as Element;

    await keys.pointer({ keys: '[MouseLeft]', target: icon });

    expect(menu()).toBeNull();
  });
});

describe('DropdownMenu: items', () => {
  it('says what each item is: radio choices and an independent check with their state, separators, disabled items', async () => {
    const { keys, trigger } = setup({ items: [...SORT, { id: 'mute', label: 'Mute', checked: true, independent: true, separator: true }] });
    await keys.click(trigger());

    const radios = screen.getAllByRole('menuitemradio');
    expect(radios.map((el) => [el.textContent?.trim(), el.getAttribute('aria-checked')])).toEqual([
      ['Alphabet', 'true'],
      ['Recently played', 'false'],
      ['Size', 'false'],
      ['Rating', 'false'],
    ]);
    expect(screen.getByRole('menuitemradio', { name: 'Size' }).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByRole('menuitemcheckbox', { name: 'Mute' }).getAttribute('aria-checked')).toBe('true');
    expect(screen.getAllByRole('separator')).toHaveLength(1);
    expect(screen.queryAllByRole('menuitem')).toEqual([]);
  });

  it('a plain item is a menuitem and a danger item keeps that role', async () => {
    const { keys, trigger } = setup();
    await keys.click(trigger());
    expect(screen.getAllByRole('menuitem').map((el) => el.textContent?.trim())).toEqual(['Archived', 'Open folder', 'Move', 'Remove']);
  });

  it('a single choice closes the menu; an independent check keeps it open with focus on the item', async () => {
    const { keys, onselect, trigger, item, menu } = setup({ items: [...SORT, { id: 'mute', label: 'Mute', checked: false, independent: true }] });
    await keys.click(trigger());

    await keys.click(item('Mute'));
    expect(onselect).toHaveBeenLastCalledWith('mute', expect.objectContaining({ independent: true }));
    expect(menu()).not.toBeNull();
    expect(document.activeElement).toBe(item('Mute'));

    await keys.keyboard('{Home}{ArrowDown}{Enter}');
    expect(onselect).toHaveBeenLastCalledWith('recent', expect.objectContaining({ id: 'recent' }));
    expect(menu()).toBeNull();
  });
});

describe('DropdownMenu: failure and dialogs', () => {
  it('a pick whose handler throws leaves the menu closed and focus on the trigger', async () => {
    const errors: unknown[] = [];
    const onError = (event: ErrorEvent) => {
      event.preventDefault();
      errors.push(event.error);
    };
    window.addEventListener('error', onError);
    const { keys, trigger, item, menu } = setup({
      onselect: () => {
        throw new Error('backend refused');
      },
    });
    try {
      await keys.click(trigger());
      await keys.click(item('Move'));
    } finally {
      window.removeEventListener('error', onError);
    }

    expect(errors).toHaveLength(1);
    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('an item that opens a dialog: the dialog takes focus and gives it back to the trigger when it closes', async () => {
    const { keys, trigger, item } = setup();
    await keys.click(trigger());

    await keys.click(item('Remove'));
    const dialog = screen.getByRole('dialog', { name: 'Confirm' });
    expect(dialog.parentElement?.contains(document.activeElement)).toBe(true);

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('a dialog that opens while the menu is open closes the menu first', async () => {
    const { keys, trigger, menu } = setup();
    await keys.click(trigger());

    render(Modal, { target: app, props: { open: true, title: 'The game closed' } });

    expect(menu()).toBeNull();
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
  });
});

describe('DropdownMenu inside a dialog', () => {
  async function inDialog() {
    const view = setup({ inDialog: true });
    await view.keys.click(screen.getByRole('button', { name: 'Open settings' }));
    const dialog = screen.getByRole('dialog', { name: 'Settings' });
    view.trigger().focus();
    await view.keys.keyboard('{ArrowDown}');
    expect(view.menu()).not.toBeNull();
    return { ...view, dialog };
  }

  it('Tab closes the menu and stays in the dialog, on the control after the trigger', async () => {
    const { keys, dialog, menu } = await inDialog();

    await keys.tab();

    expect(menu()).toBeNull();
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });

  it('Escape closes the menu first, focus on its trigger, then the dialog', async () => {
    const { keys, trigger, menu } = await inDialog();

    await keys.keyboard('{Escape}');
    expect(menu()).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();
    expect(document.activeElement).toBe(trigger());

    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('one press on the scrim closes the menu only; the next one closes the dialog', async () => {
    const { keys, dialog, menu } = await inDialog();
    const scrim = dialog.parentElement as HTMLElement;

    await keys.click(scrim);
    expect(menu()).toBeNull();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeTruthy();

    await keys.click(scrim);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
