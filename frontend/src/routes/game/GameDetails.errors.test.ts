import { describe, expect, it } from 'vitest';
import source from './GameDetails.svelte?raw';

describe('GameDetails error toasts', () => {
  it('never shows the raw backend error message to the user', () => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
  });

  const sites: Array<{ name: string; snippet: string }> = [
    { name: 'createDesktopShortcut', snippet: "toast(libraryErrorText(err, msg('games.detailShortcutCreateError')), 'danger');" },
    { name: 'removeDesktopShortcut', snippet: "toast(libraryErrorText(err, msg('games.detailShortcutRemoveError')), 'danger');" },
    { name: 'refreshMeta', snippet: "toast(metadataErrorText(err, msg('games.detailMetaRefreshError')), 'danger');" },
    { name: 'skipMeta', snippet: "toast(metadataErrorText(err, msg('games.detailMetaSkipError')), 'danger');" },
    { name: 'retryTerminalDownload', snippet: "toast(installErrorText(err, msg('games.detailRetryDownloadError')), 'danger');" },
    { name: 'removeTerminalDownload', snippet: "toast(installErrorText(err, msg('games.detailRemoveDownloadError')), 'danger');" },
    { name: 'discardTerminalDownload', snippet: "toast(installErrorText(err, msg('games.detailDiscardDownloadError')), 'danger');" },
    { name: 'downloadRelease', snippet: "toast(sourceErrorText(err, msg('games.detailPrepareDownloadError')), 'danger');" },
    { name: 'play', snippet: "toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');" },
    { name: 'addToLibrary', snippet: "toast(libraryErrorText(err, msg('games.detailAddToLibraryError')), 'danger');" },
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
