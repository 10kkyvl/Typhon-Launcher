import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Review } from '../../lib/services/reviews';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import ReviewCard from './ReviewCard.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

function review(body: string): Review {
  return {
    id: 7,
    author: { username: 'mira', displayName: 'Mira', avatarUrl: '' },
    recommended: true,
    body,
    playtimeSeconds: 3600,
    helpful: 2,
    unhelpful: 1,
    myVote: '',
    mine: false,
    createdAt: '2026-11-01T10:00:00Z',
    updatedAt: '2026-11-01T10:00:00Z',
  } as unknown as Review;
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  toasts.set([]);
  vi.restoreAllMocks();
});

describe('ReviewCard: a long review', () => {
  it('opens and closes its text with a disclosure button that says it expands the text', async () => {
    const { user } = renderDom(ReviewCard, { review: review('word '.repeat(200)), guest: false });
    const more = screen.getByRole('button', { name: msg('reviews.readMore') });
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(more.hasAttribute('aria-pressed')).toBe(false);
    const body = document.getElementById(more.getAttribute('aria-controls') ?? '');
    expect(body?.textContent?.endsWith('…')).toBe(true);
    more.focus();

    await user.keyboard('{Enter}');

    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(more.textContent?.trim()).toBe(msg('reviews.readLess'));
    expect(body?.textContent?.endsWith('…')).toBe(false);
  });
});
