import { screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/lan', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/lan')>()),
  getOffers: vi.fn(async () => get((await import('../../lib/stores/lan')).offers)),
}));

import { msg } from '../../lib/i18n';
import { getOffers } from '../../lib/services/lan';
import { offers, shares } from '../../lib/stores/lan';
import { route } from '../../lib/stores/router';
import { renderDom } from '../../lib/testing/dom';
import Lan from './Lan.svelte';
import { offer, ready, resetScreen, sectionOf, share, sharingOn } from './lanTesting';

afterEach(() => {
  resetScreen();
  vi.mocked(getOffers).mockClear();
});

const offNotice = () => screen.queryByText(msg('transfers.lanSharingOffTitle'))?.closest('.notice') ?? null;
const nearbyHeading = () => screen.queryByRole('heading', { level: 2, name: new RegExp(`^${msg('transfers.lanNearbyHeading')}`) });

describe('Lan: sharing switched off', () => {
  beforeEach(() => ready(false));

  it('says sharing is off, what that means and where it is turned on', () => {
    renderDom(Lan);

    const notice = offNotice() as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.textContent).toContain(msg('transfers.lanSharingOffText'));
    expect(notice.getAttribute('data-tone')).toBe('info');
    expect(screen.getByRole('button', { name: msg('transfers.lanOpenSettings') })).toBeTruthy();
  });

  it('its action opens the General tab of the settings', async () => {
    const { user } = renderDom(Lan);

    await user.click(screen.getByRole('button', { name: msg('transfers.lanOpenSettings') }));

    expect(get(route)).toMatchObject({ name: 'settings', params: { tab: 'general' } });
  });

  it('shows no "nothing nearby" and reads no offers: none can come while sharing is off', async () => {
    renderDom(Lan);
    await Promise.resolve();

    expect(nearbyHeading()).toBeNull();
    expect(screen.queryByText(msg('transfers.lanEmptyTitle'))).toBeNull();
    expect(getOffers).not.toHaveBeenCalled();
  });

  it('keeps the games set up for sharing in their section', () => {
    shares.set([share()]);
    renderDom(Lan);

    expect(sectionOf('transfers.lanSharingHeading').textContent).toContain('Hades');
  });
});

describe('Lan: sharing on', () => {
  beforeEach(() => ready(true));

  it('says nothing about sharing being off', () => {
    renderDom(Lan);

    expect(offNotice()).toBeNull();
    expect(nearbyHeading()).toBeTruthy();
  });

  it('turned off while the screen is shown: the notice comes and the nearby section goes; on again: it comes back and is read', async () => {
    offers.set([offer()]);
    renderDom(Lan);

    sharingOn(false);
    await waitFor(() => expect(offNotice()).not.toBeNull());
    expect(nearbyHeading()).toBeNull();

    vi.mocked(getOffers).mockClear();
    sharingOn(true);
    await waitFor(() => expect(nearbyHeading()).toBeTruthy());
    expect(offNotice()).toBeNull();
    expect(getOffers).toHaveBeenCalledTimes(1);
  });

  it('turned off with focus on an offer: focus goes to the page heading, not to the page body', async () => {
    offers.set([offer()]);
    renderDom(Lan);
    screen.getByRole('button', { name: msg('transfers.lanDownloadFor', { title: 'Portal 2', host: 'PC-1' }) }).focus();

    sharingOn(false);

    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });
});
