import { Service as TelemetryLogService } from '../../../bindings/typhon/internal/telemetrylog';
import type { EntryView as RawEntry } from '../../../bindings/typhon/internal/telemetrylog/models';
import { inWails } from './backend';

export type SentDataKind = 'diagnostics' | 'usagestats' | '';

export interface SentDataEntry {
  kind: SentDataKind;
  endpoint: string;
  sentAt: string;
  payload: string;
  formatted: boolean;
}

const unavailable = () => new Error('unavailable in browser');

function toEntry(raw: RawEntry): SentDataEntry {
  return {
    kind: raw.Kind as SentDataKind,
    endpoint: raw.Endpoint,
    sentAt: raw.SentAt,
    payload: raw.Payload,
    formatted: raw.Formatted,
  };
}

export async function listSentData(): Promise<SentDataEntry[]> {
  if (!inWails) throw unavailable();
  const raw = (await TelemetryLogService.List()) ?? [];
  return raw.map(toEntry);
}
