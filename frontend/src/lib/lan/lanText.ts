import { msg } from '../i18n';
import { sourceErrorText } from '../sources/sourceErrors';
import { progressPercent } from '../utils/format';
import type { Stats, Transfer } from '../services/lan';
import type { LibraryGame } from '../services/library';
import type { MessageKey } from '../i18n';

const REJECT_KEYS: Record<string, MessageKey> = {
  too_large: 'transfers.lanRejectTooLarge',
  bad_source_addr: 'transfers.lanRejectBadSourceAddr',
  bad_json: 'transfers.lanRejectBadJson',
  bad_version: 'transfers.lanRejectBadVersion',
  bad_id: 'transfers.lanRejectBadId',
  own_id: 'transfers.lanRejectOwnId',
  bad_host: 'transfers.lanRejectBadHost',
  bad_port: 'transfers.lanRejectBadPort',
  bad_infohash: 'transfers.lanRejectBadInfohash',
  bad_title: 'transfers.lanRejectBadTitle',
  bad_version_field: 'transfers.lanRejectBadVersionField',
  bad_gameid: 'transfers.lanRejectBadGameId',
  bad_size: 'transfers.lanRejectBadSize',
  bad_exe: 'transfers.lanRejectBadExe',
  bad_ts: 'transfers.lanRejectBadTs',
  capacity: 'transfers.lanRejectCapacity',
  rate_limited: 'transfers.lanRejectRateLimited',
  unknown: 'transfers.lanRejectUnknown',
};

function rejectedLabel(key: string): string | undefined {
  const messageKey = REJECT_KEYS[key];
  return messageKey ? msg(messageKey) : undefined;
}

/** The library title of a game being prepared for sharing; one the library no longer lists is "Game". */
export function sharedGameTitle(gameId: string, games: readonly Pick<LibraryGame, 'id' | 'title'>[]): string {
  return games.find((game) => game.id === gameId)?.title || msg('ui.game');
}

export function transferLabel(transfer: Transfer): string {
  switch (transfer.status) {
    case 'receiving': {
      if (transfer.total <= 0) return msg('transfers.lanReceiving');
      const pct = progressPercent(transfer.downloaded / transfer.total);
      return msg('transfers.lanReceivingPercent', { percent: pct });
    }
    case 'completed':
      return msg('transfers.lanReceived');
    case 'failed':
      return msg('transfers.lanTransferFailed', {
        error: sourceErrorText(transfer.error, msg('transfers.lanUnknownError')),
      });
    case 'cancelled':
      return msg('transfers.lanCancelled');
    default:
      return '';
  }
}

export function rejectedSummary(stats: Stats): string {
  if (!stats.rejected) return '';
  const entries = Object.entries(stats.rejected).filter(([, count]) => (count ?? 0) > 0);
  if (entries.length === 0) return '';
  entries.sort((a, b) => (b[1] ?? 0) - (a[1] ?? 0));
  return entries.map(([key, count]) => `${rejectedLabel(key) ?? key}: ${count}`).join(' · ');
}
