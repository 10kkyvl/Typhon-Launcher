import { describe, expect, it } from 'vitest';
import { LatestRequestGate } from './latestRequest';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('LatestRequestGate', () => {
  it('ignores an older response and keeps the newest result', async () => {
    const gate = new LatestRequestGate();
    const old = deferred<string>();
    const current = deferred<string>();
    const oldTicket = gate.begin();
    const oldRead = gate.settle(oldTicket, old.promise);
    const currentTicket = gate.begin();
    const currentRead = gate.settle(currentTicket, current.promise);

    old.resolve('stale page');
    current.resolve('current page');
    await expect(oldRead).resolves.toEqual({ kind: 'stale' });
    await expect(currentRead).resolves.toEqual({ kind: 'value', value: 'current page' });
  });

  it('returns a current failure so the page can offer retry, then accepts a retry', async () => {
    const gate = new LatestRequestGate();
    const first = gate.begin();
    const error = new Error('offline');
    await expect(gate.settle(first, Promise.reject(error))).resolves.toEqual({ kind: 'error', error });

    const retry = gate.begin();
    await expect(gate.settle(retry, Promise.resolve('recovered'))).resolves.toEqual({ kind: 'value', value: 'recovered' });
  });

  it('invalidates a pending read when its dialog or page is closed', async () => {
    const gate = new LatestRequestGate();
    const pending = deferred<string>();
    const ticket = gate.begin();
    const result = gate.settle(ticket, pending.promise);
    gate.invalidate();
    pending.reject(new Error('cancelled'));

    await expect(result).resolves.toEqual({ kind: 'stale' });
  });

  it('aborts the previous request signal when a new one begins', () => {
    const gate = new LatestRequestGate();
    const first = gate.beginRequest();
    expect(first.signal.aborted).toBe(false);

    const second = gate.beginRequest();

    expect(first.signal.aborted).toBe(true);
    expect(second.signal.aborted).toBe(false);
    expect(second.ticket).toBe(first.ticket + 1);
  });

  it('aborts a pending request signal on invalidate', () => {
    const gate = new LatestRequestGate();
    const { signal } = gate.beginRequest();
    gate.invalidate();

    expect(signal.aborted).toBe(true);
  });
});
