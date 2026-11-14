import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { mine, remove, save } from '../../lib/services/reviews';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { user } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
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

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
// Read before the press; the getter answers the words of a message that came in since, '' until then.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

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
  it('keeps the notice and the focus on its busy Retry until the answer, then focuses my review', async () => {
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

  it('keeps the notice when the retry fails again, and says it again', async () => {
    vi.mocked(mine).mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new Error('still offline'));
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    const notice = (await screen.findByText(msg('reviews.loadError'))).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    const said = sayingAgain(notice);
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('reviews.loadError')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
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

describe('a refused review text', () => {
  it('shows the reason by the field as an alert, and the field says it is invalid and is described by it', async () => {
    vi.mocked(mine).mockResolvedValueOnce({ review: null, eligibility: { canPost: true } } as unknown as Awaited<ReturnType<typeof mine>>);
    vi.mocked(save).mockRejectedValueOnce(new AccountError('review_links', 'body'));
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    await keys.click(await screen.findByRole('radio', { name: msg('reviews.composerRecommend') }));
    const field = screen.getByRole('textbox');
    expect(field.classList.contains('input')).toBe(true);
    expect(description(field)).toContain(msg('reviews.composerHint', { min: 10, max: 2000 }));
    await keys.type(field, 'See example dot com for more');

    await keys.click(screen.getByRole('button', { name: msg('reviews.composerPublish') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('reviews.errorLinks'));
    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(description(field)).toContain(msg('reviews.errorLinks'));

    await keys.type(field, '!');
    expect(field.getAttribute('aria-invalid')).not.toBe('true');
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

type MineAnswer = Awaited<ReturnType<typeof mine>>;
const mineOf = (value: unknown) => value as MineAnswer;
const inAnHour = () => new Date(Date.now() + 3_600_000).toISOString();

describe('a publish the server refuses', () => {
  it('keeps the typed text, the verdict and the form, and says when posting opens again', async () => {
    vi.mocked(mine)
      .mockResolvedValueOnce(mineOf({ review: null, eligibility: { canPost: true } }))
      .mockResolvedValueOnce(mineOf({ review: null, eligibility: { canPost: false, reason: 'review_daily_limit', retryAt: inAnHour() } }));
    vi.mocked(save).mockRejectedValueOnce(new AccountError('review_daily_limit'));
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    const yes = await screen.findByRole('radio', { name: msg('reviews.composerRecommend') });
    await keys.click(yes);
    const draft = 'A long evening with this game, and every minute of it was worth it.';
    await keys.type(screen.getByRole('textbox'), draft);

    await keys.click(screen.getByRole('button', { name: msg('reviews.composerPublish') }));

    await vi.waitFor(() => expect(vi.mocked(mine)).toHaveBeenCalledTimes(2));
    const note = msg('reviews.eligibilityDailyLimit', { time: msg('reviews.retryHours', { count: 1 }) });
    await screen.findByText(note);
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(draft);
    expect(screen.getByRole('radio', { name: msg('reviews.composerRecommend') }).getAttribute('aria-checked')).toBe('true');
    expect(get(toasts).map((t) => t.message)).toContain(msg('reviews.errorGeneric'));
  });

  it('keeps an edit of my review open with its changes when the edit is refused', async () => {
    const original = { recommended: true, body: 'Great game, I liked it a lot.', hidden: false };
    vi.mocked(mine)
      .mockResolvedValueOnce(mineOf({ review: original, eligibility: { canPost: true } }))
      .mockResolvedValueOnce(mineOf({ review: original, eligibility: { canPost: false, reason: 'review_edit_cooldown', retryAt: inAnHour() } }));
    vi.mocked(save).mockRejectedValueOnce(new AccountError('review_edit_cooldown'));
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    await keys.click(await screen.findByRole('button', { name: msg('reviews.composerEdit') }));
    await keys.click(screen.getByRole('radio', { name: msg('reviews.composerNotRecommend') }));
    await keys.type(screen.getByRole('textbox'), ' Until the last chapter.');

    await keys.click(screen.getByRole('button', { name: msg('reviews.composerSave') }));

    await vi.waitFor(() => expect(vi.mocked(mine)).toHaveBeenCalledTimes(2));
    await screen.findByText(msg('reviews.eligibilityEditCooldown', { time: msg('reviews.retryHours', { count: 1 }) }));
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(`${original.body} Until the last chapter.`);
    expect(screen.getByRole('radio', { name: msg('reviews.composerNotRecommend') }).getAttribute('aria-checked')).toBe('true');
    expect(screen.getByRole('button', { name: msg('reviews.composerCancel') })).toBeTruthy();
  });

  it('keeps the draft through the automatic reload when posting opens again', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    const soon = () => new Date(Date.now() + 500).toISOString();
    vi.mocked(mine)
      .mockResolvedValueOnce(mineOf({ review: null, eligibility: { canPost: true } }))
      .mockResolvedValueOnce(mineOf({ review: null, eligibility: { canPost: false, reason: 'review_post_cooldown', retryAt: soon() } }))
      .mockResolvedValueOnce(mineOf({ review: null, eligibility: { canPost: true } }));
    vi.mocked(save).mockRejectedValueOnce(new AccountError('review_post_cooldown'));
    render(ReviewComposer, { target: app, props: { canonicalGameId: 'g1' } });
    const keys = user();
    await keys.click(await screen.findByRole('radio', { name: msg('reviews.composerNotRecommend') }));
    const draft = 'It crashed twice in the first hour and never recovered.';
    await keys.type(screen.getByRole('textbox'), draft);
    await keys.click(screen.getByRole('button', { name: msg('reviews.composerPublish') }));
    await vi.waitFor(() => expect(vi.mocked(mine)).toHaveBeenCalledTimes(2));

    await vi.advanceTimersByTimeAsync(2000);

    await vi.waitFor(() => expect(vi.mocked(mine)).toHaveBeenCalledTimes(3));
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(draft);
    expect(screen.getByRole('radio', { name: msg('reviews.composerNotRecommend') }).getAttribute('aria-checked')).toBe('true');
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
