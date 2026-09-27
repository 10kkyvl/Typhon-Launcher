import { describe, expect, it } from 'vitest';
import source from './LibrarySetupModal.svelte?raw';

describe('LibrarySetupModal error text', () => {
  it('never shows the raw backend error message to the user', () => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
  });

  it('resolves a known error code through the direct catalog key, not raw text', () => {
    expect(source).toMatch(
      /function message\(err: unknown, fallback: string\): string \{\s*const code = errorCode\(err\);\s*return hasMessage\(code\) \? msg\(code\) : fallback;\s*\}/,
    );
  });

  it('keeps every call site fallback distinct and translated', () => {
    expect(source).toContain("failure = message(err, msg('modals.librarySetupFolderUnsuitable'));");
    expect(source).toContain("failure = message(err, msg('modals.librarySetupFreeSpaceUnknown'));");
    expect(source).toContain("failure = message(err, msg('modals.librarySetupCreateFailed'));");
  });
});
