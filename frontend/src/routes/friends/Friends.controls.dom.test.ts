import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { friendCode } from '../../lib/services/social';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import FriendCodeCard from './FriendCodeCard.svelte';
import Friends from './Friends.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  friendCode: vi.fn(),
}));

beforeEach(() => {
  // jsdom has no ResizeObserver; the page's segmented controls measure with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
});

afterEach(() => {
  authState.set('bootstrapping');
  vi.mocked(friendCode).mockReset();
  vi.unstubAllGlobals();
});

describe('Friends: the requests tab', () => {
  it('states its safety advice in an info notice of the page, read in its flow and not announced', () => {
    authState.set('authenticated');
    renderDom(Friends, { tab: 'requests' });
    const advice = screen.getByText(msg('social.friendsSafetyDesc'));
    const notice = advice.closest('.notice');
    expect(notice?.getAttribute('data-tone')).toBe('info');
    expect(advice.closest('[role="status"], [role="alert"]')).toBeNull();
    expect(screen.getByRole('heading', { name: msg('social.friendsSafetyTitle') }).closest('.notice')).toBe(notice);
  });
});

describe('FriendCodeCard', () => {
  it('shows the base spinner named "Loading" while the code loads', () => {
    vi.mocked(friendCode).mockReturnValue(new Promise(() => {}));
    renderDom(FriendCodeCard);
    const status = screen.getByRole('status');
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('social.loadingEllipsis'));
  });
});
