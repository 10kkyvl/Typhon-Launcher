import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/search', () => ({ searchAll: vi.fn() }));

import { topModalLayer } from '../layers';
import { msg } from '../i18n';
import { selfUpdateOutcome, selfUpdateStatus } from '../stores/selfupdate';
import { endUpdateShow, resumeFinishedShow } from '../stores/updateShow';
import { user } from '../testing/dom';
import SearchHarness from '../testing/SearchHarness.svelte';
import UpdateShowHarness from '../testing/UpdateShowHarness.svelte';

// The show on the window's layer stack, beside what else opens over the page: the top bar's search
// (Ctrl/Cmd+K asks the stack whether a dialog owns the keyboard) and a dialog that opens while the
// show is up. Rendered into #app, as main.ts mounts the app, so the stack makes the page inert.

const START = 1_700_000_000_000;
const APPLYING = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;
const READY = { state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0', error: 'still running', errorCode: 'apply' } as const;
const box = (left: number, top: number, width: number, height: number) => ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top, toJSON: () => ({}) }) as DOMRect;

let app: HTMLDivElement;

beforeAll(() => {
  const finished = () => ({
    cancel: () => {},
    set onfinish(done: () => void) {
      done();
    },
  });
  Object.defineProperty(Element.prototype, 'animate', { value: finished, configurable: true, writable: true });
});

afterAll(() => {
  Reflect.deleteProperty(Element.prototype, 'animate');
});

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] });
  vi.setSystemTime(START);
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    if (this.matches('[data-logo-origin]')) return box(20, 30, 28, 28);
    if (this.matches('.funnel-box')) return box(500, 300, 360, 277);
    return box(0, 0, 0, 0);
  });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  selfUpdateOutcome.set(null);
  endUpdateShow();
  vi.restoreAllMocks();
  vi.useRealTimers();
  document.body.replaceChildren();
});

const shown = () => document.querySelector<HTMLElement>('[data-testid="update-show"]');
const page = () => document.querySelector('main') as HTMLElement;

async function advance(ms: number) {
  vi.advanceTimersByTime(ms);
  await tick();
}

function mount() {
  const search = render(SearchHarness, { target: app });
  render(UpdateShowHarness, { target: app });
  const field = () => screen.getByRole('combobox', { hidden: true });
  return { search, field };
}

describe('the update show with its words, on the layer stack', () => {
  it('makes the page under it inert and owns the keyboard: Ctrl/Cmd+K does not reach the search under it', async () => {
    const { field } = mount();
    selfUpdateStatus.set(APPLYING);
    await tick();
    const show = shown() as HTMLElement;
    expect(document.activeElement).toBe(show);

    await user().keyboard('{Control>}k{/Control}');
    expect(document.activeElement).toBe(show);
    await user().keyboard('{Meta>}k{/Meta}');
    expect(document.activeElement).toBe(show);
    expect(topModalLayer()).toBe(show);
    expect(page().closest('[inert]')).not.toBeNull();
    expect(field().closest('[inert]')).not.toBeNull();
    await user().keyboard('{Tab}{Escape}');
    expect(document.activeElement).toBe(show);
    expect(shown()).toBe(show);
  });

  it('stays above a dialog opened under it: that dialog is inert and takes no focus, the dim is not; at its end focus enters the dialog', async () => {
    const { search } = mount();
    screen.getByRole('button', { name: 'Play' }).focus();
    selfUpdateStatus.set(APPLYING);
    await tick();
    const show = shown() as HTMLElement;

    await search.rerender({ dialog: true });
    const dialog = screen.getByRole('dialog', { name: 'Other', hidden: true });
    expect(dialog.closest('[inert]')).not.toBeNull();
    expect(show.closest('[inert]')).toBeNull();
    expect(topModalLayer()).toBe(show);
    expect(document.activeElement).toBe(show);

    selfUpdateStatus.set(READY);
    await advance(1000);
    expect(shown()).toBeNull();
    expect(dialog.closest('[inert]')).toBeNull();
    expect(within(dialog).queryByText('Over it')).not.toBeNull();
    const root = topModalLayer();
    expect(root?.contains(dialog)).toBe(true);
    expect(root?.contains(document.activeElement)).toBe(true);
  });
});

describe('the finish at a start after an update, which has no words', () => {
  it('is no layer: a dialog opened under it does not make it inert, so its dim still takes the pointer; it takes no focus', async () => {
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    resumeFinishedShow();
    const { search } = mount();
    await tick();
    const show = shown() as HTMLElement;
    expect(topModalLayer()).toBeNull();
    expect(page().closest('[inert]')).toBeNull();

    await search.rerender({ dialog: true });
    expect(topModalLayer()).not.toBe(show);
    expect(show.closest('[inert]')).toBeNull();
    expect(show.contains(document.activeElement)).toBe(false);
  });
});
