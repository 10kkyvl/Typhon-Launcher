import { describe, expect, it } from 'vitest';

describe('outcomeReason', () => {
  it.each([
    ['selfupdate: installer finished but left the launcher binary unchanged', 'Установщик не заменил файлы'],
    ['selfupdate: launcher did not exit before the timeout', 'Лаунчер не закрылся вовремя'],
    ['selfupdate: downloaded hash differs from the manifest', 'повреждён'],
  ])('translates %s', async (raw, expected) => {
    const { outcomeReason } = await import('./selfupdateMessages');

    expect(outcomeReason({ version: '1.2.0', ok: false, error: raw, finishedAt: '' })).toContain(expected);
  });

  it('falls back to the raw error it does not know', async () => {
    const { outcomeReason } = await import('./selfupdateMessages');

    expect(outcomeReason({ version: '1.2.0', ok: false, error: 'something else', finishedAt: '' })).toBe(
      'something else',
    );
  });

  it('returns an empty string when there is no error', async () => {
    const { outcomeReason } = await import('./selfupdateMessages');

    expect(outcomeReason({ version: '1.2.0', ok: true, finishedAt: '' })).toBe('');
  });
});

describe('statusReason', () => {
  it.each([
    [
      'download artifact: context deadline exceeded (Client.Timeout or context cancellation while reading body)',
      'не ответил вовремя',
    ],
    ['selfupdate: artifact download stalled: 1m0s', 'перестал отдавать данные'],
    ['selfupdate: manifest signature does not verify', 'Подпись обновления не совпала'],
    ['Get "https://api.example.com": dial tcp: lookup api.example.com: no such host', 'Проверьте интернет'],
  ])('translates %s', async (raw, expected) => {
    const { statusReason } = await import('./selfupdateMessages');

    expect(statusReason({ state: 'failed', currentVersion: '1.0.0', error: raw, errorCode: 'download' })).toContain(
      expected,
    );
  });

  it('falls back to the stage when the cause is unknown', async () => {
    const { statusReason } = await import('./selfupdateMessages');

    expect(statusReason({ state: 'failed', currentVersion: '1.0.0', error: 'boom', errorCode: 'download' })).toBe(
      'Не удалось загрузить обновление.',
    );
  });

  it('keeps the raw error when neither the cause nor the stage is known', async () => {
    const { statusReason } = await import('./selfupdateMessages');

    expect(statusReason({ state: 'failed', currentVersion: '1.0.0', error: 'boom', errorCode: 'weird' })).toBe('boom');
  });

  it('returns an empty string when the status carries no error', async () => {
    const { statusReason } = await import('./selfupdateMessages');

    expect(statusReason({ state: 'idle', currentVersion: '1.0.0' })).toBe('');
  });
});

describe('updateReason', () => {
  it('translates a known failure', async () => {
    const { updateReason } = await import('./selfupdateMessages');

    expect(updateReason(new Error('selfupdate: another update operation is in progress'))).toBe(
      'Другая операция обновления уже идёт.',
    );
  });

  it('falls back to the raw message', async () => {
    const { updateReason } = await import('./selfupdateMessages');

    expect(updateReason(new Error('boom'))).toBe('boom');
  });
});
