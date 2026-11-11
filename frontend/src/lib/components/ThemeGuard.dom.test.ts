import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { user } from '../testing/dom';
import MenuHarness from '../testing/MenuHarness.svelte';
import type { Theme } from '../services/theme';
import { activeTheme, endTrial, themeTrial, tryTheme } from '../stores/theme';
import { mountThemeGuard } from './ThemeGuard';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

const theme = (id: string, builtIn: boolean): Theme => ({ id, name: id, base: 'dark', tokens: {}, builtIn, updatedAt: '' });

let unmount: () => void;
let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  unmount = mountThemeGuard();
  activeTheme.set(theme('dark', true));
  expect(tryTheme(theme('draft', false))).toBeNull();
  expect(get(themeTrial)).not.toBeNull();
});

afterEach(() => {
  endTrial();
  unmount();
  activeTheme.set(null);
  document.body.replaceChildren();
});

describe('theme trial and Escape', () => {
  it('Escape that closes an open menu belongs to the menu: the trial goes on', async () => {
    render(MenuHarness, { target: app, props: { items: [{ id: 'a', label: 'Alphabet' }] } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Sort' }));
    expect(screen.getByRole('menu')).toBeTruthy();

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('menu')).toBeNull();
    expect(get(themeTrial)).not.toBeNull();
  });

  it('Escape with nothing open ends the trial', async () => {
    render(MenuHarness, { target: app, props: { items: [{ id: 'a', label: 'Alphabet' }] } });
    const keys = user();
    screen.getByRole('button', { name: 'Sort' }).focus();

    await keys.keyboard('{Escape}');

    expect(get(themeTrial)).toBeNull();
  });
});
