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

function guestMenu() {
  const view = renderDom(AccountMenu);
  const chip = view.getByRole('button', { name: new RegExp(msg('ui.guest')) });
  const item = (key: 'ui.profile' | 'ui.signIn') => view.queryByRole('menuitem', { name: msg(key) });
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
  it('a click on the chip opens the account menu, named by the chip, and the arrows reach its items', async () => {
    const { user, chip, item, getByRole } = guestMenu();
    expect(item('ui.profile')).toBeNull();
    expect(chip.getAttribute('aria-haspopup')).toBe('menu');

    await user.click(chip);

    expect(chip.getAttribute('aria-expanded')).toBe('true');
    expect(getByRole('menu', { name: new RegExp(msg('ui.guest')) }).id).toBe(chip.getAttribute('aria-controls'));
    expect(item('ui.profile')).not.toBeNull();
    expect(item('ui.signIn')).not.toBeNull();
    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(item('ui.profile'));
    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(item('ui.signIn'));
  });

  it('opens the menu down from the chip, its right edge on the chip\'s right edge', async () => {
    const { user, chip, getByRole } = guestMenu();
    const rects = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this === chip) return DOMRect.fromRect({ x: 700, y: 4, width: 200, height: 36 });
      if (this.getAttribute('role') === 'menu') return DOMRect.fromRect({ x: 0, y: 0, width: 190, height: 120 });
      return DOMRect.fromRect();
    });
    try {
      await user.click(chip);
      const menu = getByRole('menu');
      expect(menu.dataset.placement).toBe('bottom-end');
      expect([menu.style.left, menu.style.top]).toEqual([`${900 - 190}px`, `${40 + 4}px`]);
    } finally {
      rects.mockRestore();
    }
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

    await user.click(getByRole('menuitem', { name: msg('ui.signIn') }));

    expect(vi.mocked(logout)).toHaveBeenCalledTimes(1);
    expect(get(authState)).toBe('unauthenticated');
    expect(item('ui.signIn')).toBeNull();
  });

  it('Escape closes the open menu and focus is on the chip', async () => {
    const { user, chip, item } = guestMenu();
    await user.click(chip);
    expect(item('ui.profile')).not.toBeNull();

    await user.keyboard('{Escape}');

    expect(item('ui.profile')).toBeNull();
    expect(document.activeElement).toBe(chip);
  });

  it('opening the menu from the keyboard moves focus to its first item', async () => {
    const { user, chip, item } = guestMenu();
    chip.focus();

    await user.keyboard('{Enter}');

    expect(item('ui.profile')).not.toBeNull();
    expect(document.activeElement).toBe(item('ui.profile'));
  });

  it('picking an item from the keyboard returns focus to the chip', async () => {
    const { user, chip, item } = guestMenu();
    chip.focus();
    await user.keyboard('{Enter}');
    expect(document.activeElement).toBe(item('ui.profile'));

    await user.keyboard('{Enter}');

    expect(item('ui.profile')).toBeNull();
    expect(document.activeElement).toBe(chip);
  });
});
