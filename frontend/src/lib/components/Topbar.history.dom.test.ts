import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

import { locale, msg } from '../i18n';
import { navigate, resetHistory, route } from '../stores/router';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import Topbar from './Topbar.svelte';

beforeEach(() => {
  locale.set('en');
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
});

afterEach(() => {
  resetHistory();
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

function history() {
  const bar = document.querySelector<HTMLElement>('[data-part="topbar-history"]') as HTMLElement;
  return {
    back: () => within(bar).getByRole('button', { name: msg('common.back') }),
    forward: () => within(bar).getByRole('button', { name: msg('ui.forward') }),
  };
}

// A disabled button cannot hold focus: the browser drops it to <body> once the button is disabled.
// The history buttons stay focusable when they have nowhere to go, and say why.
describe('Topbar history buttons', () => {
  it('Back pressed on the way to the first page keeps focus, has nowhere to go and says why', async () => {
    navigate('game', { id: 'celeste' });
    const { user } = renderDom(Topbar);
    const { back } = history();
    back().focus();

    await user.keyboard('{Enter}');
    await tick();

    expect(get(route).name).toBe('library');
    expect(document.activeElement).toBe(back());
    expect(back().hasAttribute('disabled')).toBe(false);
    expect(back().getAttribute('aria-disabled')).toBe('true');
    expect(description(back())).toBe(msg('ui.backNowhere'));

    await user.keyboard('{Enter}');
    await user.click(back());

    expect(get(route).name).toBe('library');
  });

  it('Forward pressed on the way to the last page keeps focus the same way', async () => {
    navigate('game', { id: 'celeste' });
    const { user } = renderDom(Topbar);
    const { back, forward } = history();
    expect(forward().getAttribute('aria-disabled')).toBe('true');
    back().focus();
    await user.keyboard('{Enter}');
    await tick();
    forward().focus();

    await user.keyboard('{Enter}');
    await tick();

    expect(get(route).name).toBe('game');
    expect(document.activeElement).toBe(forward());
    expect(forward().getAttribute('aria-disabled')).toBe('true');
    expect(description(forward())).toBe(msg('ui.forwardNowhere'));
    expect(screen.getAllByRole('button').filter((button) => button.hasAttribute('disabled'))).toEqual([]);
  });
});
