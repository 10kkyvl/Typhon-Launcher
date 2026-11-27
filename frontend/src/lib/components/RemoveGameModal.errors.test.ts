import { describe, expect, it } from 'vitest';
import source from './RemoveGameModal.svelte?raw';

describe('RemoveGameModal error text', () => {
  it('never shows the raw backend error message to the user', () => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
    expect(source).not.toMatch(/String\(err\)|errorMessage\(/);
  });

  it('words the read and the removal through the install table, each with its own fallback', () => {
    expect(source).toContain("import { REASONS, installErrorText } from '../install/installErrors';");
    expect(source).toContain("readFailure = installErrorText(err, msg('modals.removeGameInspectFailed'));");
    expect(source).toContain("failure = installErrorText(err, msg('modals.removeGameDeleteFailed'));");
  });
});
