import { Service as AccountSyncService } from '../../../bindings/typhon/internal/accountsync';
import { inWails } from './backend';

export class AccountSyncError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'AccountSyncError';
  }
}

function toAccountSyncError(err: unknown): AccountSyncError {
  if (err instanceof AccountSyncError) return err;
  return new AccountSyncError(err instanceof Error ? err.message : String(err));
}

const unavailable = () => new AccountSyncError('unavailable in browser');

export async function syncNow(): Promise<void> {
  if (!inWails) throw unavailable();
  try {
    await AccountSyncService.SyncNow();
  } catch (err) {
    throw toAccountSyncError(err);
  }
}

export async function forgetRemote(): Promise<void> {
  if (!inWails) throw unavailable();
  try {
    await AccountSyncService.ForgetRemote();
  } catch (err) {
    throw toAccountSyncError(err);
  }
}
