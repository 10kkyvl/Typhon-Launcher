import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Update, VerifyState } from '../services/updates';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../services/backend', () => ({ inWails: true }));

vi.mock('../services/updates', () => ({
  buildManifest: vi.fn(),
  cancelUpdate: vi.fn(),
  listUpdates: vi.fn(async () => []),
  prepareUpdatePlan: vi.fn(),
  repairGame: vi.fn(),
  rollbackUpdate: vi.fn(),
  startUpdate: vi.fn(),
  verifyGame: vi.fn(),
}));

function makeVerify(gameId: string, patch: Partial<VerifyState> = {}): VerifyState {
  return {
    gameId,
    method: 'manifest',
    running: false,
    repairing: false,
    progress: 1,
    processedBytes: 0,
    ratio: 1,
    totalBytes: 0,
    okBytes: 0,
    missingFiles: 0,
    corruptedPieces: 0,
    unreadableFiles: 0,
    repairable: false,
    checkedAt: '2026-09-08T00:00:00Z',
    ...patch,
  };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./updates');
  const i18n = await import('../i18n');
  const toastsModule = await import('./toasts');
  await store.initUpdates();
  return { store, i18n, toastsModule };
}

function makeUpdate(patch: Partial<Update> = {}): Update {
  return {
    gameId: 'g1',
    title: 'Game One',
    state: 'update_failed',
    availability: {
      available: false,
      kind: 'none',
      gameId: 'g1',
      installedReleaseId: '',
      targetReleaseId: '',
      installedVersion: '',
      targetVersion: '',
      confidence: 0,
      strategy: '',
      estimatedDownloadBytes: 0,
      requiresFullInstall: false,
      patchCount: 0,
      targetSize: 0,
    },
    planning: false,
    progress: 0,
    canRollback: false,
    checkedAt: '2026-09-08T00:00:00Z',
    ...patch,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('verifications store', () => {
  it('does not grow without bound as more and more games get verified over a session', async () => {
    const { store } = await load();

    for (let i = 0; i < 200; i++) {
      handlers['verify:completed']({ data: makeVerify(`game-${i}`) });
    }

    expect(Object.keys(get(store.verifications)).length).toBeLessThan(200);
  });

  it('never evicts a verification that is still running, however many other games finish afterwards', async () => {
    const { store } = await load();

    handlers['verify:started']({ data: makeVerify('active', { running: true, progress: 0.4 }) });

    for (let i = 0; i < 200; i++) {
      handlers['verify:completed']({ data: makeVerify(`game-${i}`) });
    }

    expect(get(store.verifications).active?.running).toBe(true);
  });
});

describe('update error toasts', () => {
  it('translates a known backend error code into the active locale instead of showing the raw message', async () => {
    const { store, i18n, toastsModule } = await load();
    i18n.locale.set('en');

    handlers['update:failed']({
      data: makeUpdate({ error: 'typhon:updates.update_failed: не удалось применить обновление' }),
    });

    const messages = get(toastsModule.toasts).map((t) => t.message);
    expect(messages).toContain('Failed to update "Game One": Failed to apply the update');
    void store;
  });

  it('shows a translated generic fallback, not the raw backend message, when the error code is unknown', async () => {
    const { store, i18n, toastsModule } = await load();
    i18n.locale.set('en');

    handlers['update:failed']({ data: makeUpdate({ error: 'connection reset by peer' }) });

    const messages = get(toastsModule.toasts).map((t) => t.message);
    expect(messages).toContain('Failed to update "Game One": Could not complete the update');
    void store;
  });
});
