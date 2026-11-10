import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ProgressBar from './ProgressBar.svelte';
import downloadsPage from '../bigpicture/components/DownloadsPage.svelte?raw';
import gamePage from '../bigpicture/components/GamePage.svelte?raw';

function trackRem(props: { value: number; height?: number }): number {
  const html = render(ProgressBar, { props }).body;
  const match = html.match(/height:\s*([\d.]+)rem/);
  if (!match) throw new Error(`no track height in ${html}`);
  return Number(match[1]);
}

function heightsIn(source: string): number[] {
  return [...source.matchAll(/<ProgressBar\b[^>]*?\bheight=\{([\d.]+)\}/g)].map((m) => Number(m[1]));
}

describe('ProgressBar', () => {
  it('draws the main window bar at its 1.x number of rem', () => {
    expect(trackRem({ value: 50 })).toBe(0.4);
    expect(trackRem({ value: 50, height: 6 })).toBe(0.6);
  });

  // Big Picture keeps its bars at the pixels it had before the main window
  // returned to its 1.x numbers.
  it('keeps the Big Picture bars at their pixel size', () => {
    const sites = [
      { name: 'DownloadsPage', source: downloadsPage, rem: [0.77, 0.77, 0.77, 0.88] },
      { name: 'GamePage', source: gamePage, rem: [1.1] },
    ];
    for (const site of sites) {
      const rendered = heightsIn(site.source).map((height) => trackRem({ value: 50, height }));
      expect(rendered, site.name).toHaveLength(site.rem.length);
      rendered.forEach((rem, i) => expect(rem, `${site.name} bar ${i + 1}`).toBeCloseTo(site.rem[i], 6));
    }
  });
});
