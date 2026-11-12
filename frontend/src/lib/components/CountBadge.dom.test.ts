import { screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import { renderDom } from '../testing/dom';
import CountHarness from '../testing/CountHarness.svelte';
import buttonSource from './Button.svelte?raw';
import IconButton from './IconButton.svelte';
import source from './CountBadge.svelte?raw';

const bubble = () => document.querySelector<HTMLElement>('[data-part="count-badge"]');
const shown = () => bubble()?.querySelector('[aria-hidden="true"]')?.textContent;

describe('CountBadge', () => {
  it('shows nothing for nothing: zero, less, or not a number', async () => {
    const { rerender } = renderDom(CountHarness, { count: 0 });
    expect(bubble()).toBeNull();
    expect(screen.getByRole('button', { name: 'Friends' })).toBeTruthy();
    await rerender({ count: -2 });
    expect(bubble()).toBeNull();
    await rerender({ count: Number.NaN });
    expect(bubble()).toBeNull();
  });

  it('shows the number up to its cap and the cap with a plus beyond it', async () => {
    const { rerender } = renderDom(CountHarness, { count: 7 });
    expect(shown()).toBe('7');
    await rerender({ count: 99 });
    expect(shown()).toBe('99');
    await rerender({ count: 100 });
    expect(shown()).toBe('99+');
  });

  it('takes a lower cap where the room is small', () => {
    renderDom(CountHarness, { count: 12, max: 9 });
    expect(shown()).toBe('9+');
  });

  it('is said as part of its control name through the text it is given, the bare digits not at all', async () => {
    const { rerender } = renderDom(CountHarness, { count: 3 });
    expect(screen.getByRole('button', { name: 'Friends 3 requests' })).toBeTruthy();
    await rerender({ count: 120 });
    // The cap is a picture; the text says the real number.
    expect(screen.getByRole('button', { name: 'Friends 120 requests' })).toBeTruthy();
  });
});

describe('IconButton with a count', () => {
  it('shows the count on its corner and is named by the counted name while there is one', async () => {
    const { rerender } = renderDom(IconButton, { label: 'Friends', count: 3, countLabel: 'Friends, 3 requests' });
    const button = screen.getByRole('button', { name: 'Friends, 3 requests' });
    expect(button.contains(bubble())).toBe(true);
    expect(shown()).toBe('3');
    expect(bubble()?.classList.contains('corner')).toBe(true);

    await rerender({ label: 'Friends', count: 0, countLabel: 'Friends, 0 requests' });

    expect(screen.getByRole('button', { name: 'Friends' })).toBe(button);
    expect(bubble()).toBeNull();
  });

  it('on an accent fill draws the badge so it is not lost in the fill', () => {
    renderDom(IconButton, { label: 'Open chats', variant: 'primary', count: 2, countLabel: 'Open chats, 2 unread' });
    expect(bubble()?.classList.contains('on-accent')).toBe(true);
  });
});

// The badge paints pairs theme-contrast.test.ts measures in every preset: the label colour on the
// accent fill, the one the primary button uses; on an accent fill, the text colour on the page.
describe('CountBadge colours', () => {
  const css = styleOf(source, 'CountBadge.svelte');
  const primary = ruleBody(styleOf(buttonSource, 'Button.svelte'), '.primary');

  it('is the primary button label on the primary button fill', () => {
    const badge = ruleBody(css, '.count-badge');
    expect(badge.background).toBe(primary.background);
    expect(badge.color).toBe(primary.color);
  });

  it('on an accent fill it takes the text on the page, not the fill it stands on', () => {
    expect(ruleBody(css, '.count-badge.on-accent')).toEqual({ background: 'var(--bg)', color: 'var(--text)' });
  });
});
