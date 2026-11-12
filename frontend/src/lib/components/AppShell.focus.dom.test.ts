import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

import { msg } from '../i18n';
import { resetHistory } from '../stores/router';
import { renderDom } from '../testing/dom';
import RouteHarness from '../testing/RouteHarness.svelte';

// jsdom keeps no scroll position: each element remembers what it was scrolled to, as a browser does.
const scrolled = new WeakMap<Element, number>();
const scrollTop = Object.getOwnPropertyDescriptor(Element.prototype, 'scrollTop');

const frame = async () => {
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  await tick();
};
const page = () => document.querySelector<HTMLElement>('main.content') as HTMLElement;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  Object.defineProperty(Element.prototype, 'scrollTop', {
    configurable: true,
    get(this: Element) {
      return scrolled.get(this) ?? 0;
    },
    set(this: Element, value: number) {
      scrolled.set(this, value);
    },
  });
});

afterEach(() => {
  if (scrollTop) Object.defineProperty(Element.prototype, 'scrollTop', scrollTop);
  resetHistory();
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('focus after a page change', () => {
  it('a pick on the page that goes lands on the new page heading', async () => {
    const { user } = renderDom(RouteHarness, {});

    await user.click(screen.getByRole('button', { name: 'Open Celeste' }));
    await tick();

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Celeste' }));
  });

  it('a pick that opens a missing game lands on the page that says so', async () => {
    const { user } = renderDom(RouteHarness, { missing: true });

    await user.click(screen.getByRole('button', { name: 'Open Celeste' }));
    await tick();

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Game not found' }));
  });

  it('a heading that comes once the page has loaded takes focus then', async () => {
    const { user, rerender } = renderDom(RouteHarness, { loaded: false });
    await user.click(screen.getByRole('button', { name: 'Open Celeste' }));
    await tick();
    expect(document.activeElement).toBe(document.body);

    await rerender({ loaded: true });

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Celeste' }));
  });

  it('focus that moved on before the heading came is left where it is', async () => {
    const { user, rerender } = renderDom(RouteHarness, { loaded: false });
    await user.click(screen.getByRole('button', { name: 'Open Celeste' }));
    await tick();
    const back = screen.getByRole('button', { name: 'Back to the library' });
    back.focus();

    await rerender({ loaded: true });

    expect(document.activeElement).toBe(back);
  });

  it('a pick in the sidebar keeps focus on the sidebar item', async () => {
    const { user } = renderDom(RouteHarness, {});
    const sidebar = document.querySelector<HTMLElement>('[data-part="sidebar"]') as HTMLElement;
    const friends = within(sidebar).getByRole('button', { name: msg('ui.friends') });

    await user.click(friends);
    await tick();

    expect(screen.getByRole('heading', { level: 1, name: 'Friends' })).toBeTruthy();
    expect(document.activeElement).toBe(friends);
  });

  it('opening the window moves no focus', async () => {
    renderDom(RouteHarness, {});
    await tick();
    expect(document.activeElement).toBe(document.body);
  });

  it('coming back restores where the page was scrolled to, the heading focused without scrolling', async () => {
    const { user } = renderDom(RouteHarness, {});
    page().scrollTop = 300;
    page().dispatchEvent(new Event('scroll'));
    await user.click(screen.getByRole('button', { name: 'Open Celeste' }));
    await tick();
    const focus = vi.spyOn(HTMLElement.prototype, 'focus');

    await user.click(screen.getByRole('button', { name: 'Back to the library' }));
    await frame();

    const heading = screen.getByRole('heading', { level: 1, name: 'Library' });
    expect(document.activeElement).toBe(heading);
    expect(focus.mock.contexts).toContain(heading);
    expect(focus.mock.calls[focus.mock.contexts.indexOf(heading)]).toEqual([{ preventScroll: true }]);
    expect(page().scrollTop).toBe(300);
  });
});

