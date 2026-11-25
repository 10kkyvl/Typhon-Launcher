import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// The screen reads the offers again while it is shown: Go answers what the test set.
vi.mock('../../lib/services/lan', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/lan')>()),
  getOffers: vi.fn(async () => get((await import('../../lib/stores/lan')).offers)),
}));

import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { hashing, offers, shares, transfers } from '../../lib/stores/lan';
import { libraryGames } from '../../lib/stores/library';
import { route } from '../../lib/stores/router';
import { ruleBody, styleOf } from '../../lib/testing/css';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import Lan from './Lan.svelte';
import page from './Lan.svelte?raw';
import { offer, progress, ready, resetScreen, rowOf, sectionOf, share, transfer, words } from './lanTesting';

beforeEach(() => ready());
afterEach(() => resetScreen());

const game = (id: string, title: string) => ({ id, title }) as LibraryGame;
const SECTIONS = ['transfers.lanNearbyHeading', 'transfers.lanSharingHeading', 'transfers.lanTransfersHeading'] as const;

describe('Lan: sections on the base parts', () => {
  it('each section is named by its heading, which says its count and takes focus from script', () => {
    offers.set([offer()]);
    renderDom(Lan);

    for (const key of SECTIONS) {
      const section = sectionOf(key);
      const heading = document.getElementById(section.getAttribute('aria-labelledby') ?? '');
      expect(heading?.tagName, key).toBe('H2');
      expect(heading?.getAttribute('tabindex'), key).toBe('-1');
      expect(heading?.querySelector('.count'), key).not.toBeNull();
    }
  });

  it('rows are items of a list, one per offer, shared game and transfer', () => {
    offers.set([offer()]);
    libraryGames.set([game('g2', 'Celeste')]);
    hashing.set(new Map([['g2', progress('g2', 50)]]));
    shares.set([share()]);
    transfers.set([transfer({ title: 'Inside' })]);
    renderDom(Lan);

    expect(within(sectionOf('transfers.lanNearbyHeading')).getAllByRole('listitem')).toHaveLength(1);
    expect(within(sectionOf('transfers.lanSharingHeading')).getAllByRole('listitem')).toHaveLength(2);
    expect(within(sectionOf('transfers.lanTransfersHeading')).getAllByRole('listitem')).toHaveLength(1);
  });
});

describe('Lan: the game a row names', () => {
  it('a shared game the library holds is a link to its page', async () => {
    libraryGames.set([game('g1', 'Hades')]);
    shares.set([share()]);
    const { user } = renderDom(Lan);

    await user.click(within(rowOf('Hades')).getByRole('button', { name: 'Hades' }));

    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
  });

  it('a game being prepared that the library holds is a link to its page', () => {
    libraryGames.set([game('g2', 'Celeste')]);
    hashing.set(new Map([['g2', progress('g2', 50)]]));
    renderDom(Lan);

    expect(within(sectionOf('transfers.lanSharingHeading')).getByRole('button', { name: 'Celeste' })).toBeTruthy();
  });

  it('a shared game the library no longer holds is plain text, not a control that goes nowhere', () => {
    shares.set([share()]);
    renderDom(Lan);

    expect(within(rowOf('Hades')).queryByRole('button', { name: 'Hades' })).toBeNull();
  });

  // An offer's game id is the other computer's library id: it names no game of this library.
  it('an offer and a transfer name their game as plain text', () => {
    libraryGames.set([game('remote-g1', 'Portal 2'), game('remote-g2', 'Celeste')]);
    offers.set([offer()]);
    transfers.set([transfer()]);
    renderDom(Lan);

    expect(within(rowOf('Portal 2')).queryByRole('button', { name: 'Portal 2' })).toBeNull();
    expect(within(rowOf('Celeste')).queryByRole('button', { name: 'Celeste' })).toBeNull();
  });
});

describe('Lan: offers by computer', () => {
  it('with one computer nearby, the offers form one list and each says where it comes from', () => {
    offers.set([offer(), offer({ title: 'Inside', infoHash: 'e'.repeat(40) })]);
    renderDom(Lan);

    const nearby = sectionOf('transfers.lanNearbyHeading');
    expect(within(nearby).queryAllByRole('heading', { level: 3 })).toEqual([]);
    expect(words(rowOf('Inside'))).toContain(msg('transfers.lanFromHost', { host: 'PC-1' }));
  });

  it('with more than one computer, the offers are grouped under each computer with its count', () => {
    offers.set([
      offer(),
      offer({ peerId: 'p2', host: 'PC-2', title: 'Inside', infoHash: 'e'.repeat(40) }),
      offer({ peerId: 'p2', host: 'PC-2', title: 'Limbo', infoHash: 'f'.repeat(40) }),
    ]);
    renderDom(Lan);

    const nearby = sectionOf('transfers.lanNearbyHeading');
    const groups = within(nearby).getAllByRole('heading', { level: 3 });
    expect(groups.map(words)).toEqual([`${msg('transfers.lanHostHeading', { host: 'PC-1' })} 1`, `${msg('transfers.lanHostHeading', { host: 'PC-2' })} 2`]);
    const second = groups[1].closest('section') as HTMLElement;
    expect(within(second).getAllByRole('listitem').map((row) => within(row).getByText(/Inside|Limbo/).textContent)).toEqual(['Inside', 'Limbo']);
    expect(words(rowOf('Inside'))).not.toContain(msg('transfers.lanFromHost', { host: 'PC-2' }));
  });

  it('two computers that share a name are two groups', () => {
    offers.set([offer(), offer({ peerId: 'p2', title: 'Inside', infoHash: 'e'.repeat(40) })]);
    renderDom(Lan);

    expect(within(sectionOf('transfers.lanNearbyHeading')).getAllByRole('heading', { level: 3 })).toHaveLength(2);
  });
});

describe('Lan: progress in words', () => {
  it('a transfer’s bar says how much of how much arrived', () => {
    transfers.set([transfer({ downloaded: 50 * 1024 ** 2, total: 200 * 1024 ** 2 })]);
    renderDom(Lan);

    const bar = screen.getByRole('progressbar', { name: 'Celeste' });
    expect(bar.getAttribute('aria-valuenow')).toBe('25');
    expect(bar.getAttribute('aria-valuetext')).toBe(
      msg('transfers.lanTransferProgress', { done: bytesSize(50 * 1024 ** 2), total: bytesSize(200 * 1024 ** 2), percent: 25 }),
    );
  });

  it('a transfer whose size is not known yet claims no amount', () => {
    transfers.set([transfer({ downloaded: 0, total: 0 })]);
    renderDom(Lan);

    const bar = screen.getByRole('progressbar', { name: 'Celeste' });
    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
    expect(bar.getAttribute('aria-busy')).toBe('true');
    expect(words(rowOf('Celeste'))).toContain(msg('transfers.lanReceiving'));
  });

  it('a game being prepared says its share in words', () => {
    libraryGames.set([game('g2', 'Celeste')]);
    hashing.set(new Map([['g2', progress('g2', 50)]]));
    renderDom(Lan);

    expect(screen.getByRole('progressbar', { name: 'Celeste' }).getAttribute('aria-valuetext')).toBe(msg('transfers.lanPreparingPercent', { percent: 25 }));
  });
});

describe('Lan: actions named after their game', () => {
  it('Download, Stop sharing and Cancel each say which game they act on', () => {
    offers.set([offer()]);
    shares.set([share()]);
    transfers.set([transfer()]);
    renderDom(Lan);

    expect(screen.getByRole('button', { name: msg('transfers.lanDownloadFor', { title: 'Portal 2', host: 'PC-1' }) })).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('transfers.lanStopSharingFor', { title: 'Hades' }) })).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('transfers.lanCancelFor', { title: 'Celeste' }) })).toBeTruthy();
  });

  it('the page writes no native hint, no raw button and no spacing off the scale', () => {
    offers.set([offer()]);
    shares.set([share()]);
    transfers.set([transfer()]);
    const { container } = renderDom(Lan);

    expect(container.querySelectorAll('[title]')).toHaveLength(0);
    expect(page).not.toMatch(/<button\b/);
    const style = page.slice(page.indexOf('<style>'));
    expect(style.match(/(?:gap|margin[\w-]*|padding[\w-]*):[^;]*\b\d*\.?\d+(?:rem|px)\b[^;]*;/g) ?? []).toEqual([]);
  });

  // jsdom lays nothing out: what can be read here is that nothing refuses to shrink at the smallest
  // window (1100 px). The frame itself is for a real browser.
  it('lets a row’s words wrap and the figures move to a second line rather than widen the page', () => {
    const css = styleOf(page, 'Lan');

    expect(ruleBody(css, '.stats')['flex-wrap']).toBe('wrap');
    expect(ruleBody(css, '.main')['min-width']).toBe('0');
    expect(ruleBody(css, '.title')['overflow-wrap']).toBe('anywhere');
    expect(ruleBody(css, '.title')['white-space']).toBeUndefined();
  });
});
