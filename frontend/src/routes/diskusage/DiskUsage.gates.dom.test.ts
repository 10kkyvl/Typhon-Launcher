import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../../lib/i18n';
import { cachedUsage, scanUsage } from '../../lib/services/diskusage';
import type { LibraryGame } from '../../lib/services/library';
import type { MoveJob } from '../../lib/services/relocate';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { moves } from '../../lib/stores/relocate';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';
import { game, report, rowOf } from './diskUsageTesting';

// Move and Remove on a game row beyond the scan's own flags: a game being moved offers neither, the
// library being moved holds every game, an emulated game's Move follows Go (library.CheckRelocatable
// refuses every emulated game: emulators.game_unsupported). The flags come from the scan and go stale:
// when the library's installed games change (a removal or a move ended), the page measures again.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
}));

const LIBRARY = [
  { id: 'g-hades', title: 'Hades', installDir: 'C:\\Games\\Hades' },
  { id: 'g-celeste', title: 'Celeste', installDir: 'C:\\Games\\Celeste' },
  { id: 'g-ps2', title: 'Okami', installDir: 'C:\\Games\\Okami', emulation: { platform: 'ps2' } },
] as LibraryGame[];

const job = (patch: Partial<MoveJob>) => ({ id: 'm1', scope: 'game', stage: 'copy', title: 'Hades', ...patch }) as MoveJob;

function description(el: HTMLElement): string {
  return (el.getAttribute('aria-describedby') ?? '')
    .split(/\s+/)
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim() ?? '')
    .join(' ');
}

function gate(title: string, name: MessageKey): { off: boolean; why: string } {
  const button = within(rowOf('diskusage.gamesTitle', title)).getByRole('button', { name: msg(name) });
  return { off: button.getAttribute('aria-disabled') === 'true' || button.matches(':disabled'), why: description(button) };
}

async function settle() {
  for (let i = 0; i < 5; i++) {
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(scanUsage).mockResolvedValue(report({ games: [game('g-hades', 'Hades'), game('g-celeste', 'Celeste'), game('g-ps2', 'Okami')] }));
  libraryGames.set(LIBRARY);
  libraryLoaded.set(true);
  moves.set([]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
  libraryLoaded.set(false);
  moves.set([]);
});

async function shown() {
  renderDom(DiskUsage);
  await screen.findByText('Okami');
}

describe('DiskUsage: a game being moved', () => {
  it('offers neither Move nor Remove, saying why; the other games keep theirs', async () => {
    moves.set([job({ gameId: 'g-hades' })]);
    await shown();
    for (const name of ['diskusage.move', 'diskusage.remove'] as const) {
      expect(gate('Hades', name), name).toEqual({ off: true, why: msg('diskusage.blockedMoving') });
      expect(gate('Celeste', name).off, name).toBe(false);
    }
  });

  it('with two games being moved, holds each of them, not only the first listed', async () => {
    moves.set([job({ id: 'm1', gameId: 'g-hades' }), job({ id: 'm2', gameId: 'g-celeste', title: 'Celeste' })]);
    await shown();
    for (const title of ['Hades', 'Celeste']) {
      for (const name of ['diskusage.move', 'diskusage.remove'] as const) {
        expect(gate(title, name), `${title} ${name}`).toEqual({ off: true, why: msg('diskusage.blockedMoving') });
      }
    }
  });

  it('while the library is moved, holds every game, saying why', async () => {
    moves.set([job({ scope: 'library', gameId: undefined, title: '' })]);
    await shown();
    for (const title of ['Hades', 'Celeste']) {
      for (const name of ['diskusage.move', 'diskusage.remove'] as const) {
        expect(gate(title, name), `${title} ${name}`).toEqual({ off: true, why: msg('diskusage.blockedLibraryMoving') });
      }
    }
  });
});

describe('DiskUsage: an emulated game', () => {
  it('offers no Move, as Go refuses it, saying why; Remove stays', async () => {
    await shown();
    expect(gate('Okami', 'diskusage.move')).toEqual({ off: true, why: msg('diskusage.blockedEmulated') });
    expect(gate('Okami', 'diskusage.remove').off).toBe(false);
  });
});

describe('DiskUsage: the library changing under the report', () => {
  it('measures again when a game leaves the installed games or its folder changes', async () => {
    await shown();
    await settle();
    expect(scanUsage).toHaveBeenCalledTimes(1);

    libraryGames.set(LIBRARY.map((g) => (g.id === 'g-celeste' ? { ...g, uninstalled: true } : g)));
    await settle();
    expect(scanUsage).toHaveBeenCalledTimes(2);

    libraryGames.set(LIBRARY.map((g) => (g.id === 'g-hades' ? { ...g, installDir: 'E:\\Games\\Hades' } : g)));
    await settle();
    expect(scanUsage).toHaveBeenCalledTimes(3);
  });

  it('does not measure again for a change that moves no files (a mark, a play)', async () => {
    await shown();
    await settle();
    libraryGames.set(LIBRARY.map((g) => ({ ...g, favorite: true })));
    await settle();
    expect(scanUsage).toHaveBeenCalledTimes(1);
  });

  it('does not take the library’s first load for a change', async () => {
    libraryGames.set([]);
    libraryLoaded.set(false);
    await shown();
    await settle();
    libraryGames.set(LIBRARY);
    libraryLoaded.set(true);
    await settle();
    expect(scanUsage).toHaveBeenCalledTimes(1);
  });
});
