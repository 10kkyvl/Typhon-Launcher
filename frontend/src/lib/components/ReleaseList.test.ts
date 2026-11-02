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

describe('console images', () => {
  const image = (id: string, platform: 'ps1' | 'ps2' | 'psp'): ReleaseGroup => {
    const base = group(id, `Game (${platform})`);
    return { ...base, release: { ...base.release, platform } };
  };

  it('lists PC releases first and opens the console group with a heading', () => {
    const html = render(ReleaseList, { props: { groups: [image('img', 'ps2'), group('pc', 'Game')], ondownload: () => {} } }).body;
    expect(html.indexOf('Game</span>')).toBeLessThan(html.indexOf('Game (ps2)'));
    expect(html).toContain('Образы для консолей');
    expect(html).toContain('console-tag');
    expect(html).toContain('PS2');
  });

  it('marks each image with its console without a heading when no PC release exists', () => {
    const html = render(ReleaseList, { props: { groups: [image('a', 'ps1'), image('b', 'psp')], ondownload: () => {} } }).body;
    expect(html).not.toContain('Образы для консолей');
    expect(html).toContain('PS1');
    expect(html).toContain('PSP');
  });

  it('shows no console tag on a PC release', () => {
    const html = render(ReleaseList, { props: { groups: [group('pc', 'Game')], ondownload: () => {} } }).body;
    expect(html).not.toContain('console-tag');
  });
});
