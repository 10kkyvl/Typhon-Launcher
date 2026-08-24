import { Service as DiscoveryService } from '../../../bindings/typhon/internal/discovery';
import { inWails } from './backend';

export interface DiscoveryProblem {
  path: string;
  reason: string;
}

export interface DiscoveryResult {
  roots: number;
  rootsSkipped: number;
  candidates: number;
  added: number;
  updated: number;
  known: number;
  skipped: number;
  errors: number;
  cancelled: boolean;
  problems?: DiscoveryProblem[];
}

export interface DiscoveryProgress {
  processed: number;
  total: number;
}

const unavailable = () => new Error('unavailable in browser');

export async function scanInstalledGames(): Promise<DiscoveryResult> {
  if (!inWails) throw unavailable();
  return (await DiscoveryService.Scan()) as unknown as DiscoveryResult;
}

export async function cancelScan(): Promise<void> {
  if (!inWails) throw unavailable();
  await DiscoveryService.CancelScan();
}

export async function isScanning(): Promise<boolean> {
  if (!inWails) return false;
  return await DiscoveryService.Scanning();
}
