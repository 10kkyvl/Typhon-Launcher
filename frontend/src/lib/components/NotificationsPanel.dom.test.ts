import { get } from 'svelte/store';
import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
// The list itself (what counts, its order, the read ids) is the store's and has its own tests; here
// it is a plain list, and "mark all read" empties it as the store does.
vi.mock('../stores/notifications', async (actual) => {
  const { writable } = await import('svelte/store');
  const notifications = writable<unknown[]>([]);
  return {
    ...(await actual<typeof import('../stores/notifications')>()),
    notifications,
    markAllRead: vi.fn(() => notifications.set([])),
  };
});

import { locale, msg } from '../i18n';
import { markAllRead, notifications, type Notification } from '../stores/notifications';
import { navigate, resetHistory, route } from '../stores/router';
import { user } from '../testing/dom';
import Topbar from './Topbar.svelte';
import type { Writable } from 'svelte/store';

const notes = notifications as unknown as Writable<Notification[]>;
const onOpen = vi.fn();
const UNREAD: Notification[] = [
  { id: 'a', title: 'Hades installed', text: 'Ready to play', route: 'game', params: { id: 'g-hades' }, onOpen, terminal: true },
  { id: 'b', title: 'Celeste updated', text: 'Version 1.4', route: 'library', terminal: true },
];

let app: HTMLDivElement;

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  locale.set('ru');
  notes.set([]);
  resetHistory();
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

function setup(list: Notification[] = UNREAD) {
  notes.set(list);
  render(Topbar, { target: app });
  const keys = user();
  const slot = app.querySelector('[data-part="topbar-action"][data-id="notifications"]') as HTMLElement;
  const bell = () => within(slot).getByRole('button');
  const panel = () => screen.queryByRole('dialog', { name: msg('ui.notifications') });
  return { keys, slot, bell, panel };
}

describe('Notifications bell', () => {
  // The badge is a picture of the count: the bell's name is what a screen reader hears.
  const badge = (slot: HTMLElement) => slot.querySelector('[data-part="count-badge"]');
  const digits = (slot: HTMLElement) => badge(slot)?.querySelector('[aria-hidden="true"]');

  it('carries the unread count in its name, the badge hidden from assistive tech', () => {
    locale.set('en');
    const { slot, bell } = setup();
    expect(bell().getAttribute('aria-label')).toBe('Notifications, 2 unread');
    expect(digits(slot)?.textContent?.trim()).toBe('2');
    expect(bell().contains(badge(slot))).toBe(true);
  });

  it('names the count with the Russian plural forms', async () => {
    locale.set('ru');
    const { bell } = setup([UNREAD[0]]);
    expect(bell().getAttribute('aria-label')).toBe('Уведомления, 1 новое');
    notes.set([...UNREAD, { ...UNREAD[1], id: 'c' }]);
    await Promise.resolve();
    expect(bell().getAttribute('aria-label')).toBe('Уведомления, 3 новых');
  });

  it('with nothing unread is named "Notifications" and shows no badge', () => {
    locale.set('en');
    const { slot, bell } = setup([]);
    expect(bell().getAttribute('aria-label')).toBe('Notifications');
    expect(badge(slot)).toBeNull();
  });
});

describe('Notifications panel', () => {
  it('the bell in its slot says it opens a dialog and is not expanded', () => {
    const { bell } = setup();
    expect(bell().getAttribute('aria-haspopup')).toBe('dialog');
    expect(bell().getAttribute('aria-expanded')).toBe('false');
  });

  it('opens under the bell, right-aligned, out of the top bar, named, listing the unread notifications', async () => {
    const { keys, bell, panel } = setup();
    const rects = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this === bell()) return DOMRect.fromRect({ x: 800, y: 8, width: 36, height: 36 });
      if (this.getAttribute('role') === 'dialog') return DOMRect.fromRect({ x: 0, y: 0, width: 320, height: 240 });
      return DOMRect.fromRect();
    });
    try {
      await keys.click(bell());

      const opened = panel() as HTMLElement;
      expect(opened).not.toBeNull();
      expect(bell().getAttribute('aria-expanded')).toBe('true');
      expect(bell().getAttribute('aria-controls')).toBe(opened.id);
      expect(app.contains(opened)).toBe(false);
      expect(opened.dataset.part).toBe('popover');
      expect(opened.dataset.id).toBe('notifications');
      expect(opened.dataset.placement).toBe('bottom-end');
      expect(Number.parseFloat(opened.style.left)).toBe(836 - 320);
      expect(Number.parseFloat(opened.style.top)).toBeGreaterThan(44);
      const items = within(within(opened).getByRole('list')).getAllByRole('listitem');
      expect(items.map((item) => item.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
        'Hades installed Ready to play',
        'Celeste updated Version 1.4',
      ]);
      expect(within(opened).getByRole('button', { name: msg('ui.markAllRead') })).toBeTruthy();
      expect(within(opened).getByRole('button', { name: msg('ui.allHistory') })).toBeTruthy();
    } finally {
      rects.mockRestore();
    }
  });

  it('with nothing unread shows the empty state and history, without "mark all read"', async () => {
    const { keys, bell, panel } = setup([]);
    await keys.click(bell());
    const opened = panel() as HTMLElement;
    expect(within(opened).getByText(msg('ui.nothingNewYet'))).toBeTruthy();
    expect(within(opened).queryByRole('button', { name: msg('ui.markAllRead') })).toBeNull();
    expect(within(opened).getByRole('button', { name: msg('ui.allHistory') })).toBeTruthy();
  });

  it('Enter on the bell opens it with focus on its first control', async () => {
    const { keys, bell, panel } = setup();
    bell().focus();

    await keys.keyboard('{Enter}');

    expect(panel()).not.toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('ui.markAllRead') }));
  });

  it('pressing a notification closes the panel, opens its page and runs its action', async () => {
    const { keys, bell, panel } = setup();
    await keys.click(bell());

    await keys.click(screen.getByRole('button', { name: /Hades installed/ }));

    expect(panel()).toBeNull();
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-hades' } });
    expect(onOpen).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(bell());
  });

  it('"mark all read" keeps the panel open on the empty state, with focus still in it', async () => {
    const { keys, bell, panel } = setup();
    await keys.click(bell());

    await keys.click(screen.getByRole('button', { name: msg('ui.markAllRead') }));

    expect(markAllRead).toHaveBeenCalledOnce();
    const opened = panel() as HTMLElement;
    expect(opened).not.toBeNull();
    expect(within(opened).getByText(msg('ui.nothingNewYet'))).toBeTruthy();
    expect(opened.contains(document.activeElement)).toBe(true);
  });

  it('"all history" closes the panel and opens the history', async () => {
    const { keys, bell, panel } = setup();
    await keys.click(bell());

    await keys.click(screen.getByRole('button', { name: msg('ui.allHistory') }));

    expect(panel()).toBeNull();
    expect(get(route).name).toBe('history');
  });

  it('a route change from elsewhere closes it', async () => {
    const { keys, bell, panel } = setup();
    await keys.click(bell());
    expect(panel()).not.toBeNull();

    navigate('downloads');
    await Promise.resolve();

    expect(panel()).toBeNull();
    expect(bell().getAttribute('aria-expanded')).toBe('false');
  });

  it('Escape closes it and focus is back on the bell; a press on the bell closes it too', async () => {
    const { keys, bell, panel } = setup();
    await keys.click(bell());

    await keys.keyboard('{Escape}');
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(bell());

    await keys.click(bell());
    await keys.click(bell());
    expect(panel()).toBeNull();
  });

  it('a press elsewhere in the top bar closes it', async () => {
    const { keys, bell, panel } = setup();
    await keys.click(bell());

    await keys.click(app.querySelector('header[data-part="topbar"]') as HTMLElement);

    expect(panel()).toBeNull();
    expect(get(route).name).toBe('library');
  });
});

describe('Notifications panel controls', () => {
  it('marks all read and opens the history with base buttons the keyboard presses', async () => {
    const { keys, bell, panel } = setup();
    await keys.click(bell());
    const opened = panel() as HTMLElement;
    const markRead = within(opened).getByRole('button', { name: msg('ui.markAllRead') });
    const history = within(opened).getByRole('button', { name: msg('ui.allHistory') });
    expect(markRead.classList.contains('btn')).toBe(true);
    expect(history.classList.contains('btn')).toBe(true);
    markRead.focus();

    await keys.keyboard('{Enter}');

    expect(markAllRead).toHaveBeenCalledOnce();
  });

  it('says there is nothing new with the base empty state', async () => {
    const { keys, bell, panel } = setup([]);
    await keys.click(bell());
    expect(within(panel() as HTMLElement).getByText(msg('ui.nothingNewYet')).closest('.empty')).toBeTruthy();
  });
});
