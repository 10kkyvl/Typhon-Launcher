import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import { accountMenuItems, chipPresence } from './accountMenu';
import { STATUS_LABELS, statusDot } from '../social/presence';
import { msg } from '../i18n';

const games = [
  { id: 'g1', title: 'Hollow Knight' },
  { id: 'g2', title: 'Celeste' },
];
const none = new Set<string>();

describe('account chip presence', () => {
  it('reads the user status with its dot when no game runs', () => {
    for (const status of ['online', 'away', 'busy'] as const) {
      expect(chipPresence('user', status, none, games)).toEqual({ line: STATUS_LABELS[status], dot: statusDot(status) });
    }
  });

  it('reads an invisible user as invisible with the offline dot, also while a game runs', () => {
    const invisible = { line: STATUS_LABELS.invisible, dot: 'offline' };
    expect(chipPresence('user', 'invisible', none, games)).toEqual(invisible);
    expect(chipPresence('user', 'invisible', new Set(['g1']), games)).toEqual(invisible);
  });

  it('names the running game with the in-game dot whatever the status', () => {
    for (const status of ['online', 'away', 'busy'] as const) {
      expect(chipPresence('user', status, new Set(['g2']), games)).toEqual({
        line: msg('social.playingNamed', { name: 'Celeste' }),
        dot: 'online',
      });
    }
  });

  it('names the first running game the library knows when several run', () => {
    expect(chipPresence('user', 'online', new Set(['gone', 'g1', 'g2']), games).line).toBe(
      msg('social.playingNamed', { name: 'Hollow Knight' }),
    );
  });

  it('says playing without a title when the library does not know the game', () => {
    expect(chipPresence('user', 'online', new Set(['gone']), games)).toEqual({ line: msg('social.playing'), dot: 'online' });
  });

  it('shows a guest as having no account and no dot', () => {
    expect(chipPresence('guest', 'online', new Set(['g1']), games)).toEqual({ line: msg('ui.noAccount') });
  });

  it('shows nothing under the name of a signed-out window', () => {
    expect(chipPresence('none', 'online', none, games)).toEqual({ line: '' });
  });
});

// The menu the sidebar's account block opened before the chip moved to the top bar (Sidebar.svelte at b48dc23b).
describe('account menu items', () => {
  it('offers a guest the profile and sign-in', () => {
    expect(accountMenuItems(true, 'online')).toEqual([
      { id: 'profile', label: msg('ui.profile') },
      { id: 'login', label: msg('ui.signIn'), separator: true },
    ]);
  });

  it('offers a signed-in user the profile, every status with the current one checked, and sign-out last', () => {
    expect(accountMenuItems(false, 'busy')).toEqual([
      { id: 'profile', label: msg('ui.profile') },
      { id: 'status:online', label: STATUS_LABELS.online, dot: statusDot('online'), checked: false, separator: true },
      { id: 'status:away', label: STATUS_LABELS.away, dot: statusDot('away'), checked: false, separator: false },
      { id: 'status:busy', label: STATUS_LABELS.busy, dot: statusDot('busy'), checked: true, separator: false },
      { id: 'status:invisible', label: STATUS_LABELS.invisible, dot: statusDot('invisible'), checked: false, separator: false },
      { id: 'logout', label: msg('ui.signOut'), danger: true, separator: true },
    ]);
  });
});
