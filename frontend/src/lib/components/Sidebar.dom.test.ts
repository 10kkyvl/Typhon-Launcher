import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { resetHistory } from '../stores/router';
import { unreadReplies } from '../stores/support';
import { renderDom } from '../testing/dom';
import Sidebar from './Sidebar.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// jsdom lays nothing out: the nav is given a 200px view and an item its place inside the nav's
// scrolled content, the numbers Sidebar reads when the route changes.
function layout(element: HTMLElement, values: Record<string, unknown>) {
  for (const [key, value] of Object.entries(values)) {
    Object.defineProperty(element, key, { value, writable: true, configurable: true });
  }
}

function sidebar() {
  const view = renderDom(Sidebar);
  const nav = view.container.querySelector<HTMLElement>('[data-part="sidebar-nav"]');
  if (!nav) throw new Error('no sidebar nav');
  layout(nav, { clientHeight: 200, scrollTop: 0 });
  const item = (label: string, offsetTop: number) => {
    const button = view.getByRole('button', { name: label });
    layout(button, { offsetParent: nav, offsetTop, offsetHeight: 40 });
    return button;
  };
  return { ...view, nav, item };
}

afterEach(() => {
  resetHistory();
  unreadReplies.set(0);
});

describe('Sidebar plate in a DOM', () => {
  it('scrolls the nav down to an active item below its view', async () => {
    const { user, nav, item } = sidebar();
    const history = item(msg('ui.history'), 400);

    await user.click(history);
    await tick();

    expect(history.getAttribute('aria-current')).toBe('page');
    expect(nav.scrollTop).toBe(240);
  });

  it('scrolls the nav up to an active item above its view', async () => {
    const { user, nav, item } = sidebar();
    nav.scrollTop = 300;
    const catalog = item(msg('ui.allGames'), 48);

    await user.click(catalog);
    await tick();

    expect(nav.scrollTop).toBe(48);
  });

  it('leaves the scroll alone when the active item is already in view', async () => {
    const { user, nav, item } = sidebar();
    nav.scrollTop = 100;
    const friends = item(msg('ui.friends'), 150);

    await user.click(friends);
    await tick();

    expect(nav.scrollTop).toBe(100);
  });
});

describe('Sidebar counts', () => {
  it('an item with something new says how much as part of its name, and drops it once there is nothing', async () => {
    unreadReplies.set(2);
    const view = renderDom(Sidebar);
    const support = view.getByRole('button', { name: `${msg('tickets.nav')} ${msg('tickets.unreadCount', { count: 2 })}` });
    expect(support.querySelector('[data-part="count-badge"]')?.textContent).toContain('2');

    unreadReplies.set(0);
    await tick();

    expect(view.getByRole('button', { name: msg('tickets.nav') })).toBe(support);
    expect(support.querySelector('[data-part="count-badge"]')).toBeNull();
  });
});
