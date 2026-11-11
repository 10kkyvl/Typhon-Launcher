import type { PresenceDot } from '../social/presence';

/** An item of a menu (DropdownMenu, MenuList). */
export interface MenuItem {
  id: string;
  label: string;
  danger?: boolean;
  /** Shown and announced, skipped by the arrows, does nothing when pressed. */
  disabled?: boolean;
  /** A separator is drawn above this item. */
  separator?: boolean;
  dot?: PresenceDot;
  /**
   * Set (true or false) on a checkable item: whether it is checked now. Such an item is one choice of
   * the menu's set (`menuitemradio`: a sort order, a filter value, a status) and picking it closes the
   * menu, unless `independent` makes it an on/off of its own (`menuitemcheckbox`), which keeps the
   * menu open so several can be switched in a row.
   */
  checked?: boolean;
  independent?: boolean;
}

export type MenuItemRole = 'menuitem' | 'menuitemradio' | 'menuitemcheckbox';

export function menuItemRole(item: MenuItem): MenuItemRole {
  if (item.checked === undefined) return 'menuitem';
  return item.independent ? 'menuitemcheckbox' : 'menuitemradio';
}

/** A plain item or a single choice closes the menu when picked; an independent check keeps it open. */
export function closesOnPick(item: MenuItem): boolean {
  return menuItemRole(item) !== 'menuitemcheckbox';
}

/** What a menu button needs: spread it on the button (`<Button {...props}>`), wire nothing by hand. */
export interface MenuTriggerProps {
  id: string;
  'aria-haspopup': 'menu';
  'aria-expanded': boolean;
  'aria-controls': string | undefined;
  onclick: (event: MouseEvent) => void;
  onkeydown: (event: KeyboardEvent) => void;
  onkeyup: (event: KeyboardEvent) => void;
}

/** Where focus goes as a menu opens: its first or last enabled item (from the keyboard), or the menu itself (by pointer). */
export type MenuFocus = 'first' | 'last' | 'menu';

/** A menu as its button sees it. `close` unmounts the menu before it returns, so focus is back on the button. */
export interface MenuButtonState {
  open: () => boolean;
  show: (focus: MenuFocus) => void;
  close: () => void;
}

/**
 * The pointer and key handling of a menu button (WAI-ARIA menu button): a click toggles the menu;
 * Enter and Space toggle it too, opening on its first item; ArrowDown opens on the first, ArrowUp on the last.
 */
export function menuButtonHandlers(menu: MenuButtonState): Pick<MenuTriggerProps, 'onclick' | 'onkeydown' | 'onkeyup'> {
  return {
    onclick: (event) => {
      if (menu.open()) menu.close();
      // detail 0: a click from script (Big Picture's confirm), treated as the keyboard.
      else menu.show(event.detail === 0 ? 'first' : 'menu');
    },
    // Enter and Space act on keydown, never on the browser's click: a held key repeats keydown (and
    // Enter its click), and the Space keyup after a pick would land on the trigger and reopen.
    onkeydown: (event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        if (event.repeat) return;
        if (menu.open()) menu.close();
        else menu.show('first');
        return;
      }
      if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
      event.preventDefault();
      if (event.repeat && menu.open()) return;
      menu.close();
      menu.show(event.key === 'ArrowDown' ? 'first' : 'last');
    },
    onkeyup: (event) => {
      if (event.key === ' ') event.preventDefault();
    },
  };
}
