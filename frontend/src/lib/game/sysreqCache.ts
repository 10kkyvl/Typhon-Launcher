import type { SysreqReport } from '../services/sysreq';

interface Entry {
  report: Promise<SysreqReport>;
  at: number;
}

export const REPORT_TTL_MS = 60_000;
export const REPORT_LIMIT = 20;

export function createReportCache(now: () => number = Date.now, ttl = REPORT_TTL_MS, limit = REPORT_LIMIT) {
  const entries = new Map<string, Entry>();

  function drop(key: string, report: Promise<SysreqReport>) {
    if (entries.get(key)?.report === report) entries.delete(key);
  }

  return {
    get size() {
      return entries.size;
    },
    fetch(key: string, load: () => Promise<SysreqReport>, fresh = false): Promise<SysreqReport> {
      const found = entries.get(key);
      if (found && !fresh && now() - found.at <= ttl) return found.report;
      entries.delete(key);
      const report = load();
      entries.set(key, { report, at: now() });
      while (entries.size > limit) {
        const oldest = entries.keys().next().value as string;
        entries.delete(oldest);
      }
      report.then(
        (value) => {
          if (value.verdict === 'unknown') drop(key, report);
        },
        () => drop(key, report),
      );
      return report;
    },
  };
}
