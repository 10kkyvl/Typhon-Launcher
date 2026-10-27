import { describe, expect, it } from 'vitest';
import source from './AddSourceModal.svelte?raw';
import { modals as ru } from '../i18n/catalog/ru/modals';
import { modals as en } from '../i18n/catalog/en/modals';

describe('AddSourceModal background add', () => {
  it('offers Minimize instead of Back while the source is being added', () => {
    expect(source).toMatch(
      /\{#if adding\}\s*<Button onclick=\{\(\) => \(open = false\)\}>\{msg\('modals\.addSourceMinimize'\)\}<\/Button>\s*\{:else\}\s*<Button onclick=\{back\}>/,
    );
    expect(source).toContain("msg('modals.addSourceBackgroundNote')");
  });

  it('bumps the run token on every open so a finished background add cannot touch a reopened window', () => {
    expect(source).toMatch(/function reset\(\) \{\s*run\+\+;/);
    expect(source).toMatch(/const token = run;/);
    expect(source).toMatch(/if \(token === run\) open = false;/);
    expect(source).toMatch(/finally \{\s*if \(token === run\) adding = false;\s*\}/);
  });

  it('shows the error in the window only while it is still open for this add', () => {
    expect(source).toMatch(/if \(token === run && open\) \{\s*error = sourceErrorText\(err\);\s*step = 'input';/);
  });

  it('hands a background failure to the store, which knows whether source:error already showed it', () => {
    expect(source).toMatch(/const mark = errorMark\(\);\s*adding = true;/);
    expect(source).toMatch(/\} else \{\s*toastUnannounced\(err, mark\);\s*\}/);
    expect(source).not.toContain('known.has');
  });

  it('drops a check that finishes after the window was reopened', () => {
    expect(source).toMatch(
      /async function check\(\) \{[\s\S]*?const token = run;[\s\S]*?if \(token !== run\) return;\s*preview = result;[\s\S]*?catch \(err\) \{\s*if \(token !== run\) return;/,
    );
  });

  it('has both translations for the new strings', () => {
    for (const key of ['modals.addSourceMinimize', 'modals.addSourceBackgroundNote'] as const) {
      expect(ru[key]).toBeTruthy();
      expect(en[key]).toBeTruthy();
    }
  });
});
