import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { createTransferActionRunner, transferJourneyStage } from './transfers';

describe('Big Picture transfer actions', () => {
  it('keeps an action pending once, exposes its failure, then clears it on retry', async () => {
    const actions = createTransferActionRunner((error) => error instanceof Error ? error.message : 'unknown');
    let rejectAction!: (reason: Error) => void;
    const first = actions.run('download:fixture:resume', () => new Promise<void>((_resolve, reject) => {
      rejectAction = reject;
    }));

    expect(actions.isPending('download:fixture:resume')).toBe(true);
    expect(await actions.run('download:fixture:resume', async () => undefined)).toBe(false);
    rejectAction(new Error('disk unavailable'));
    expect(await first).toBe(false);
    expect(actions.isPending('download:fixture:resume')).toBe(false);
    expect(get(actions.state).errors['download:fixture:resume']).toBe('disk unavailable');

    expect(await actions.run('download:fixture:resume', async () => undefined)).toBe(true);
    expect(get(actions.state).errors['download:fixture:resume']).toBeUndefined();
  });
});

describe('Big Picture download to library handoff', () => {
  it('moves through download, installer, and the registered library game', () => {
    expect(transferJourneyStage('failed')).toBe('download-error');
    expect(transferJourneyStage('completed')).toBe('install-ready');
    expect(transferJourneyStage('completed', 'installing')).toBe('install-progress');
    expect(transferJourneyStage('completed', 'waiting_for_user')).toBe('choose-executable');
    expect(transferJourneyStage('completed', 'failed')).toBe('install-error');
    expect(transferJourneyStage('completed', 'completed')).toBe('installed');
    expect(transferJourneyStage('completed', 'completed', 'fixture-game')).toBe('library');
  });

  it('keeps installation progress and the registered game when the download record is gone', () => {
    expect(transferJourneyStage(undefined, 'verifying')).toBe('install-progress');
    expect(transferJourneyStage(undefined, 'completed', 'fixture-game')).toBe('library');
  });
});
