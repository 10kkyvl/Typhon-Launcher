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

describe('SteamExtras: a DLC', () => {
  // Its store page has no address here: the launcher builds it and opens it, so the chip is a button.
  it('is a chip button named by the DLC, the external mark out of its name, opened by the launcher', async () => {
    const onopendlc = vi.fn();
    const { user } = renderDom(SteamExtras, { steam, onopendlc });
    const chip = screen.getByRole('button', { name: 'Pack 2' });
    expect(chip.classList.contains('chip')).toBe(true);
    expect(chip.querySelector('.end')?.getAttribute('aria-hidden')).toBe('true');
    chip.focus();

    await user.keyboard('{Enter}');

    expect(onopendlc).toHaveBeenCalledWith(11);
  });
});

// The page passes another game's Steam data to the same block when it moves on without a remount.
describe('SteamExtras: another game in the same block', () => {
  const requirements = (tag: string) => ({
    minimum: [{ label: 'OS', value: `${tag} minimum` }],
    recommended: [{ label: 'OS', value: `${tag} recommended` }],
  });
  const first = { ...steam, appId: 1, requirements: requirements('A') } as SteamInfo;
  const second = { ...steam, appId: 2, requirements: requirements('B') } as SteamInfo;

  it('starts from the minimum requirements and a collapsed DLC list', async () => {
    const { user, rerender } = renderDom(SteamExtras, { steam: first, onopendlc: vi.fn() });
    await user.click(screen.getByRole('tab', { name: msg('games.detailRequirementsRecommended') }));
    await user.click(screen.getByRole('button', { name: msg('games.detailDlcShowAll') }));
    expect(screen.getByText('A recommended')).toBeTruthy();

    await rerender({ steam: second, onopendlc: vi.fn() });

    expect(screen.getByRole('tab', { name: msg('games.detailRequirementsMinimum') }).getAttribute('aria-selected')).toBe('true');
    expect(screen.getByText('B minimum')).toBeTruthy();
    const toggle = screen.getByRole('button', { name: msg('games.detailDlcShowAll') });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
  });

  it('keeps what the player opened when the same game\'s data comes again', async () => {
    const { user, rerender } = renderDom(SteamExtras, { steam: first, onopendlc: vi.fn() });
    await user.click(screen.getByRole('tab', { name: msg('games.detailRequirementsRecommended') }));
    await user.click(screen.getByRole('button', { name: msg('games.detailDlcShowAll') }));

    await rerender({ steam: { ...first }, onopendlc: vi.fn() });

    expect(screen.getByText('A recommended')).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('games.detailDlcCollapse') }).getAttribute('aria-expanded')).toBe('true');
  });
});

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
