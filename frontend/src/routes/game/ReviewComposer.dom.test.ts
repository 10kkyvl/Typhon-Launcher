import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { remove } from '../../lib/services/reviews';
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
  app.remove();
  authState.set('bootstrapping');
  toasts.set([]);
  vi.restoreAllMocks();
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
