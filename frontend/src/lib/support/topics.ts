import { msg } from '../i18n';
import type { MessageKey } from '../i18n';
import { SUPPORT_TOPICS, type SupportStatus, type SupportTopic } from '../services/support';

const TOPIC_KEYS: Record<SupportTopic, MessageKey> = {
  install: 'tickets.topicInstall',
  launch: 'tickets.topicLaunch',
  account: 'tickets.topicAccount',
  launcher: 'tickets.topicLauncher',
  other: 'tickets.topicOther',
};

const LOGS_BY_DEFAULT: ReadonlySet<SupportTopic> = new Set<SupportTopic>(['install', 'launch', 'launcher']);

export function isTopic(value: unknown): value is SupportTopic {
  return typeof value === 'string' && (SUPPORT_TOPICS as readonly string[]).includes(value);
}

export function topicOptions(): { id: SupportTopic; label: string }[] {
  return SUPPORT_TOPICS.map((id) => ({ id, label: msg(TOPIC_KEYS[id]) }));
}

export function topicLabel(topic: string): string {
  return isTopic(topic) ? msg(TOPIC_KEYS[topic]) : '';
}

export function attachLogsByDefault(topic: SupportTopic): boolean {
  return LOGS_BY_DEFAULT.has(topic);
}

export function parseTopic(value: string | undefined): SupportTopic {
  return isTopic(value) ? value : 'install';
}

export function parseTicketNumber(value: string | undefined): number | null {
  if (!value || !/^\d+$/.test(value)) return null;
  const number = Number(value);
  return Number.isSafeInteger(number) && number > 0 ? number : null;
}

export interface StatusView {
  kind: 'accent' | 'warning' | 'success';
  label: string;
}

export function statusView(status: string): StatusView | null {
  const known = status as SupportStatus;
  switch (known) {
    case 'open':
      return { kind: 'accent', label: msg('tickets.statusOpen') };
    case 'waiting_user':
      return { kind: 'warning', label: msg('tickets.statusWaitingUser') };
    case 'resolved':
      return { kind: 'success', label: msg('tickets.statusResolved') };
    default:
      return null;
  }
}
