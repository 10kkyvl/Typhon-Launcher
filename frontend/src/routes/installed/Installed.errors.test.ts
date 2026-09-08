import { describe, expect, it } from 'vitest';
import source from './Installed.svelte?raw';

describe('Installed error toasts', () => {
  it('never shows the raw backend error message to the user', () => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
  });

  const sites: Array<{ name: string; snippet: string }> = [
    { name: 'submitAdd', snippet: "toast(libraryErrorText(err, msg('games.installedAddGameError')), 'danger');" },
    { name: 'play', snippet: "toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');" },
    { name: 'findGames', snippet: "toast(sourceErrorText(err, msg('games.installedScanError')), 'danger');" },
    { name: 'chooseExecutable', snippet: "toast(libraryErrorText(err, msg('games.installedChooseExeError')), 'danger');" },
  ];

  it.each(sites)('$name resolves the error through a translated catalog lookup, not raw text', ({ snippet }) => {
    expect(source).toContain(snippet);
  });

  it('resolves an unrecognised library error code through the direct catalog key with a translated fallback', () => {
    expect(source).toMatch(
      /function libraryErrorText\(err: unknown, fallback: string\): string \{\s*const code = errorCode\(err\);\s*return hasMessage\(code\) \? msg\(code\) : fallback;\s*\}/,
    );
  });
});
