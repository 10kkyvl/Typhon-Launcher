import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import { accountMenuItems } from './accountMenu';
import { STATUS_LABELS, statusDot } from '../social/presence';
import { msg } from '../i18n';

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
