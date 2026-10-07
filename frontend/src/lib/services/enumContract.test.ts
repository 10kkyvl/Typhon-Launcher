import { describe, expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: false }));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));

import { bindingSources, frontendSources } from '../testing/sources';
import { installStatusLabels, installTypeLabels } from '../stores/install';
import { statusLabels } from '../stores/downloads';
import { historyLabel } from '../history/historyText';
import { stageLabel } from '../relocate/moveText';
import { transferLabel } from '../lan/lanText';
import { snapshotKindLabel } from '../savebackup/messages';
import { msg } from '../i18n';
import type { InstallStatus, InstallType } from './install';
import type { DownloadStatus } from './downloads';
import type { MoveStage } from './relocate';
import type { TransferStatus, Transfer } from './lan';
import type { SnapshotKind } from './savebackup';
import type { Record as HistoryRecord } from './history';

function generatedEnum(pkg: string, name: string): string[] {
  const source = bindingSources[`typhon/internal/${pkg}/models.ts`];
  expect(source, `bindings for ${pkg}`).toBeTypeOf('string');
  const block = new RegExp(String.raw`export enum ${name} \{([\s\S]*?)\n\};`).exec(source)?.[1];
  expect(block, `enum ${pkg}.${name}`).toBeTypeOf('string');
  return [...block!.matchAll(/^\s*\w+ = "([^"]*)",?/gm)].map((match) => match[1]).filter((value) => value !== '');
}

function declaredUnion(file: string, name: string): string[] {
  const source = frontendSources[file];
  expect(source, file).toBeTypeOf('string');
  const body = new RegExp(String.raw`export type ${name}\s*=\s*((?:\s*\|?\s*'[^']*')+)\s*;`).exec(source)?.[1];
  expect(body, `type ${name} in ${file}`).toBeTypeOf('string');
  return [...body!.matchAll(/'([^']*)'/g)].map((match) => match[1]).filter((value) => value !== '');
}

const PAIRS: Array<[file: string, type: string, pkg: string, enumName: string]> = [
  ['lib/services/install.ts', 'InstallStatus', 'install', 'Status'],
  ['lib/services/install.ts', 'InstallType', 'install', 'Type'],
  ['lib/services/install.ts', 'RemovalMethod', 'install', 'RemovalMethod'],
  ['lib/services/downloads.ts', 'DownloadStatus', 'download', 'Status'],
  ['lib/services/downloads.ts', 'DownloadPurpose', 'download', 'Purpose'],
  ['lib/services/updates.ts', 'UpdateState', 'updates', 'State'],
  ['lib/services/updates.ts', 'AvailabilityKind', 'updates', 'AvailabilityKind'],
  ['lib/services/updates.ts', 'StrategyType', 'updates', 'StrategyType'],
  ['lib/services/updates.ts', 'VerifyMethod', 'updates', 'VerifyMethod'],
  ['lib/services/updates.ts', 'StepKind', 'updates', 'StepKind'],
  ['lib/services/relocate.ts', 'MoveScope', 'relocate', 'Scope'],
  ['lib/services/relocate.ts', 'MoveStage', 'relocate', 'Stage'],
  ['lib/services/lan.ts', 'TransferStatus', 'lan', 'TransferStatus'],
  ['lib/services/savebackup.ts', 'SnapshotKind', 'savebackup', 'Kind'],
  ['lib/services/savebackup.ts', 'BackupStatus', 'savebackup', 'Status'],
  ['lib/services/sources.ts', 'SourceType', 'sources', 'Type'],
  ['lib/services/sources.ts', 'SourceStatus', 'sources', 'Status'],
  ['lib/services/sources.ts', 'SourceHealth', 'sources', 'Health'],
  ['lib/services/sources.ts', 'Availability', 'sources', 'Availability'],
  ['lib/services/sources.ts', 'MatchStatus', 'catalog', 'Status'],
  ['lib/services/metadata.ts', 'MetadataMatch', 'metadata', 'MatchState'],
  ['lib/services/compat.ts', 'CompatState', 'compat', 'State'],
  ['lib/services/selfupdate.ts', 'ReleaseChangeKind', 'selfupdate', 'ChangeKind'],
  ['lib/services/telemetryLog.ts', 'SentDataKind', 'telemetrylog', 'Kind'],
];

describe('string types that mirror Go enums', () => {
  it.each(PAIRS)('%s: %s lists exactly the values of %s.%s', (file, type, pkg, enumName) => {
    expect(declaredUnion(file, type).sort()).toEqual(generatedEnum(pkg, enumName).sort());
  });
});

describe('every value Go can send has a face in the interface', () => {
  const nonEmpty = (text: unknown) => typeof text === 'string' && text.trim() !== '';

  it('names each install status', () => {
    for (const status of generatedEnum('install', 'Status')) {
      expect(nonEmpty(installStatusLabels(status as InstallStatus)), status).toBe(true);
    }
  });

  it('names each install type', () => {
    for (const type of generatedEnum('install', 'Type')) {
      expect(nonEmpty(installTypeLabels(type as InstallType)), type).toBe(true);
    }
  });

  it('names each download status', () => {
    for (const status of generatedEnum('download', 'Status')) {
      expect(nonEmpty(statusLabels(status as DownloadStatus)), status).toBe(true);
    }
  });

  it('titles each kind of history record with something better than the unknown title', () => {
    const unknown = historyLabel({ kind: '__unknown__', title: 'Game', bytesKnown: false } as unknown as HistoryRecord).title;
    expect(unknown).toBe(msg('transfers.historyUnknownTitle', { title: 'Game' }));
    for (const kind of generatedEnum('history', 'Kind')) {
      const label = historyLabel({ id: 'r', kind, at: '', title: 'Game', bytesKnown: false } as unknown as HistoryRecord);
      expect(label.title, kind).not.toBe(unknown);
    }
  });

  it('labels each stage of a move instead of echoing the raw stage', () => {
    for (const stage of generatedEnum('relocate', 'Stage')) {
      expect(stageLabel(stage as MoveStage), stage).not.toBe(stage);
    }
  });

  it('describes each state of a LAN transfer', () => {
    for (const status of generatedEnum('lan', 'TransferStatus')) {
      const label = transferLabel({ status: status as TransferStatus, downloaded: 1, total: 10, error: '' } as unknown as Transfer);
      expect(nonEmpty(label), status).toBe(true);
    }
  });

  it('names each kind of save snapshot instead of echoing the raw kind', () => {
    for (const kind of generatedEnum('savebackup', 'Kind')) {
      expect(snapshotKindLabel(kind as SnapshotKind), kind).not.toBe(kind);
    }
  });
});
