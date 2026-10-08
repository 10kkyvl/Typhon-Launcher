import { describe, expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: false }));
vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Events: { On: vi.fn(() => vi.fn()) },
}));

import { compatOnlyWorking } from './sources';
import { installerLikely } from './install';
import { legalDocumentIds } from './legalMessages';
import { currentTelemetryConsent } from '../stores/telemetryConsent';
import { themeDisplayName } from '../theme/apply';
import { goSources, frontendSources } from '../testing/sources';

function go(path: string): string {
  const source = goSources[path];
  expect(source, path).toBeTypeOf('string');
  return source;
}

describe('values the frontend repeats from Go', () => {
  it('asks for the telemetry consent version Go currently requires', () => {
    const declared = /CurrentTelemetryConsent\s*=\s*(\d+)/.exec(go('internal/settings/settings.go'))?.[1];

    expect(declared).toBeDefined();
    expect(currentTelemetryConsent).toBe(Number(declared));
  });

  it('filters the catalog by the compatibility value Go understands', () => {
    const declared = /CompatOnlyWorking\s*=\s*"([^"]+)"/.exec(go('internal/catalog/browse.go'))?.[1];

    expect(declared).toBeDefined();
    expect(compatOnlyWorking).toBe(declared);
  });

  it('lists the legal documents Go ships, and no other', () => {
    const block = /var Required = \[\]Meta\{([\s\S]*?)\n\}/.exec(go('internal/legal/legal.go'))?.[1] ?? '';
    const ids = [...block.matchAll(/ID:\s*"([^"]+)"/g)].map((match) => match[1]);

    expect(ids.length).toBeGreaterThan(2);
    expect([...legalDocumentIds].sort()).toEqual([...ids].sort());
  });

  it('judges an installer by the same file extensions as Go', () => {
    const body = /func InstallerLikely\([\s\S]*?\n\}/.exec(go('internal/install/auto.go'))?.[0] ?? '';
    const accepted = new Set([...(/case ([^:]+):/.exec(body)?.[1] ?? '').matchAll(/"(\.[a-z0-9]+)"/g)].map((match) => match[1]));
    const probes = ['.exe', '.msi', '.bat', '.cmd', '.com', '.scr', '.msix', '.appx', '.zip', '.rar', '.7z', '.bin', '.iso', '.dll', '.txt', '.sh', '.pkg', '.dmg', '.apk', '.jar'];

    expect(accepted.size).toBeGreaterThan(0);
    for (const extension of probes) {
      expect(installerLikely([`Setup${extension}`]), extension).toBe(accepted.has(extension));
      expect(installerLikely([`SETUP${extension.toUpperCase()}`]), extension.toUpperCase()).toBe(accepted.has(extension));
    }
  });

  it('knows every built-in theme Go ships by its id, so its name follows the language', () => {
    const ids = [...go('internal/theme/presets.go').matchAll(/^\tID:\s*"([^"]+)"/gm)].map((match) => match[1]);

    expect(ids.length).toBeGreaterThan(1);
    for (const id of ids) {
      expect(themeDisplayName({ id, name: 'stored-name', builtIn: true }), id).not.toBe('stored-name');
    }
    expect(themeDisplayName({ id: 'dark', name: 'My dark', builtIn: false })).toBe('My dark');
    expect(themeDisplayName({ id: 'new-preset', name: 'stored-name', builtIn: true })).toBe('stored-name');
  });

  it('reads the same progress fields from a download tick as Go sends', () => {
    const struct = /type ProgressUpdate struct \{([\s\S]*?)\n\}/.exec(go('internal/download/model.go'))?.[1] ?? '';
    const sent = [...struct.matchAll(/`json:"(\w+)[^"]*"`/g)].map((match) => match[1]).sort();
    const source = frontendSources['lib/stores/downloads.ts'];
    const body = /export interface DownloadProgress \{([\s\S]*?)\n\}/.exec(source)?.[1] ?? '';
    const read = [...body.matchAll(/^ {2}(\w+)\??:/gm)].map((match) => match[1]).sort();

    expect(sent.length).toBeGreaterThan(8);
    expect(read).toEqual(sent);
  });
});

describe('LAN announce rejections', () => {
  function goReasons(): string[] {
    const source = go('internal/lan/announce.go');
    const body = /func reasonFor\([\s\S]*?\n\}/.exec(source)?.[0] ?? '';
    const reasons = new Set([...body.matchAll(/return "([a-z_]+)"/g)].map((match) => match[1]));
    for (const path of Object.keys(goSources).filter((file) => file.startsWith('internal/lan/'))) {
      for (const match of goSources[path].matchAll(/\.reject\("([a-z_]+)"\)/g)) reasons.add(match[1]);
    }
    return [...reasons].sort();
  }

  it('has a readable name for every reason Go can count', () => {
    const reasons = goReasons();
    const table = frontendSources['lib/lan/lanText.ts'];
    const named = new Set([...table.matchAll(/^\s*(\w+):\s*'transfers\.lanReject\w+',/gm)].map((match) => match[1]));

    expect(reasons.length).toBeGreaterThan(15);
    expect(reasons.filter((reason) => !named.has(reason))).toEqual([]);
  });

  it('has no name for a reason Go no longer counts', () => {
    const reasons = new Set(goReasons());
    const table = frontendSources['lib/lan/lanText.ts'];
    const named = [...table.matchAll(/^\s*(\w+):\s*'transfers\.lanReject\w+',/gm)].map((match) => match[1]);

    expect(named.filter((reason) => !reasons.has(reason))).toEqual([]);
  });
});
