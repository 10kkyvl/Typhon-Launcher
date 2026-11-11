import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { logout } from '../services/account';
import { authState, currentUser } from '../stores/user';
import { resetHistory } from '../stores/router';
import { renderDom } from '../testing/dom';
import AccountMenu from './AccountMenu.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/account', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/account')>()),
  logout: vi.fn(async () => {}),
}));

const GAP = 'rebuilt in the base components stage';

function guestMenu() {
  const view = renderDom(AccountMenu);
  const chip = view.getByRole('button', { name: new RegExp(msg('ui.guest')) });
  const item = (key: 'ui.profile' | 'ui.signIn') => view.queryByRole('button', { name: msg(key) });
  return { ...view, chip, item };
}

beforeEach(() => {
  vi.clearAllMocks();
  authState.set('guest');
});

afterEach(() => {
  authState.set('bootstrapping');
  currentUser.set(null);
  resetHistory();
});

describe('AccountMenu in a DOM', () => {
  it('a click on the chip opens the menu and Tab reaches its items', async () => {
    const { user, chip, item } = guestMenu();
    expect(item('ui.profile')).toBeNull();

    await user.click(chip);

    expect(item('ui.profile')).not.toBeNull();
    expect(item('ui.signIn')).not.toBeNull();
    await user.tab();
    expect(document.activeElement).toBe(item('ui.profile'));
    await user.tab();
    expect(document.activeElement).toBe(item('ui.signIn'));
  });

  it('a click outside closes the menu', async () => {
    const { user, chip, item } = guestMenu();
    await user.click(chip);
    expect(item('ui.profile')).not.toBeNull();

    await user.click(document.body);

    expect(item('ui.profile')).toBeNull();
  });

  it('"Sign in" for a guest leaves the guest session once and closes the menu', async () => {
    const { user, chip, item, getByRole } = guestMenu();
    await user.click(chip);

    await user.click(getByRole('button', { name: msg('ui.signIn') }));

    expect(vi.mocked(logout)).toHaveBeenCalledTimes(1);
    expect(get(authState)).toBe('unauthenticated');
    expect(item('ui.signIn')).toBeNull();
  });

  it.fails(`Escape closes the open menu (${GAP})`, async () => {
    const { user, chip, item } = guestMenu();
    await user.click(chip);
    expect(item('ui.profile')).not.toBeNull();

    await user.keyboard('{Escape}');

    expect(item('ui.profile')).toBeNull();
  });

  it.fails(`opening the menu from the keyboard moves focus to its first item (${GAP})`, async () => {
    const { user, chip, item } = guestMenu();
    chip.focus();

    await user.keyboard('{Enter}');

    expect(item('ui.profile')).not.toBeNull();
    expect(document.activeElement).toBe(item('ui.profile'));
  });

  it.fails(`picking an item from the keyboard returns focus to the chip (${GAP})`, async () => {
    const { user, chip, item } = guestMenu();
    chip.focus();
    await user.keyboard('{Enter}');
    await user.tab();
    expect(document.activeElement).toBe(item('ui.profile'));

    await user.keyboard('{Enter}');

    expect(item('ui.profile')).toBeNull();
    expect(document.activeElement).toBe(chip);
  });
});
