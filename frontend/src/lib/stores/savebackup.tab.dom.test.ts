import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { LibraryGame } from '../services/library';
import type { Settings } from '../services/settings';
import { saveBackupsEnabled, savesTab } from './savebackup';
import { settings } from './settings';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
vi.mock('../services/backend', () => ({ inWails: false }));

const game = (patch: Partial<LibraryGame> = {}) => ({ id: 'g1', title: 'Hades', canonicalGameId: 'igdb:1', ...patch }) as LibraryGame;
const emulated = game({ emulation: { platform: 'ps2' } } as Partial<LibraryGame>);

afterEach(() => {
  saveBackupsEnabled.set(false);
  settings.set(null);
});

describe('which game gets a Saves tab', () => {
  it('gives one to a library game whenever backups are on', () => {
    saveBackupsEnabled.set(true);
    settings.set({ cloudSaves: false } as Settings);

    expect(get(savesTab)(game())).toBe(true);
    expect(get(savesTab)(game({ canonicalGameId: '' }))).toBe(true);
  });

  it('gives one with backups off only when the cloud can hold the game: cloud saves on and a catalog id', () => {
    settings.set({ cloudSaves: true } as Settings);

    expect(get(savesTab)(game())).toBe(true);
    expect(get(savesTab)(game({ canonicalGameId: '' }))).toBe(false);

    settings.set({ cloudSaves: false } as Settings);
    expect(get(savesTab)(game())).toBe(false);
  });

  it('gives none to an emulated game, a page without a library game, or before the settings load', () => {
    saveBackupsEnabled.set(true);
    settings.set({ cloudSaves: true } as Settings);

    expect(get(savesTab)(emulated)).toBe(false);
    expect(get(savesTab)(undefined)).toBe(false);
    expect(get(savesTab)(null)).toBe(false);

    saveBackupsEnabled.set(false);
    settings.set(null);
    expect(get(savesTab)(game())).toBe(false);
  });

  it('follows backups turning out to be on after the page asked', () => {
    const answers: boolean[] = [];
    const off = savesTab.subscribe((has) => answers.push(has(game())));
    saveBackupsEnabled.set(true);
    off();

    expect(answers).toEqual([false, true]);
  });
});
