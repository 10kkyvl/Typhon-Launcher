import { errorCode, hasMessage, msg } from '../i18n';
import type { ExtlibSummary } from '../services/extlib';

export type ToastKind = 'success' | 'info' | 'danger';

export interface ImportReport {
  text: string;
  kind: ToastKind;
  details: string[];
}

export function providerName(provider: string): string {
  switch (provider) {
    case 'steam':
      return 'Steam';
    case 'gog':
      return 'GOG Galaxy';
    case 'epic':
      return 'Epic';
    default:
      return provider;
  }
}

export function summarizeImport(summary: ExtlibSummary): ImportReport {
  const providers = summary.providers ?? [];
  const details: string[] = [];
  for (const result of providers) {
    const name = providerName(result.provider);
    if (result.error) details.push(msg('settings.extlibDetailError', { provider: name, error: result.error }));
    for (const item of result.skipped ?? []) {
      details.push(msg('settings.extlibDetailSkipped', { provider: name, title: item.title, reason: item.reason }));
    }
  }
  for (const item of summary.metadataErrors ?? []) {
    details.push(msg('settings.extlibDetailMetadata', { error: item }));
  }

  const failed = providers.some((result) => result.error);
  if (providers.length > 0 && providers.every((result) => result.notInstalled)) {
    return { text: msg('settings.extlibNoneFound'), kind: 'info', details };
  }
  let text = msg('settings.extlibDone', { added: summary.added, updated: summary.updated });
  if (details.length > 0) text = `${text}. ${msg('settings.extlibHasDetails', { count: details.length })}`;
  return { text, kind: failed ? 'danger' : 'success', details };
}

export function importErrorText(err: unknown): string {
  const code = errorCode(err);
  if (hasMessage(code)) return msg(code);
  return msg('settings.extlibFailed');
}
