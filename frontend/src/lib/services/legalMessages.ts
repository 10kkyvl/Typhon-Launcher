import { msg, type MessageKey } from '../i18n';

const titleKeys: Record<string, MessageKey> = {
  terms: 'settings.aboutLegalDocTerms',
  privacy: 'settings.aboutLegalDocPrivacy',
  copyright: 'settings.aboutLegalDocCopyright',
  'third-party': 'settings.aboutLegalDocThirdParty',
};

// Идентификаторы приходят из internal/legal.Required — при добавлении документа
// туда нужен новый ключ каталога, иначе заголовок останется на языке бэкенда.
export const legalDocumentIds = Object.keys(titleKeys);

export function legalTitle(id: string, fallback: string): string {
  const key = titleKeys[id];
  return key ? msg(key) : fallback;
}
