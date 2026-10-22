import { describe, expect, it } from 'vitest';
import source from './UserCommonModal.svelte?raw';

describe('UserCommonModal load errors', () => {
  it('turns a failed page into a visible, translated error', () => {
    expect(source).toContain("failure = accountErrorText(err, msg('social.userCommonFailed'));");
    expect(source).not.toMatch(/catch\s*(\([^)]*\))?\s*\{\s*\}/);
  });

  it('shows the empty state only after a page loaded successfully', () => {
    expect(source).toMatch(/\{:else if loaded && games\.length === 0\}/);
    expect(source).toMatch(/loaded = true;\s*\} catch/);
  });

  it('keeps already loaded games when a later page fails and offers a retry', () => {
    expect(source).toContain('{#if failure && games.length === 0}');
    expect(source).toContain("{failure ? msg('common.retry') : msg('social.userCommonMore')}");
  });

  it('ignores answers of a superseded load', () => {
    expect(source.match(/if \(run !== serial\) return;/g)).toHaveLength(2);
  });
});
