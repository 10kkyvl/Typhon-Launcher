import { Events } from '@wailsio/runtime';
import { Service as WrappersService } from '../../../bindings/typhon/internal/wrappers';
import { inWails } from './backend';

export type WrapperId = 'dgvoodoo' | 'dxvk';
export type WrapperArch = 'x86' | 'x64' | '';
export type WrapperStage = 'download' | 'extract' | 'apply';

export interface WrapperApplied {
  wrapper: WrapperId;
  version: string;
  preset: string;
  files: string[];
  appliedAt: string;
}

export interface WrapperProblem {
  code: string;
  file?: string;
}

export interface WrapperInfo {
  supported: boolean;
  arch: WrapperArch;
  imports: string[];
  suggested: WrapperId | '';
  active: WrapperApplied | null;
  problem: WrapperProblem | null;
}

export interface WrapperProgress {
  gameId: string;
  stage: WrapperStage;
  done: number;
  total: number;
}

const unavailable = () => new Error('unavailable in browser');

export async function inspectWrapper(gameId: string): Promise<WrapperInfo> {
  if (!inWails) return { supported: false, arch: '', imports: [], suggested: '', active: null, problem: null };
  const raw = (await WrappersService.Inspect(gameId)) as unknown as Partial<WrapperInfo> & {
    active?: Partial<WrapperApplied> | null;
  };
  return {
    supported: Boolean(raw.supported),
    arch: raw.arch ?? '',
    imports: raw.imports ?? [],
    suggested: raw.suggested ?? '',
    active: raw.active ? ({ ...raw.active, files: raw.active.files ?? [] } as WrapperApplied) : null,
    problem: raw.problem ?? null,
  };
}

export async function enableWrapper(gameId: string, wrapper: WrapperId, preset: string): Promise<void> {
  if (!inWails) throw unavailable();
  await WrappersService.Enable(gameId, wrapper, preset);
}

export async function disableWrapper(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await WrappersService.Disable(gameId);
}

export function onWrapperProgress(handler: (event: WrapperProgress) => void): () => void {
  if (!inWails) return () => {};
  return Events.On('wrappers:progress', (event) => handler(event.data as WrapperProgress));
}
