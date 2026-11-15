import { describe, expect, it } from 'vitest';
import page from './Installed.svelte?raw';
import addGame from './InstalledAddGame.svelte?raw';
import list from './InstalledList.svelte?raw';
import game from './installedGame.ts?raw';

const screen = import.meta.glob<string>(['./*.svelte', './*.ts', '!./*.test.ts'], { query: '?raw', import: 'default', eager: true });

describe('Installed error toasts', () => {
  it('reads every file of the page', () => {
    expect(Object.keys(screen).sort()).toEqual([
      './Installed.svelte',
      './InstalledAddGame.svelte',
      './InstalledGrid.svelte',
      './InstalledList.svelte',
      './InstalledStorage.svelte',
      './InstalledToolbar.svelte',
      './installedGame.ts',
    ]);
  });

  it.each(Object.entries(screen))('%s never shows the raw backend error message to the user', (_name, source) => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
  });

  const sites: Array<{ name: string; source: string; snippet: string }> = [
    { name: 'submitAdd', source: addGame, snippet: "toast(libraryErrorText(err, msg('games.installedAddGameError')), 'danger');" },
    { name: 'play', source: game, snippet: "toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');" },
    { name: 'findGames', source: page, snippet: "toast(sourceErrorText(err, msg('games.installedScanError')), 'danger');" },
    { name: 'chooseExecutable', source: list, snippet: "toast(libraryErrorText(err, msg('games.installedChooseExeError')), 'danger');" },
  ];

  it.each(sites)('$name resolves the error through a translated catalog lookup, not raw text', ({ source, snippet }) => {
    expect(source).toContain(snippet);
  });

  it('resolves an unrecognised library error code through the direct catalog key with a translated fallback', () => {
    expect(game).toMatch(
      /function libraryErrorText\(err: unknown, fallback: string\): string \{\s*const code = errorCode\(err\);\s*return hasMessage\(code\) \? msg\(code\) : fallback;\s*\}/,
    );
  });
});
