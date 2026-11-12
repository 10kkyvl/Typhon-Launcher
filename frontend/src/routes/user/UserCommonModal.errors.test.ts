import { describe, expect, it } from 'vitest';
import source from './UserCommonModal.svelte?raw';

describe('UserCommonModal load errors', () => {
  it('turns a failed page into a visible, translated error', () => {
    expect(source).toContain("failure = accountErrorText(err, msg('social.userCommonFailed'));");
    expect(source).not.toMatch(/catch\s*(\([^)]*\))?\s*\{\s*\}/);
  });

  it('shows the empty state only after a page loaded successfully', () => {
    expect(source).toMatch(/\{:else if loaded && games\.length === 0 && !failure && !retrying\}\s*<EmptyState/);
    expect(source).toMatch(/loaded = true;\s*\} catch/);
  });

  it('keeps already loaded games when a later page fails and offers a retry', () => {
    expect(source).toMatch(/\{#if games\.length > 0\}\s*<div class="list"/);
    expect(source).toMatch(/\{#if failure \|\| retrying\}\s*<Notice[^>]*run: retry/);
    expect(source).toContain('await load(!loaded, true);');
  });

  it('ignores answers of a superseded load', () => {
    expect(source.match(/if \(run !== serial\) return;/g)).toHaveLength(2);
  });
});
