import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { gameArt } from '../stores/metadata';
import { goBack, navigate, resetHistory } from '../stores/router';
import { claimStage } from '../stores/stage';
import AppShell from './AppShell.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// jsdom lays nothing out: every element gets a scroll position the page's content always has room for.
// Setting it sends `scroll`, as a browser does when the position moves.
const tops = new WeakMap<Element, number>();
const frame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

let app: HTMLDivElement;
let release: () => void = () => {};

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'scrollTop', {
    configurable: true,
    get(this: HTMLElement) {
      return tops.get(this) ?? 0;
    },
    set(this: HTMLElement, value: number) {
      if ((tops.get(this) ?? 0) === value) return;
      tops.set(this, value);
      this.dispatchEvent(new Event('scroll'));
    },
  });
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  document.documentElement.classList.add('no-anim');
  resetHistory();
  gameArt.set({ elden: { cover: '', hero: '/media/elden-hero.jpg', heroIsPoster: false } });
  app = document.createElement('div');
  document.body.append(app);
});

afterEach(async () => {
  release();
  await Promise.resolve();
  await tick();
  Reflect.deleteProperty(HTMLElement.prototype, 'scrollTop');
  document.documentElement.classList.remove('no-anim');
  vi.restoreAllMocks();
  gameArt.set({});
  app.remove();
});

const content = () => app.querySelector<HTMLElement>('main.content') as HTMLElement;
const stage = () => app.querySelector<HTMLElement>('.stage') as HTMLElement;

describe('the stage follows the page scroll', () => {
  it('drifts up and fades by transform and opacity written on the stage itself', async () => {
    render(AppShell, { target: app });
    release = claimStage('elden');
    await tick();
    expect(stage().style.opacity).toBe('1');
    content().scrollTop = 110;
    await frame();
    expect(stage().style.transform).toMatch(/^translate3d\(0(px)?, -55px, 0(px)?\)$/);
    expect(stage().style.opacity).toBe('0.5');
    // Not through a custom property on the shell, which would restyle every card on every frame.
    expect(app.querySelector<HTMLElement>('.shell')?.getAttribute('style') ?? '').not.toMatch(/--/);
  });

  it('a new page starts the stage at the top', async () => {
    render(AppShell, { target: app });
    release = claimStage('elden');
    await tick();
    content().scrollTop = 300;
    await frame();
    expect(stage().style.opacity).toBe('0');
    navigate('downloads');
    await tick();
    expect(stage().style.opacity).toBe('1');
  });

  it('coming back to a scrolled page never shows the stage at full strength on the way', async () => {
    render(AppShell, { target: app });
    release = claimStage('elden');
    await tick();
    content().scrollTop = 300;
    await frame();
    navigate('downloads');
    await tick();
    content().scrollTop = 110;
    await frame();
    expect(stage().style.opacity).toBe('0.5');

    // Every value the stage's style held from here on: each change records the value before it.
    const before: (string | null)[] = [];
    const watcher = new MutationObserver((records) => {
      for (const record of records) before.push(record.oldValue);
    });
    watcher.observe(stage(), { attributes: true, attributeFilter: ['style'], attributeOldValue: true });
    goBack();
    await tick();
    await frame();
    await frame();
    for (const record of watcher.takeRecords()) before.push(record.oldValue);
    watcher.disconnect();
    const opacity = (style: string | null) => /opacity:\s*([^;]+)/.exec(style ?? '')?.[1]?.trim();
    const written = [...before.slice(1), stage().getAttribute('style')].map(opacity);

    expect(content().scrollTop).toBe(300);
    expect(before.length).toBeGreaterThan(0);
    expect(written).not.toContain('1');
    expect(stage().style.opacity).toBe('0');
  });
});
