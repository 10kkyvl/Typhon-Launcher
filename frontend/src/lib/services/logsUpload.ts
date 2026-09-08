import { Service as DiagnosticsService } from '../../../bindings/typhon/internal/diagnostics';
import { inWails } from './backend';

export interface SendLogsResult {
  id: string;
  dropped: string[];
}

// sendLogs uploads exactly the bundle exportLogs() would save to disk (see
// internal/app/logs.go BuildLogUpload) — only ever on a direct button press,
// never automatically and never retried on failure.
export async function sendLogs(): Promise<SendLogsResult> {
  if (!inWails) throw new Error('unavailable in browser');
  const result = await DiagnosticsService.SendLogs();
  return { id: result.id, dropped: result.dropped ?? [] };
}
