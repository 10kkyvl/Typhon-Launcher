import { Service as RelocateService } from '../../../bindings/typhon/internal/relocate';
import { inWails } from './backend';

export type MoveScope = 'game' | 'library';

export type MoveStage =
  | 'prepare'
  | 'copy'
  | 'verify'
  | 'commit'
  | 'repoint'
  | 'cleanup'
  | 'done'
  | 'failed'
  | 'cancelled';

export interface MoveJob {
  id: string;
  scope: MoveScope;
  stage: MoveStage;
  gameId?: string;
  title: string;
  source: string;
  target: string;
  staging: string;
  renamed: boolean;
  totalBytes: number;
  copiedBytes: number;
  phase: string;
  currentFile: string;
  queue?: string[] | null;
  startedAt: string;
  updatedAt: string;
  error?: string;
}

const unavailable = () => new Error('unavailable in browser');

export async function listMoves(): Promise<MoveJob[]> {
  if (!inWails) return [];
  return ((await RelocateService.List()) ?? []) as unknown as MoveJob[];
}

export async function moveGame(gameId: string, target: string): Promise<MoveJob> {
  if (!inWails) throw unavailable();
  return (await RelocateService.MoveGame(gameId, target)) as unknown as MoveJob;
}

export async function moveLibrary(parent: string): Promise<MoveJob> {
  if (!inWails) throw unavailable();
  return (await RelocateService.MoveLibrary(parent)) as unknown as MoveJob;
}

export async function cancelMove(jobId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await RelocateService.Cancel(jobId);
}

export async function selectMoveTargetFolder(): Promise<string> {
  if (!inWails) return '';
  return await RelocateService.SelectTargetFolder();
}
