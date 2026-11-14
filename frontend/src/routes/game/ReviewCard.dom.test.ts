import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { report as sendReport, vote, type Review } from '../../lib/services/reviews';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import ReviewCard from './ReviewCard.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/reviews', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/reviews')>()),
  vote: vi.fn(),
  report: vi.fn(),
}));

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
  vi.mocked(vote).mockReset();
  vi.mocked(sendReport).mockReset();
  toasts.set([]);
  vi.restoreAllMocks();
});

describe('ReviewCard: voting', () => {
  it('votes with a pressed chip that the keyboard toggles', async () => {
    vi.mocked(vote).mockResolvedValue({ helpful: 3, unhelpful: 1, myVote: 'helpful' } as Awaited<ReturnType<typeof vote>>);
    const { user } = renderDom(ReviewCard, { review: review('Fine.'), guest: false });
    const yes = screen.getByRole('button', { name: `${msg('reviews.helpfulYes')} (2)` });
    expect(yes.classList.contains('chip')).toBe(true);
    expect(yes.getAttribute('aria-pressed')).toBe('false');
    yes.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(yes.getAttribute('aria-pressed')).toBe('true'));
    expect(vote).toHaveBeenCalledWith(7, 'helpful');
    expect(yes.textContent?.trim()).toBe(`${msg('reviews.helpfulYes')} (3)`);
    expect(screen.getByRole('button', { name: `${msg('reviews.helpfulNo')} (1)` }).getAttribute('aria-pressed')).toBe('false');
  });
});

describe('ReviewCard: a vote in flight', () => {
  it('keeps the focus on the pressed chip and refuses a second press until the answer', async () => {
    let answer!: (value: Awaited<ReturnType<typeof vote>>) => void;
    vi.mocked(vote).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { user } = renderDom(ReviewCard, { review: review('Fine.'), guest: false });
    const yes = screen.getByRole('button', { name: `${msg('reviews.helpfulYes')} (2)` });
    yes.focus();

    await user.keyboard('{Enter}');

    expect(yes.hasAttribute('disabled')).toBe(false);
    expect(yes.getAttribute('aria-disabled')).toBe('true');
    expect(yes.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(yes);
    await user.keyboard('{Enter}');
    expect(vote).toHaveBeenCalledTimes(1);

    // The other chip is not what runs: it refuses presses and keeps its count in sight.
    const no = screen.getByRole('button', { name: `${msg('reviews.helpfulNo')} (1)` });
    expect(no.hasAttribute('aria-busy')).toBe(false);
    expect(no.getAttribute('aria-disabled')).toBe('true');
    expect(no.querySelector('.content')?.classList.contains('waiting')).toBe(false);
    await user.click(no);
    expect(vote).toHaveBeenCalledTimes(1);
    yes.focus();

    answer({ helpful: 3, unhelpful: 1, myVote: 'helpful' } as Awaited<ReturnType<typeof vote>>);
    await vi.waitFor(() => expect(yes.hasAttribute('aria-busy')).toBe(false));
    expect(yes.hasAttribute('aria-disabled')).toBe(false);
    expect(document.activeElement).toBe(yes);
  });

  it('shows the vote at once and takes it back, both counts and my vote, when it is refused', async () => {
    let refuse!: (err: unknown) => void;
    vi.mocked(vote).mockReturnValueOnce(new Promise((_, reject) => (refuse = reject)));
    const { user } = renderDom(ReviewCard, { review: { ...review('Fine.'), myVote: 'unhelpful' }, guest: false });
    const [yes, no] = screen.getAllByRole('button').filter((button) => button.classList.contains('chip'));
    expect([yes.textContent?.trim(), no.getAttribute('aria-pressed')]).toEqual([`${msg('reviews.helpfulYes')} (2)`, 'true']);

    await user.click(yes);

    expect([yes.textContent?.trim(), no.textContent?.trim()]).toEqual([`${msg('reviews.helpfulYes')} (3)`, `${msg('reviews.helpfulNo')} (0)`]);
    expect([yes.getAttribute('aria-pressed'), no.getAttribute('aria-pressed')]).toEqual(['true', 'false']);

    refuse(new AccountError('rate_limited'));

    await vi.waitFor(() => expect(yes.textContent?.trim()).toBe(`${msg('reviews.helpfulYes')} (2)`));
    expect(no.textContent?.trim()).toBe(`${msg('reviews.helpfulNo')} (1)`);
    expect([yes.getAttribute('aria-pressed'), no.getAttribute('aria-pressed')]).toEqual(['false', 'true']);
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[msg('reviews.errorRateLimited'), 'danger']]);
  });
});

describe('ReviewCard: reporting', () => {
  it('opens the report dialog from a base button the keyboard presses', async () => {
    const { user } = renderDom(ReviewCard, { review: review('Fine.'), guest: false });
    const report = screen.getByRole('button', { name: msg('reviews.report') });
    expect(report.classList.contains('btn')).toBe(true);
    report.focus();

    await user.keyboard('{Enter}');

    expect(screen.getByRole('dialog', { name: msg('reviews.reportTitle') })).toBeTruthy();
  });
});

describe('ReviewCard: sending a report', () => {
  it('keeps focus on the pressed reason, busy, while it sends, then returns it to Report', async () => {
    let sent!: () => void;
    vi.mocked(sendReport).mockReturnValueOnce(new Promise<void>((resolve) => (sent = resolve)));
    const { user } = renderDom(ReviewCard, { review: review('Fine.'), guest: false });
    const report = screen.getByRole('button', { name: msg('reviews.report') });
    report.focus();
    await user.keyboard('{Enter}');
    const dialog = screen.getByRole('dialog', { name: msg('reviews.reportTitle') });
    const spam = within(dialog).getByRole('button', { name: msg('reviews.reportReasonSpam') });
    const other = within(dialog).getByRole('button', { name: msg('reviews.reportReasonOther') });
    spam.focus();

    await user.keyboard('{Enter}');

    expect(spam.hasAttribute('disabled')).toBe(false);
    expect(spam.getAttribute('aria-busy')).toBe('true');
    expect(other.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(spam);
    await user.click(other);
    expect(vi.mocked(sendReport)).toHaveBeenCalledTimes(1);

    sent();
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(report));
  });
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
