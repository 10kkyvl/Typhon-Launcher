import { describe, expect, it } from 'vitest';
import { locale } from '../i18n';
import { legalDocumentIds, legalTitle } from './legalMessages';

const cyrillic = /[а-яА-ЯёЁ]/;

describe('legalTitle', () => {
  it('translates every backend document id', () => {
    locale.set('en');
    const titles = legalDocumentIds.map((id) => legalTitle(id, 'Условия использования'));
    expect(titles.filter((title) => cyrillic.test(title))).toEqual([]);
    expect(titles).toEqual([
      'Terms of use',
      'Privacy policy',
      'Copyright and takedown requests',
      'Third-party licenses',
    ]);
  });

  it('keeps russian titles for the russian locale', () => {
    locale.set('ru');
    expect(legalTitle('terms', 'fallback')).toBe('Условия использования');
  });

  it('falls back to the backend title for an unknown id', () => {
    locale.set('en');
    expect(legalTitle('unknown', 'Backend title')).toBe('Backend title');
  });
});
