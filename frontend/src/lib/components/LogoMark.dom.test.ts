import { afterEach, describe, expect, it, vi } from 'vitest';
import { tick } from 'svelte';
import type { Settings as SettingsData } from '../services/settings';
import { settings } from '../stores/settings';
import { logoArcsAway } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import LogoMark from './LogoMark.svelte';
import source from './LogoMark.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

afterEach(() => {
  logoArcsAway.set(null);
  settings.set(null as unknown as SettingsData);
});

const picture = () => document.querySelector('img.logo-mark') as HTMLImageElement;
const arcs = () => [...document.querySelectorAll<SVGPathElement>('[data-logo-origin] [data-arc]')];

describe('the sidebar logo', () => {
  it('rests as the picture, with no vector arcs drawn, and says nothing to a screen reader', () => {
    renderDom(LogoMark);
    expect(picture().getAttribute('src')).toBe('/typhon.png');
    expect(picture().getAttribute('alt')).toBe('');
    expect(arcs()).toEqual([]);
    expect(document.querySelector('[data-logo-origin]')).not.toBeNull();
  });

  it('while the update show flies its arcs, shows the vector mark in the same box with the flying arcs hidden', async () => {
    renderDom(LogoMark);
    logoArcsAway.set([true, false, true, false, false]);
    await tick();
    const origin = document.querySelector('[data-logo-origin]') as HTMLElement;
    expect(origin.contains(picture())).toBe(true);
    expect(picture().classList.contains('away')).toBe(true);
    expect(arcs().map((arc) => arc.getAttribute('data-arc'))).toEqual(['0', '1', '2', '3', '4']);
    expect(arcs().map((arc) => arc.style.opacity)).toEqual(['0', '', '0', '', '']);
    expect(origin.querySelector('svg[data-logo-mark]')?.getAttribute('aria-hidden')).toBe('true');

    logoArcsAway.set(null);
    await tick();
    expect(arcs()).toEqual([]);
    expect(picture().classList.contains('away')).toBe(false);
  });

  it('draws the flying logo in the accent when the logo is tinted, in its own colours when not', async () => {
    settings.set({ tintLogo: true } as SettingsData);
    renderDom(LogoMark);
    logoArcsAway.set([true, true, true, true, true]);
    await tick();
    expect(document.querySelector('svg[data-logo-mark]')?.classList.contains('tint')).toBe(true);
    settings.set({ tintLogo: false } as SettingsData);
    await tick();
    expect(document.querySelector('svg[data-logo-mark]')?.classList.contains('tint')).toBe(false);
  });

  // jsdom lays nothing out and applies no component styles: the box and the hiding are pinned in the source.
  it('keeps the picture\'s 2.8rem box for both', () => {
    expect(/\n {2}\.mark \{([^}]*)\}/.exec(source)?.[1]).toMatch(/width: 2\.8rem;[\s\S]*height: 2\.8rem;/);
    expect(/\n {2}\.logo-mark \{([^}]*)\}/.exec(source)?.[1]).toMatch(/width: 100%;[\s\S]*height: 100%;/);
    expect(/\n {2}\.vector \{([^}]*)\}/.exec(source)?.[1]).toMatch(/position: absolute;[\s\S]*inset: 0;/);
    expect(/\n {2}\.logo-mark\.away \{([^}]*)\}/.exec(source)?.[1]).toMatch(/visibility: hidden;/);
  });
});
