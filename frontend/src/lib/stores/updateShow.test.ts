import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SelfUpdateStatus } from '../services/selfupdate';
import { FRESH_SHELL_MS, beginOwedFinish, endUpdateShow, hostUpdateShow, logoArcsAway, resumeFinishedShow, showFollowsStatus, takeOverShow, updateShow, updateShowHandoff } from './updateShow';

const status = (state: SelfUpdateStatus['state'], currentVersion = '1.4.0'): SelfUpdateStatus => ({ state, currentVersion, availableVersion: '1.5.0' });

afterEach(() => endUpdateShow());

describe('the update show follows the update status', () => {
  it('starts when applying begins, keeping the moment it began and the version it installs', () => {
    showFollowsStatus(status('applying'), 1000);
    expect(get(updateShow)).toEqual({ startedAt: 1000, phase: 'run', since: 1000, version: '1.5.0', from: '1.4.0' });
  });

  it('never starts from a downloaded update alone, nor from any other state', () => {
    for (const state of ['idle', 'checking', 'available', 'downloading', 'ready', 'failed'] as const) {
      showFollowsStatus(status(state), 1000);
      expect(get(updateShow), state).toBeNull();
    }
  });

  it('keeps its start when applying is said again', () => {
    showFollowsStatus(status('applying'), 1000);
    showFollowsStatus(status('applying'), 5000);
    expect(get(updateShow)?.startedAt).toBe(1000);
  });

  it('sends the arcs home when applying ends without a new version', () => {
    showFollowsStatus(status('applying'), 1000);
    showFollowsStatus({ ...status('ready'), error: 'parent still running', errorCode: 'apply' }, 9000);
    expect(get(updateShow)).toMatchObject({ startedAt: 1000, phase: 'home', since: 9000 });
  });

  it('goes home, not to the finish, when applying ends with the same version still running', () => {
    showFollowsStatus(status('applying'), 1000);
    showFollowsStatus(status('idle', '1.4.0'), 9000);
    expect(get(updateShow)).toMatchObject({ phase: 'home', since: 9000 });
  });

  it('finishes when the running version changed', () => {
    showFollowsStatus(status('applying'), 1000);
    showFollowsStatus(status('idle', '1.5.0'), 9000);
    expect(get(updateShow)).toMatchObject({ startedAt: 1000, phase: 'finish', since: 9000 });
  });

  it('stays up through a long applying: only the status ends it', () => {
    showFollowsStatus(status('applying'), 1000);
    showFollowsStatus(status('applying'), 1000 + 105_000);
    expect(get(updateShow)?.phase).toBe('run');
  });
});

describe('the start after an update', () => {
  it('owes the finish from the held show, with no words of its own, timed from the first frame drawn', () => {
    expect(resumeFinishedShow()).toBe(true);
    expect(get(updateShow)).toMatchObject({ startedAt: 0, phase: 'finish', version: '', from: '', owed: true });

    beginOwedFinish(5000);
    expect(get(updateShow)).toEqual({ startedAt: 0, phase: 'finish', since: 5000, version: '', from: '' });
    beginOwedFinish(9000);
    expect(get(updateShow)?.since).toBe(5000);
  });

  it('still plays over a shell that has just appeared; plays nothing once it has been up for the player to use, unless asked to play over it', () => {
    vi.useFakeTimers({ toFake: ['performance'] });
    try {
      const gone = hostUpdateShow();
      vi.advanceTimersByTime(FRESH_SHELL_MS);
      expect(resumeFinishedShow()).toBe(true);
      endUpdateShow();

      vi.advanceTimersByTime(1);
      expect(resumeFinishedShow()).toBe(false);
      expect(get(updateShow)).toBeNull();
      expect(resumeFinishedShow(true)).toBe(true);
      endUpdateShow();
      gone();
      gone();
      vi.advanceTimersByTime(5000);
      expect(resumeFinishedShow()).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it('never replaces a show that is up, a successor\'s held one included', () => {
    takeOverShow(1000, '1.5.0');
    expect(resumeFinishedShow()).toBe(false);
    expect(get(updateShow)).toMatchObject({ held: true, phase: 'run', version: '1.5.0' });
  });

  it('keeps for a successor\'s curtain only a colour the old window could have handed over', () => {
    takeOverShow(1000, '1.5.0', '#A1b2C3');
    expect(get(updateShow)?.background).toBe('#A1b2C3');
    for (const handed of ['', 'red', '#abc', 'url(x)', '#123456; } body { display: none']) {
      takeOverShow(1000, '1.5.0', handed);
      expect(get(updateShow)?.background).toBeUndefined();
    }
  });
});

describe('ending the show', () => {
  it('clears the show and gives the logo its arcs back', () => {
    showFollowsStatus(status('applying'), 1000);
    logoArcsAway.set([true, true, true, true, true]);
    endUpdateShow();
    expect(get(updateShow)).toBeNull();
    expect(get(logoArcsAway)).toBeNull();
  });
});

describe('what a successor process is handed', () => {
  it('is the moment the running show began, and nothing when no show runs', () => {
    expect(updateShowHandoff()).toBeNull();
    showFollowsStatus(status('applying'), 1234);
    expect(updateShowHandoff()).toEqual({ startedAt: 1234 });
    showFollowsStatus(status('ready'), 2000);
    expect(updateShowHandoff()).toBeNull();
  });
});
