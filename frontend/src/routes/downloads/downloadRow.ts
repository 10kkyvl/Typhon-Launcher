import type { Download } from '../../lib/services/downloads';
import type { GameArt } from '../../lib/services/metadata';
import type { Source } from '../../lib/services/sources';
import { msg } from '../../lib/i18n';

/** What kind of download a row of the screen names in a tag; '' for a download of no known kind. */
export function typeTag(d: Download): string {
  if (d.origin.platform) return msg('state.installTypeImage');
  if (d.origin.purpose === 'update') return msg('transfers.downloadsTypeUpdate');
  if (d.origin.purpose === 'repair') return msg('transfers.downloadsTypeRepair');
  if (d.origin.gameId || d.origin.releaseId) return msg('transfers.downloadsTypeGame');
  return '';
}

/** The name of the source the download came from; '' when it came from none of them. */
export function sourceTag(d: Download, sources: Source[]): string {
  return sources.find((s) => s.id === d.origin.sourceId)?.name ?? '';
}

/** The cover of the download's game, '' while there is none. */
export function coverOf(d: Download, art: Record<string, GameArt>): string {
  return (d.origin.gameId && art[d.origin.gameId]?.cover) || '';
}
