import { get } from 'svelte/store';
import { beforeEach, describe, expect, it } from 'vitest';
import { CRASH_DETAIL_WINDOW_MS } from '../redist/crashView';
import type { CrashEvent, SessionEvent } from '../services/library';
import { CRASH_QUIET_MS, clearCrashes, closeCrash, crashTarget, recordCrash, recordCrashDetail, resetCrashes } from './crash';

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

describe('crash detail', () => {
  const detail = (over: Partial<CrashEvent> = {}): CrashEvent => ({
    gameId: 'g1',
    exitCode: 3221225477,
    exitKnown: true,
    reasonCode: 'access_violation',
    faultModule: 'msvcp140.dll',
    exceptionCode: '0xC0000005',
    hint: 'vcredist',
    missingFile: '',
    sessionSeconds: 900,
    dumpFound: false,
    journalUnknown: false,
    ...over,
  });

  it('completes the open modal instead of opening a second one', () => {
    recordCrash(event(), 1000);
    recordCrashDetail(detail(), 2000);

    const target = get(crashTarget);
    expect(target?.count).toBe(1);
    expect(target?.detail?.faultModule).toBe('msvcp140.dll');
    expect(target?.late).toBeUndefined();
  });

  it('does not reopen a modal the user closed right after the stop event', () => {
    recordCrash(event(), 1000);
    closeCrash();

    recordCrashDetail(detail(), 1000 + CRASH_DETAIL_WINDOW_MS - 1);
    expect(get(crashTarget)).toBeNull();
  });

  it('opens a late modal for a crash without a stop event', () => {
    recordCrashDetail(detail(), 1000);

    const target = get(crashTarget);
    expect(target?.late).toBe(true);
    expect(target?.detail?.hint).toBe('vcredist');
  });

  it('stays quiet for a repeated late crash within five minutes', () => {
    recordCrashDetail(detail(), 1000);
    closeCrash();

    recordCrashDetail(detail(), 1000 + CRASH_QUIET_MS - 1);
    expect(get(crashTarget)).toBeNull();
  });

  it('opens again after the quiet period', () => {
    recordCrashDetail(detail(), 1000);
    closeCrash();

    recordCrashDetail(detail(), 1000 + CRASH_QUIET_MS);
    expect(get(crashTarget)?.count).toBe(2);
  });
});
