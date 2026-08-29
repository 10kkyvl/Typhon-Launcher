import { describe, expect, it } from 'vitest';
import { accountSyncReason } from './accountSyncMessages';

describe('accountSyncReason', () => {
  it.each([
    ['accountsync: not authenticated', 'войдите заново'],
    ['push account sync: accountsync: settings revision conflict', 'изменились на другом устройстве'],
    ['accountsync: too many games in request', 'Слишком много игр'],
    ['accountsync: device limit reached', 'лимит устройств'],
    ['accountsync: sync already in progress', 'уже выполняется'],
    ['accountsync: service is not started', 'ещё не запущена'],
    ['fetch account sync snapshot: accountsync: network error: dial tcp: connect timeout', 'Нет связи'],
    ['delete account sync data: accountsync: server error, status 503', 'сейчас недоступен'],
    ['accountsync: request rejected (bad_field, field "games")', 'отклонил запрос'],
    ['unavailable in browser', 'только в desktop-сборке'],
  ])('translates %s', (raw, expected) => {
    expect(accountSyncReason(new Error(raw), 'запасной текст')).toContain(expected);
  });

  it('falls back when the error is not recognized', () => {
    expect(accountSyncReason(new Error('something else'), 'запасной текст')).toBe('запасной текст');
  });

  it('falls back for a non-Error value', () => {
    expect(accountSyncReason('boom', 'запасной текст')).toBe('запасной текст');
  });
});
