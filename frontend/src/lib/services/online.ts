import { Service as OnlineService } from '../../../bindings/typhon/internal/online';
import { inWails } from './backend';

export const PRESENCE_STATUSES = ['online', 'away', 'busy', 'invisible'] as const;

export type PresenceStatus = (typeof PRESENCE_STATUSES)[number];

export const DEFAULT_PRESENCE: PresenceStatus = 'online';

export function toPresenceStatus(value: string): PresenceStatus {
  return (PRESENCE_STATUSES as readonly string[]).includes(value) ? (value as PresenceStatus) : DEFAULT_PRESENCE;
}

export async function status(): Promise<PresenceStatus> {
  if (!inWails) return DEFAULT_PRESENCE;
  return toPresenceStatus(await OnlineService.Status());
}

export async function setStatus(next: PresenceStatus): Promise<void> {
  if (!inWails) return;
  await OnlineService.SetStatus(next);
}
