import { afterEach, expect, it } from 'vitest';
import { applyLanguage } from '../i18n';
import { sourceWarningText } from './sourceWarnings';
import { sourceErrorText } from './sourceErrors';

afterEach(() => applyLanguage('ru'));

it('translates persisted feed warnings with their counts and limits', () => {
  applyLanguage('en');
  expect(sourceWarningText('1234 записей: список URI обрезан до лимита 8'))
    .toBe('Link lists were limited to 8 per entry. Entries affected: 1,234.');
  expect(sourceWarningText('1 дублирующихся записей объединено')).toBe('Duplicate entries were merged: 1.');
  applyLanguage('ru');
  expect(sourceWarningText('1 записей пропущено из-за некорректного заголовка'))
    .toBe('Записи с некорректным заголовком пропущены: 1.');
});

it('preserves source-supplied warnings it does not recognize', () => {
  expect(sourceWarningText('Provider notice')).toBe('Provider notice');
});

it('keeps the HTTP status when translating an error from the Go feed reader', () => {
  applyLanguage('en');
  expect(sourceErrorText(new Error('typhon:sources.feed_bad_status: сервер вернул статус 403')))
    .toBe('The source server returned HTTP 403');
  expect(sourceErrorText('typhon:sources.feed_bad_status')).toBe('The server returned an error status');
});
