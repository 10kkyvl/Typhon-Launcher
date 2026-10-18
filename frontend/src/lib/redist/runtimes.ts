import type { PackageArch, RedistRuntime } from '../services/redist';

const ARCHES: PackageArch[] = ['x64', 'x86'];

function major(version: string): number {
  return Number.parseInt(version.split('.')[0] ?? '', 10);
}

export function modernVc(runtimes: RedistRuntime[], arch: PackageArch): boolean {
  return runtimes.some((runtime) => runtime.arch === arch && major(runtime.version) >= 14);
}

export function missingVcArches(runtimes: RedistRuntime[]): PackageArch[] {
  return ARCHES.filter((arch) => !modernVc(runtimes, arch));
}
