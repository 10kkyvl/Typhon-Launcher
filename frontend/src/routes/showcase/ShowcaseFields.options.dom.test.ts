import { screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());

import { ruleBody, styleOf } from '../../lib/testing/css';
import { renderDom } from '../../lib/testing/dom';
import ShowcaseFields from './ShowcaseFields.svelte';
import source from './ShowcaseFields.svelte?raw';

afterEach(() => {
  vi.useRealTimers();
  document.body.replaceChildren();
});

const checkedIn = (group: HTMLElement) => within(group).getAllByRole('radio').filter((row) => row.getAttribute('aria-checked') === 'true');

describe('the showcase option lists', () => {
  // jsdom lays nothing out: the bound box is read from the source; the scrolling itself is seen in a browser.
  it('shows a list of twenty rows inside a box that scrolls', () => {
    renderDom(ShowcaseFields);
    const group = screen.getByRole('radiogroup', { name: /^Twenty folders/ });
    expect(within(group).getAllByRole('radio')).toHaveLength(20);
    const box = group.closest('.scroll-box');
    expect(box).not.toBeNull();
    const rule = ruleBody(styleOf(source, 'ShowcaseFields.svelte'), '.scroll-box');
    expect(rule['max-height']).toBeTruthy();
    expect(rule['overflow-y']).toBe('auto');
  });

  it('holds a list while the page saves the choice, says why, and lets go after the save', async () => {
    vi.useFakeTimers();
    const { user } = renderDom(ShowcaseFields);
    const group = screen.getByRole('radiogroup', { name: /^Sync saves to/ });
    const second = within(group).getAllByRole('radio')[1];

    await user.click(second);
    expect(group.getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByText('Saving the choice…')).toBeTruthy();
    expect(checkedIn(group)).not.toContain(second);
    await user.click(within(group).getAllByRole('radio')[2]);

    await vi.advanceTimersByTimeAsync(1000);
    expect(group.hasAttribute('aria-disabled')).toBe(false);
    expect(checkedIn(group)).toEqual([second]);
  });

  it('shows a list the arrows walk without choosing, applying a choice on Enter', async () => {
    const { user } = renderDom(ShowcaseFields);
    const group = screen.getByRole('radiogroup', { name: /^Download region/ });
    const rows = within(group).getAllByRole('radio');
    rows[0].focus();

    await user.keyboard('{ArrowDown}{ArrowDown}');
    expect(screen.getByText('Applied 0 times')).toBeTruthy();
    expect(document.activeElement).toBe(rows[2]);

    await user.keyboard('{Enter}');
    expect(screen.getByText('Applied 1 time')).toBeTruthy();
    expect(checkedIn(group)).toEqual([rows[2]]);
  });
});
