import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ReleaseList from './ReleaseList.svelte';
import type { ReleaseGroup } from '../services/sources';
const group = (id: string, rawTitle: string): ReleaseGroup => ({
  sourceName: 'Fixture source',
  release: { id, sourceId: 'fixture', rawTitle, title: '11F', normalizedTitle: '11f', canonicalGameId: 'game', version: '', size: 1024, uploadedAt: null, uris: [], matchStatus: 'matched', matchConfidence: 1, matchMethod: 'override', availability: 'available', firstSeenAt: '', lastSeenAt: '', createdAt: '' },
});
describe('release variants', () => {
  it('shows original names so Windows fix variants are distinguishable', () => {
    const html = render(ReleaseList, { props: { groups: [group('one','11F'),group('fix','11F (+ Windows 7 Fix, )')], ondownload: () => {} } }).body;
    expect(html).toContain('11F (+ Windows 7 Fix, )');
    expect(html.match(/class="release-row/g)).toHaveLength(2);
  });
  it('shows loading without fabricating releases', () => {
    const html = render(ReleaseList, { props: { groups: [], loading: true, ondownload: () => {} } }).body;
    expect(html).toContain('class="hint');
    expect(html).not.toContain('class="release-row');
  });
});
