import { describe, expect, it } from 'vitest';
import type { CrashEvent, SessionEvent } from '../services/library';
import { crashView, mergeCrash, recordFromCrash, recordFromStopped, toDetail } from './crashView';

const crash = (over: Partial<CrashEvent> = {}): CrashEvent => ({
  gameId: 'g1',
  exitCode: 3221225477,
  exitKnown: true,
  reasonCode: 'access_violation',
  faultModule: 'msvcp140.dll',
  exceptionCode: '0xC0000005',
  hint: 'vcredist',
  missingFile: '',
  sessionSeconds: 1200,
  dumpFound: false,
  journalUnknown: false,
  ...over,
});

const stopped: SessionEvent = {
  gameId: 'g1',
  sessionSeconds: 3,
  playtimeSeconds: 0,
  exitCode: 3221225781,
  exitKnown: true,
  crashed: true,
  reasonCode: 'dll_not_found',
};

const view = (over: Partial<CrashEvent> = {}) => crashView(recordFromCrash(crash(over), 1, 0));

describe('crashView by hint', () => {
  it('vcredist offers libraries and names the module', () => {
    const v = view();
    expect(v.reason).toEqual({ key: 'redist.hintVcredist', params: { module: 'msvcp140.dll' } });
    expect(v.actions).toEqual(['libs', 'verify']);
    expect(v.title).toBe('redist.crashTitleLate');
  });

  it('directx offers libraries', () => {
    const v = view({ hint: 'directx', faultModule: 'd3dx9_43.dll' });
    expect(v.reason?.key).toBe('redist.hintDirectx');
    expect(v.actions).toEqual(['libs', 'verify']);
  });

  it('compat offers game properties', () => {
    const v = view({ hint: 'compat', faultModule: 'ddraw.dll' });
    expect(v.reason?.key).toBe('redist.hintCompat');
    expect(v.actions).toEqual(['compat', 'verify']);
  });

  it('antivirus names the file, verifies and adds the exclusion tip', () => {
    const v = view({ hint: 'antivirus', missingFile: 'game.exe' });
    expect(v.reason).toEqual({ key: 'redist.hintAntivirus', params: { file: 'game.exe' } });
    expect(v.actions).toEqual(['verify']);
    expect(v.tips).toContain('redist.crashAntivirusTip');
  });

  it('antivirus without a file name falls back to the bare text', () => {
    expect(view({ hint: 'antivirus', missingFile: '' }).reason?.key).toBe('redist.hintAntivirusBare');
  });

  it('runtimes offers libraries', () => {
    const v = view({ hint: 'runtimes' });
    expect(v.reason?.key).toBe('redist.hintRuntimes');
    expect(v.actions).toEqual(['libs', 'verify']);
  });

  it('unknown offers a report', () => {
    const v = view({ hint: 'unknown' });
    expect(v.reason?.key).toBe('redist.hintUnknown');
    expect(v.actions).toEqual(['report', 'verify']);
  });

  it('an empty or foreign hint leaves no reason and the old buttons', () => {
    for (const hint of ['', 'something']) {
      const v = view({ hint });
      expect(v.reason).toBeNull();
      expect(v.actions).toEqual(['libs', 'verify']);
    }
  });

  it('drops the module from the text and tech lines when it is empty', () => {
    const v = view({ faultModule: '', exceptionCode: '' });
    expect(v.reason?.key).toBe('redist.hintVcredistBare');
    expect(v.tech.map((l) => l.key)).toEqual(['redist.crashCode']);
  });
});

describe('crashView details', () => {
  it('lists module, exception and exit code', () => {
    const v = view();
    expect(v.tech).toEqual([
      { key: 'redist.crashModule', params: { module: 'msvcp140.dll' } },
      { key: 'redist.crashException', params: { code: '0xC0000005' } },
      { key: 'redist.crashCode', params: { code: '0xC0000005' } },
    ]);
  });

  it('skips the exit code when it is unknown', () => {
    expect(view({ exitKnown: false }).tech.map((l) => l.key)).not.toContain('redist.crashCode');
  });

  it('warns when the journal was unavailable and mentions a dump', () => {
    const v = view({ journalUnknown: true, dumpFound: true });
    expect(v.tips).toEqual(['redist.crashDump', 'redist.crashJournalUnknown']);
  });

  it('keeps the short-session title and the old text for a stopped-only record', () => {
    const v = crashView(recordFromStopped(stopped, 1, 0));
    expect(v.title).toBe('redist.crashTitle');
    expect(v.reason).toBeNull();
    expect(v.actions).toEqual(['libs', 'verify']);
    expect(v.tech).toEqual([{ key: 'redist.crashCode', params: { code: '0xC0000135' } }]);
  });
});

describe('merge', () => {
  it('adds the details to a stopped record and keeps the short title and count', () => {
    const base = recordFromStopped(stopped, 2, 5);
    const merged = mergeCrash(base, crash({ exitCode: 3221225781, reasonCode: '' }));
    expect(merged.count).toBe(2);
    expect(merged.at).toBe(5);
    expect(merged.late).toBeUndefined();
    expect(merged.reasonCode).toBe('dll_not_found');
    expect(merged.detail?.hint).toBe('vcredist');
    expect(crashView(merged).title).toBe('redist.crashTitle');
  });

  it('toDetail normalises missing fields', () => {
    const d = toDetail({ ...crash(), faultModule: undefined as unknown as string, hint: 'weird' });
    expect(d.faultModule).toBe('');
    expect(d.hint).toBe('');
  });
});
