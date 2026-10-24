import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import { contrast } from '../theme/accent';
import { appearanceOf, pageBackground, pageGradient, PROFILE_THEMES, smoothStops, themeOf } from './appearance';
import { PROFILE_TEXT_3 } from './appearanceColor';
import cover from '../components/ProfileCover.svelte?raw';
import canvas from '../components/ProfileCanvas.svelte?raw';
import panel from '../../routes/profile/ProfileAppearancePanel.svelte?raw';

const art = { accent: '#ff8800', background: '#1a0f08', surface: '#241812' };
const hexes = (css: string) => css.match(/#[0-9a-f]{6}/gi) ?? [];

describe('profile page background', () => {
  it('is a gradient for every preset, custom and auto theme', () => {
    for (const id of PROFILE_THEMES.map((t) => t.id)) {
      expect(pageGradient(themeOf(appearanceOf({ theme: id })))).toMatch(/^linear-gradient\(125deg, #[0-9a-f]{6} 0%, .* 100%\)$/);
    }
    expect(pageGradient(themeOf(appearanceOf({ theme: 'custom', customFrom: '#ff0000', customTo: '#0000ff', customAngle: 90 })))).toMatch(/^linear-gradient\(90deg, /);
    expect(pageGradient(themeOf(appearanceOf({ theme: 'auto' }), art))).toMatch(/^linear-gradient\(125deg, /);
  });

  it('keeps muted text readable on every stop of a darkened gradient', () => {
    const cases = [
      ...PROFILE_THEMES.map((t) => themeOf(appearanceOf({ theme: t.id }))),
      themeOf(appearanceOf({ theme: 'custom', customFrom: '#ffffff', customTo: '#f3b56b' })),
      themeOf(appearanceOf({ theme: 'custom', customFrom: '#ff00ff', customTo: '#00ffff' })),
      themeOf(appearanceOf({ theme: 'auto' }), { accent: '#ffffff', background: '#202020', surface: '#303030' }),
    ];
    for (const theme of cases) {
      for (const color of hexes(pageGradient(theme))) {
        expect(contrast(PROFILE_TEXT_3, color), `${theme.id} ${color}`).toBeGreaterThanOrEqual(4.5);
      }
    }
  });

  it('keeps the ink readable on every stop of a pure gradient', () => {
    const pairs = [['#ffffff', '#000000'], ['#fff7a8', '#0a1a4a'], ['#000000', '#ffffff'], ['#808080', '#ff00ff'], ['#ff0000', '#00ff00']];
    for (const [from, to] of pairs) {
      const theme = themeOf(appearanceOf({ theme: 'custom', customFrom: from, customTo: to, customPure: true }));
      for (const color of hexes(pageGradient(theme))) {
        expect(contrast(theme.ink!.text, color), `${from}>${to} ${color}`).toBeGreaterThanOrEqual(4.5);
      }
    }
  });

  it('moves only the end that fails, and only when neither ink fits', () => {
    const readable = themeOf(appearanceOf({ theme: 'custom', customFrom: '#fff7a8', customTo: '#f4eedd', customPure: true }));
    expect(readable.stops).toEqual(['#fff7a8', '#f4eedd']);
    const split = themeOf(appearanceOf({ theme: 'custom', customFrom: '#ffffff', customTo: '#000000', customPure: true }));
    expect(split.stops).toHaveLength(2);
    expect(split.stops.filter((color, i) => color !== ['#ffffff', '#000000'][i])).toHaveLength(1);
  });

  it('shows pure colours untouched and without noise', () => {
    const theme = themeOf(appearanceOf({ theme: 'custom', customFrom: '#ffffff', customTo: '#f4eedd', customPure: true, customAngle: 40 }));
    expect(pageBackground(theme)).toBe('linear-gradient(40deg, #ffffff 0%, #fbf9f4 33%, #f8f4e8 67%, #f4eedd 100%)');
  });

  it('adds noise to darkened gradients only', () => {
    expect(pageBackground(themeOf(appearanceOf({ theme: 'midnight' })))).toMatch(/^url\("data:image\/svg\+xml/);
    expect(pageBackground(themeOf(appearanceOf({ theme: 'custom', customPure: true })))).not.toContain('url(');
  });

  it('adds intermediate stops against banding', () => {
    expect(smoothStops(['#000000', '#ffffff'])).toHaveLength(4);
    expect(smoothStops(['#000000', '#808080', '#ffffff'])).toHaveLength(7);
  });

  it('uses one palette for the banner and the canvas in auto mode', () => {
    expect(cover).not.toMatch(/themeOf/);
    expect(canvas).toMatch(/themeOf\(a, art\)/);
    expect(canvas).toMatch(/pageBackground\(theme\)/);
  });

  it('lets only the main canvas report the art status', () => {
    expect(canvas).toContain("if (!quiet) artStatus.set('idle')");
    expect(canvas).toContain('if (!quiet) artStatus.set(result.ok');
    expect(panel).toContain('<ProfileCanvas appearance={preview} {autoArt} quiet>');
  });

  it('does not clip the canvas, so frames may overflow it', () => {
    expect(canvas).not.toMatch(/overflow:\s*(clip|hidden)/);
  });

  it('draws the dim only together with the cover image', () => {
    const source = cover.replace(/\r\n/g, '\n');
    const before = source.slice(0, source.indexOf('class="dim"'));
    expect(before.lastIndexOf('{#if image && !failed}')).toBeGreaterThan(before.lastIndexOf('{/if}'));
    expect(source).not.toContain('class="fade"');
  });
});
