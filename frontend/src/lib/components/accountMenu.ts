import { msg } from '../i18n';
import { PRESENCE_STATUSES, type PresenceStatus } from '../services/online';
import { STATUS_LABELS, statusDot } from '../social/presence';
import type { MenuItem } from './DropdownMenu.svelte';

export function accountMenuItems(guest: boolean, current: PresenceStatus): MenuItem[] {
  if (guest) {
    return [
      { id: 'profile', label: msg('ui.profile') },
      { id: 'login', label: msg('ui.signIn'), separator: true },
    ];
  }
  return [
    { id: 'profile', label: msg('ui.profile') },
    ...PRESENCE_STATUSES.map((status, index) => ({
      id: `status:${status}`,
      label: STATUS_LABELS[status],
      dot: statusDot(status),
      checked: current === status,
      separator: index === 0,
    })),
    { id: 'logout', label: msg('ui.signOut'), danger: true, separator: true },
  ];
}
