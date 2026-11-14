import { get } from 'svelte/store';
import { describe, expect, it, vi } from 'vitest';
import { runSavesWork, savesInFlight } from './savebackup';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
vi.mock('../services/backend', () => ({ inWails: false }));

const by = Symbol('test');

describe('saves actions in flight', () => {
  it('runs one action per game and lane, refuses a second one meanwhile, and lets other lanes and games run', async () => {
    let finish!: () => void;
    const first = runSavesWork('g1', 'backup', { kind: 'create', by }, () => new Promise<void>((resolve) => (finish = resolve)));
    const second = vi.fn(async () => {});

    expect(await runSavesWork('g1', 'backup', { kind: 'delete', snapshot: 's1', by }, second)).toBe(false);
    expect(second).not.toHaveBeenCalled();
    expect(get(savesInFlight).get('g1')?.backup?.kind).toBe('create');
    expect(await runSavesWork('g1', 'cloud', { kind: 'sync', by }, async () => {})).toBe(true);
    expect(await runSavesWork('g2', 'backup', { kind: 'create', by }, async () => {})).toBe(true);

    finish();
    expect(await first).toBe(true);
    expect(get(savesInFlight).has('g1')).toBe(false);
  });

  it('frees the lane when the action fails, and the failure reaches the caller', async () => {
    await expect(runSavesWork('g1', 'cloud', { kind: 'pull', by }, async () => Promise.reject(new Error('typhon:cloud.network')))).rejects.toThrow('cloud.network');

    expect(get(savesInFlight).has('g1')).toBe(false);
  });
});
