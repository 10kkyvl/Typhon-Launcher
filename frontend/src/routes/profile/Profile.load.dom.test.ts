import { cleanup, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import type { CurrentUser, ProfileBlock } from '../../lib/services/account';
import { EMPTY_SNAPSHOT, getProfilePreview, getProfileSnapshot, type ProfileSnapshot } from '../../lib/services/profile';
import { profileDraft, profileFailed, profileSnapshot, refreshProfile } from '../../lib/stores/profile';
import { navigate, resetHistory, stayHere } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Profile from './Profile.svelte';
import { player, ref, SNAPSHOT } from './profileFixtures';
import { pending } from './profileTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));

// The first read of the own profile: the figures and the blocks built from the snapshot say they are
// loading until it comes, a failed read is a danger notice with Retry, and so is a failed preview of
// the showcase.

const block = (type: string): ProfileBlock => ({ id: `b-${type}`, type, width: 'full', config: {} });
const withBlocks = (...types: string[]): CurrentUser => player({ layout: { version: 1, blocks: types.map(block) } });

const loading = () => screen.queryAllByRole('status').some((status) => status.textContent?.includes(msg('profile.loadingStats')));
// The figures drawn anywhere on the page: the header's tiles, the stats block's and the fingerprint's.
const values = () => [...document.querySelectorAll('.tile .value, .facts .num')].map((value) => value.textContent?.trim());
const retry = () => screen.getByRole('button', { name: msg('common.retry') });
// The header's mark that holds the figures' place while they load.
const headerSpinner = () => document.querySelector('.profile-header .stats .spinner');

function signIn(me: CurrentUser = withBlocks('stats', 'fingerprint', 'about')) {
  authState.set('authenticated');
  currentUser.set(me);
}

beforeEach(() => {
  locale.set('en');
  resetBindings();
  navigate('profile');
  vi.mocked(getProfilePreview).mockResolvedValue(SNAPSHOT);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(getProfileSnapshot).mockReset();
  vi.mocked(getProfilePreview).mockReset();
  stayHere();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  profileFailed.set(false);
  profileDraft.set(null);
  toasts.set([]);
  document.body.replaceChildren();
});

describe('the first read of the profile', () => {
  it('until the snapshot comes the page says it is loading and shows no figure; then the figures come', async () => {
    const answer = pending<ProfileSnapshot>();
    vi.mocked(getProfileSnapshot).mockReturnValue(answer.promise);
    signIn();
    renderDom(Profile);

    expect(loading()).toBeTruthy();
    expect(values()).toEqual([]);
    expect(headerSpinner()).not.toBeNull();

    answer.resolve(SNAPSHOT);

    await waitFor(() => expect(screen.getAllByText('12').length).toBeGreaterThan(0));
    expect(loading()).toBeFalsy();
  });

  it('a snapshot held from an earlier visit is shown at once, while the page reads it again', async () => {
    vi.mocked(getProfileSnapshot).mockReturnValue(pending<ProfileSnapshot>().promise);
    profileSnapshot.set(SNAPSHOT);
    signIn();
    renderDom(Profile);

    expect(screen.getAllByText('12').length).toBeGreaterThan(0);
    expect(loading()).toBeFalsy();
  });

  it('a failed read is a danger notice with Retry and shows no figure; Retry that works brings the figures', async () => {
    vi.mocked(getProfileSnapshot).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(SNAPSHOT);
    signIn();
    const { user } = renderDom(Profile);

    const alert = await screen.findByText(msg('profile.loadFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(loading()).toBeFalsy();
    expect(values()).toEqual([]);
    expect(headerSpinner()).toBeNull();

    await user.click(retry());

    await waitFor(() => expect(screen.queryByText(msg('profile.loadFailed'))).toBeNull());
    expect(getProfileSnapshot).toHaveBeenCalledTimes(2);
    expect(screen.getAllByText('12').length).toBeGreaterThan(0);
  });

  it('a Retry that fails again keeps the notice and the focus on Retry', async () => {
    vi.mocked(getProfileSnapshot).mockRejectedValue(new Error('offline'));
    signIn();
    const { user } = renderDom(Profile);
    const before = (await screen.findByText(msg('profile.loadFailed'))).closest('.message');

    await user.click(retry());

    await waitFor(() => expect(getProfileSnapshot).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(retry().getAttribute('aria-busy')).toBeNull());
    expect(document.activeElement).toBe(retry());
    // Said again: the failure comes into the alert anew.
    await waitFor(() => expect(screen.getByText(msg('profile.loadFailed')).closest('.message')).not.toBe(before));
    expect(values()).toEqual([]);
  });
});

describe('a snapshot Retry replaced by a newer read of the store', () => {
  it('stays busy until the newer read answers, and says nothing failed when that one comes', async () => {
    vi.mocked(getProfileSnapshot).mockRejectedValueOnce(new Error('offline'));
    signIn();
    const { user } = renderDom(Profile);
    await screen.findByText(msg('profile.loadFailed'));
    const pressed = pending<ProfileSnapshot>();
    const newer = pending<ProfileSnapshot>();
    vi.mocked(getProfileSnapshot).mockReturnValueOnce(pressed.promise).mockReturnValueOnce(newer.promise);

    await user.click(retry());
    const replacing = refreshProfile();
    pressed.reject(new Error('offline'));
    await new Promise((settled) => setTimeout(settled, 0));

    expect(retry().getAttribute('aria-busy')).toBe('true');
    newer.resolve(SNAPSHOT);
    expect(await replacing).toBe(true);
    await waitFor(() => expect(screen.queryByText(msg('profile.loadFailed'))).toBeNull());
    expect(screen.getAllByText('12').length).toBeGreaterThan(0);
  });
});

describe('the preview of the showcase', () => {
  async function editing() {
    vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
    signIn();
    const view = renderDom(Profile);
    await view.user.click(await screen.findByRole('button', { name: msg('profileStyle.edit') }));
    return view;
  }

  it('a failed preview is a danger notice with Retry; Retry that works takes it away', async () => {
    vi.mocked(getProfilePreview).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(SNAPSHOT);
    const { user } = await editing();

    const notice = (await screen.findByText(msg('profile.previewLoadFailed'))).closest('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));

    await waitFor(() => expect(screen.queryByText(msg('profile.previewLoadFailed'))).toBeNull());
    expect(getProfilePreview).toHaveBeenCalledTimes(2);
  });

  it('a preview Retry that fails again keeps the notice and the focus on Retry', async () => {
    vi.mocked(getProfilePreview).mockRejectedValue(new Error('offline'));
    const { user } = await editing();
    const text = await screen.findByText(msg('profile.previewLoadFailed'));
    const before = text.closest('.message');
    const notice = text.closest('[data-tone="danger"]') as HTMLElement;
    const again = within(notice).getByRole('button', { name: msg('common.retry') });

    await user.click(again);

    await waitFor(() => expect(getProfilePreview).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(again.getAttribute('aria-busy')).toBeNull());
    expect(document.activeElement).toBe(again);
    await waitFor(() => expect(screen.getByText(msg('profile.previewLoadFailed')).closest('.message')).not.toBe(before));
  });

  it('a preview Retry still on its way when the editor closes leaves no notice behind', async () => {
    vi.mocked(getProfilePreview).mockRejectedValueOnce(new Error('offline')).mockReturnValue(pending<ProfileSnapshot>().promise);
    const { user } = await editing();
    const notice = (await screen.findByText(msg('profile.previewLoadFailed'))).closest('[data-tone="danger"]') as HTMLElement;

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));

    await waitFor(() => expect(screen.queryByText(msg('profile.previewLoadFailed'))).toBeNull());
  });

  // The answers below land after the editor closed: the read they answer was replaced by the closing.
  const settle = () => new Promise((resolve) => setTimeout(resolve, 20));
  const shelf = (title: string): ProfileSnapshot => ({ ...SNAPSHOT, showcase: [{ kind: 'most_played', games: [ref('g9', title)] }] });

  it('a preview Retry that fails after the editor closed leaves no notice on the closed page', async () => {
    const late = pending<ProfileSnapshot>();
    vi.mocked(getProfilePreview).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(late.promise);
    const { user } = await editing();
    const notice = (await screen.findByText(msg('profile.previewLoadFailed'))).closest('[data-tone="danger"]') as HTMLElement;

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    await waitFor(() => expect(screen.queryByText(msg('profile.previewLoadFailed'))).toBeNull());

    late.reject(new Error('offline'));
    await settle();

    expect(screen.queryByText(msg('profile.previewLoadFailed'))).toBeNull();
  });

  it('the first preview read failing after the editor closed leaves no notice either', async () => {
    const late = pending<ProfileSnapshot>();
    vi.mocked(getProfilePreview).mockReturnValueOnce(late.promise);
    const { user } = await editing();

    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    late.reject(new Error('offline'));
    await settle();

    expect(screen.queryByText(msg('profile.previewLoadFailed'))).toBeNull();
  });

  it('a preview Retry answered after the editor closed is not painted when the editor opens again', async () => {
    const late = pending<ProfileSnapshot>();
    const next = pending<ProfileSnapshot>();
    vi.mocked(getProfilePreview).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(late.promise).mockReturnValueOnce(next.promise);
    vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
    signIn(player({ layout: { version: 1, blocks: [{ id: 'b-most', type: 'collection', width: 'full', config: { source: 'most_played' } }] } }));
    const { user } = renderDom(Profile);
    await user.click(await screen.findByRole('button', { name: msg('profileStyle.edit') }));
    const notice = (await screen.findByText(msg('profile.previewLoadFailed'))).closest('[data-tone="danger"]') as HTMLElement;

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    late.resolve(shelf('Stale Game'));
    await settle();
    expect(screen.queryByText('Stale Game')).toBeNull();

    await user.click(screen.getByRole('button', { name: msg('profileStyle.edit') }));
    await settle();
    expect(screen.queryByText('Stale Game')).toBeNull();

    next.resolve(shelf('Fresh Game'));
    expect(await screen.findByText('Fresh Game')).toBeTruthy();
  });

  it('a failure clears when the preview is read again on the next opening of the editor', async () => {
    vi.mocked(getProfilePreview).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(SNAPSHOT);
    const { user } = await editing();
    await screen.findByText(msg('profile.previewLoadFailed'));

    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    await user.click(screen.getByRole('button', { name: msg('profileStyle.edit') }));

    await waitFor(() => expect(getProfilePreview).toHaveBeenCalledTimes(2));
    expect(screen.queryByText(msg('profile.previewLoadFailed'))).toBeNull();
  });
});
