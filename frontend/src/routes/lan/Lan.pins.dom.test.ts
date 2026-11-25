import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/lan', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/lan')>()),
  // The screen reads the offers again while it is shown: Go answers what the test set.
  getOffers: vi.fn(async () => get((await import('../../lib/stores/lan')).offers)),
  receive: vi.fn(),
  unshare: vi.fn(),
  cancel: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { cancel, receive, unshare } from '../../lib/services/lan';
import { hashing, lanStats, offers, shares, transfers } from '../../lib/stores/lan';
import { libraryGames } from '../../lib/stores/library';
import type { LibraryGame } from '../../lib/services/library';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize, formatCount } from '../../lib/utils/format';
import Lan from './Lan.svelte';
import { offer, progress, ready, resetScreen, rowButton, rowOf, sectionOf, share, stats, transfer, words } from './lanTesting';

beforeEach(() => {
  ready();
  vi.mocked(receive).mockImplementation(async (infoHash, peerId) => transfer({ id: 'new', infoHash, peerId, title: 'Portal 2' }));
  vi.mocked(unshare).mockResolvedValue(undefined);
  vi.mocked(cancel).mockResolvedValue(undefined);
});

afterEach(() => {
  resetScreen();
  vi.mocked(receive).mockReset();
  vi.mocked(unshare).mockReset();
  vi.mocked(cancel).mockReset();
});

const heading = (text: string) => screen.getByRole('heading', { level: 2, name: text });

// Stage 1: what the screen shows and does today, pinned before it is rebuilt.
describe('Lan pins: the three figures', () => {
  it('says computers nearby, games available and games shared, each value over its label', () => {
    lanStats.set(stats({ peersKnown: 2, offersKnown: 1234, sharesActive: 3 }));
    const { container } = renderDom(Lan);

    const tiles = [...container.querySelectorAll('.tile')].map(words);
    expect(tiles).toEqual([
      `${formatCount(2)} ${msg('transfers.lanPeersLabel')}`,
      `${formatCount(1234)} ${msg('transfers.lanOffersLabel')}`,
      `${formatCount(3)} ${msg('transfers.lanSharesLabel')}`,
    ]);
  });
});

describe('Lan pins: the sections and their counts', () => {
  it('names each section with its count: offers, shared games, receiving transfers', () => {
    offers.set([offer(), offer({ peerId: 'p2', host: 'PC-2', title: 'Hollow Knight', infoHash: 'd'.repeat(40) })]);
    shares.set([share()]);
    transfers.set([transfer(), transfer({ id: 't2', title: 'Inside' })]);
    renderDom(Lan);

    expect(heading(`${msg('transfers.lanNearbyHeading')} 2`)).toBeTruthy();
    expect(heading(`${msg('transfers.lanSharingHeading')} 1`)).toBeTruthy();
    expect(heading(`${msg('transfers.lanTransfersHeading')} 2`)).toBeTruthy();
  });

  it('an offer row says its title, version, size and the computer it comes from', () => {
    offers.set([offer({ sizeBytes: 5 * 1024 ** 3 })]);
    renderDom(Lan);

    const row = words(rowOf('Portal 2'));
    expect(row).toContain('Portal 2');
    expect(row).toContain('1.0');
    expect(row).toContain(bytesSize(5 * 1024 ** 3));
    expect(row).toContain(msg('transfers.lanFromHost', { host: 'PC-1' }));
  });

  it('a shared game row says its title and size, and sits in the Sharing section', () => {
    shares.set([share({ sizeBytes: 3 * 1024 ** 3 })]);
    renderDom(Lan);

    expect(sectionOf('transfers.lanSharingHeading').contains(rowOf('Hades'))).toBe(true);
    expect(words(rowOf('Hades'))).toContain(bytesSize(3 * 1024 ** 3));
  });

  it('a game being prepared sits in the Sharing section with a bar named after it', () => {
    libraryGames.set([{ id: 'g2', title: 'Celeste' } as LibraryGame]);
    hashing.set(new Map([['g2', progress('g2', 50)]]));
    renderDom(Lan);

    const bar = within(sectionOf('transfers.lanSharingHeading')).getByRole('progressbar', { name: 'Celeste' });
    expect(bar.getAttribute('aria-valuenow')).toBe('25');
  });

  it('a receiving transfer says its title and how far it got, with a bar named after it', () => {
    transfers.set([transfer({ downloaded: 50, total: 200 })]);
    renderDom(Lan);

    const section = sectionOf('transfers.lanTransfersHeading');
    expect(within(section).getByRole('progressbar', { name: 'Celeste' }).getAttribute('aria-valuenow')).toBe('25');
    expect(words(rowOf('Celeste'))).toContain(msg('transfers.lanReceivingPercent', { percent: 25 }));
  });
});

describe('Lan pins: the actions', () => {
  it('Download asks Go for the offer by its infohash and its peer', async () => {
    offers.set([offer()]);
    const { user } = renderDom(Lan);

    await user.click(rowButton('Portal 2', 'transfers.lanDownload'));

    expect(receive).toHaveBeenCalledExactlyOnceWith('a'.repeat(40), 'p1');
  });

  it('Stop sharing asks Go to stop sharing the game by its id', async () => {
    shares.set([share()]);
    const { user } = renderDom(Lan);

    await user.click(rowButton('Hades', 'transfers.lanStopSharing'));

    expect(unshare).toHaveBeenCalledExactlyOnceWith('g1');
  });

  it('Cancel of a transfer that has received nothing asks Go to cancel it by its id', async () => {
    transfers.set([transfer({ downloaded: 0 })]);
    const { user } = renderDom(Lan);

    await user.click(rowButton('Celeste', 'transfers.lanCancelLabel'));

    expect(cancel).toHaveBeenCalledExactlyOnceWith('t1');
  });
});

describe('Lan pins: the empty sections', () => {
  it('nothing nearby is a titled empty state with what to check; nothing shared and nothing coming in say so', () => {
    renderDom(Lan);

    const nearby = sectionOf('transfers.lanNearbyHeading');
    expect(within(nearby).getByRole('heading', { level: 3, name: msg('transfers.lanEmptyTitle') })).toBeTruthy();
    expect(words(nearby)).toContain(msg('transfers.lanEmptyDescription'));
    expect(words(sectionOf('transfers.lanSharingHeading'))).toContain(msg('transfers.lanNoShares'));
    expect(words(sectionOf('transfers.lanTransfersHeading'))).toContain(msg('transfers.lanNoTransfers'));
  });

  it('with nothing nearby, announces Go turned away are summed up under the empty state', () => {
    lanStats.set(stats({ rejected: { rate_limited: 2 } }));
    renderDom(Lan);

    expect(words(sectionOf('transfers.lanNearbyHeading'))).toContain(
      msg('transfers.lanRejectedPrefix', { summary: `${msg('transfers.lanRejectRateLimited')}: 2` }),
    );
  });
});
