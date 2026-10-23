import { describe, expect, it } from 'vitest';

describe('outcomeReason', () => {
  it.each([
    [
      'typhon:selfupdate.not_replaced: selfupdate: installer finished but left the launcher binary unchanged',
      'Установщик не заменил файлы',
    ],
    [
      'typhon:selfupdate.parent_still_running: selfupdate: launcher did not exit before the timeout',
      'Лаунчер не закрылся вовремя',
    ],
    ['typhon:selfupdate.hash_mismatch: downloaded hash differs from the manifest', 'повреждён'],
  ])('translates %s', async (raw, expected) => {
    const { outcomeReason } = await import('./selfupdateMessages');

    expect(outcomeReason({ version: '1.2.0', ok: false, error: raw, finishedAt: '' })).toContain(expected);
  });

  it('falls back to a translated generic reason, not the raw error, when the cause is unknown', async () => {
    const { outcomeReason } = await import('./selfupdateMessages');

    expect(outcomeReason({ version: '1.2.0', ok: false, error: 'something else', finishedAt: '' })).toBe(
      'При обновлении что-то пошло не так.',
    );
  });

  it('returns an empty string when there is no error', async () => {
    const { outcomeReason } = await import('./selfupdateMessages');

    expect(outcomeReason({ version: '1.2.0', ok: true, finishedAt: '' })).toBe('');
  });
});

describe('rollback reasons', () => {
  const codes = [
    'selfupdate.rollback_unsupported',
    'selfupdate.rollback_none',
    'selfupdate.rollback_broken',
    'selfupdate.rollback_hash',
    'selfupdate.rollback_state_hash',
    'selfupdate.rollback_not_older',
    'selfupdate.rollback_version_differs',
    'selfupdate.snapshot_version',
    'selfupdate.swap_hash',
  ];

  it.each(codes)('translates %s instead of the generic fallback', async (code) => {
    const { outcomeReason, rollbackReason } = await import('./selfupdateMessages');
    const generic = ['При обновлении что-то пошло не так.', 'При возврате на прежнюю версию что-то пошло не так.'];

    const fromInfo = rollbackReason({ available: false, version: '', reason: 'x', reasonCode: code });
    const fromOutcome = outcomeReason({
      kind: 'rolled_back',
      version: '1.1.0',
      ok: false,
      error: 'typhon:' + code + ': x',
      finishedAt: '',
    });

    expect(fromInfo).not.toBe('');
    expect(generic).not.toContain(fromInfo);
    expect(fromOutcome).toBe(fromInfo);
  });

  it('has no reason for an available rollback', async () => {
    const { rollbackReason } = await import('./selfupdateMessages');

    expect(rollbackReason({ available: true, version: '1.1.0', reason: '', reasonCode: '' })).toBe('');
  });

  it('falls back to a rollback wording, not the update one, for an unknown failed rollback', async () => {
    const { outcomeReason } = await import('./selfupdateMessages');

    expect(outcomeReason({ kind: 'rolled_back', version: '1.1.0', ok: false, error: 'strange', finishedAt: '' })).toBe(
      'При возврате на прежнюю версию что-то пошло не так.',
    );
  });
});

describe('statusReason', () => {
  it.each([
    [
      'download artifact: context deadline exceeded (Client.Timeout or context cancellation while reading body)',
      'не ответил вовремя',
    ],
    ['typhon:selfupdate.stalled: selfupdate: artifact download stalled: 1m0s', 'перестал отдавать данные'],
    ['typhon:selfupdate.bad_signature: manifest signature does not verify', 'Подпись обновления не совпала'],
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

  it('falls back to a translated generic reason, not the raw error, when neither the cause nor the stage is known', async () => {
    const { statusReason } = await import('./selfupdateMessages');

    expect(statusReason({ state: 'failed', currentVersion: '1.0.0', error: 'boom', errorCode: 'weird' })).toBe(
      'При обновлении что-то пошло не так.',
    );
  });

  it('returns an empty string when the status carries no error', async () => {
    const { statusReason } = await import('./selfupdateMessages');

    expect(statusReason({ state: 'idle', currentVersion: '1.0.0' })).toBe('');
  });
});

describe('updateReason', () => {
  it('translates a known failure', async () => {
    const { updateReason } = await import('./selfupdateMessages');

    expect(
      updateReason(new Error('typhon:selfupdate.busy: another update operation is in progress')),
    ).toBe(
      'Другая операция обновления уже идёт.',
    );
  });

  it('falls back to a translated generic reason, not the raw message', async () => {
    const { updateReason } = await import('./selfupdateMessages');

    expect(updateReason(new Error('boom'))).toBe('При обновлении что-то пошло не так.');
  });

  it('falls back to a translated generic reason for bare backend text without a code', async () => {
    const { updateReason } = await import('./selfupdateMessages');

    const bare = 'selfupdate: another update operation is in progress';
    expect(updateReason(new Error(bare))).toBe('При обновлении что-то пошло не так.');
  });
});
