import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import UpdateCard from './UpdateCard.svelte';
import { locale } from '../i18n';
import type { Update } from '../services/updates';
function update(reason: string): Update {
  return { gameId: 'g', title: 'Game', state: 'update_available', planning: false, progress: 0, canRollback: false, checkedAt: '', availability: { available: true, kind: 'new_release', gameId: 'g', installedReleaseId: 'r1', targetReleaseId: 'r2', installedVersion: '1', targetVersion: '2', confidence: .6, strategy: 'full_release', estimatedDownloadBytes: 10, requiresFullInstall: true, patchCount: 0, targetSize: 10, reason } };
}
describe('update reason localization', () => {
  it.each([['низкая уверенность в сопоставлении версий', 'Versions not comparable'], ['размер раздачи сильно меньше установленной игры', 'The release is much smaller'], ['unknown backend detail', 'Check this release'], ['release_much_smaller', 'The release is much smaller'], ['different_edition', 'A different edition'], ['different_language', 'The release languages differ'], ['edition_unknown', 'The release edition is unspecified'], ['edition_unverified', 'The game edition could not be matched']])('translates reason %s', (reason, expected) => {
    locale.set('en');
    const html = render(UpdateCard, { props: { update: update(reason), running: false } }).body;
    expect(html).not.toContain(reason);
    expect(html).toContain(expected);
    expect(html.match(/New release available/g)).toHaveLength(1);
  });
  it('keeps distribution revision distinct from incomparable versions', () => {
    locale.set('en');
    const html = render(UpdateCard, { props: { update: update('new_distribution_revision'), running: false } }).body;
    expect(html).toContain('The source published a new revision');
    expect(html).not.toContain('Versions not comparable');
  });
});
function upToDate(patch: Partial<Update> = {}): Update {
  return { gameId: 'g', title: 'Game', state: 'idle', planning: false, progress: 1, canRollback: true, checkedAt: '', availability: { available: false, kind: 'none', gameId: 'g', installedReleaseId: '', targetReleaseId: '', installedVersion: '2', targetVersion: '', confidence: 0, strategy: '', estimatedDownloadBytes: 0, requiresFullInstall: false, patchCount: 0, targetSize: 0 }, ...patch };
}
describe('card with no newer release', () => {
  it('offers a rollback without calling the installed version a new release', () => {
    locale.set('en');
    const html = render(UpdateCard, { props: { update: upToDate(), running: false } }).body;
    expect(html).toContain('Latest version installed');
    expect(html).toContain('Restore previous version');
    expect(html).toContain('History');
    expect(html).not.toContain('New release available');
    expect(html).not.toContain('Calculate update');
    expect(html).not.toContain('Versions not comparable');
  });
  it.each([['updating', 'Update in progress'], ['update_downloading', 'Update in progress'], ['update_failed', 'Update failed']] as const)('names state %s without a new release headline', (state, headline) => {
    locale.set('en');
    const html = render(UpdateCard, { props: { update: upToDate({ state }), running: false } }).body;
    expect(html).toContain(headline);
    expect(html).not.toContain('New release available');
    expect(html).not.toContain('Calculate update');
  });
});
