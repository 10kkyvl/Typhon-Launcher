import { describe, expect, it } from 'vitest';
import source from './RemoveGameModal.svelte?raw';

describe('RemoveGameModal error text', () => {
  it('never shows the raw backend error message to the user', () => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
  });

  it('resolves errors through installErrorText, which never returns raw text', () => {
    expect(source).toContain("import { installErrorText } from '../install/installErrors';");
    expect(source).toMatch(/function message\(err: unknown, fallback: string\) \{\s*return installErrorText\(err, fallback\);\s*\}/);
  });

  it('keeps the load() and confirm() fallbacks distinct and translated', () => {
    expect(source).toContain("failure = message(err, msg('modals.removeGameInspectFailed'));");
    expect(source).toContain("failure = message(err, msg('modals.removeGameDeleteFailed'));");
  });
});
