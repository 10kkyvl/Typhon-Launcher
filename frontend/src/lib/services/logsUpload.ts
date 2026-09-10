import { Service as DiagnosticsService } from '../../../bindings/typhon/internal/diagnostics';
import { Events } from '@wailsio/runtime';
import { inWails } from './backend';

export interface SendLogsResult {
  id: string;
  dropped: string[];
}

export type LogUploadStatus = {
  state: 'preparing' | 'sending' | 'waiting' | 'success' | 'error';
  sentBytes?: number;
  totalBytes?: number;
};

export function onLogUploadStatus(handler: (status: LogUploadStatus) => void): () => void {
  if (!inWails) return () => {};
  return Events.On('diagnostics:logs_status', (event) => {
    const data = event.data as Partial<LogUploadStatus>;
    if (
      data.state === 'preparing' ||
      data.state === 'sending' ||
      data.state === 'waiting' ||
      data.state === 'success' ||
      data.state === 'error'
    ) {
      handler({
        state: data.state,
        sentBytes: typeof data.sentBytes === 'number' ? data.sentBytes : undefined,
        totalBytes: typeof data.totalBytes === 'number' ? data.totalBytes : undefined,
      });
    }
  });
}

// sendLogs uploads exactly the bundle exportLogs() would save to disk (see
// internal/app/logs.go BuildLogUpload) — only ever on a direct button press,
// never automatically and never retried on failure.
export async function sendLogs(): Promise<SendLogsResult> {
  if (!inWails) throw new Error('unavailable in browser');
  const result = await DiagnosticsService.SendLogs();
  return { id: result.id, dropped: result.dropped ?? [] };
}
