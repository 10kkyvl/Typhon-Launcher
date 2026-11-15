import { describe, expect, it } from 'vitest';
import { bindingSources, goSources } from '../testing/sources';
import type { DownloadStatus } from '../services/downloads';
import type { PlayThrottle } from '../services/playThrottle';
import { downloadActionNames, downloadActions, goAccepts, type DownloadAction } from './actions';

const open: PlayThrottle = { active: false, mode: 'off', downloadLimit: 0, uploadLimit: 0 };
const closed: PlayThrottle = { ...open, schedule: { blocked: true, nextOpenAt: '2026-11-16T01:00:00Z' } };

function goStatuses(): DownloadStatus[] {
  const source = bindingSources['typhon/internal/download/models.ts'];
  expect(source, 'bindings for download').toBeTypeOf('string');
  const block = /export enum Status \{([\s\S]*?)\n\};/.exec(source)?.[1] ?? '';
  return [...block.matchAll(/^\s*\w+ = "([^"]*)",?/gm)].map((match) => match[1]).filter(Boolean) as DownloadStatus[];
}

// What each Manager method refuses with errUnavailable, read from its own guard in Go: a guard that
// changes there turns this red instead of leaving a button Go refuses.
function goGuards(): Record<DownloadAction, DownloadStatus[]> {
  const model = goSources['internal/download/model.go'];
  const names = new Map([...model.matchAll(/(Status\w+)\s+Status = "(\w+)"/g)].map((match) => [match[1], match[2] as DownloadStatus]));
  const every = [...names.values()];
  const go = goSources['internal/download/manager.go'] + goSources['internal/download/schedule.go'];
  const body = (signature: string) => {
    const found = new RegExp(String.raw`func \(m \*Manager\) ${signature}[^\n]*\{\n([\s\S]*?)\n\}`).exec(go)?.[1];
    expect(found, signature).toBeTypeOf('string');
    return found!;
  };
  const accepted = (signature: string): DownloadStatus[] => {
    const guard = /if ([^{\n]*Status[^{\n]*)\{\s*return errUnavailable/.exec(body(signature))?.[1];
    if (!guard) return every;
    const named = [...guard.matchAll(/(Status\w+)/g)].map((match) => names.get(match[1])!).filter(Boolean);
    return guard.includes('!=') ? named : every.filter((status) => !named.includes(status));
  };
  return {
    pause: accepted(String.raw`Pause\(`),
    resume: accepted(String.raw`Resume\(`),
    forceStart: accepted(String.raw`ForceStart\(`),
    downloadNow: accepted(String.raw`DownloadNow\(`),
    moveUp: accepted(String.raw`move\(`),
    moveDown: accepted(String.raw`move\(`),
    moveTo: accepted(String.raw`MoveTo\(`),
    cancel: accepted(String.raw`discard\(`),
    remove: accepted(String.raw`discard\(`),
  };
}

describe('what Go accepts for a download, by status', () => {
  const EXPECTED: Record<DownloadStatus, DownloadAction[]> = {
    queued: ['pause', 'forceStart', 'downloadNow', 'moveUp', 'moveDown', 'moveTo', 'cancel', 'remove'],
    metadata: ['downloadNow', 'cancel', 'remove'],
    downloading: ['pause', 'downloadNow', 'cancel', 'remove'],
    paused: ['resume', 'forceStart', 'downloadNow', 'cancel', 'remove'],
    verifying: ['downloadNow', 'cancel', 'remove'],
    completed: ['cancel', 'remove'],
    failed: ['resume', 'forceStart', 'downloadNow', 'cancel', 'remove'],
  };

  it.each(Object.entries(EXPECTED))('%s: exactly the actions the Manager lets through', (status, actions) => {
    for (const action of downloadActionNames) {
      expect(goAccepts(status as DownloadStatus, action), `${status} ${action}`).toBe(actions.includes(action));
    }
  });

  it('answers for every status Go can send, as the guards in internal/download read', () => {
    const statuses = goStatuses();
    const guards = goGuards();
    expect(statuses.length).toBeGreaterThan(6);
    expect(guards.pause.length).toBeGreaterThan(0);
    expect([...downloadActionNames].sort()).toEqual(Object.keys(guards).sort());
    for (const status of statuses) {
      for (const action of downloadActionNames) {
        expect(goAccepts(status, action), `${status} ${action}`).toBe(guards[action].includes(status));
      }
    }
  });

  it('offers only cancel and remove for a status it does not know', () => {
    expect(downloadActions({ status: 'archived' as DownloadStatus }, open)).toEqual(['cancel', 'remove']);
  });
});

describe('the actions a download offers', () => {
  it('offers Pause on a queued download and no Continue (Go resumes only paused or failed)', () => {
    const actions = downloadActions({ status: 'queued' }, open);
    expect(actions).toContain('pause');
    expect(actions).not.toContain('resume');
  });

  it('offers no Pause while the metadata is fetched (Go pauses only downloading or queued)', () => {
    expect(downloadActions({ status: 'metadata' }, open)).not.toContain('pause');
  });

  it('offers "Download now" only where the schedule holds the download back', () => {
    expect(downloadActions({ status: 'queued' }, closed)).toContain('downloadNow');
    expect(downloadActions({ status: 'downloading' }, closed)).toContain('downloadNow');
    expect(downloadActions({ status: 'queued' }, open)).not.toContain('downloadNow');
    expect(downloadActions({ status: 'queued', ignoreSchedule: true }, closed)).not.toContain('downloadNow');
  });

  it('does not offer "Download now" on a paused download: it would lift the schedule and leave it paused', () => {
    expect(downloadActions({ status: 'paused' }, closed)).not.toContain('downloadNow');
    expect(downloadActions({ status: 'failed' }, closed)).not.toContain('downloadNow');
  });

  it('keeps every offer inside what Go accepts, for every status and schedule', () => {
    for (const status of goStatuses()) {
      for (const throttle of [open, closed]) {
        for (const action of downloadActions({ status }, throttle)) {
          expect(goAccepts(status, action), `${status} ${action}`).toBe(true);
        }
      }
    }
  });
});
