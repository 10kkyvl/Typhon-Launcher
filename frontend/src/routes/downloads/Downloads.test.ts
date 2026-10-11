import { render } from 'svelte/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../lib/services/backend', () => ({ inWails: false }));

import Downloads from './Downloads.svelte';
import { downloads } from '../../lib/stores/downloads';
import { installations } from '../../lib/stores/install';
import { locale } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import type { Installation } from '../../lib/services/install';

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: 'C:\\Games\\d1',
    status: 'completed',
    progress: 1,
    downloaded: 1000,
    total: 1000,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-09-08T00:00:00Z',
    completedAt: '2026-09-08T01:00:00Z',
    error: '',
    origin: {},
    ...patch,
  };
}

function installation(patch: Partial<Installation> = {}): Installation {
  return {
    id: 'i1',
    downloadId: 'd1',
    gameId: '',
    name: 'Game One',
    type: 'archive_zip',
    status: 'installing',
    mode: 'move',
    sourcePath: '',
    contentRoot: '',
    destination: '',
    installerPath: '',
    workingDir: '',
    archivePath: '',
    progress: 0.5,
    currentFile: '',
    bytesDone: 0,
    bytesTotal: 100,
    executable: '',
    candidates: null,
    detectedVersion: '',
    versionSource: '',
    startedAt: '2026-09-08T02:00:00Z',
    completedAt: null,
    error: '',
    engine: '',
    silent: false,
    ...patch,
  };
}

function page(): string {
  return render(Downloads, { props: {} }).body;
}

beforeEach(() => {
  locale.set('en');
  downloads.set([]);
  installations.set([]);
});

afterEach(() => {
  locale.set('ru');
});

describe('downloads screen', () => {
  it('shows empty sections and no failed section when nothing was downloaded', () => {
    const html = page();

    expect(html).toContain('No active downloads');
    expect(html).not.toContain('Failed');
    expect(html).not.toMatch(/class="row failed[ "]/);
  });

  it('lists a failed download with its translated error, a retry and a remove button', () => {
    downloads.set([
      download({ id: 'bad', name: 'Broken Game', status: 'failed', error: 'typhon:download.no_free_space: no room on D:' }),
    ]);

    const html = page();

    expect(html).toMatch(/class="row failed[ "]/);
    expect(html).toContain('Broken Game');
    expect(html).toContain('Retry');
    expect(html).not.toContain('typhon:');
    expect(html).not.toContain('no_free_space');
  });

  it('keeps a failed download off the active, queue and completed lists', () => {
    downloads.set([download({ id: 'bad', name: 'Broken Game', status: 'failed', error: '' })]);

    const html = page();

    const rows = html.match(/class="row(?: [^"]*)?"/g) ?? [];
    expect(rows).toHaveLength(1);
    expect(rows[0]).toContain('failed');
    expect(html).toContain('Broken Game');
    expect(html).not.toContain('Install</button>');
  });

  it('offers to install a finished download that has no installation yet', () => {
    downloads.set([download({ name: 'Ready Game' })]);

    const html = page();

    expect(html).toContain('Ready Game');
    expect(html).toContain('Install');
  });

  it('shows the progress of an installation that is under way, not the install button', () => {
    downloads.set([download()]);
    installations.set([installation({ status: 'extracting' })]);

    const html = page();

    expect(html).toContain('Extracting');
    expect(html).not.toContain('>Install</button>');
  });

  it('shows an installation without a known total as indeterminate', () => {
    downloads.set([download()]);
    installations.set([installation({ status: 'installing', bytesTotal: 0, silent: true })]);

    const unknownTotal = page();
    installations.set([installation({ status: 'installing', bytesTotal: 100, silent: true })]);
    const knownTotal = page();

    expect(unknownTotal).not.toBe(knownTotal);
    expect(unknownTotal).toMatch(/indeterminate/);
    expect(knownTotal).not.toMatch(/indeterminate/);
  });

  it('asks the player to continue when the installer waits for them', () => {
    downloads.set([download()]);
    installations.set([installation({ status: 'waiting_for_user' })]);

    expect(page()).toContain('Continue');
  });

  it('marks an installed game as installed', () => {
    downloads.set([download()]);
    installations.set([installation({ status: 'completed' })]);

    expect(page()).toContain('Installed');
  });

  it('draws attention to a failed install and keeps cancelled and interrupted ones apart', () => {
    downloads.set([download()]);

    installations.set([installation({ status: 'failed' })]);
    const failed = page();
    installations.set([installation({ status: 'cancelled' })]);
    const cancelled = page();
    installations.set([installation({ status: 'interrupted' })]);
    const interrupted = page();

    expect(failed).toMatch(/class="btn danger/);
    expect(cancelled).not.toMatch(/class="btn danger/);
    expect(cancelled).not.toBe(interrupted);
  });
});
