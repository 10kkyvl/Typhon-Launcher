import { get } from 'svelte/store';
import { beforeEach, describe, expect, it } from 'vitest';
import type { SessionEvent } from '../services/library';
import { CRASH_QUIET_MS, clearCrashes, closeCrash, crashTarget, recordCrash, resetCrashes } from './crash';

const event = (over: Partial<SessionEvent> = {}): SessionEvent => ({
  gameId: 'g1',
  sessionSeconds: 3,
  playtimeSeconds: 0,
  exitCode: 3221225781,
  exitKnown: true,
  crashed: true,
  reasonCode: 'dll_not_found',
  ...over,
});

beforeEach(() => resetCrashes());

describe('crash store', () => {
  it('remembers the crash and opens the target', () => {
    recordCrash(event(), 1000);

    expect(get(crashTarget)).toEqual({
      gameId: 'g1',
      exitCode: 3221225781,
      exitKnown: true,
      reasonCode: 'dll_not_found',
      sessionSeconds: 3,
      at: 1000,
      count: 1,
    });
  });

  it('stays quiet for the same game within five minutes but keeps counting', () => {
    recordCrash(event(), 1000);
    closeCrash();

    recordCrash(event(), 1000 + CRASH_QUIET_MS - 1);
    expect(get(crashTarget)).toBeNull();

    recordCrash(event(), 1000 + CRASH_QUIET_MS);
    expect(get(crashTarget)?.count).toBe(3);
  });

  it('does not let one game silence another', () => {
    recordCrash(event(), 1000);
    closeCrash();

    recordCrash(event({ gameId: 'g2' }), 2000);

    expect(get(crashTarget)?.gameId).toBe('g2');
    expect(get(crashTarget)?.count).toBe(1);
  });

  it('updates the open dialog for a repeat crash of the same game', () => {
    recordCrash(event(), 1000);

    recordCrash(event({ sessionSeconds: 5 }), 2000);

    expect(get(crashTarget)?.count).toBe(2);
    expect(get(crashTarget)?.sessionSeconds).toBe(5);
  });

  it('starts counting again after a normal session', () => {
    recordCrash(event(), 1000);
    clearCrashes('g1');
    closeCrash();

    recordCrash(event(), 1000 + CRASH_QUIET_MS);

    expect(get(crashTarget)?.count).toBe(1);
  });
});
