import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { mine, remove } from '../../lib/services/reviews';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { user } from '../../lib/testing/dom';
import ReviewComposer from './ReviewComposer.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/reviews', () => ({
  mine: vi.fn(async () => ({
    review: { recommended: true, body: 'Great game', hidden: false },
    eligibility: { canPost: true },
  })),
  limits: vi.fn(async () => ({ minBodyRunes: 10, maxBodyRunes: 2000 })),
  remove: vi.fn(async () => {}),
  save: vi.fn(async () => {}),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  vi.mocked(mine).mockReset();
  app.remove();
  authState.set('bootstrapping');
  toasts.set([]);
  vi.restoreAllMocks();
});

describe('retrying a failed load', () => {
  it('keeps the notice and the focus on its busy Retry until the answer, then focuses what loaded', async () => {
    let answer!: (value: Awaited<ReturnType<typeof mine>>) => void;
    vi.mocked(mine)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    const notice = (await screen.findByText(msg('reviews.loadError'))).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    answer({ review: { recommended: true, body: 'Great game', hidden: false }, eligibility: { canPost: true } } as unknown as Awaited<ReturnType<typeof mine>>);
    const edit = await screen.findByRole('button', { name: msg('reviews.composerEdit') });
    expect(notice.isConnected).toBe(false);
    await vi.waitFor(() => expect(document.activeElement).toBe(edit));
  });
});

describe('choosing whether I recommend the game', () => {
  it('is a named radio group the arrows move through, one Tab stop, nothing chosen at first', async () => {
    vi.mocked(mine).mockResolvedValueOnce({ review: null, eligibility: { canPost: true } } as unknown as Awaited<ReturnType<typeof mine>>);
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    const group = await screen.findByRole('radiogroup', { name: msg('reviews.composerMineTitle') });
    const yes = within(group).getByRole('radio', { name: msg('reviews.composerRecommend') });
    const no = within(group).getByRole('radio', { name: msg('reviews.composerNotRecommend') });
    expect([yes.getAttribute('aria-checked'), no.getAttribute('aria-checked')]).toEqual(['false', 'false']);
    expect([yes.tabIndex, no.tabIndex]).toEqual([0, -1]);

    await keys.tab();
    expect(document.activeElement).toBe(yes);
    await keys.keyboard('{ArrowRight}');

    expect(document.activeElement).toBe(no);
    expect([yes.getAttribute('aria-checked'), no.getAttribute('aria-checked')]).toEqual(['false', 'true']);
    expect([yes.tabIndex, no.tabIndex]).toEqual([-1, 0]);

    await keys.keyboard('{ArrowLeft}');
    expect(yes.getAttribute('aria-checked')).toBe('true');
    await keys.keyboard('{ArrowLeft}');
    expect(no.getAttribute('aria-checked')).toBe('true');
  });
});

describe('deleting my review', () => {
  it('a refused delete keeps the confirmation open with the review error, not a toast under it', async () => {
    vi.mocked(remove).mockRejectedValueOnce(new AccountError('rate_limited'));
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    await keys.click(await screen.findByRole('button', { name: msg('reviews.composerDelete') }));
    const dialog = screen.getByRole('dialog', { name: msg('reviews.composerDeleteConfirmTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('reviews.composerDeleteConfirmButton') }));

    expect((await screen.findByRole('alert')).textContent?.trim()).toBe(msg('reviews.errorRateLimited'));
    expect(dialog.isConnected).toBe(true);
    expect(get(toasts)).toEqual([]);
  });
});
