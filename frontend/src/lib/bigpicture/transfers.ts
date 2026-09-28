import { get, writable, type Writable } from 'svelte/store';
import type { DownloadStatus } from '../services/downloads';
import type { InstallStatus } from '../services/install';

export type TransferJourneyStage =
  | 'downloading'
  | 'download-error'
  | 'install-ready'
  | 'install-progress'
  | 'choose-executable'
  | 'install-error'
  | 'installed'
  | 'library';

const activeInstallStatuses: InstallStatus[] = ['pending', 'preparing', 'installing', 'extracting', 'verifying'];

/** Describes the user-facing handoff from a completed transfer to its library game. */
export function transferJourneyStage(
  downloadStatus: DownloadStatus | null | undefined,
  installStatus?: InstallStatus | null,
  gameId = '',
): TransferJourneyStage {
  if (installStatus === 'completed') return gameId ? 'library' : 'installed';
  if (installStatus === 'waiting_for_user') return 'choose-executable';
  if (installStatus && activeInstallStatuses.includes(installStatus)) return 'install-progress';
  if (installStatus === 'failed' || installStatus === 'cancelled' || installStatus === 'interrupted') return 'install-error';
  if (downloadStatus === 'failed') return 'download-error';
  if (downloadStatus && downloadStatus !== 'completed') return 'downloading';
  if (downloadStatus === 'completed' && !installStatus) return 'install-ready';
  return 'install-ready';
}

export interface TransferActionSnapshot {
  pending: string[];
  errors: Record<string, string>;
}

export interface TransferActionRunner {
  state: Writable<TransferActionSnapshot>;
  run(key: string, action: () => Promise<unknown>): Promise<boolean>;
  isPending(key: string): boolean;
  clear(prefix: string): void;
}

function defaultErrorText(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return String(error || 'Operation failed');
}

/** Tracks one in-flight action per key and retains failures for inline retry UI. */
export function createTransferActionRunner(errorText = defaultErrorText): TransferActionRunner {
  const state = writable<TransferActionSnapshot>({ pending: [], errors: {} });

  return {
    state,
    isPending(key) {
      return get(state).pending.includes(key);
    },
    clear(prefix) {
      state.update((current) => ({
        pending: current.pending,
        errors: Object.fromEntries(Object.entries(current.errors).filter(([key]) => !key.startsWith(prefix))),
      }));
    },
    async run(key, action) {
      const before = get(state);
      if (before.pending.includes(key)) return false;
      const errors = { ...before.errors };
      delete errors[key];
      state.set({ pending: [...before.pending, key], errors });
      try {
        await action();
        return true;
      } catch (error) {
        state.update((current) => ({
          pending: current.pending,
          errors: { ...current.errors, [key]: errorText(error) },
        }));
        return false;
      } finally {
        state.update((current) => ({
          pending: current.pending.filter((entry) => entry !== key),
          errors: current.errors,
        }));
      }
    },
  };
}
