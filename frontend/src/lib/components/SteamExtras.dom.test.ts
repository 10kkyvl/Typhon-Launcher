import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { SteamInfo } from '../services/metadata';
import { renderDom } from '../testing/dom';
import SteamExtras from './SteamExtras.svelte';

const steam = {
  appId: 1,
  url: '',
  dlc: Array.from({ length: 12 }, (_, index) => ({ appId: index + 10, name: `Pack ${index + 1}` })),
} as SteamInfo;

describe('SteamExtras: a long list of DLC', () => {
  it('shows the rest of the list with a disclosure button tied to the list', async () => {
    const { user } = renderDom(SteamExtras, { steam, onopendlc: vi.fn() });
    const toggle = screen.getByRole('button', { name: msg('games.detailDlcShowAll') });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(toggle.hasAttribute('aria-pressed')).toBe(false);
    const list = document.getElementById(toggle.getAttribute('aria-controls') ?? '');
    expect(list?.tagName).toBe('UL');
    const before = list?.children.length ?? 0;
    toggle.focus();

    await user.keyboard('{Enter}');

    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(list?.children.length).toBe(12);
    expect(before).toBeLessThan(12);
  });
});
