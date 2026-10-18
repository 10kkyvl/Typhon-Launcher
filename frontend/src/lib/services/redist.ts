import { Service as RedistService } from '../../../bindings/typhon/internal/redist';
import { inWails } from './backend';

export type PackageKind = 'vcredist' | 'directx' | 'dotnet' | 'other';
export type PackageArch = 'x86' | 'x64' | '';

export interface RedistPackage {
  name: string;
  path: string;
  kind: PackageKind;
  arch: PackageArch;
  version: string;
  sizeBytes: number;
}

export interface RedistRuntime {
  name: string;
  version: string;
  arch: PackageArch;
}

export interface RedistResult {
  exitCode: number;
  rebootRequired: boolean;
  alreadyInstalled: boolean;
}

export interface RedistLink {
  name: string;
  url: string;
  arch: PackageArch;
}

const unavailable = () => new Error('unavailable in browser');

export async function scanRedist(gameId: string): Promise<RedistPackage[]> {
  if (!inWails) throw unavailable();
  return ((await RedistService.Scan(gameId)) ?? []) as unknown as RedistPackage[];
}

export async function installedRuntimes(): Promise<RedistRuntime[]> {
  if (!inWails) throw unavailable();
  return ((await RedistService.InstalledRuntimes()) ?? []) as unknown as RedistRuntime[];
}

export async function installRedist(gameId: string, path: string): Promise<RedistResult> {
  if (!inWails) throw unavailable();
  return (await RedistService.Install(gameId, path)) as unknown as RedistResult;
}

export async function officialLinks(): Promise<RedistLink[]> {
  if (!inWails) throw unavailable();
  return ((await RedistService.OfficialLinks()) ?? []) as unknown as RedistLink[];
}
