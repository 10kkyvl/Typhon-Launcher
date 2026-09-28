import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  Limits: vi.fn(),
  List: vi.fn(),
  Mine: vi.fn(),
  Save: vi.fn(),
  Delete: vi.fn(),
  Vote: vi.fn(),
  Report: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/reviews', () => ({ Service: bindings }));

describe('reviews service, inside Wails', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
    vi.doMock('./backend', () => ({ inWails: true }));
  });

  it.each([
    ['review_too_short', 'body'],
    ['review_too_long', 'body'],
    ['review_low_effort', 'body'],
    ['review_links', 'body'],
    ['review_duplicate', 'body'],
    ['review_bad_reason', 'reason'],
    ['review_not_played', ''],
    ['review_account_too_new', ''],
    ['review_own', ''],
    ['review_post_cooldown', ''],
    ['review_daily_limit', ''],
    ['review_repost_cooldown', ''],
    ['review_edit_cooldown', ''],
    ['review_report_limit', ''],
    ['review_not_found', ''],
    ['account_muted', ''],
    ['unknown_game', ''],
    ['bad_request', ''],
    ['rate_limited', ''],
  ])('maps known backend code %s to field %s', async (code, field) => {
    const { save } = await import('./reviews');
    bindings.Save.mockRejectedValueOnce(new Error(code));
    const err = await save('canonical-1', true, 'text').catch((e) => e);
    expect(err).toMatchObject({ name: 'AccountError', code, field });
  });

  it('does not let an unknown backend code look like success', async () => {
    const { save } = await import('./reviews');
    bindings.Save.mockRejectedValueOnce(new Error('some_unmapped_thing'));
    await expect(save('canonical-1', true, 'text')).rejects.toMatchObject({ code: 'server_error', field: '' });
  });

  it('passes List through unchanged and defaults a missing reviews array', async () => {
    const { list } = await import('./reviews');
    bindings.List.mockResolvedValueOnce({ summary: { total: 3, positive: 2 }, reviews: null, next: 'abc' });
    const page = await list('canonical-1', 'helpful', 'all', '');
    expect(page).toEqual({ summary: { total: 3, positive: 2 }, reviews: [], next: 'abc' });
    expect(bindings.List).toHaveBeenCalledWith('canonical-1', 'helpful', 'all', '');
  });

  it('passes Mine through and defaults a missing eligibility', async () => {
    const { mine } = await import('./reviews');
    bindings.Mine.mockResolvedValueOnce({ review: null });
    const result = await mine('canonical-1');
    expect(result.review).toBeNull();
    expect(result.eligibility).toEqual({ canPost: false, reason: '', retryAt: '', playtimeSeconds: 0, requiredPlaytimeSeconds: 0 });
  });

  it('forwards Vote and Report arguments untouched', async () => {
    const { vote, report } = await import('./reviews');
    bindings.Vote.mockResolvedValueOnce({ helpful: 5, unhelpful: 1, myVote: 'helpful' });
    await expect(vote(42, 'helpful')).resolves.toEqual({ helpful: 5, unhelpful: 1, myVote: 'helpful' });
    expect(bindings.Vote).toHaveBeenCalledWith(42, 'helpful');

    bindings.Report.mockResolvedValueOnce(undefined);
    await report(42, 'spam');
    expect(bindings.Report).toHaveBeenCalledWith(42, 'spam');
  });

  it('deletes through the backend', async () => {
    const { remove } = await import('./reviews');
    bindings.Delete.mockResolvedValueOnce(undefined);
    await expect(remove('canonical-1')).resolves.toBeUndefined();
    expect(bindings.Delete).toHaveBeenCalledWith('canonical-1');
  });

  it('reads Limits from the backend', async () => {
    const { limits } = await import('./reviews');
    bindings.Limits.mockResolvedValueOnce({ minBodyRunes: 20, maxBodyRunes: 5000, minPlaytimeSeconds: 1800 });
    await expect(limits()).resolves.toEqual({ minBodyRunes: 20, maxBodyRunes: 5000, minPlaytimeSeconds: 1800 });
  });
});

describe('reviews service, outside Wails', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
    vi.doMock('./backend', () => ({ inWails: false }));
  });

  it('returns an empty page for List instead of throwing', async () => {
    const { list } = await import('./reviews');
    await expect(list('canonical-1', 'helpful', 'all')).resolves.toEqual({
      summary: { total: 0, positive: 0 },
      reviews: [],
      next: '',
    });
    expect(bindings.List).not.toHaveBeenCalled();
  });

  it('returns an empty Mine instead of throwing', async () => {
    const { mine } = await import('./reviews');
    const result = await mine('canonical-1');
    expect(result.review).toBeNull();
    expect(result.eligibility.canPost).toBe(false);
    expect(bindings.Mine).not.toHaveBeenCalled();
  });

  it('rejects writes with the unauthenticated error, never a silent success', async () => {
    const { save, remove, vote, report } = await import('./reviews');
    await expect(save('c', true, 'x')).rejects.toMatchObject({ name: 'AccountError', code: 'unauthenticated' });
    await expect(remove('c')).rejects.toMatchObject({ code: 'unauthenticated' });
    await expect(vote(1, 'helpful')).rejects.toMatchObject({ code: 'unauthenticated' });
    await expect(report(1, 'spam')).rejects.toMatchObject({ code: 'unauthenticated' });
    expect(bindings.Save).not.toHaveBeenCalled();
    expect(bindings.Delete).not.toHaveBeenCalled();
    expect(bindings.Vote).not.toHaveBeenCalled();
    expect(bindings.Report).not.toHaveBeenCalled();
  });

  it('rejects Limits instead of inventing numbers', async () => {
    const { limits } = await import('./reviews');
    await expect(limits()).rejects.toMatchObject({ code: 'unauthenticated' });
    expect(bindings.Limits).not.toHaveBeenCalled();
  });
});
