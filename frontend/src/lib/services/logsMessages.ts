import { msg } from '../i18n';

function reasons(): [string, string][] {
  return [
    ['файлы журнала не найдены', msg('state.logsReasonNotFound')],
    ['downloads folder', msg('state.logsReasonNoDownloadsFolder')],
    ['config dir', msg('state.logsReasonNoConfigDir')],
    ['Access is denied', msg('state.logsReasonAccessDenied')],
    ['permission denied', msg('state.logsReasonAccessDenied')],
    ['not enough space', msg('state.logsReasonNoSpace')],
    ['no space left', msg('state.logsReasonNoSpace')],
  ];
}

export function logsReason(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  const known = reasons().find(([marker]) => raw.includes(marker));
  return known ? known[1] : msg('state.logsReasonGeneric');
}
