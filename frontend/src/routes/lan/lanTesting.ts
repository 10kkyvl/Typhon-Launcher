import { screen, within } from '@testing-library/svelte';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import type { HashProgress, Offer, Share, Stats, Transfer } from '../../lib/services/lan';
import { hashing, lanLoaded, lanLoadError, lanStats, offers, peers, shares, transfers } from '../../lib/stores/lan';
import { libraryGames } from '../../lib/stores/library';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';

// Fixtures of the LAN screen's dom tests. Each test file mocks the services itself (vi.mock is hoisted
// per file); this holds the records and the queries they share.

export function offer(patch: Partial<Offer> = {}): Offer {
  return {
    peerId: 'p1',
    host: 'PC-1',
    addr: '192.168.1.10',
    port: 42817,
    gameId: 'remote-g1',
    title: 'Portal 2',
    version: '1.0',
    exe: 'portal2.exe',
    sizeBytes: 2048,
    infoHash: 'a'.repeat(40),
    lastSeen: '',
    ...patch,
  };
}

export function share(patch: Partial<Share> = {}): Share {
  return {
    gameId: 'g1',
    title: 'Hades',
    version: '1.0',
    exe: 'hades.exe',
    root: 'C:/games/hades',
    infoHash: 'b'.repeat(40),
    sizeBytes: 4096,
    fingerprint: 'fp',
    builtAt: '',
    enabled: true,
    ...patch,
  };
}

export function transfer(patch: Partial<Transfer> = {}): Transfer {
  return {
    id: 't1',
    infoHash: 'c'.repeat(40),
    peerId: 'p1',
    gameId: 'remote-g2',
    title: 'Celeste',
    downloaded: 0,
    total: 200,
    status: 'receiving',
    startedAt: '',
    updatedAt: '',
    ...patch,
  };
}

export function progress(gameId: string, processedBytes: number): HashProgress {
  return { gameId, processedBytes, totalBytes: 200, currentFile: '', done: false };
}

export function stats(patch: Partial<Stats> = {}): Stats {
  return { announcesSent: 0, announcesReceived: 0, rejected: null, peersKnown: 0, offersKnown: 0, sharesActive: 0, ...patch };
}

/** LAN sharing as the settings hold it. */
export function sharingOn(on = true): void {
  settings.set({ lanSharing: on } as Settings);
}

/** What the screen shows once its stores hold a first read; LAN sharing is on unless `sharing` says otherwise. */
export function ready(sharing = true): void {
  sharingOn(sharing);
  lanLoaded.set(true);
}

/** The stores the screen reads, back as they start. */
export function resetScreen(): void {
  offers.set([]);
  shares.set([]);
  transfers.set([]);
  peers.set([]);
  hashing.set(new Map());
  lanStats.set(stats());
  lanLoaded.set(false);
  lanLoadError.set(null);
  libraryGames.set([]);
  settings.set(null);
  toasts.set([]);
}

const escaped = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

export const words = (el: Element | null | undefined) => (el?.textContent ?? '').replace(/\s+/g, ' ').trim();

/** The section named by the heading that starts with `key`'s words. */
export function sectionOf(key: 'transfers.lanNearbyHeading' | 'transfers.lanSharingHeading' | 'transfers.lanTransfersHeading'): HTMLElement {
  const heading = screen.getByRole('heading', { level: 2, name: new RegExp(`^${escaped(msg(key))}(\\s|$)`) });
  const section = heading.closest('section');
  if (!section) throw new Error(`no section for ${key}`);
  return section;
}

/** The row that names `title`. */
export function rowOf(title: string): HTMLElement {
  const row = screen.getByText(title).closest<HTMLElement>('[data-lan-row], .row');
  if (!row) throw new Error(`no row for ${title}`);
  return row;
}

/** The button of `title`'s row whose name starts with `key`'s words. */
export function rowButton(title: string, key: 'transfers.lanDownload' | 'transfers.lanStopSharing' | 'transfers.lanCancelLabel'): HTMLElement {
  return within(rowOf(title)).getByRole('button', { name: new RegExp(`^${escaped(msg(key))}`) });
}
