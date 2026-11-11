import { msg } from '../i18n';
import { PRESENCE_STATUSES, type PresenceStatus } from '../services/online';
import { STATUS_LABELS, ownStatusLine, statusDot, type PresenceDot } from '../social/presence';
import type { MenuItem } from './menu';

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

export type ChipPresence = { line: string; dot?: PresenceDot };

// The line under the name on the account chip. A running game outranks the status and lights the dot
// as playing, except for an invisible user, who stays invisible. With several games running, the first
// the launcher reports that the library can name is shown.
export function chipPresence(
  who: 'user' | 'guest' | 'none',
  status: PresenceStatus,
  running: ReadonlySet<string>,
  games: readonly { id: string; title: string }[],
): ChipPresence {
  if (who === 'guest') return { line: msg('ui.noAccount') };
  if (who === 'none') return { line: '' };
  const own = ownStatusLine(status, running.size > 0);
  if (own) return { line: own, dot: statusDot(status) };
  const titles = new Map(games.map((game) => [game.id, game.title]));
  const title = [...running].map((id) => titles.get(id)).find((name) => !!name);
  return { line: title ? msg('social.playingNamed', { name: title }) : msg('social.playing'), dot: 'online' };
}
