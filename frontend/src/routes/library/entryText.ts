import { msg } from '../../lib/i18n';
import type { Narrowing } from '../../lib/library/empty';
import type { LibraryEntry } from '../../lib/library/entries';
import { statusLabels } from '../../lib/stores/downloads';
import { bytesSize } from '../../lib/utils/format';

export function entrySubtitle(entry: LibraryEntry): string {
  if (entry.kind === 'watched') return msg('games.watchedMeta');
  const bits: string[] = [];
  if (entry.kind === 'download') {
    if (entry.catalog?.releaseYear) bits.push(String(entry.catalog.releaseYear));
    if (entry.catalog?.developer) bits.push(entry.catalog.developer);
    if (entry.download) bits.push(statusLabels(entry.download.status));
  } else if (!entry.installed) {
    bits.push(msg('games.gameNotInstalledWord'));
  } else {
    if (entry.game?.version) bits.push(entry.game.version);
    if (entry.sizeBytes > 0) bits.push(bytesSize(entry.sizeBytes));
  }
  return bits.join(' · ');
}

export function entryMeta(entry: LibraryEntry): string {
  if (entry.sizeBytes > 0) return bytesSize(entry.sizeBytes);
  return entry.installed ? '' : msg('games.gameNotInstalledWord');
}

const narrowingKeys = {
  search: 'library.narrowSearch',
  filter: 'library.narrowFilter',
  source: 'library.narrowSource',
  status: 'library.narrowStatus',
  collection: 'library.narrowCollection',
} as const satisfies Record<Narrowing, string>;

export function narrowingText(by: readonly Narrowing[]): string {
  return msg('library.noMatchDescription', { what: by.map((item) => msg(narrowingKeys[item])).join(', ') });
}
