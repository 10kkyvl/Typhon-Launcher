import { Service as SysreqService } from '../../../bindings/typhon/internal/sysreq';
import { inWails } from './backend';

export type SysreqVerdict = 'good' | 'minimum' | 'not_enough' | 'unknown';
export type SysreqReason = '' | 'no_requirements' | 'not_comparable' | 'partial';
export type SysreqKind = 'os' | 'cpu' | 'ram' | 'gpu' | 'vram' | 'directx' | 'storage';
export type SysreqStatus = 'ok' | 'below_recommended' | 'below_minimum' | 'unknown';

export interface SysreqHave {
  text: string;
  cores: number;
  bits: number;
  bytes: number;
  free: number | null;
  directX: number;
  installed: boolean;
}

export interface SysreqCheck {
  kind: SysreqKind;
  required: string;
  recommended: string;
  have: SysreqHave;
  status: SysreqStatus;
}

export interface SysreqReport {
  verdict: SysreqVerdict;
  reason: SysreqReason;
  checks: SysreqCheck[];
}

export async function checkSystemRequirements(gameId: string, installPath: string, installed = false): Promise<SysreqReport> {
  if (!inWails) return { verdict: 'unknown', reason: 'no_requirements', checks: [] };
  const report = await SysreqService.Check(gameId, installPath, installed);
  return { ...report, checks: report.checks ?? [] } as SysreqReport;
}
