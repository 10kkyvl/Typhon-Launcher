import type { AuthState } from '../stores/user';
import type { CurrentUser } from './account';
import type { SelfUpdateStatus, UpdateChannel } from './selfupdate';

export function betaToggleShown(auth: AuthState, user: Pick<CurrentUser, 'tester'> | null): boolean {
  return auth === 'authenticated' && user?.tester === true;
}

export function isBeta(channel: UpdateChannel | string | undefined): boolean {
  return channel === 'beta';
}

export function stableOfferVersion(status: Pick<SelfUpdateStatus, 'stableOffer'>): string {
  return status.stableOffer?.trim() ?? '';
}

export function betaReportRoute(auth: AuthState): 'ticket' | 'logs' {
  return auth === 'authenticated' ? 'ticket' : 'logs';
}
