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
