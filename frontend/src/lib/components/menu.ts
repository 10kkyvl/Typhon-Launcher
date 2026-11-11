import type { PresenceDot } from '../social/presence';

/** An item of a menu (DropdownMenu). */
export interface MenuItem {
  id: string;
  label: string;
  danger?: boolean;
  /** A separator is drawn above this item. */
  separator?: boolean;
  checked?: boolean;
  dot?: PresenceDot;
}
