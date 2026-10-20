import { msg, type MessageKey } from '../i18n';
import { bytesSize } from '../utils/format';
import type { SysreqCheck, SysreqKind, SysreqReport, SysreqStatus, SysreqVerdict } from '../services/sysreq';

export type SysreqTone = 'success' | 'warning' | 'danger' | 'neutral';

export function verdictLabelKey(report: Pick<SysreqReport, 'verdict' | 'reason'>): MessageKey {
  switch (report.verdict) {
    case 'good':
      return 'sysreq.verdictGood';
    case 'minimum':
      return 'sysreq.verdictMinimum';
    case 'not_enough':
      return 'sysreq.verdictNotEnough';
    default:
      if (report.reason === 'no_requirements') return 'sysreq.verdictNoData';
      return report.reason === 'partial' ? 'sysreq.verdictPartial' : 'sysreq.verdictUnknown';
  }
}

export function verdictTone(verdict: SysreqVerdict): SysreqTone {
  switch (verdict) {
    case 'good':
      return 'success';
    case 'minimum':
      return 'warning';
    case 'not_enough':
      return 'danger';
    default:
      return 'neutral';
  }
}

export function statusTone(status: SysreqStatus): SysreqTone {
  switch (status) {
    case 'ok':
      return 'success';
    case 'below_recommended':
      return 'warning';
    case 'below_minimum':
      return 'danger';
    default:
      return 'neutral';
  }
}

export function statusLabelKey(status: SysreqStatus): MessageKey {
  switch (status) {
    case 'ok':
      return 'sysreq.statusOk';
    case 'below_recommended':
      return 'sysreq.statusBelowRecommended';
    case 'below_minimum':
      return 'sysreq.statusBelowMinimum';
    default:
      return 'sysreq.statusUnknown';
  }
}

const KINDS: Record<SysreqKind, MessageKey> = {
  os: 'sysreq.kindOs',
  cpu: 'sysreq.kindCpu',
  gpu: 'sysreq.kindGpu',
  ram: 'sysreq.kindRam',
  vram: 'sysreq.kindVram',
  directx: 'sysreq.kindDirectx',
  storage: 'sysreq.kindStorage',
};

export function kindLabelKey(kind: SysreqKind): MessageKey {
  return KINDS[kind];
}

const KIND_ORDER = Object.keys(KINDS);
const STATUS_RANK: Record<SysreqStatus, number> = { below_minimum: 0, below_recommended: 1, unknown: 2, ok: 3 };

export function sortChecks(checks: readonly SysreqCheck[]): SysreqCheck[] {
  return [...checks].sort(
    (a, b) => STATUS_RANK[a.status] - STATUS_RANK[b.status] || KIND_ORDER.indexOf(a.kind) - KIND_ORDER.indexOf(b.kind),
  );
}

export function cacheKey(gameId: string, installPath: string, revision = ''): string {
  return JSON.stringify([gameId, installPath, revision]);
}

export function haveText(check: SysreqCheck): string {
  const have = check.have;
  switch (check.kind) {
    case 'os':
      return have.text && have.bits ? msg('sysreq.haveOs', { os: have.text, bits: have.bits }) : have.text;
    case 'cpu':
      return have.text && have.cores ? msg('sysreq.haveCpu', { cpu: have.text, count: have.cores }) : have.text;
    case 'ram':
    case 'vram':
      return have.bytes > 0 ? bytesSize(have.bytes) : '';
    case 'gpu':
      return have.text;
    case 'directx':
      return have.directX > 0 ? msg('sysreq.haveDirectx', { version: have.directX }) : '';
    case 'storage':
      return have.free === null ? '' : msg('sysreq.haveFree', { size: bytesSize(have.free) });
  }
}
